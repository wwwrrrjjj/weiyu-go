//go:build integration

// 集成测试：需要本地可用的 Redis 与 RabbitMQ。
// 运行方式：
//
//	REDIS_ADDR=127.0.0.1:6379 REDIS_DB=15 \
//	RABBITMQ_URL=amqp://guest:guest@127.0.0.1:5672/ \
//	go test -tags integration ./mq -run TestRabbitMQ -v -count=1
//
// 注意：测试会清空 REDIS_DB 指定的库以及 weiyu.notify 相关队列，请勿指向生产环境。
package mq

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/wwwrrrjjj/weiyu-go/config"
)

func amqpURL() string {
	if u := os.Getenv("RABBITMQ_URL"); u != "" {
		return u
	}
	return "amqp://guest:guest@127.0.0.1:5672/"
}

func setupMQ(t *testing.T) {
	t.Helper()

	if err := config.InitRedis(); err != nil {
		t.Fatalf("初始化 Redis 失败: %v", err)
	}
	t.Cleanup(func() { _ = config.CloseRedis() })
	if err := config.Redis.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("清空测试 Redis 库失败: %v", err)
	}

	if err := Init(amqpURL()); err != nil {
		t.Fatalf("初始化 RabbitMQ 失败: %v", err)
	}
	t.Cleanup(func() { _ = Close() })

	purgeQueue(t, QueueName)
	purgeQueue(t, DLQName)
}

func startConsumer(t *testing.T, handler Handler) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	if err := StartConsumer(ctx, handler); err != nil {
		cancel()
		t.Fatalf("启动消费者失败: %v", err)
	}

	t.Cleanup(func() {
		cancel()
		time.Sleep(100 * time.Millisecond) // 给消费循环一点退出时间
	})
}

func purgeQueue(t *testing.T, queue string) {
	t.Helper()

	conn, err := amqp.Dial(amqpURL())
	if err != nil {
		t.Fatalf("连接 RabbitMQ 失败: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("创建 channel 失败: %v", err)
	}
	defer ch.Close()

	if _, err := ch.QueuePurge(queue, false); err != nil {
		t.Fatalf("清空队列 %s 失败: %v", queue, err)
	}
}

func queueDepth(queue string) (int, error) {
	conn, err := amqp.Dial(amqpURL())
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return 0, err
	}
	defer ch.Close()

	q, err := ch.QueueInspect(queue)
	if err != nil {
		return 0, err
	}
	return q.Messages, nil
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// TestRabbitMQPublishConsume 验证「发布确认 + 消费 ack」主链路。
func TestRabbitMQPublishConsume(t *testing.T) {
	setupMQ(t)

	received := make(chan Event, 4)
	startConsumer(t, func(_ context.Context, event Event) error {
		received <- event
		return nil
	})

	event := Event{
		ID:        NewEventID(),
		Type:      EventLikeCreated,
		PostID:    10,
		ActorID:   20,
		CreatedAt: time.Now(),
	}
	if err := Publish(context.Background(), event); err != nil {
		t.Fatalf("发布事件失败: %v", err)
	}

	select {
	case got := <-received:
		if got.ID != event.ID || got.PostID != event.PostID || got.ActorID != event.ActorID || got.Type != event.Type {
			t.Fatalf("收到的事件与原事件不一致: got=%+v want=%+v", got, event)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("5s 内未消费到消息")
	}
}

// TestRabbitMQConsumeIdempotent 验证相同事件 ID 只会被处理一次。
func TestRabbitMQConsumeIdempotent(t *testing.T) {
	setupMQ(t)

	var calls int32
	startConsumer(t, func(_ context.Context, _ Event) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	event := Event{
		ID:        NewEventID(),
		Type:      EventLikeCreated,
		PostID:    11,
		ActorID:   21,
		CreatedAt: time.Now(),
	}

	for i := 0; i < 3; i++ {
		if err := Publish(context.Background(), event); err != nil {
			t.Fatalf("第 %d 次发布失败: %v", i+1, err)
		}
	}

	// 等待足够长时间，确认重复消息已被消费但未触发 handler
	time.Sleep(1 * time.Second)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("幂等失效：handler 被调用 %d 次，期望 1 次", got)
	}

	if depth, err := queueDepth(QueueName); err != nil || depth != 0 {
		t.Fatalf("队列应已排空, depth=%d err=%v", depth, err)
	}
}

// TestRabbitMQRetryThenDeadLetter 验证失败重试与死信队列。
func TestRabbitMQRetryThenDeadLetter(t *testing.T) {
	setupMQ(t)

	var calls int32
	startConsumer(t, func(_ context.Context, _ Event) error {
		atomic.AddInt32(&calls, 1)
		return context.DeadlineExceeded // 模拟处理失败
	})

	event := Event{
		ID:        NewEventID(),
		Type:      EventCommentCreated,
		PostID:    12,
		ActorID:   22,
		CreatedAt: time.Now(),
	}
	if err := Publish(context.Background(), event); err != nil {
		t.Fatalf("发布事件失败: %v", err)
	}

	// 首次投递 + MaxRetry 次重试
	wantCalls := int32(MaxRetry + 1)
	if ok := waitFor(t, 8*time.Second, func() bool {
		return atomic.LoadInt32(&calls) >= wantCalls
	}); !ok {
		t.Fatalf("未达到预期重试次数: calls=%d want=%d", atomic.LoadInt32(&calls), wantCalls)
	}

	// 超过重试上限后应进入死信队列
	if ok := waitFor(t, 5*time.Second, func() bool {
		depth, err := queueDepth(DLQName)
		return err == nil && depth >= 1
	}); !ok {
		t.Fatal("消息未进入死信队列")
	}

	if got := atomic.LoadInt32(&calls); got != wantCalls {
		t.Fatalf("重试次数不符合预期: calls=%d want=%d", got, wantCalls)
	}
}
