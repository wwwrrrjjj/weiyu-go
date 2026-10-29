package mq

import "testing"

func TestNewEventIDUniqueAndFixedLength(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)

	for i := 0; i < n; i++ {
		id := NewEventID()
		if len(id) != 32 {
			t.Fatalf("事件 ID 应为 32 位十六进制字符串, got=%q", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("事件 ID 重复: %s", id)
		}
		seen[id] = struct{}{}
	}
}

func TestHeaderInt(t *testing.T) {
	headers := map[string]any{
		"int32":   int32(2),
		"int64":   int64(3),
		"int":     4,
		"string":  "4",
		"missing": nil,
	}

	cases := []struct {
		key  string
		want int
	}{
		{"int32", 2},
		{"int64", 3},
		{"int", 4},
		{"string", 0},
		{"missing", 0},
	}

	for _, c := range cases {
		if got := headerInt(headers, c.key); got != c.want {
			t.Errorf("headerInt(%q): got=%d want=%d", c.key, got, c.want)
		}
	}
}
