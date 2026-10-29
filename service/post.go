package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/wwwrrrjjj/weiyu-go/config"
	"github.com/wwwrrrjjj/weiyu-go/models"
	"gorm.io/gorm"
)

var (
	ErrEmptyContent   = errors.New("动态内容不能为空")
	ErrContentTooLong = errors.New("动态内容不能超过500个字符")
	ErrInvalidUserID  = errors.New("用户ID无效")
	ErrInvalidPostID  = errors.New("动态ID无效")
)

// GetPosts 分页获取动态列表，按发布时间倒序。
func GetPosts(ctx context.Context, page, pageSize int) ([]models.Post, error) {
	if page <= 0 {
		return nil, errors.New("page 不合法")
	}
	if pageSize <= 0 || pageSize > 100 {
		return nil, errors.New("pageSize 不合法")
	}

	offset := (page - 1) * pageSize
	posts := make([]models.Post, 0, pageSize)

	// 排序键追加 id 作为兜底，避免同一时间戳的记录在翻页时重复或漏出
	err := config.DB.WithContext(ctx).
		Order("created_at DESC").
		Order("id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&posts).Error
	if err != nil {
		return nil, err
	}

	return posts, nil
}

// CreatePost 发布一条动态。
func CreatePost(ctx context.Context, userID int64, content string) (*models.Post, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrEmptyContent
	}
	if utf8.RuneCountInString(content) > 500 {
		return nil, ErrContentTooLong
	}

	post := models.Post{
		UserID:  userID,
		Content: content,
	}
	if err := config.DB.WithContext(ctx).Create(&post).Error; err != nil {
		return nil, err
	}

	return &post, nil
}

// GetPostByID 查询动态详情，读取路径为「Redis 缓存 → singleflight 回源 → MySQL」。
func GetPostByID(ctx context.Context, id int64) (*models.Post, error) {
	if id <= 0 {
		return nil, ErrInvalidPostID
	}

	// 1. 先查缓存，命中直接返回
	if post, ok, err := getCachedPost(ctx, id); err != nil {
		slog.Warn("读取帖子缓存失败，降级查库", "post_id", id, "err", err)
	} else if ok {
		return post, nil
	}

	// 2. 空值缓存：不存在的 ID 直接返回 404，避免缓存穿透打库
	if cached, err := isNullCached(ctx, id); err == nil && cached {
		return nil, gorm.ErrRecordNotFound
	}

	// 3. 缓存未命中，singleflight 合并同一 key 的并发回源，防止热点 key 击穿
	value, err, _ := postDetailGroup.Do(postDetailKey(id), func() (any, error) {
		// 双重检查：排队期间可能已有其它请求回填过缓存
		if post, ok, err := getCachedPost(ctx, id); err == nil && ok {
			return post, nil
		}

		var post models.Post
		err := config.DB.WithContext(ctx).Where("id = ?", id).Take(&post).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := markNullCached(ctx, id); err != nil {
				slog.Warn("写入空值缓存失败", "post_id", id, "err", err)
			}
			return nil, gorm.ErrRecordNotFound
		}
		if err != nil {
			return nil, err
		}

		if err := setCachedPost(ctx, &post); err != nil {
			slog.Warn("回填帖子缓存失败", "post_id", id, "err", err)
		}
		return &post, nil
	})
	if err != nil {
		return nil, err
	}

	post, ok := value.(*models.Post)
	if !ok || post == nil {
		return nil, gorm.ErrRecordNotFound
	}

	return post, nil
}
