package models

import "time"

type Like struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	PostID    int64     `gorm:"not null;uniqueIndex:idx_post_user" json:"post_id"`
	UserID    int64     `gorm:"not null;uniqueIndex:idx_post_user" json:"user_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}
