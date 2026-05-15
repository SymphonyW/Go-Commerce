package notification

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/streadway/amqp"

	"go-commerce/pkg/events"
)

// Consumer 处理订单创建通知事件。
type Consumer struct {
	logger *log.Logger
}

func NewConsumer(logger *log.Logger) *Consumer {
	if logger == nil {
		logger = log.Default()
	}
	return &Consumer{logger: logger}
}

// HandleDelivery 反序列化事件并发送确认；坏消息不重回队列，避免 poison message 无限循环。
func (c *Consumer) HandleDelivery(delivery amqp.Delivery) error {
	var event events.OrderCreatedEvent
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		_ = delivery.Nack(false, false)
		return fmt.Errorf("decode order.created event: %w", err)
	}

	c.logger.Printf(
		"notification_event_received event_type=%s event_id=%s order_id=%d user_id=%d action=%q",
		event.EventType,
		event.EventID,
		event.OrderID,
		event.UserID,
		"发送下单成功通知",
	)

	if err := delivery.Ack(false); err != nil {
		return fmt.Errorf("ack order.created event: %w", err)
	}
	return nil
}
