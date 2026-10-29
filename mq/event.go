package mq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	// ExchangeName 业务事件交换机，topic 类型，便于后续按 like.* / comment.* 拆分路由。
	ExchangeName = "weiyu.events"
	ExchangeKind = "topic"
	// RoutingKey 当前所有通知类事件共用一个队列。
	RoutingKey = "weiyu.notify"
	QueueName  = "weiyu.notify"
	// DLXName / DLQName 死信交换机与死信队列，承载重试超限的消息。
	DLXName = "weiyu.dlx"
	DLQName = "weiyu.notify.dlq"
	// RetryHeader 记录消息已重试次数，超过 MaxRetry 后进入死信队列。
	RetryHeader = "x-retry"
	MaxRetry    = 3
	ConsumerTag = "weiyu-notify-consumer"
	// IdempotentKeyPrefix 消费幂等键前缀，与业务 key 隔离命名空间。
	IdempotentKeyPrefix = "weiyu:mq:idem:"
)

// Event 是投递到 RabbitMQ 的领域事件，消费者据此生成用户通知。
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	PostID    int64     `json:"post_id"`
	ActorID   int64     `json:"actor_id"`
	CommentID int64     `json:"comment_id,omitempty"`
	Content   string    `json:"content,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

const (
	EventLikeCreated    = "like.created"
	EventCommentCreated = "comment.created"
)

var (
	conn    *amqp.Connection
	channel *amqp.Channel
)

// Init 建立连接、声明交换机/队列/死信拓扑，并开启发布确认。
func Init(amqpURL string) error {
	if amqpURL == "" {
		amqpURL = "amqp://guest:guest@127.0.0.1:5672/"
	}

	c, err := amqp.Dial(amqpURL)
	if err != nil {
		return fmt.Errorf("连接 RabbitMQ 失败: %w", err)
	}

	ch, err := c.Channel()
	if err != nil {
		_ = c.Close()
		return fmt.Errorf("创建 RabbitMQ channel 失败: %w", err)
	}

	if err := declareTopology(ch); err != nil {
		_ = ch.Close()
		_ = c.Close()
		return err
	}

	conn, channel = c, ch
	slog.Info("RabbitMQ 已连接", "exchange", ExchangeName, "queue", QueueName)
	return nil
}

func declareTopology(ch *amqp.Channel) error {
	// 1. 死信交换机 + 死信队列
	if err := ch.ExchangeDeclare(DLXName, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("声明死信交换机失败: %w", err)
	}
	if _, err := ch.QueueDeclare(DLQName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("声明死信队列失败: %w", err)
	}
	if err := ch.QueueBind(DLQName, DLQName, DLXName, false, nil); err != nil {
		return fmt.Errorf("绑定死信队列失败: %w", err)
	}

	// 2. 业务交换机
	if err := ch.ExchangeDeclare(ExchangeName, ExchangeKind, true, false, false, false, nil); err != nil {
		return fmt.Errorf("声明业务交换机失败: %w", err)
	}

	// 3. 业务队列，绑定死信交换机：被 reject(requeue=false) 的消息自动转入 DLQ
	args := amqp.Table{
		"x-dead-letter-exchange":    DLXName,
		"x-dead-letter-routing-key": DLQName,
	}
	if _, err := ch.QueueDeclare(QueueName, true, false, false, false, args); err != nil {
		return fmt.Errorf("声明业务队列失败: %w", err)
	}
	if err := ch.QueueBind(QueueName, RoutingKey, ExchangeName, false, nil); err != nil {
		return fmt.Errorf("绑定业务队列失败: %w", err)
	}

	// 4. 开启发布确认：Publish 中通过 DeferredConfirmation 等待 broker 落盘确认
	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("开启发布确认失败: %w", err)
	}

	return nil
}

// Publish 发布一条持久化事件，并同步等待 broker 的 confirm 回执。
// 调用方应传入带超时的 context，避免 broker 异常时长时间阻塞。
func Publish(ctx context.Context, event Event) error {
	if channel == nil || conn == nil || conn.IsClosed() {
		return errors.New("RabbitMQ 未连接")
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("序列化事件失败: %w", err)
	}

	conf, err := channel.PublishWithDeferredConfirmWithContext(
		ctx,
		ExchangeName,
		RoutingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    event.ID,
			Timestamp:    event.CreatedAt,
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("发布事件失败: %w", err)
	}

	acked, err := conf.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("等待 broker 确认失败: %w", err)
	}
	if !acked {
		return errors.New("broker 拒绝接收消息")
	}

	return nil
}

// Healthy 供健康检查使用。
func Healthy() bool {
	if conn == nil {
		return false
	}
	return !conn.IsClosed()
}

func Close() error {
	var err error
	if channel != nil {
		err = channel.Close()
	}
	if conn != nil {
		if cerr := conn.Close(); err == nil {
			err = cerr
		}
	}
	return err
}

// NewEventID 生成 16 字节随机事件 ID，同时用作 MQ MessageId 与消费幂等键。
func NewEventID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
