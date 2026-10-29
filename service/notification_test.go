package service

import "testing"

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		name  string
		input string
		limit int
		want  string
	}{
		{"短文本不截断", "hello", 10, "hello"},
		{"恰好等于上限", "hello", 5, "hello"},
		{"英文超长截断", "hello world", 5, "hello"},
		{"中文按字符截断", "今天天气不错", 3, "今天天"},
		{"空字符串", "", 5, ""},
	}

	for _, c := range cases {
		if got := truncateRunes(c.input, c.limit); got != c.want {
			t.Errorf("%s: got=%q want=%q", c.name, got, c.want)
		}
	}
}
