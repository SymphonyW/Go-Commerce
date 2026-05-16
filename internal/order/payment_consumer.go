package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/streadway/amqp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"go-commerce/internal/outbox"
	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"
)

var (
	ErrOrderPaymentMismatch = errors.New("order payment mismatch")
	ErrOrderCannotBePaid    = errors.New("order cannot be paid")
)

// PaymentSucceededConsumer 将支付成功事件转化为订单状态迁移。
type PaymentSucceededConsumer struct {
	db         *gorm.DB
	publisher  mq.Publisher
	logger     *log.Logger
	outboxRepo outbox.EventRepository
}

func NewPaymentSucceededConsumer(db *gorm.DB, publisher mq.Publisher, logger *log.Logger) *PaymentSucceededConsumer {
	if publisher == nil {
		publisher = mq.NopPublisher{}
	}
	if logger == nil {
		logger = log.Default()
	}
	return &PaymentSucceededConsumer{db: db, publisher: publisher, logger: logger, outboxRepo: outbox.NewRepository(db)}
}

func (c *PaymentSucceededConsumer) HandleDelivery(delivery amqp.Delivery) error {
	var event events.PaymentSucceededEvent
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		_ = delivery.Nack(false, false)
		return fmt.Errorf("decode payment.succeeded event: %w", err)
	}

	_, changed, err := MarkOrderPaid(c.db, event.OrderID, event.UserID, event.Amount, func(tx *gorm.DB, order *Order) error {
		statusEvent := newOrderStatusChangedEvent(context.Background(), events.OrderPaidType, order, OrderStatusPending, OrderStatusPaid)
		_, err := c.outboxRepo.Create(context.Background(), tx, outbox.NewEventInput{
			AggregateType: "order",
			AggregateID:   strconv.FormatUint(uint64(order.ID), 10),
			EventType:     events.OrderPaidType,
			Payload:       statusEvent,
		})
		return err
	})
	if err != nil {
		_ = delivery.Nack(false, false)
		return err
	}
	_ = changed

	c.logger.Printf(
		"payment_event_consumed event_type=%s event_id=%s payment_id=%d order_id=%d user_id=%d",
		event.EventType,
		event.EventID,
		event.PaymentID,
		event.OrderID,
		event.UserID,
	)

	if err := delivery.Ack(false); err != nil {
		return fmt.Errorf("ack payment.succeeded event: %w", err)
	}
	return nil
}

// MarkOrderPaid 只允许金额一致的 pending 订单进入 paid；重复事件保持幂等。
func MarkOrderPaid(db *gorm.DB, orderID, userID int64, amount float64, afterChange func(tx *gorm.DB, order *Order) error) (*Order, bool, error) {
	var order Order
	changed := false

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", orderID, userID).
			First(&order).Error; err != nil {
			return err
		}
		if order.TotalAmount != amount {
			return ErrOrderPaymentMismatch
		}
		if order.Status == OrderStatusPaid {
			return nil
		}
		if err := TransitionTo(&order, OrderStatusPaid); err != nil {
			return fmt.Errorf("%w: %v", ErrOrderCannotBePaid, err)
		}

		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		if afterChange != nil {
			if err := afterChange(tx, &order); err != nil {
				return err
			}
		}
		changed = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &order, changed, nil
}
