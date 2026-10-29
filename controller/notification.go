package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wwwrrrjjj/weiyu-go/service"
	"github.com/wwwrrrjjj/weiyu-go/utils"
)

// ListNotifications 查询当前用户收到的互动通知（由 MQ 消费者异步写入）。
func ListNotifications(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	limit := 20
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			utils.Fail(c, http.StatusBadRequest, "limit 参数不合法")
			return
		}
		limit = n
	}

	list, err := service.ListNotifications(c.Request.Context(), userID, limit)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "查询通知失败")
		return
	}

	utils.Success(c, list)
}
