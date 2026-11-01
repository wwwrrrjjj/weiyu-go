package service

import (
	"context"
	"time"

	"github.com/wwwrrrjjj/weiyu-go/config"
)

// revokedTokenKeyPrefix 是 JWT 吊销名单的 key 前缀。
// 命名空间和业务缓存、MQ 幂等键分开，便于在 redis-cli 里按前缀统计。
const revokedTokenKeyPrefix = "weiyu:jwt:revoked:"

// RevokeToken 把某个 jti 加入吊销名单。
//
// TTL 取「这张 token 距离自然过期还有多久」：token 一旦到期，签名校验本身就会失败，
// 名单里再留着这个 jti 没有任何意义，只会让 Redis 里堆垃圾。
func RevokeToken(ctx context.Context, jti string, ttl time.Duration) error {
	if jti == "" || config.Redis == nil {
		return nil
	}
	if ttl <= 0 {
		// token 已经过期或即将过期，给一个很短的保护期即可
		ttl = time.Minute
	}
	return config.Redis.Set(ctx, revokedTokenKeyPrefix+jti, 1, ttl).Err()
}

// IsTokenRevoked 判断一张 token 是否已被主动吊销。
//
// 设计取舍：Redis 不可用时返回 false（放行）。
// 这是「可用性优先」的选择——鉴权链路依赖 Redis 的话，Redis 一抖全站 401。
// 代价是 Redis 挂掉期间登出功能失效，属于可以接受的降级。
func IsTokenRevoked(ctx context.Context, jti string) bool {
	if jti == "" || config.Redis == nil {
		return false
	}
	n, err := config.Redis.Exists(ctx, revokedTokenKeyPrefix+jti).Result()
	if err != nil {
		return false
	}
	return n > 0
}
