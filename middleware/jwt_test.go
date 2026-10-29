package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wwwrrrjjj/weiyu-go/utils"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func newProtectedRouter() *gin.Engine {
	r := gin.New()
	r.GET("/protected", JWTAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": c.GetInt64(ContextUserID)})
	})
	return r
}

func TestJWTAuth(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef") // 32 字节

	validToken, err := utils.GenerateToken(42)
	if err != nil {
		t.Fatalf("签发 token 失败: %v", err)
	}

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
		wantUserID int64
	}{
		{"缺少 Authorization", "", http.StatusUnauthorized, 0},
		{"认证方案不是 Bearer", "Basic abcdef", http.StatusUnauthorized, 0},
		{"Bearer 后缺少 token", "Bearer", http.StatusUnauthorized, 0},
		{"token 无法解析", "Bearer not-a-jwt", http.StatusUnauthorized, 0},
		{"合法 token", "Bearer " + validToken, http.StatusOK, 42},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if c.authHeader != "" {
				req.Header.Set("Authorization", c.authHeader)
			}
			w := httptest.NewRecorder()

			newProtectedRouter().ServeHTTP(w, req)

			if w.Code != c.wantStatus {
				t.Fatalf("状态码不符: got=%d want=%d body=%s", w.Code, c.wantStatus, w.Body.String())
			}
		})
	}
}

// TestJWTAuthRejectsForeignSecret 验证换密钥后旧 token 立即失效。
func TestJWTAuthRejectsForeignSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	token, err := utils.GenerateToken(7)
	if err != nil {
		t.Fatalf("签发 token 失败: %v", err)
	}

	t.Setenv("JWT_SECRET", "fedcba9876543210fedcba9876543210")
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	newProtectedRouter().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("使用其它密钥签发的 token 应被拒绝, got=%d", w.Code)
	}
}

// TestJWTAuthInjectsUserID 验证中间件把用户 ID 正确写入 Context。
func TestJWTAuthInjectsUserID(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	token, err := utils.GenerateToken(12345)
	if err != nil {
		t.Fatalf("签发 token 失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	newProtectedRouter().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("合法 token 应通过鉴权, got=%d", w.Code)
	}
	if body := w.Body.String(); body != `{"user_id":12345}` {
		t.Fatalf("Context 中的 userID 不正确: %s", body)
	}
}
