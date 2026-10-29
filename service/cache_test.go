package service

import (
	"testing"
	"time"
)

func TestJitterTTLWithinRange(t *testing.T) {
	base := 10 * time.Minute
	for i := 0; i < 100; i++ {
		got := jitterTTL(base)
		if got < base {
			t.Fatalf("抖动后 TTL 不应小于基础值: base=%v got=%v", base, got)
		}
		// 抖动上限为基础值的 50%
		if got > base+base/2 {
			t.Fatalf("抖动后 TTL 超出上限: base=%v got=%v", base, got)
		}
	}
}

func TestJitterTTLZero(t *testing.T) {
	if got := jitterTTL(0); got != 0 {
		t.Fatalf("基础 TTL 为 0 时应原样返回, got=%v", got)
	}
}

func TestCacheKeys(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"post detail", postDetailKey(42), "weiyu:post:detail:42"},
		{"post null", postNullKey(42), "weiyu:post:null:42"},
		{"like count", likeCountKey(42), "weiyu:like:count:42"},
	}

	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got=%q want=%q", c.name, c.got, c.want)
		}
	}
}
