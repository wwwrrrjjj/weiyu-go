package router

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wwwrrrjjj/weiyu-go/config"
	"github.com/wwwrrrjjj/weiyu-go/controller"
	"github.com/wwwrrrjjj/weiyu-go/middleware"
)

const (
	// apiRateLimitPerMin 单 IP 每分钟请求上限，挡住最粗暴的刷接口。
	apiRateLimitPerMin = 300
	// authRateLimitPerMin 登录用户每分钟请求上限，比 IP 维度更准。
	authRateLimitPerMin = 60
)

// New 注册全部路由。
// 读接口公开，写接口统一挂在鉴权分组下，登录态由 JWT 中间件保证。
func New() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	api := r.Group("/api/v1")
	// 第一道限流：按来源 IP。放在最前面，未登录的请求也能被挡住。
	// 本机压测用 RATE_LIMIT_ENABLED=false 关闭，避免压测工具先被限流挡住。
	if config.RateLimitEnabled() {
		api.Use(middleware.RateLimit("api", apiRateLimitPerMin, time.Minute))
	}

	// 公开接口
	api.GET("/health", controller.Health)
	api.POST("/users/register", controller.Register)
	api.POST("/users/login", controller.Login)

	api.GET("/posts", controller.GetPosts)
	api.GET("/posts/:id", controller.GetPostByID)
	api.GET("/posts/:id/comments", controller.GetCommentsByPostID)
	api.GET("/posts/:id/likes/count", controller.GetLikeCount)

	// 需要登录的接口
	auth := api.Group("")
	// 顺序有意义：先验明身份，限流才能按用户维度算，而不是退化成 IP 维度。
	auth.Use(middleware.JWTAuth())
	if config.RateLimitEnabled() {
		auth.Use(middleware.RateLimit("auth", authRateLimitPerMin, time.Minute))
	}
	{
		auth.GET("/users/me", controller.Me)
		auth.POST("/users/logout", controller.Logout)
		auth.GET("/notifications", controller.ListNotifications)

		auth.POST("/posts", controller.CreatePost)
		auth.DELETE("/posts/:id", controller.DeletePost)
		auth.POST("/posts/:id/comments", controller.CreateComment)
		auth.POST("/posts/:id/likes", controller.LikePost)
		auth.DELETE("/posts/:id/likes", controller.UnlikePost)
	}

	return r
}
