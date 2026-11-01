package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/wwwrrrjjj/weiyu-go/config"
	"github.com/wwwrrrjjj/weiyu-go/utils"
)

// rateLimitScript 是固定窗口计数器：INCR 一次代表消耗一个配额，
// 只在第一次计数时设置过期时间。整段脚本在 Redis 内原子执行，
// 避免「INCR 成功、EXPIRE 失败」导致 key 永不过期、用户被永久锁死。
var rateLimitScript = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return current
`)

// RateLimit 基于 Redis 的固定窗口限流，多实例共享同一份计数。
//
// 取舍说明：
//   - 固定窗口的已知缺陷是「窗口边界双倍流量」：第 59 秒打满一次、第 61 秒再打满一次，
//     两秒内实际通过了 2 倍配额。要更平滑得换滑动窗口或令牌桶，代价是更多存储与计算。
//   - Redis 故障时放行（fail-open）：限流是保护措施，不应该变成新的故障点。
//   - 依赖 c.ClientIP()，生产环境必须配置 gin 的 trusted proxies，
//     否则客户端伪造 X-Forwarded-For 就能绕过 IP 维度的限流。
func RateLimit(scope string, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limit <= 0 || window <= 0 || config.Redis == nil {
			c.Next()
			return
		}

		// 限流本身最多等 200ms，不能让它把正常请求拖慢
		ctx, cancel := context.WithTimeout(c.Request.Context(), 200*time.Millisecond)
		current, err := rateLimitScript.Run(
			ctx,
			config.Redis,
			[]string{rateLimitKey(scope, c)},
			window.Milliseconds(),
		).Int64()
		cancel()

		if err != nil {
			// 计数失败就放行，宁可少拦一次，也不要因为 Redis 抖动全站 429
			c.Next()
			return
		}

		remaining := int64(limit) - current
		if remaining < 0 {
			remaining = 0
		}
		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

		if current > int64(limit) {
			c.Header("Retry-After", strconv.Itoa(int(window.Seconds())))
			c.Abort()
			utils.Fail(c, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			return
		}

		c.Next()
	}
}

// rateLimitKey 生成限流 key：已经登录就按用户维度，否则按 IP 维度。
// 按用户算更准——同一间办公室几十个人共用一个出口 IP，按 IP 限流会误伤。
func rateLimitKey(scope string, c *gin.Context) string {
	if v, ok := c.Get(ContextUserID); ok {
		if uid, ok := v.(int64); ok && uid > 0 {
			return fmt.Sprintf("weiyu:ratelimit:%s:user:%d", scope, uid)
		}
	}
	return fmt.Sprintf("weiyu:ratelimit:%s:ip:%s", scope, c.ClientIP())
}
