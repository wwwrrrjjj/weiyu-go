package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wwwrrrjjj/weiyu-go/config"
	"github.com/wwwrrrjjj/weiyu-go/models"
	"github.com/wwwrrrjjj/weiyu-go/mq"
)

var (
	ErrEmptyComment   = errors.New("评论为空")
	ErrCommentTooLong = errors.New("评论内容超出最长限制")
)

// GetCommentsByPostID 查询指定动态下的评论，按时间正序，最多返回 50 条。
func GetCommentsByPostID(ctx context.Context, postID int64) ([]models.Comment, error) {
	if postID <= 0 {
		return nil, ErrInvalidPostID
	}

	comments := make([]models.Comment, 0)
	err := config.DB.WithContext(ctx).
		Where("post_id = ?", postID).
		Order("created_at ASC").
		Order("id ASC").
		Limit(50).
		Find(&comments).Error
	if err != nil {
		return nil, err
	}

	return comments, nil
}

// CreateComment 发表评论。
func CreateComment(ctx context.Context, postID, userID int64, content string) (*models.Comment, error) {
	if postID <= 0 {
		return nil, ErrInvalidPostID
	}
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrEmptyComment
	}
	if utf8.RuneCountInString(content) > 500 {
		return nil, ErrCommentTooLong
	}

	// 评论前确认动态存在，避免给已删除的动态挂评论
	if _, err := GetPostByID(ctx, postID); err != nil {
		return nil, err
	}

	comment := models.Comment{
		PostID:  postID,
		UserID:  userID,
		Content: content,
	}
	if err := config.DB.WithContext(ctx).Create(&comment).Error; err != nil {
		return nil, err
	}

	// 投递异步通知事件，内容截断后随事件传递，消费端无需回查评论表
	publishEvent(ctx, mq.Event{
		ID:        mq.NewEventID(),
		Type:      mq.EventCommentCreated,
		PostID:    postID,
		ActorID:   userID,
		CommentID: comment.ID,
		Content:   content,
		CreatedAt: time.Now(),
	})

	return &comment, nil
}
