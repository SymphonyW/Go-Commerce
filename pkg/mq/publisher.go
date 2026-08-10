package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/streadway/amqp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"go-commerce/pkg/observability"
)

const (
	DefaultExchangeName = "ecommerce.events"
	exchangeKind        = "topic"
)

// Publisher 抽象消息发布能力，让业务层不直接依赖 RabbitMQ SDK。
type Publisher interface {
	Publish(ctx context.Context, routingKey string, event interface{}) error
}

type PublishObserver interface {
	RecordMQPublish(eventType string, success bool)
}

// Channel 描述 RabbitMQPublisher 真正需要的 AMQP 能力，便于在单元测试中替换。
type Channel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	Publish(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

type identifiedEvent interface {
	GetEventID() string
}

// RawEvent 让 Outbox worker 复用现有 Publisher 的同时，保留已经落库的原始 JSON 负载和 event_id。
type RawEvent struct {
	EventID   string
	EventType string
	RequestID string
	TraceID   string
	Body      json.RawMessage
}

func (e RawEvent) GetEventID() string {
	return e.EventID
}

func (e RawEvent) MarshalJSON() ([]byte, error) {
	return e.Body.MarshalJSON()
}

// RabbitMQPublisher 使用 topic exchange 将领域事件写入 RabbitMQ。
type RabbitMQPublisher struct {
	channel  Channel
	exchange string
	now      func() time.Time
}

// NewRabbitMQPublisher 创建 RabbitMQ 发布器；exchange 为空时使用默认事件交换机。
func NewRabbitMQPublisher(channel Channel, exchange string) *RabbitMQPublisher {
	if exchange == "" {
		exchange = DefaultExchangeName
	}

	return &RabbitMQPublisher{
		channel:  channel,
		exchange: exchange,
		now:      time.Now,
	}
}

// Publish 将事件序列化为 JSON，并写入持久化 topic exchange。
func (p *RabbitMQPublisher) Publish(ctx context.Context, routingKey string, event interface{}) error {
	if p == nil || p.channel == nil {
		return errors.New("rabbitmq publisher channel is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	eventID := ""
	correlationRequestID := ""
	correlationTraceID := ""
	if identified, ok := event.(identifiedEvent); ok {
		eventID = identified.GetEventID()
	}
	eventType := routingKey
	if raw, ok := event.(RawEvent); ok {
		if raw.EventType != "" {
			eventType = raw.EventType
		}
		if raw.RequestID != "" {
			correlationRequestID = raw.RequestID
			ctx = observability.WithRequestID(ctx, raw.RequestID)
		}
		if raw.TraceID != "" {
			correlationTraceID = raw.TraceID
			ctx = observability.WithTraceID(ctx, raw.TraceID)
		}
	}

	ctx, span := observability.StartSpan(ctx,
		"rabbitmq publish "+routingKey,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination.name", p.exchange),
			attribute.String("messaging.rabbitmq.routing_key", routingKey),
			attribute.String("event.type", eventType),
			attribute.String("event.id", eventID),
			attribute.String("correlation.request_id", correlationRequestID),
			attribute.String("correlation.trace_id", correlationTraceID),
		),
	)

	body, err := json.Marshal(event)
	if err != nil {
		observability.EndSpan(span, err)
		return fmt.Errorf("marshal event: %w", err)
	}

	if err := p.channel.ExchangeDeclare(
		p.exchange,
		exchangeKind,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		observability.EndSpan(span, err)
		return fmt.Errorf("declare exchange %s: %w", p.exchange, err)
	}

	headers := observability.InjectIntoAMQP(ctx, amqp.Table{})
	requestID := observability.RequestIDFromContext(ctx)
	if correlationRequestID != "" {
		requestID = correlationRequestID
	}
	if requestID != "" {
		headers[observability.RequestIDMetadataKey] = requestID
		headers["request_id"] = requestID
		headers["correlation_id"] = requestID
	}
	traceID := observability.TraceIDFromContext(ctx)
	if correlationTraceID != "" {
		traceID = correlationTraceID
	}
	if traceID != "" {
		headers[observability.TraceIDMetadataKey] = traceID
		headers["trace_id"] = traceID
	}
	if eventID != "" {
		headers["event_id"] = eventID
	}
	headers["event_type"] = eventType

	message := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    p.now().UTC(),
		Headers:      headers,
		Body:         body,
	}
	if requestID != "" {
		message.CorrelationId = requestID
	}
	if eventID != "" {
		message.MessageId = eventID
	}

	if err := p.channel.Publish(p.exchange, routingKey, false, false, message); err != nil {
		observability.EndSpan(span, err)
		return fmt.Errorf("publish event %s: %w", routingKey, err)
	}

	observability.EndSpan(span, nil)
	return nil
}

// NopPublisher 用于消息组件暂不可用时保持主交易链路可运行，代价是当前事件会丢失。
type NopPublisher struct{}

func (NopPublisher) Publish(ctx context.Context, routingKey string, event interface{}) error {
	return nil
}

// InstrumentedPublisher 在不改变业务层发布接口的前提下，补充 MQ 成功/失败指标。
type InstrumentedPublisher struct {
	inner    Publisher
	observer PublishObserver
}

func NewInstrumentedPublisher(inner Publisher, observer PublishObserver) Publisher {
	if inner == nil {
		inner = NopPublisher{}
	}
	return &InstrumentedPublisher{inner: inner, observer: observer}
}

func (p *InstrumentedPublisher) Publish(ctx context.Context, routingKey string, event interface{}) error {
	err := p.inner.Publish(ctx, routingKey, event)
	if p.observer != nil {
		p.observer.RecordMQPublish(routingKey, err == nil)
	}
	return err
}
