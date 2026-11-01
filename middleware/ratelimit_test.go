package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newRateLimitTestContext(t *testing.T, remoteAddr string) *gin.Context {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	c.Request = req
	return c
}

// 已登录时按用户维度计数：同一出口 IP 下的多个用户不会互相挤占配额。
func TestRateLimitKeyUsesUserWhenLoggedIn(t *testing.T) {
	c := newRateLimitTestContext(t, "10.0.0.1:1234")
	c.Set(ContextUserID, int64(7))

	if got, want := rateLimitKey("api", c), "weiyu:ratelimit:api:user:7"; got != want {
		t.Fatalf("限流 key 不符: got=%q want=%q", got, want)
	}
}

// 未登录时退回 IP 维度。
func TestRateLimitKeyFallsBackToIP(t *testing.T) {
	c := newRateLimitTestContext(t, "10.0.0.1:1234")

	if got, want := rateLimitKey("api", c), "weiyu:ratelimit:api:ip:10.0.0.1"; got != want {
		t.Fatalf("限流 key 不符: got=%q want=%q", got, want)
	}
}

// Redis 不可用时必须放行：限流是保护措施，不能变成新的故障点。
func TestRateLimitPassesThroughWithoutRedis(t *testing.T) {
	r := gin.New()
	r.GET("/ping", RateLimit("test", 1, time.Minute), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "10.0.0.9:1234"
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("第 %d 次请求应放行，实际状态码 %d", i+1, w.Code)
		}
	}
}
