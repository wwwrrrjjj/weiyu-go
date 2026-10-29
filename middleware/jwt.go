package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wwwrrrjjj/weiyu-go/utils"
)

// ContextUserID 是登录用户 ID 在 Gin Context 中的键名，
// 控制器通过 middleware.ContextUserID 取值，避免魔法字符串散落各处。
const ContextUserID = "userID"

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
		userID, err := utils.ParseToken(parts[1])
		if err != nil {
			c.Abort()
			utils.Fail(c, http.StatusUnauthorized, "登录凭证无效或已过期")
			return
		}

		// 3. 注入用户身份，交给后续 handler 使用
		c.Set(ContextUserID, userID)
		c.Next()
	}
}
