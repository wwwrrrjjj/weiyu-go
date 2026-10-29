package router

import (
	"github.com/gin-gonic/gin"
	"github.com/wwwrrrjjj/weiyu-go/controller"
	"github.com/wwwrrrjjj/weiyu-go/middleware"
)

// New 注册全部路由。
// 读接口公开，写接口统一挂在鉴权分组下，登录态由 JWT 中间件保证。
func New() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	api := r.Group("/api/v1")

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
	auth.Use(middleware.JWTAuth())
	{
		auth.GET("/users/me", controller.Me)
		auth.GET("/notifications", controller.ListNotifications)

		auth.POST("/posts", controller.CreatePost)
		auth.POST("/posts/:id/comments", controller.CreateComment)
		auth.POST("/posts/:id/likes", controller.LikePost)
		auth.DELETE("/posts/:id/likes", controller.UnlikePost)
	}

	return r
}
