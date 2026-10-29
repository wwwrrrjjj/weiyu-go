package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wwwrrrjjj/weiyu-go/config"
	"github.com/wwwrrrjjj/weiyu-go/mq"
	"github.com/wwwrrrjjj/weiyu-go/utils"
)

// Health 健康检查：逐个探测 MySQL / Redis / RabbitMQ，任一不可用返回 503。
// 供容器编排的 readiness/liveness 探针与网关路由使用。
func Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	deps := gin.H{
		"mysql":    "ok",
		"redis":    "ok",
		"rabbitmq": "ok",
	}
	healthy := true

	if err := config.PingMySQL(ctx); err != nil {
		deps["mysql"] = "down"
		healthy = false
	}
	if err := config.PingRedis(ctx); err != nil {
		deps["redis"] = "down"
		healthy = false
	}
	if !mq.Healthy() {
		deps["rabbitmq"] = "down"
		healthy = false
	}

	status := "ok"
	httpStatus := http.StatusOK
	code := 0
	if !healthy {
		status = "degraded"
		httpStatus = http.StatusServiceUnavailable
		code = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, utils.Response{
		Code:    code,
		Message: status,
		Data: gin.H{
			"status":       status,
			"dependencies": deps,
		},
	})
}
