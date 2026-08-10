package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/streadway/amqp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"go-commerce/internal/inbox"
	"go-commerce/internal/outbox"
	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"
	"go-commerce/pkg/observability"
)

var (
	ErrOrderPaymentMismatch = errors.New("order payment mismatch")
	ErrOrderCannotBePaid    = errors.New("order cannot be paid")
)

const paymentSucceededConsumerName = "order.payment_succeeded"

// PaymentSucceededConsumer 将支付成功事件转化为订单状态迁移。
type PaymentSucceededConsumer struct {
	db         *gorm.DB
	publisher  mq.Publisher
	logger     *log.Logger
	outboxRepo outbox.EventRepository
	metrics    *observability.Metrics
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

func (c *PaymentSucceededConsumer) SetMetrics(metrics *observability.Metrics) {
	c.metrics = metrics
}

func (c *PaymentSucceededConsumer) HandleDelivery(delivery amqp.Delivery) error {
	ctx := observability.ContextFromAMQPDelivery(context.Background(), delivery)
	correlationRequestID := observability.RequestIDFromContext(ctx)
	correlationTraceID := observability.TraceIDFromContext(ctx)
	ctx, span := observability.StartSpan(ctx,
		"rabbitmq consume "+events.PaymentSucceededType,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("event.type", events.PaymentSucceededType),
			attribute.String("consumer.name", paymentSucceededConsumerName),
			attribute.String("correlation.request_id", correlationRequestID),
			attribute.String("correlation.trace_id", correlationTraceID),
		),
	)
	var handleErr error
	defer func() { observability.EndSpan(span, handleErr) }()

	var event events.PaymentSucceededEvent
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		_ = delivery.Nack(false, false)
		c.metrics.RecordConsumerFailure(paymentSucceededConsumerName, events.PaymentSucceededType, false)
		handleErr = fmt.Errorf("decode payment.succeeded event: %w", err)
		return handleErr
	}
	span.SetAttributes(attribute.String("event.id", event.EventID), attribute.Int64("order.id", event.OrderID), attribute.Int64("payment.id", event.PaymentID))
	if event.EventID == "" {
		_ = delivery.Nack(false, false)
		c.metrics.RecordConsumerFailure(paymentSucceededConsumerName, events.PaymentSucceededType, false)
		handleErr = inbox.ErrMissingEventID
		return handleErr
	}
	if event.EventType == "" {
		event.EventType = events.PaymentSucceededType
	}

	changed := false
	processed, err := inbox.ProcessOnce(ctx, c.db, paymentSucceededConsumerName, event.EventID, event.EventType, func(tx *gorm.DB) error {
		_, paidChanged, err := markOrderPaidInTx(tx, event.OrderID, event.UserID, event.AmountCents, func(tx *gorm.DB, order *Order) error {
			statusEvent := newOrderStatusChangedEvent(ctx, events.OrderPaidType, order, OrderStatusPending, OrderStatusPaid)
			_, err := c.outboxRepo.Create(ctx, tx, outbox.NewEventInput{
				AggregateType: "order",
				AggregateID:   strconv.FormatUint(uint64(order.ID), 10),
				EventType:     events.OrderPaidType,
				Payload:       statusEvent,
			})
			return err
		})
		changed = paidChanged
		return err
	})
	if err != nil {
		if isPermanentPaymentEventError(err) {
			_ = delivery.Nack(false, false)
			c.metrics.RecordConsumerFailure(paymentSucceededConsumerName, event.EventType, false)
			handleErr = err
			return handleErr
		}
		_ = delivery.Nack(false, true)
		c.metrics.RecordConsumerFailure(paymentSucceededConsumerName, event.EventType, true)
		handleErr = err
		return handleErr
	}
	if !processed {
		c.logger.Printf(
			"payment_event_duplicate consumer=%s event_type=%s event_id=%s payment_id=%d order_id=%d user_id=%d",
			paymentSucceededConsumerName,
			event.EventType,
			event.EventID,
			event.PaymentID,
			event.OrderID,
			event.UserID,
		)
		if err := delivery.Ack(false); err != nil {
			return fmt.Errorf("ack duplicate payment.succeeded event: %w", err)
		}
		return nil
	}
	_ = changed
	c.metrics.RecordOrderPaid(changed)

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
func MarkOrderPaid(db *gorm.DB, orderID, userID int64, amountCents int64, afterChange func(tx *gorm.DB, order *Order) error) (*Order, bool, error) {
	var order *Order
	changed := false

	err := db.Transaction(func(tx *gorm.DB) error {
		updated, didChange, err := markOrderPaidInTx(tx, orderID, userID, amountCents, afterChange)
		order = updated
		changed = didChange
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return order, changed, nil
}

func markOrderPaidInTx(tx *gorm.DB, orderID, userID int64, amountCents int64, afterChange func(tx *gorm.DB, order *Order) error) (*Order, bool, error) {
	var order Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", orderID, userID).
		First(&order).Error; err != nil {
		return nil, false, err
	}
	if order.TotalAmountCents != amountCents {
		return nil, false, ErrOrderPaymentMismatch
	}
	if order.Status == OrderStatusPaid {
		return &order, false, nil
	}
	if err := TransitionTo(&order, OrderStatusPaid); err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrOrderCannotBePaid, err)
	}

	if err := tx.Save(&order).Error; err != nil {
		return nil, false, err
	}
	if afterChange != nil {
		if err := afterChange(tx, &order); err != nil {
			return nil, false, err
		}
	}
	return &order, true, nil
}

func isPermanentPaymentEventError(err error) bool {
	return errors.Is(err, inbox.ErrMissingEventID) ||
		errors.Is(err, gorm.ErrRecordNotFound) ||
		errors.Is(err, ErrOrderPaymentMismatch) ||
		errors.Is(err, ErrOrderCannotBePaid)
}
