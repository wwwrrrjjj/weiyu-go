package utils

import (
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	jwtIssuer = "weiyu"
	// TokenTTL 是访问令牌的有效期，登录接口会把它一并返回给客户端。
	TokenTTL = 2 * time.Hour
)

// 从环境变量获取 JWT 签名密钥
func getJWTSecret() ([]byte, error) {
	secret := os.Getenv("JWT_SECRET")

	if len(secret) < 32 {
		return nil, errors.New("JWT_SECRET 未配置或长度不足 32 字节")
	}

	return []byte(secret), nil
}

// GenerateToken 根据用户 ID 生成 JWT
func GenerateToken(userID int64) (string, error) {
	if userID <= 0 {
		return "", errors.New("用户 ID 无效")
	}

	secret, err := getJWTSecret()
	if err != nil {
		return "", err
	}

	now := time.Now()

	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		Issuer:    jwtIssuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL)),
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	)

	return token.SignedString(secret)
}

// ParseToken 验证 JWT，并返回用户 ID
func ParseToken(tokenString string) (int64, error) {
	secret, err := getJWTSecret()
	if err != nil {
		return 0, err
	}

	claims := &jwt.RegisteredClaims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (interface{}, error) {
			return secret, nil
		},
		// 只接受 HS256，避免 alg=none / 算法混淆攻击
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer(jwtIssuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)

	if err != nil || !token.Valid {
		return 0, errors.New("Token 无效或已经过期")
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return 0, errors.New("Token 中的用户 ID 无效")
	}

	return userID, nil
}
