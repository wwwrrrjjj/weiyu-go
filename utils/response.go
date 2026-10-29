package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 是全站统一的响应结构。
// code 为 0 表示成功；失败时与 HTTP 状态码保持一致，方便客户端统一拦截处理。
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// Success 返回 200 + 业务成功码。
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{Code: 0, Message: "success", Data: data})
}

// Created 返回 201 + 业务成功码，用于资源创建类接口。
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Response{Code: 0, Message: "created", Data: data})
}

// Fail 返回错误响应，code 与 HTTP 状态码一致。
func Fail(c *gin.Context, status int, message string) {
	c.JSON(status, Response{Code: status, Message: message, Data: nil})
}
