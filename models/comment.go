package models

import "time"

type Comment struct {
	ID        int64     `gorm:"type:bigint;primaryKey;autoIncrement" json:"id"`
	PostID    int64     `gorm:"type:bigint;not null;index" json:"post_id"`
	UserID    int64     `gorm:"type:bigint;not null;index" json:"user_id"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
