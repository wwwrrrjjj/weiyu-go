package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wwwrrrjjj/weiyu-go/service"
	"github.com/wwwrrrjjj/weiyu-go/utils"
	"gorm.io/gorm"
)

// GetPosts 分页获取社区动态列表。
func GetPosts(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page <= 0 {
		utils.Fail(c, http.StatusBadRequest, "page 参数不合法")
		return
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		utils.Fail(c, http.StatusBadRequest, "page_size 需在 1~100 之间")
		return
	}

	posts, err := service.GetPosts(c.Request.Context(), page, pageSize)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "获取动态列表失败")
		return
	}

	utils.Success(c, posts)
}

type CreatePostRequest struct {
	Content string `json:"content" binding:"required"`
}

// CreatePost 发布动态。
func CreatePost(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var req CreatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, http.StatusBadRequest, "请求参数不合法")
		return
	}

	post, err := service.CreatePost(c.Request.Context(), userID, req.Content)
	if errors.Is(err, service.ErrEmptyContent) || errors.Is(err, service.ErrContentTooLong) {
		utils.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "发布动态失败")
		return
	}

	utils.Created(c, post)
}

// GetPostByID 获取动态详情（带缓存）。
func GetPostByID(c *gin.Context) {
	id, ok := parsePostID(c)
	if !ok {
		return
	}

	post, err := service.GetPostByID(c.Request.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		utils.Fail(c, http.StatusNotFound, "动态不存在")
		return
	}
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "获取动态详情失败")
		return
	}

	utils.Success(c, post)
}
