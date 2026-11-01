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
	ErrPostNotFound   = errors.New("动态不存在")
	ErrNotPostOwner   = errors.New("只能删除自己的动态")
)

const (
	defaultPostLimit = 10
	maxPostLimit     = 50
)

// GetPostsByCursor 按 (created_at, id) 游标分页拉取动态列表。
//
// 和 offset 分页的区别：
//   - offset 分页：LIMIT 10 OFFSET 10000，MySQL 要先扫出 10010 行再丢掉前 10000 行，
//     翻得越深越慢；而且期间有人发新动态，会让同一行在两页里重复出现。
//   - 游标分页：把「上一页最后一行的排序键」当作锚点，直接 WHERE ... < 锚点，
//     借助 (created_at, id) 索引定位，无论翻到第几页成本都一样。
//
// 多取一条（limit+1）是为了判断「还有没有下一页」，省掉一次 COUNT(*)。
func GetPostsByCursor(ctx context.Context, limit int, cursor *PostCursor) ([]models.Post, *PostCursor, error) {
	if limit <= 0 {
		limit = defaultPostLimit
	}
	if limit > maxPostLimit {
		limit = maxPostLimit
	}

	query := config.DB.WithContext(ctx).
		Order("created_at DESC").
		Order("id DESC").
		Limit(limit + 1)

	if cursor != nil {
		// 排序键是 (created_at, id) 的降序，所以「下一页」是严格小于锚点的那些行。
		// 必须用括号把 OR 包起来，否则和以后追加的其它条件组合时优先级会出错。
		query = query.Where(
			"(created_at < ? OR (created_at = ? AND id < ?))",
			cursor.CreatedAt, cursor.CreatedAt, cursor.ID,
		)
	}

	posts := make([]models.Post, 0, limit+1)
	if err := query.Find(&posts).Error; err != nil {
		return nil, nil, err
	}

	var next *PostCursor
	if len(posts) > limit {
		posts = posts[:limit]
		last := posts[len(posts)-1]
		next = &PostCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}

	return posts, next, nil
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

// DeletePost 删除自己发布的动态，连带清掉它下面的点赞和评论。
//
// 三张表的删除放在一个事务里：如果只删了 posts 而 likes/comments 没删，
// 就会留下永远查不到的孤儿数据，点赞计数也会对不上。
func DeletePost(ctx context.Context, postID, userID int64) error {
	if postID <= 0 {
		return ErrInvalidPostID
	}
	if userID <= 0 {
		return ErrInvalidUserID
	}

	err := config.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 用 (id, user_id) 一起做条件，顺手完成鉴权：
		// 别人的动态查不出来，自然也就删不掉。
		var post models.Post
		err := tx.Where("id = ? AND user_id = ?", postID, userID).Take(&post).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 再确认一次：到底是「不存在」还是「不是你的」，好让接口返回 404 / 403
			var exists int64
			if cerr := tx.Model(&models.Post{}).Where("id = ?", postID).Count(&exists).Error; cerr != nil {
				return cerr
			}
			if exists == 0 {
				return ErrPostNotFound
			}
			return ErrNotPostOwner
		}
		if err != nil {
			return err
		}

		if err := tx.Where("post_id = ?", postID).Delete(&models.Like{}).Error; err != nil {
			return err
		}
		if err := tx.Where("post_id = ?", postID).Delete(&models.Comment{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Post{}, postID).Error
	})
	if err != nil {
		return err
	}

	// 缓存必须在事务提交之后删：顺序反过来的话，事务一但回滚，
	// 就会留下「缓存没了、数据还在」的状态（虽然能自愈，但没必要）。
	deletePostCache(ctx, postID)
	return nil
}

// GetPostByID 查询动态详情，读取路径为「Redis 缓存 → singleflight 回源 → MySQL」。
func GetPostByID(ctx context.Context, id int64) (*models.Post, error) {
	if id <= 0 {
		return nil, ErrInvalidPostID
	}

	// 压测对照：缓存关闭时直接查库。
	// 不能只让 getCachedPost 返回 miss——singleflight 仍会把并发请求合并成极少次查库，
	// 这样测出来的「无缓存」是假的。
	if !config.CacheEnabled() {
		var post models.Post
		if err := config.DB.WithContext(ctx).Where("id = ?", id).Take(&post).Error; err != nil {
			return nil, err
		}
		return &post, nil
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
