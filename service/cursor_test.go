package service

import (
	"errors"
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	original := PostCursor{
		CreatedAt: time.UnixMilli(1761900000123),
		ID:        42,
	}

	decoded, err := DecodeCursor(EncodeCursor(original))
	if err != nil {
		t.Fatalf("解析游标失败: %v", err)
	}
	if decoded == nil {
		t.Fatal("解析结果不应为 nil")
	}
	if decoded.ID != original.ID {
		t.Fatalf("ID 不一致: got=%d want=%d", decoded.ID, original.ID)
	}
	// 游标按毫秒编码，解码后必须与原时间完全相等，
	// 否则 WHERE created_at = ? 的等值条件会永远匹配不上。
	if !decoded.CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("时间不一致: got=%v want=%v", decoded.CreatedAt, original.CreatedAt)
	}
}

func TestDecodeCursorEmptyMeansFirstPage(t *testing.T) {
	cursor, err := DecodeCursor("")
	if err != nil {
		t.Fatalf("空游标不应报错: %v", err)
	}
	if cursor != nil {
		t.Fatalf("空游标应返回 nil，实际是 %+v", cursor)
	}
}

func TestDecodeCursorRejectsMalformedInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"不是 base64", "not-base64!!"},
		{"缺少分隔符", "Zm9v"},        // "foo"
		{"缺少 ID", "MTIzOg"},        // "123:"
		{"时间为 0", "MDox"},         // "0:1"
		{"ID 为 0", "MTIzOjA"},       // "123:0"
		{"ID 不是数字", "MTIzOmFiYw"}, // "123:abc"
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := DecodeCursor(c.input); !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("应返回 ErrInvalidCursor，实际: %v", err)
			}
		})
	}
}
