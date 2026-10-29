package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/wwwrrrjjj/weiyu-go/config"
)

const (
	// 单条消息的处理超时，避免 handler 卡死拖住整个消费者。
	handleTimeout = 10 * time.Second
	// 幂等键保留 24 小时，覆盖 MQ 重投的合理时间窗口。
	idempotentTTL = 24 * time.Hour
	prefetchCount = 10
)

// Handler 处理一条事件，返回 error 表示需要重试。
type Handler func(ctx context.Context, event Event) error

// StartConsumer 启动消费者：手动 ack、失败重试、超限进入死信队列。
// 返回后消费者在后台 goroutine 中运行，直到 ctx 被取消。
func StartConsumer(ctx context.Context, handler Handler) error {
	if conn == nil {
		return errors.New("RabbitMQ 未连接")
	}

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("创建消费 channel 失败: %w", err)
	}

	// 预取 10 条，避免一个消费者一次性拉走大量消息造成其它实例空转。
	if err := ch.Qos(prefetchCount, 0, false); err != nil {
		_ = ch.Close()
		return fmt.Errorf("设置预取数量失败: %w", err)
	}

	// autoAck=false：处理成功后才确认，进程崩溃时消息会重回队列
	deliveries, err := ch.Consume(QueueName, ConsumerTag, false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		return fmt.Errorf("启动消费者失败: %w", err)
	}

	go func() {
		defer func() { _ = ch.Close() }()
		for {
			select {
			case <-ctx.Done():
				slog.Info("通知消费者收到退出信号")
				return
			case delivery, ok := <-deliveries:
				if !ok {
					slog.Warn("消费 channel 已关闭，消费者退出")
					return
				}
				handleDelivery(ctx, ch, delivery, handler)
			}
		}
	}()

	slog.Info("通知消费者已启动", "queue", QueueName, "prefetch", prefetchCount)
	return nil
}

func handleDelivery(ctx context.Context, ch *amqp.Channel, delivery amqp.Delivery, handler Handler) {
	var event Event
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		// 无法解析的消息不具备重试价值，直接投递到死信队列
		slog.Error("消息反序列化失败，转入死信队列", "err", err)
		_ = delivery.Reject(false)
		return
	}
	if event.ID == "" || event.PostID <= 0 || event.ActorID <= 0 {
		slog.Error("消息字段不完整，转入死信队列", "message_id", delivery.MessageId)
		_ = delivery.Reject(false)
		return
	}

	retry := headerInt(delivery.Headers, RetryHeader)

	// 幂等：同一条消息（相同 event.ID）只处理一次
	idempotentKey := IdempotentKeyPrefix + event.ID
	if config.Redis != nil {
		acquired, err := config.Redis.SetNX(ctx, idempotentKey, 1, idempotentTTL).Result()
		switch {
		case err != nil:
			// Redis 不可用时放行，由 Notification.EventID 唯一索引兜底
			slog.Warn("幂等去重失败，降级依赖数据库唯一索引", "err", err)
		case !acquired:
			slog.Info("重复消息，直接确认", "event_id", event.ID)
			_ = delivery.Ack(false)
			return
		}
	}

	handleCtx, cancel := context.WithTimeout(ctx, handleTimeout)
	err := handler(handleCtx, event)
	cancel()

	if err == nil {
		_ = delivery.Ack(false)
		return
	}

	// 处理失败：释放幂等占位，否则重试会被误判为重复消息
	if config.Redis != nil {
		_ = config.Redis.Del(ctx, idempotentKey).Err()
	}

	if retry < MaxRetry {
		if rerr := republish(ctx, ch, event, retry+1); rerr == nil {
			slog.Warn("事件处理失败，重新入队重试", "event_id", event.ID, "retry", retry+1, "err", err)
			_ = delivery.Ack(false)
			return
		} else {
			slog.Error("重新入队失败", "event_id", event.ID, "err", rerr)
		}
	}

	slog.Error("事件处理失败且重试超限，转入死信队列",
		"event_id", event.ID, "retry", retry, "err", err)
	_ = delivery.Reject(false)
}

// republish 把消息带着递增的重试次数重新投回业务队列。
// 直接 publish 而非 nack(requeue=true)，避免失败消息在队首反复空转。
func republish(ctx context.Context, ch *amqp.Channel, event Event, retry int) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return ch.PublishWithContext(ctx, "", QueueName, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    event.ID,
		Timestamp:    event.CreatedAt,
		Headers:      amqp.Table{RetryHeader: int32(retry)},
		Body:         body,
	})
}

func headerInt(headers amqp.Table, key string) int {
	switch v := headers[key].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
