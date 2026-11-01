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

// GetPosts 游标分页获取动态列表。
// 第一页不传 cursor；之后每页把上一页返回的 next_cursor 原样带回来即可。
func GetPosts(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if err != nil || limit < 1 || limit > 50 {
		utils.Fail(c, http.StatusBadRequest, "limit 需在 1~50 之间")
		return
	}

	cursor, err := service.DecodeCursor(c.Query("cursor"))
	if err != nil {
		utils.Fail(c, http.StatusBadRequest, "cursor 不合法")
		return
	}

	posts, next, err := service.GetPostsByCursor(c.Request.Context(), limit, cursor)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "获取动态列表失败")
		return
	}

	data := gin.H{
		"items":    posts,
		"has_more": next != nil,
	}
	if next != nil {
		data["next_cursor"] = service.EncodeCursor(*next)
	}

	utils.Success(c, data)
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

// DeletePost 删除自己发布的动态。
func DeletePost(c *gin.Context) {
	postID, ok := parsePostID(c)
	if !ok {
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	err := service.DeletePost(c.Request.Context(), postID, userID)
	if errors.Is(err, service.ErrPostNotFound) {
		utils.Fail(c, http.StatusNotFound, "动态不存在")
		return
	}
	if errors.Is(err, service.ErrNotPostOwner) {
		utils.Fail(c, http.StatusForbidden, "只能删除自己的动态")
		return
	}
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "删除动态失败")
		return
	}

	utils.Success(c, nil)
}
