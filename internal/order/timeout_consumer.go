package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/streadway/amqp"
	"gorm.io/gorm"

	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"
)

// OrderTimeoutConsumer 消费经 DLX 转发后的超时检查消息。
type OrderTimeoutConsumer struct {
	db        *gorm.DB
	publisher mq.Publisher
	logger    *log.Logger
}

func NewOrderTimeoutConsumer(db *gorm.DB, publisher mq.Publisher, logger *log.Logger) *OrderTimeoutConsumer {
	if publisher == nil {
		publisher = mq.NopPublisher{}
	}
	if logger == nil {
		logger = log.Default()
	}
	return &OrderTimeoutConsumer{db: db, publisher: publisher, logger: logger}
}

func (c *OrderTimeoutConsumer) HandleDelivery(delivery amqp.Delivery) error {
	var event events.OrderTimeoutCheckEvent
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		_ = delivery.Nack(false, false)
		return fmt.Errorf("decode order timeout event: %w", err)
	}

	order, changed, err := cancelOrderWithReason(c.db, event.OrderID, event.UserID, OrderCancelReasonPaymentTimeout)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.logger.Printf(
				"order_timeout_skipped reason=order_not_found event_id=%s order_id=%d user_id=%d",
				event.EventID,
				event.OrderID,
				event.UserID,
			)
		case errors.Is(err, ErrInvalidOrderTransition):
			c.logger.Printf(
				"order_timeout_skipped reason=order_not_pending event_id=%s order_id=%d user_id=%d error=%v",
				event.EventID,
				event.OrderID,
				event.UserID,
				err,
			)
		default:
			_ = delivery.Nack(false, true)
			return err
		}

		if err := delivery.Ack(false); err != nil {
			return fmt.Errorf("ack skipped order timeout event: %w", err)
		}
		return nil
	}

	if !changed {
		c.logger.Printf(
			"order_timeout_skipped reason=already_cancelled event_id=%s order_id=%d user_id=%d",
			event.EventID,
			event.OrderID,
			event.UserID,
		)
		if err := delivery.Ack(false); err != nil {
			return fmt.Errorf("ack already-cancelled timeout event: %w", err)
		}
		return nil
	}

	c.publishTimeoutCancellationEvents(order)
	c.logger.Printf(
		"order_timeout_cancelled event_id=%s order_id=%d user_id=%d",
		event.EventID,
		event.OrderID,
		event.UserID,
	)

	if err := delivery.Ack(false); err != nil {
		return fmt.Errorf("ack order timeout event: %w", err)
	}
	return nil
}

func (c *OrderTimeoutConsumer) publishTimeoutCancellationEvents(order *Order) {
	cancelledEvent := events.OrderCancelledEvent{
		BaseEvent: events.NewBaseEvent(events.OrderCancelledType, time.Now()),
		OrderID:   int64(order.ID),
		UserID:    int64(order.UserID),
		Reason:    OrderCancelReasonPaymentTimeout,
	}
	if err := c.publisher.Publish(context.Background(), events.OrderCancelledType, cancelledEvent); err != nil {
		c.logger.Printf(
			"event_publish_failed event_type=%s event_id=%s order_id=%d user_id=%d error=%v",
			cancelledEvent.EventType,
			cancelledEvent.EventID,
			order.ID,
			order.UserID,
			err,
		)
	}

	timeoutCancelledEvent := events.OrderTimeoutCancelledEvent{
		BaseEvent: events.NewBaseEvent(events.OrderTimeoutCancelledType, time.Now()),
		OrderID:   int64(order.ID),
		UserID:    int64(order.UserID),
		Reason:    OrderCancelReasonPaymentTimeout,
	}
	if err := c.publisher.Publish(context.Background(), events.OrderTimeoutCancelledType, timeoutCancelledEvent); err != nil {
		c.logger.Printf(
			"event_publish_failed event_type=%s event_id=%s order_id=%d user_id=%d error=%v",
			timeoutCancelledEvent.EventType,
			timeoutCancelledEvent.EventID,
			order.ID,
			order.UserID,
			err,
		)
	}
}
