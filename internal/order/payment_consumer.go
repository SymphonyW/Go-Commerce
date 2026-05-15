package order

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/streadway/amqp"
	"gorm.io/gorm"

	"go-commerce/pkg/events"
)

var (
	ErrOrderPaymentMismatch = errors.New("order payment mismatch")
	ErrOrderCannotBePaid    = errors.New("order cannot be paid")
)

// PaymentSucceededConsumer 将支付成功事件转化为订单状态迁移。
type PaymentSucceededConsumer struct {
	db     *gorm.DB
	logger *log.Logger
}

func NewPaymentSucceededConsumer(db *gorm.DB, logger *log.Logger) *PaymentSucceededConsumer {
	if logger == nil {
		logger = log.Default()
	}
	return &PaymentSucceededConsumer{db: db, logger: logger}
}

func (c *PaymentSucceededConsumer) HandleDelivery(delivery amqp.Delivery) error {
	var event events.PaymentSucceededEvent
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		_ = delivery.Nack(false, false)
		return fmt.Errorf("decode payment.succeeded event: %w", err)
	}

	if err := MarkOrderPaid(c.db, event.OrderID, event.UserID, event.Amount); err != nil {
		_ = delivery.Nack(false, false)
		return err
	}

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
func MarkOrderPaid(db *gorm.DB, orderID, userID int64, amount float64) error {
	var order Order
	if err := db.Where("id = ? AND user_id = ?", orderID, userID).First(&order).Error; err != nil {
		return err
	}
	if order.TotalAmount != amount {
		return ErrOrderPaymentMismatch
	}
	if order.Status == "paid" {
		return nil
	}
	if order.Status != "pending" {
		return ErrOrderCannotBePaid
	}

	order.Status = "paid"
	return db.Save(&order).Error
}
