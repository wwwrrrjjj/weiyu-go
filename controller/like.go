package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wwwrrrjjj/weiyu-go/service"
	"github.com/wwwrrrjjj/weiyu-go/utils"
	"gorm.io/gorm"
)

// GetLikeCount 获取指定动态的点赞数（走 Redis 计数缓存）。
func GetLikeCount(c *gin.Context) {
	postID, ok := parsePostID(c)
	if !ok {
		return
	}

	count, err := service.GetLikeCount(c.Request.Context(), postID)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "查询点赞数失败")
		return
	}

	utils.Success(c, gin.H{
		"post_id":    postID,
		"like_count": count,
	})
}

// LikePost 点赞。
func LikePost(c *gin.Context) {
	postID, ok := parsePostID(c)
	if !ok {
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	err := service.LikePost(c.Request.Context(), postID, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		utils.Fail(c, http.StatusNotFound, "动态不存在")
		return
	}
	if errors.Is(err, service.ErrAlreadyLiked) {
		utils.Fail(c, http.StatusConflict, service.ErrAlreadyLiked.Error())
		return
	}
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "点赞失败")
		return
	}

	utils.Created(c, nil)
}

// UnlikePost 取消点赞。
func UnlikePost(c *gin.Context) {
	postID, ok := parsePostID(c)
	if !ok {
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	err := service.UnlikePost(c.Request.Context(), postID, userID)
	if errors.Is(err, service.ErrLikeNotFound) {
		utils.Fail(c, http.StatusNotFound, "尚未点赞")
		return
	}
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "取消点赞失败")
		return
	}

	utils.Success(c, nil)
}
