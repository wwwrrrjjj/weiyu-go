package models

import "time"

// Post 社区动态。
//
// 复合索引 idx_posts_created_id (created_at, id) 服务游标分页：
// 列表固定按 created_at DESC, id DESC 排序，有了这个索引就能直接从索引上
// 取 limit+1 行，不用像 offset 分页那样先把前面所有行扫一遍再丢掉。
type Post struct {
	ID        int64     `gorm:"type:bigint;primaryKey;autoIncrement;index:idx_posts_created_id,priority:2" json:"id"`
	UserID    int64     `gorm:"type:bigint;not null;index" json:"user_id"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	CreatedAt time.Time `gorm:"autoCreateTime;index:idx_posts_created_id,priority:1" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
