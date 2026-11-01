package service

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/wwwrrrjjj/weiyu-go/config"
	"github.com/wwwrrrjjj/weiyu-go/models"
	"golang.org/x/sync/singleflight"
)

const (
	postDetailKeyPrefix = "weiyu:post:detail:"
	postNullKeyPrefix   = "weiyu:post:null:"
	likeCountKeyPrefix  = "weiyu:like:count:"

	postCacheTTL      = 10 * time.Minute
	postNullCacheTTL  = time.Minute
	likeCountCacheTTL = 10 * time.Minute
)

// postDetailGroup 用 singleflight 合并同一篇帖子的并发回源请求：
// 热点 key 失效瞬间，只放一个请求打到 MySQL，其余请求等待并共享结果。
var postDetailGroup singleflight.Group

func postDetailKey(postID int64) string {
	return postDetailKeyPrefix + strconv.FormatInt(postID, 10)
}

func postNullKey(postID int64) string {
	return postNullKeyPrefix + strconv.FormatInt(postID, 10)
}

func likeCountKey(postID int64) string {
	return likeCountKeyPrefix + strconv.FormatInt(postID, 10)
}

// jitterTTL 在基础 TTL 上叠加 0~50% 的随机抖动，
// 避免同一批写入的 key 在同一秒集体过期，造成缓存雪崩。
func jitterTTL(base time.Duration) time.Duration {
	if base <= 0 {
		return base
	}
	return base + time.Duration(rand.Int63n(int64(base)/2+1))
}

func cacheGetJSON(ctx context.Context, key string, dest any) (bool, error) {
	if config.Redis == nil {
		return false, nil
	}

	raw, err := config.Redis.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if err := json.Unmarshal(raw, dest); err != nil {
		// 缓存内容损坏时删除，让下次请求回源重建
		_ = config.Redis.Del(ctx, key).Err()
		return false, err
	}

	return true, nil
}

func cacheSetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	if config.Redis == nil {
		return nil
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return config.Redis.Set(ctx, key, raw, ttl).Err()
}

// getCachedPost 读取帖子详情缓存，ok=false 表示未命中。
func getCachedPost(ctx context.Context, postID int64) (*models.Post, bool, error) {
	var post models.Post
	ok, err := cacheGetJSON(ctx, postDetailKey(postID), &post)
	if !ok || err != nil {
		return nil, false, err
	}
	return &post, true, nil
}

func setCachedPost(ctx context.Context, post *models.Post) error {
	return cacheSetJSON(ctx, postDetailKey(post.ID), post, jitterTTL(postCacheTTL))
}

// isNullCached 判断该 ID 是否已被标记为「不存在」，
// 用于缓存穿透防护：不存在的 ID 不反复穿透到 MySQL。
func isNullCached(ctx context.Context, postID int64) (bool, error) {
	if config.Redis == nil {
		return false, nil
	}
	n, err := config.Redis.Exists(ctx, postNullKey(postID)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func markNullCached(ctx context.Context, postID int64) error {
	if config.Redis == nil {
		return nil
	}
	return config.Redis.Set(ctx, postNullKey(postID), 1, postNullCacheTTL).Err()
}

// deletePostCache 在写操作后删除与这条动态相关的全部缓存，
// 配合读时回填构成 Cache-Aside。
//
// 一次删三个 key：详情、空值标记、点赞计数。
// 只删详情的话，动态都删了、计数还留在 Redis 里，重建时又会把旧值读回来。
func deletePostCache(ctx context.Context, postID int64) {
	if config.Redis == nil {
		return
	}
	_ = config.Redis.Del(ctx,
		postDetailKey(postID),
		postNullKey(postID),
		likeCountKey(postID),
	).Err()
}

func getCachedLikeCount(ctx context.Context, postID int64) (int64, bool, error) {
	if config.Redis == nil {
		return 0, false, nil
	}

	count, err := config.Redis.Get(ctx, likeCountKey(postID)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}

	return count, true, nil
}

func setCachedLikeCount(ctx context.Context, postID, count int64) error {
	if config.Redis == nil {
		return nil
	}
	return config.Redis.Set(ctx, likeCountKey(postID), count, jitterTTL(likeCountCacheTTL)).Err()
}

// incrLikeCountScript 只在计数 key 已存在时才做增减，
// 避免 key 未命中时用 +1 覆盖掉真实的历史计数。
// 返回 -1 表示 key 不存在，由下一次读请求回源重建。
var incrLikeCountScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then
  local v = redis.call('INCRBY', KEYS[1], ARGV[1])
  if v < 0 then
    redis.call('SET', KEYS[1], 0)
    return 0
  end
  return v
end
return -1
`)

func incrCachedLikeCount(ctx context.Context, postID, delta int64) error {
	if config.Redis == nil {
		return nil
	}
	_, err := incrLikeCountScript.Run(ctx, config.Redis, []string{likeCountKey(postID)}, delta).Int64()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	return err
}
