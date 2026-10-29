package config

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis 全局客户端，供缓存、点赞计数、MQ 幂等去重共用。
var Redis *redis.Client

// InitRedis 初始化 Redis 连接，并做一次 Ping 探活。
func InitRedis() error {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}

	db, err := strconv.Atoi(envOrDefault("REDIS_DB", "0"))
	if err != nil {
		return fmt.Errorf("REDIS_DB 配置不合法: %w", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     os.Getenv("REDIS_PASSWORD"),
		DB:           db,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		PoolSize:     50,
		MinIdleConns: 5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return fmt.Errorf("redis 连接失败: %w", err)
	}

	Redis = client
	return nil
}

// PingRedis 供健康检查使用。
func PingRedis(ctx context.Context) error {
	if Redis == nil {
		return fmt.Errorf("redis 未初始化")
	}
	return Redis.Ping(ctx).Err()
}

func CloseRedis() error {
	if Redis == nil {
		return nil
	}
	return Redis.Close()
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
