package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/streadway/amqp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"

	"go-commerce/internal/inbox"
	"go-commerce/pkg/events"
	"go-commerce/pkg/observability"
)

const orderCreatedConsumerName = "notification.order_created"

// Consumer 处理订单创建通知事件。
type Consumer struct {
	db      *gorm.DB
	logger  *log.Logger
	metrics *observability.Metrics
}

func NewConsumer(db *gorm.DB, logger *log.Logger) *Consumer {
	if logger == nil {
		logger = log.Default()
	}
	return &Consumer{db: db, logger: logger}
}

func (c *Consumer) SetMetrics(metrics *observability.Metrics) {
	c.metrics = metrics
}

// HandleDelivery 反序列化事件并发送确认；坏消息不重回队列，避免 poison message 无限循环。
func (c *Consumer) HandleDelivery(delivery amqp.Delivery) error {
	ctx := observability.ContextFromAMQPDelivery(context.Background(), delivery)
	correlationRequestID := observability.RequestIDFromContext(ctx)
	correlationTraceID := observability.TraceIDFromContext(ctx)
	ctx, span := observability.StartSpan(ctx,
		"rabbitmq consume "+events.OrderCreatedType,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("event.type", events.OrderCreatedType),
			attribute.String("consumer.name", orderCreatedConsumerName),
			attribute.String("correlation.request_id", correlationRequestID),
			attribute.String("correlation.trace_id", correlationTraceID),
		),
	)
	var handleErr error
	defer func() { observability.EndSpan(span, handleErr) }()

	var event events.OrderCreatedEvent
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		_ = delivery.Nack(false, false)
		c.metrics.RecordConsumerFailure(orderCreatedConsumerName, events.OrderCreatedType, false)
		handleErr = fmt.Errorf("decode order.created event: %w", err)
		return handleErr
	}
	span.SetAttributes(attribute.String("event.id", event.EventID), attribute.Int64("order.id", event.OrderID))
	if event.EventID == "" {
		_ = delivery.Nack(false, false)
		c.metrics.RecordConsumerFailure(orderCreatedConsumerName, events.OrderCreatedType, false)
		handleErr = inbox.ErrMissingEventID
		return handleErr
	}
	if event.EventType == "" {
		event.EventType = events.OrderCreatedType
	}

	processed, err := inbox.ProcessOnce(ctx, c.db, orderCreatedConsumerName, event.EventID, event.EventType, nil)
	if err != nil {
		_ = delivery.Nack(false, true)
		c.metrics.RecordConsumerFailure(orderCreatedConsumerName, event.EventType, true)
		handleErr = err
		return handleErr
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
