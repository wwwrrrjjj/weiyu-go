package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/wwwrrrjjj/weiyu-go/config"
	"github.com/wwwrrrjjj/weiyu-go/models"
	"github.com/wwwrrrjjj/weiyu-go/mq"
	"gorm.io/gorm"
)

var (
	ErrAlreadyLiked = errors.New("你已经点赞过这条动态")
	ErrLikeNotFound = errors.New("尚未点赞")
)

// GetLikeCount 获取动态点赞数：优先读 Redis 计数，未命中回源 COUNT(*) 重建。
func GetLikeCount(ctx context.Context, postID int64) (int64, error) {
	if postID <= 0 {
		return 0, ErrInvalidPostID
	}

	if count, ok, err := getCachedLikeCount(ctx, postID); err != nil {
		slog.Warn("读取点赞计数缓存失败，降级查库", "post_id", postID, "err", err)
	} else if ok {
		return count, nil
	}

	var count int64
	err := config.DB.WithContext(ctx).
		Model(&models.Like{}).
		Where("post_id = ?", postID).
		Count(&count).Error
	if err != nil {
		return 0, err
	}

	if err := setCachedLikeCount(ctx, postID, count); err != nil {
		slog.Warn("回填点赞计数缓存失败", "post_id", postID, "err", err)
	}

	return count, nil
}

// LikePost 点赞。
// 并发重复点赞不靠「先查后插」，而是由 (post_id, user_id) 唯一索引兜底，
// 插入冲突翻译为 ErrAlreadyLiked，从根本上消除检查与写入之间的竞态窗口。
func LikePost(ctx context.Context, postID, userID int64) error {
	if postID <= 0 {
		return ErrInvalidPostID
	}
	if userID <= 0 {
		return ErrInvalidUserID
	}

	// 校验动态是否存在（这一步走缓存，不存在时返回 gorm.ErrRecordNotFound）
	if _, err := GetPostByID(ctx, postID); err != nil {
		return err
	}

	like := models.Like{
		PostID: postID,
		UserID: userID,
	}
	err := config.DB.WithContext(ctx).Create(&like).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrAlreadyLiked
	}
	if err != nil {
		return err
	}

	// 计数缓存同步自增；key 不存在时脚本返回 -1，交给读请求回源重建
	if err := incrCachedLikeCount(ctx, postID, 1); err != nil {
		slog.Warn("更新点赞计数缓存失败", "post_id", postID, "err", err)
	}

	// 投递异步通知事件，解耦「点赞」与「通知」两条链路
	publishEvent(ctx, mq.Event{
		ID:        mq.NewEventID(),
		Type:      mq.EventLikeCreated,
		PostID:    postID,
		ActorID:   userID,
		CreatedAt: time.Now(),
	})

	return nil
}

// UnlikePost 取消点赞。
func UnlikePost(ctx context.Context, postID, userID int64) error {
	if postID <= 0 {
		return ErrInvalidPostID
	}
	if userID <= 0 {
		return ErrInvalidUserID
	}

	result := config.DB.WithContext(ctx).
		Where("post_id = ? AND user_id = ?", postID, userID).
		Delete(&models.Like{})
	if result.Error != nil {
		return result.Error
	}
	// 影响行数为 0，说明本来就没点过赞
	if result.RowsAffected == 0 {
		return ErrLikeNotFound
	}

	if err := incrCachedLikeCount(ctx, postID, -1); err != nil {
		slog.Warn("更新点赞计数缓存失败", "post_id", postID, "err", err)
	}

	return nil
}
