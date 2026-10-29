package utils

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func TestGenerateTokenRejectsInvalidUserID(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)

	if _, err := GenerateToken(0); err == nil {
		t.Fatal("userID 为 0 时应返回错误")
	}
	if _, err := GenerateToken(-1); err == nil {
		t.Fatal("userID 为负数时应返回错误")
	}
}

func TestGenerateTokenRequiresStrongSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "too-short")

	if _, err := GenerateToken(1); err == nil {
		t.Fatal("密钥长度不足 32 字节时应返回错误")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)

	token, err := GenerateToken(9527)
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}

	userID, err := ParseToken(token)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if userID != 9527 {
		t.Fatalf("用户 ID 不一致: got=%d want=9527", userID)
	}
}

func TestParseTokenRejectsOtherSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	token, err := GenerateToken(1)
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}

	t.Setenv("JWT_SECRET", "fedcba9876543210fedcba9876543210")
	if _, err := ParseToken(token); err == nil {
		t.Fatal("其它密钥签发的 token 应解析失败")
	}
}

// TestParseTokenRejectsAlgNone 验证 alg=none 的伪造 token 会被算法白名单拦截。
func TestParseTokenRejectsAlgNone(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)

	claims := jwt.RegisteredClaims{
		Subject:   "1",
		Issuer:    jwtIssuer,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	unsigned := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	token, err := unsigned.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("构造 alg=none token 失败: %v", err)
	}

	if _, err := ParseToken(token); err == nil {
		t.Fatal("alg=none 的 token 必须被拒绝")
	}
}

func TestParseTokenRejectsExpired(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)

	claims := jwt.RegisteredClaims{
		Subject:   "1",
		Issuer:    jwtIssuer,
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-3 * time.Hour)),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
	}
	expired := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err := expired.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("构造过期 token 失败: %v", err)
	}

	if _, err := ParseToken(token); err == nil {
		t.Fatal("过期 token 必须被拒绝")
	}
}
