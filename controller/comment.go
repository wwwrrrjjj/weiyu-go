package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wwwrrrjjj/weiyu-go/service"
	"github.com/wwwrrrjjj/weiyu-go/utils"
	"gorm.io/gorm"
)

// GetCommentsByPostID 获取指定动态下的评论列表。
func GetCommentsByPostID(c *gin.Context) {
	postID, ok := parsePostID(c)
	if !ok {
		return
	}

	comments, err := service.GetCommentsByPostID(c.Request.Context(), postID)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "查询评论失败")
		return
	}

	utils.Success(c, comments)
}

type CreateCommentRequest struct {
	Content string `json:"content" binding:"required"`
}

// CreateComment 发表评论。
func CreateComment(c *gin.Context) {
	postID, ok := parsePostID(c)
	if !ok {
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var req CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, http.StatusBadRequest, "请求参数不合法")
		return
	}

	comment, err := service.CreateComment(c.Request.Context(), postID, userID, req.Content)
	if errors.Is(err, service.ErrEmptyComment) || errors.Is(err, service.ErrCommentTooLong) {
		utils.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		utils.Fail(c, http.StatusNotFound, "动态不存在")
		return
	}
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "发表评论失败")
		return
	}

	utils.Created(c, comment)
}
