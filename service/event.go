package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/wwwrrrjjj/weiyu-go/mq"
)

// publishTimeout 事件投递的等待上限，超过则认为通知链路暂不可用。
const publishTimeout = 2 * time.Second

// publishEvent 异步通知的事件投递。
// 通知属于旁路能力，投递失败不能影响点赞/评论主流程，
// 因此这里只记录日志、不向上返回错误（后续可用本地消息表做最终一致补偿）。
func publishEvent(ctx context.Context, event mq.Event) {
	// 请求 context 可能因客户端断开而取消，通知投递不应被牵连，这里脱钩后单独设超时
	pubCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), publishTimeout)
	defer cancel()

	if err := mq.Publish(pubCtx, event); err != nil {
		slog.Warn("投递事件失败，通知链路降级",
			"type", event.Type, "post_id", event.PostID, "event_id", event.ID, "err", err)
	}
}
