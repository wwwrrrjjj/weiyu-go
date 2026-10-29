//go:build integration

// 集成测试：需要本地可用的 Redis。
// 运行方式：
//
//	REDIS_ADDR=127.0.0.1:6379 REDIS_DB=15 \
//	go test -tags integration ./service -run TestRedis -v -count=1
//
// 注意：测试会清空 REDIS_DB 指定的库，请勿指向生产环境。
package service

import (
	"context"
	"testing"

	"github.com/wwwrrrjjj/weiyu-go/config"
	"github.com/wwwrrrjjj/weiyu-go/models"
)

func setupRedis(t *testing.T) context.Context {
	t.Helper()

	if err := config.InitRedis(); err != nil {
		t.Fatalf("初始化 Redis 失败: %v", err)
	}
	t.Cleanup(func() { _ = config.CloseRedis() })

	ctx := context.Background()
	if err := config.Redis.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("清空测试 Redis 库失败: %v", err)
	}

	return ctx
}

// TestRedisPostDetailCache 验证详情缓存读写与空值缓存。
func TestRedisPostDetailCache(t *testing.T) {
	ctx := setupRedis(t)

	post := models.Post{ID: 1, UserID: 2, Content: "hello weiyu"}
	if err := setCachedPost(ctx, &post); err != nil {
		t.Fatalf("写入缓存失败: %v", err)
	}

	got, ok, err := getCachedPost(ctx, 1)
	if err != nil || !ok {
		t.Fatalf("缓存未命中: ok=%v err=%v", ok, err)
	}
	if got.ID != post.ID || got.Content != post.Content || got.UserID != post.UserID {
		t.Fatalf("缓存内容不一致: got=%+v want=%+v", got, post)
	}

	// TTL 应落在 [基础 TTL, 基础 TTL * 1.5] 区间内
	ttl := config.Redis.TTL(ctx, postDetailKey(1)).Val()
	if ttl <= 0 || ttl > postCacheTTL+postCacheTTL/2+1e9 {
		t.Fatalf("缓存 TTL 不在预期区间: %v", ttl)
	}

	// 空值缓存
	cached, err := isNullCached(ctx, 404)
	if err != nil || cached {
		t.Fatalf("未写入前不应命中空值缓存: cached=%v err=%v", cached, err)
	}
	if err := markNullCached(ctx, 404); err != nil {
		t.Fatalf("写入空值缓存失败: %v", err)
	}
	cached, err = isNullCached(ctx, 404)
	if err != nil || !cached {
		t.Fatalf("空值缓存未命中: cached=%v err=%v", cached, err)
	}

	// 删除缓存后应回落到未命中
	deletePostCache(ctx, 1)
	if _, ok, _ := getCachedPost(ctx, 1); ok {
		t.Fatal("删除缓存后仍然命中")
	}
}

// TestRedisLikeCounter 验证点赞计数缓存与 Lua 脚本的边界行为。
func TestRedisLikeCounter(t *testing.T) {
	ctx := setupRedis(t)

	// 1. key 不存在时，自增脚本应跳过，避免把真实计数覆盖成 1
	if err := incrCachedLikeCount(ctx, 7, 1); err != nil {
		t.Fatalf("自增失败: %v", err)
	}
	if _, ok, _ := getCachedLikeCount(ctx, 7); ok {
		t.Fatal("key 不存在时不应创建新 key")
	}

	// 2. 命中的 key 正常自增
	if err := setCachedLikeCount(ctx, 7, 5); err != nil {
		t.Fatalf("写入计数失败: %v", err)
	}
	if err := incrCachedLikeCount(ctx, 7, 1); err != nil {
		t.Fatalf("自增失败: %v", err)
	}
	count, ok, _ := getCachedLikeCount(ctx, 7)
	if !ok || count != 6 {
		t.Fatalf("自增结果错误: ok=%v count=%d want=6", ok, count)
	}

	// 3. 减到负数时兜底为 0
	if err := incrCachedLikeCount(ctx, 7, -100); err != nil {
		t.Fatalf("自减失败: %v", err)
	}
	count, _, _ = getCachedLikeCount(ctx, 7)
	if count != 0 {
		t.Fatalf("计数不应为负: count=%d", count)
	}
}
