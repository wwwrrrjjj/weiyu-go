package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wwwrrrjjj/weiyu-go/service"
	"github.com/wwwrrrjjj/weiyu-go/utils"
)

// 登录用户在 Gin Context 中的键名。
// 控制器通过 middleware.ContextUserID 等常量取值，避免魔法字符串散落各处。
const (
	ContextUserID   = "userID"
	ContextTokenID  = "tokenJTI"
	ContextTokenExp = "tokenExp"
)

// JWTAuth 校验 Authorization: Bearer <token>，
// 通过后将用户 ID 写入 Gin Context，后续 handler 从上下文中取用。
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 校验 Bearer 格式
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.Abort()
			utils.Fail(c, http.StatusUnauthorized, "请先登录")
			return
		}

		// 2. 校验签名、算法、签发方与有效期
		info, err := utils.ParseToken(parts[1])
		if err != nil {
			c.Abort()
			utils.Fail(c, http.StatusUnauthorized, "登录凭证无效或已过期")
			return
		}

		// 3. 校验是否已被登出。
		// JWT 是无状态的，登出只能靠一张「黑名单」：签名依然合法，但 jti 在名单里就不放行。
		if service.IsTokenRevoked(c.Request.Context(), info.JTI) {
			c.Abort()
			utils.Fail(c, http.StatusUnauthorized, "登录凭证已失效，请重新登录")
			return
		}

		// 4. 注入用户身份，交给后续 handler 使用
		c.Set(ContextUserID, info.UserID)
		c.Set(ContextTokenID, info.JTI)
		c.Set(ContextTokenExp, info.ExpiresAt)
		c.Next()
	}
}
