package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/streadway/amqp"
	"gorm.io/gorm"

	"go-commerce/internal/inbox"
	"go-commerce/internal/outbox"
	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"
)

const orderTimeoutConsumerName = "order.timeout"

// OrderTimeoutConsumer 消费经 DLX 转发后的超时检查消息。
type OrderTimeoutConsumer struct {
	db         *gorm.DB
	publisher  mq.Publisher
	logger     *log.Logger
	outboxRepo outbox.EventRepository
}

func NewOrderTimeoutConsumer(db *gorm.DB, publisher mq.Publisher, logger *log.Logger) *OrderTimeoutConsumer {
	if publisher == nil {
		publisher = mq.NopPublisher{}
	}
	if logger == nil {
		logger = log.Default()
	}
	return &OrderTimeoutConsumer{db: db, publisher: publisher, logger: logger, outboxRepo: outbox.NewRepository(db)}
}

func (c *OrderTimeoutConsumer) HandleDelivery(delivery amqp.Delivery) error {
	var event events.OrderTimeoutCheckEvent
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		_ = delivery.Nack(false, false)
		return fmt.Errorf("decode order timeout event: %w", err)
	}
	if event.EventID == "" {
		_ = delivery.Nack(false, false)
		return inbox.ErrMissingEventID
	}
	if event.EventType == "" {
		event.EventType = events.OrderTimeoutCheckType
	}

	outcome := "cancelled"
	processed, err := inbox.ProcessOnce(context.Background(), c.db, orderTimeoutConsumerName, event.EventID, event.EventType, func(tx *gorm.DB) error {
		_, changed, err := cancelOrderWithReasonInTx(tx, event.OrderID, event.UserID, OrderCancelReasonPaymentTimeout, func(tx *gorm.DB, order *Order) error {
			cancelledEvent := events.OrderCancelledEvent{
				BaseEvent: events.NewBaseEvent(events.OrderCancelledType, time.Now()),
				OrderID:   int64(order.ID),
				UserID:    int64(order.UserID),
				Reason:    OrderCancelReasonPaymentTimeout,
			}
			if _, err := c.outboxRepo.Create(tx.Statement.Context, tx, outbox.NewEventInput{
				AggregateType: "order",
				AggregateID:   strconv.FormatUint(uint64(order.ID), 10),
				EventType:     events.OrderCancelledType,
				Payload:       cancelledEvent,
			}); err != nil {
				return err
			}

			timeoutCancelledEvent := events.OrderTimeoutCancelledEvent{
				BaseEvent: events.NewBaseEvent(events.OrderTimeoutCancelledType, time.Now()),
				OrderID:   int64(order.ID),
				UserID:    int64(order.UserID),
				Reason:    OrderCancelReasonPaymentTimeout,
			}
			_, err := c.outboxRepo.Create(tx.Statement.Context, tx, outbox.NewEventInput{
				AggregateType: "order",
				AggregateID:   strconv.FormatUint(uint64(order.ID), 10),
				EventType:     events.OrderTimeoutCancelledType,
				Payload:       timeoutCancelledEvent,
			})
			return err
		})
		if err != nil {
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				outcome = "order_not_found"
				return nil
			case errors.Is(err, ErrInvalidOrderTransition):
				outcome = "order_not_pending"
				return nil
			default:
				return err
			}
		}
		if !changed {
			outcome = "already_cancelled"
		}
		return nil
	})
	if err != nil {
		_ = delivery.Nack(false, true)
		return err
	}
	if !processed {
		c.logger.Printf(
			"order_timeout_duplicate consumer=%s event_id=%s order_id=%d user_id=%d",
			orderTimeoutConsumerName,
			event.EventID,
			event.OrderID,
			event.UserID,
		)
		if err := delivery.Ack(false); err != nil {
			return fmt.Errorf("ack duplicate order timeout event: %w", err)
		}
		return nil
	}

	switch outcome {
	case "order_not_found":
		c.logger.Printf(
			"order_timeout_skipped reason=order_not_found event_id=%s order_id=%d user_id=%d",
			event.EventID,
			event.OrderID,
			event.UserID,
		)
	case "order_not_pending":
		c.logger.Printf(
			"order_timeout_skipped reason=order_not_pending event_id=%s order_id=%d user_id=%d",
			event.EventID,
			event.OrderID,
			event.UserID,
		)
	case "already_cancelled":
		c.logger.Printf(
			"order_timeout_skipped reason=already_cancelled event_id=%s order_id=%d user_id=%d",
			event.EventID,
			event.OrderID,
			event.UserID,
		)
	default:
		c.logger.Printf(
			"order_timeout_cancelled event_id=%s order_id=%d user_id=%d",
			event.EventID,
			event.OrderID,
			event.UserID,
		)
	}

	if err := delivery.Ack(false); err != nil {
		return fmt.Errorf("ack order timeout event: %w", err)
	}
	return nil
}
