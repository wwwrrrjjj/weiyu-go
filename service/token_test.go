package service

import (
	"context"
	"testing"
	"time"
)

// config.Redis 在单元测试里是 nil，吊销相关函数必须安全降级：
// 不 panic、不阻塞，且不能把任何 token 误判成已吊销（否则全站 401）。
func TestTokenRevocationDegradesWithoutRedis(t *testing.T) {
	ctx := context.Background()

	if err := RevokeToken(ctx, "some-jti", time.Minute); err != nil {
		t.Fatalf("Redis 不可用时吊销应静默成功: %v", err)
	}
	if IsTokenRevoked(ctx, "some-jti") {
		t.Fatal("Redis 不可用时不应把 token 判为已吊销")
	}
}

func TestRevokeTokenIgnoresEmptyJTI(t *testing.T) {
	if err := RevokeToken(context.Background(), "", time.Minute); err != nil {
		t.Fatalf("空 jti 应直接返回: %v", err)
	}
	if IsTokenRevoked(context.Background(), "") {
		t.Fatal("空 jti 不应被判定为已吊销")
	}
}
