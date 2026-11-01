package config

import (
	"os"
	"strings"
)

// envBool 读取布尔型环境变量，缺省返回 def。
// 接受 true/false、1/0、yes/no、on/off，大小写不敏感。
func envBool(key string, def bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw == "" {
		return def
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

// CacheEnabled 控制读缓存是否启用，默认开启。
// 压测/排查问题时可以设为 false，走纯数据库路径做对照。
func CacheEnabled() bool { return envBool("CACHE_ENABLED", true) }

// RateLimitEnabled 控制限流中间件是否挂载，默认开启。
// 本机压测时需要关闭，否则单 IP 300 次/分钟会先把压测工具挡住。
func RateLimitEnabled() bool { return envBool("RATE_LIMIT_ENABLED", true) }
