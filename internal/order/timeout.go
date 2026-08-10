package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/streadway/amqp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"go-commerce/pkg/events"
	"go-commerce/pkg/observability"
)

const (
	DefaultOrderPaymentTimeout = 15 * time.Minute

	OrderTimeoutDelayExchange = "order.timeout.delay.exchange"
	OrderTimeoutDLX           = "order.timeout.dlx"
	OrderTimeoutDelayQueue    = "order.timeout.delay.queue"
	OrderTimeoutCancelQueue   = "order.timeout.cancel.queue"
)

// TimeoutScheduler 抽象订单超时消息投递，便于业务层与 RabbitMQ 细节解耦。
type TimeoutScheduler interface {
	Schedule(ctx context.Context, event events.OrderTimeoutCheckEvent, delay time.Duration) error
}

// NopTimeoutScheduler 让 RabbitMQ 暂不可用时主链路仍可继续运行。
type NopTimeoutScheduler struct{}

func (NopTimeoutScheduler) Schedule(ctx context.Context, event events.OrderTimeoutCheckEvent, delay time.Duration) error {
	return nil
}

type timeoutPublishingChannel interface {
	Publish(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

// RabbitMQTimeoutScheduler 把超时检查消息投递到 TTL 队列对应的交换机。
type RabbitMQTimeoutScheduler struct {
	channel  timeoutPublishingChannel
	exchange string
	now      func() time.Time
}

func NewRabbitMQTimeoutScheduler(channel timeoutPublishingChannel, exchange string) *RabbitMQTimeoutScheduler {
	if exchange == "" {
		exchange = OrderTimeoutDelayExchange
	}
	return &RabbitMQTimeoutScheduler{
		channel:  channel,
		exchange: exchange,
		now:      time.Now,
	}
}

func (s *RabbitMQTimeoutScheduler) Schedule(ctx context.Context, event events.OrderTimeoutCheckEvent, delay time.Duration) error {
	if s == nil || s.channel == nil {
		return errors.New("rabbitmq timeout scheduler channel is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay <= 0 {
		delay = time.Millisecond
	}

	ctx, span := observability.StartSpan(ctx,
		"rabbitmq publish "+events.OrderTimeoutCheckType,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination.name", s.exchange),
			attribute.String("messaging.rabbitmq.routing_key", events.OrderTimeoutCheckType),
			attribute.String("event.id", event.EventID),
			attribute.String("event.type", events.OrderTimeoutCheckType),
			attribute.Int64("order.id", event.OrderID),
		),
	)

	body, err := json.Marshal(event)
	if err != nil {
		observability.EndSpan(span, err)
		return fmt.Errorf("marshal timeout event: %w", err)
	}

	ttlMillis := delay.Milliseconds()
	if ttlMillis <= 0 {
		ttlMillis = 1
	}

	headers := observability.InjectIntoAMQP(ctx, amqp.Table{})
	requestID := observability.RequestIDFromContext(ctx)
	headers[observability.RequestIDMetadataKey] = requestID
	headers["request_id"] = requestID
	headers["correlation_id"] = requestID
	headers[observability.TraceIDMetadataKey] = observability.TraceIDFromContext(ctx)
	headers["event_id"] = event.EventID
	headers["event_type"] = event.EventType
	err = s.channel.Publish(s.exchange, events.OrderTimeoutCheckType, false, false, amqp.Publishing{
		ContentType:   "application/json",
		DeliveryMode:  amqp.Persistent,
		MessageId:     event.EventID,
		CorrelationId: requestID,
		Timestamp:     s.now().UTC(),
		Expiration:    strconv.FormatInt(ttlMillis, 10),
		Headers:       headers,
		Body:          body,
	})
	observability.EndSpan(span, err)
	return err
}

// ParseOrderPaymentTimeoutMinutes 支持整数和小数分钟，便于本地使用 0.5 分钟快速演示。
func ParseOrderPaymentTimeoutMinutes(raw string) (time.Duration, error) {
	if raw == "" {
		return DefaultOrderPaymentTimeout, nil
	}

	minutes, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("parse order payment timeout minutes: %w", err)
	}
	if minutes <= 0 {
		return 0, errors.New("order payment timeout minutes must be greater than zero")
	}
	return time.Duration(minutes * float64(time.Minute)), nil
}
