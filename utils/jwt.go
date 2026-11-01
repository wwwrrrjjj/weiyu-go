package utils

import (
	"crypto/rand"
	"encoding/hex"
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

// TokenInfo 是校验通过后从 token 里取出的信息。
//
// 除了用户 ID，还带出 jti 和过期时间：
//   - jti 用于登出时把这枚 token 加入吊销名单；
//   - ExpiresAt 用于决定吊销名单里那条记录该保留多久。
type TokenInfo struct {
	UserID    int64
	JTI       string
	ExpiresAt time.Time
}

// 从环境变量获取 JWT 签名密钥
func getJWTSecret() ([]byte, error) {
	secret := os.Getenv("JWT_SECRET")

	if len(secret) < 32 {
		return nil, errors.New("JWT_SECRET 未配置或长度不足 32 字节")
	}

	return []byte(secret), nil
}

// newTokenID 生成 token 的唯一编号（jti）。
// 用随机数而不是自增，保证不同用户、不同登录之间不会撞号。
func newTokenID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// 随机源异常时退化用纳秒时间戳，至少保证不重复
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(buf[:])
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
		ID:        newTokenID(), // jti：这张 token 的唯一编号，登出时按它拉黑
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL)),
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	)

	return token.SignedString(secret)
}

// ParseToken 验证 JWT，并返回用户信息。
func ParseToken(tokenString string) (*TokenInfo, error) {
	secret, err := getJWTSecret()
	if err != nil {
		return nil, err
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
		return nil, errors.New("Token 无效或已经过期")
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return nil, errors.New("Token 中的用户 ID 无效")
	}

	info := &TokenInfo{
		UserID: userID,
		JTI:    claims.ID,
	}
	if claims.ExpiresAt != nil {
		info.ExpiresAt = claims.ExpiresAt.Time
	}

	return info, nil
}
