package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/wwwrrrjjj/weiyu-go/config"
	"github.com/wwwrrrjjj/weiyu-go/models"
	"github.com/wwwrrrjjj/weiyu-go/mq"
	"gorm.io/gorm"
)

// HandleEvent 消费 MQ 事件并写入通知表。
// 返回 error 会触发消费者重试；业务上「无需处理」的情况（帖子已删、自己操作自己）
// 返回 nil，避免无意义的重试与死信堆积。
func HandleEvent(ctx context.Context, event mq.Event) error {
	if event.PostID <= 0 || event.ActorID <= 0 {
		return fmt.Errorf("事件参数不合法: post_id=%d actor_id=%d", event.PostID, event.ActorID)
	}

	var post models.Post
	err := config.DB.WithContext(ctx).
		Select("id", "user_id").
		Where("id = ?", event.PostID).
		Take(&post).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	// 自己赞自己 / 自己评论自己，不产生通知
	if post.UserID == event.ActorID {
		return nil
	}

	notification := models.Notification{
		UserID:    post.UserID,
		ActorID:   event.ActorID,
		PostID:    event.PostID,
		CommentID: event.CommentID,
		Content:   truncateRunes(event.Content, 50),
		EventID:   event.ID,
	}

	switch event.Type {
	case mq.EventLikeCreated:
		notification.Type = models.NotificationTypeLike
	case mq.EventCommentCreated:
		notification.Type = models.NotificationTypeComment
	default:
		// 未知事件类型不重试，直接丢弃
		return nil
	}

	if err := config.DB.WithContext(ctx).Create(&notification).Error; err != nil {
		// EventID 唯一索引：重复消息视为处理成功
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil
		}
		return err
	}

	return nil
}

// ListNotifications 查询当前用户最近收到的通知。
func ListNotifications(ctx context.Context, userID int64, limit int) ([]models.Notification, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	list := make([]models.Notification, 0, limit)
	err := config.DB.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Order("id DESC").
		Limit(limit).
		Find(&list).Error
	if err != nil {
		return nil, err
	}

	return list, nil
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}
