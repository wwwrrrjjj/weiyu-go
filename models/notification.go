package models

import "time"

const (
	NotificationTypeLike    = "like"
	NotificationTypeComment = "comment"
)

// Notification 由 MQ 消费者异步写入，记录用户收到的互动通知。
// EventID 上的唯一索引是消息幂等的最后一道防线：
// 即使 Redis 去重键丢失（例如 Redis 重启），重复消息也不会写出重复通知。
type Notification struct {
	ID        int64     `gorm:"type:bigint;primaryKey;autoIncrement" json:"id"`
	UserID    int64     `gorm:"type:bigint;not null;index:idx_user_created,priority:1" json:"user_id"`
	ActorID   int64     `gorm:"type:bigint;not null" json:"actor_id"`
	Type      string    `gorm:"type:varchar(20);not null" json:"type"`
	PostID    int64     `gorm:"type:bigint;not null;index" json:"post_id"`
	CommentID int64     `gorm:"type:bigint;not null;default:0" json:"comment_id"`
	Content   string    `gorm:"type:varchar(255);not null;default:''" json:"content"`
	EventID   string    `gorm:"type:varchar(64);not null;uniqueIndex" json:"event_id"`
	CreatedAt time.Time `gorm:"autoCreateTime;index:idx_user_created,priority:2" json:"created_at"`
}
