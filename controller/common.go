package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wwwrrrjjj/weiyu-go/middleware"
	"github.com/wwwrrrjjj/weiyu-go/utils"
)

// currentUserID 从鉴权中间件写入的 Context 中取出当前用户 ID。
// 理论上网关已保证登录态，这里再做一次防御性校验，避免中间件被误跳过。
func currentUserID(c *gin.Context) (int64, bool) {
	userID := c.GetInt64(middleware.ContextUserID)
	if userID <= 0 {
		utils.Fail(c, http.StatusUnauthorized, "请先登录")
		return 0, false
	}
	return userID, true
}

// parsePostID 解析路径参数 :id，非法时直接写回 400。
func parsePostID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		utils.Fail(c, http.StatusBadRequest, "动态 ID 不合法")
		return 0, false
	}
	return id, true
}
