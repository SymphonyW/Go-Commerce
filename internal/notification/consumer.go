package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/streadway/amqp"
	"gorm.io/gorm"

	"go-commerce/internal/inbox"
	"go-commerce/pkg/events"
)

const orderCreatedConsumerName = "notification.order_created"

// Consumer 处理订单创建通知事件。
type Consumer struct {
	db     *gorm.DB
	logger *log.Logger
}

func NewConsumer(db *gorm.DB, logger *log.Logger) *Consumer {
	if logger == nil {
		logger = log.Default()
	}
	return &Consumer{db: db, logger: logger}
}

// HandleDelivery 反序列化事件并发送确认；坏消息不重回队列，避免 poison message 无限循环。
func (c *Consumer) HandleDelivery(delivery amqp.Delivery) error {
	var event events.OrderCreatedEvent
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		_ = delivery.Nack(false, false)
		return fmt.Errorf("decode order.created event: %w", err)
	}
	if event.EventID == "" {
		_ = delivery.Nack(false, false)
		return inbox.ErrMissingEventID
	}
	if event.EventType == "" {
		event.EventType = events.OrderCreatedType
	}

	processed, err := inbox.ProcessOnce(context.Background(), c.db, orderCreatedConsumerName, event.EventID, event.EventType, nil)
	if err != nil {
		_ = delivery.Nack(false, true)
		return err
	}
	if !processed {
		c.logger.Printf(
			"notification_event_duplicate consumer=%s event_type=%s event_id=%s order_id=%d user_id=%d",
			orderCreatedConsumerName,
			event.EventType,
			event.EventID,
			event.OrderID,
			event.UserID,
		)
		if err := delivery.Ack(false); err != nil {
			return fmt.Errorf("ack duplicate order.created event: %w", err)
		}
		return nil
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
