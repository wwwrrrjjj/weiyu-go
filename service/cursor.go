package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidCursor 表示客户端传上来的游标无法解析。
var ErrInvalidCursor = errors.New("cursor 不合法")

// PostCursor 是动态列表的翻页锚点，由「上一页最后一行的排序键」组成。
//
// 翻页条件写成 (created_at, id) < (锚点 created_at, 锚点 id)，
// 因为排序是 created_at DESC, id DESC，所以「下一行」一定是严格小于锚点的第一条。
//
// 为什么必须带上 id：同一毫秒可能有多条动态，只用 created_at 会在
// 「时间相等」的边界上重复或漏掉数据。
type PostCursor struct {
	CreatedAt time.Time
	ID        int64
}

// EncodeCursor 把锚点编码成对客户端不透明的字符串。
//
// 用 base64 而不是直接拼明文，是为了让调用方不要依赖内部格式——
// 以后换成别的排序键（比如 score + id），客户端代码不用改。
//
// 时间用毫秒：MySQL 的 datetime(3) 就是毫秒精度，用毫秒可以做到
// 编码再解码后与库里的值完全相等，等值比较不会因为精度丢失而错位。
func EncodeCursor(c PostCursor) string {
	raw := fmt.Sprintf("%d:%d", c.CreatedAt.UnixMilli(), c.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor 解析游标。空字符串表示「从头开始」，返回 (nil, nil)。
func DecodeCursor(s string) (*PostCursor, error) {
	if s == "" {
		return nil, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, ErrInvalidCursor
	}

	parts := strings.Split(string(decoded), ":")
	if len(parts) != 2 {
		return nil, ErrInvalidCursor
	}

	ms, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || ms <= 0 {
		return nil, ErrInvalidCursor
	}

	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return nil, ErrInvalidCursor
	}

	return &PostCursor{CreatedAt: time.UnixMilli(ms), ID: id}, nil
}
