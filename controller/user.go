package controller

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wwwrrrjjj/weiyu-go/middleware"
	"github.com/wwwrrrjjj/weiyu-go/service"
	"github.com/wwwrrrjjj/weiyu-go/utils"
)

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=8"`
}

// Register 注册用户。
func Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, http.StatusBadRequest, "用户名需 3~50 位，密码至少 8 位")
		return
	}

	user, err := service.RegisterUser(c.Request.Context(), req.Username, req.Password)
	if errors.Is(err, service.ErrUsernameExists) {
		utils.Fail(c, http.StatusConflict, "用户名已存在")
		return
	}
	if errors.Is(err, service.ErrInvalidUsername) {
		utils.Fail(c, http.StatusBadRequest, service.ErrInvalidUsername.Error())
		return
	}
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "注册失败")
		return
	}

	utils.Created(c, gin.H{
		"id":       user.ID,
		"username": user.Username,
	})
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login 校验账号密码并签发 JWT。
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, http.StatusBadRequest, "请求参数不合法")
		return
	}

	user, err := service.LoginUser(c.Request.Context(), req.Username, req.Password)
	if errors.Is(err, service.ErrInvalidCredentials) {
		utils.Fail(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "登录失败")
		return
	}

	token, err := utils.GenerateToken(user.ID)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "签发登录凭证失败")
		return
	}

	utils.Success(c, gin.H{
		"token":      token,
		"token_type": "Bearer",
		"expires_in": int64(utils.TokenTTL.Seconds()),
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
		},
	})
}

// Me 返回当前登录用户的资料。
func Me(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	user, err := service.GetUserByID(c.Request.Context(), userID)
	if errors.Is(err, service.ErrUserNotFound) {
		utils.Fail(c, http.StatusNotFound, "用户不存在")
		return
	}
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "查询用户失败")
		return
	}

	utils.Success(c, gin.H{
		"id":         user.ID,
		"username":   user.Username,
		"created_at": user.CreatedAt,
	})
}

// Logout 主动让当前 token 失效。
//
// JWT 是无状态的，服务端没有会话可以删，所以只能在 Redis 里维护一张「吊销名单」：
// 把这张 token 的 jti 记下来，中间件每次鉴权时顺带查一次。
// 名单条目的 TTL 取 token 剩余寿命，到期即自然清理，不会无限增长。
func Logout(c *gin.Context) {
	jti := c.GetString(middleware.ContextTokenID)
	expiresAt := c.GetTime(middleware.ContextTokenExp)

	if err := service.RevokeToken(c.Request.Context(), jti, time.Until(expiresAt)); err != nil {
		utils.Fail(c, http.StatusInternalServerError, "退出登录失败")
		return
	}

	utils.Success(c, nil)
}
