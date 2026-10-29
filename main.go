package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wwwrrrjjj/weiyu-go/config"
	"github.com/wwwrrrjjj/weiyu-go/models"
	"github.com/wwwrrrjjj/weiyu-go/mq"
	"github.com/wwwrrrjjj/weiyu-go/router"
	"github.com/wwwrrrjjj/weiyu-go/service"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// 1. 基础设施初始化：MySQL → Redis → RabbitMQ，任一不可用直接失败退出，
	//    避免服务「带病启动」后出现大量难以定位的降级错误。
	if err := config.InitMySQL(
		&models.User{},
		&models.Post{},
		&models.Comment{},
		&models.Like{},
		&models.Notification{},
	); err != nil {
		log.Fatalf("MySQL 初始化失败: %v", err)
	}
	defer func() { _ = config.CloseMySQL() }()

	if err := config.InitRedis(); err != nil {
		log.Fatalf("Redis 初始化失败: %v", err)
	}
	defer func() { _ = config.CloseRedis() }()

	if err := mq.Init(os.Getenv("RABBITMQ_URL")); err != nil {
		log.Fatalf("RabbitMQ 初始化失败: %v", err)
	}
	defer func() { _ = mq.Close() }()

	// 2. 启动 MQ 消费者：点赞/评论事件 → 异步写通知表
	consumerCtx, stopConsumer := context.WithCancel(context.Background())
	defer stopConsumer()

	if err := mq.StartConsumer(consumerCtx, service.HandleEvent); err != nil {
		log.Fatalf("启动通知消费者失败: %v", err)
	}

	// 3. 启动 HTTP 服务
	srv := &http.Server{
		Addr:              listenAddr(),
		Handler:           router.New(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("HTTP 服务已启动", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 服务启动失败: %v", err)
		}
	}()

	// 4. 优雅关闭：先停止接入新请求，再停消费者，最后按 defer 逆序释放连接
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("收到退出信号，开始优雅关闭")

	stopConsumer()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP 服务关闭超时", "err", err)
	}

	slog.Info("服务已退出")
}

// listenAddr 本地开发只监听回环地址，生产容器监听全部网卡。
func listenAddr() string {
	if addr := os.Getenv("HTTP_ADDR"); addr != "" {
		return addr
	}
	if os.Getenv("APP_ENV") == "production" {
		return ":8080"
	}
	return "127.0.0.1:8080"
}
