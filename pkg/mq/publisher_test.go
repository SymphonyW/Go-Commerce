package mq

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/streadway/amqp"

	"go-commerce/pkg/events"
	"go-commerce/pkg/observability"
)

type fakeChannel struct {
	declaredExchange string
	declaredKind     string
	publishedKey     string
	publishedMessage amqp.Publishing
	declareErr       error
	publishErr       error
}

func (f *fakeChannel) ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	f.declaredExchange = name
	f.declaredKind = kind
	return f.declareErr
}

func (f *fakeChannel) Publish(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	f.publishedKey = key
	f.publishedMessage = msg
	return f.publishErr
}

func TestRabbitMQPublisherPublishesJSONMessage(t *testing.T) {
	channel := &fakeChannel{}
	publisher := NewRabbitMQPublisher(channel, "ecommerce.events")
	fixedTime := time.Date(2026, time.May, 15, 10, 0, 0, 0, time.UTC)
	publisher.now = func() time.Time { return fixedTime }

	event := events.OrderCreatedEvent{
		BaseEvent: events.BaseEvent{
			EventID:    "evt-1",
			EventType:  events.OrderCreatedType,
			OccurredAt: fixedTime.Format(time.RFC3339Nano),
		},
		OrderID:     10,
		UserID:      20,
		TotalAmount: 99.9,
	}

	if err := publisher.Publish(context.Background(), events.OrderCreatedType, event); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	if got, want := channel.declaredExchange, "ecommerce.events"; got != want {
		t.Fatalf("unexpected exchange: got %q want %q", got, want)
	}
	if got, want := channel.declaredKind, "topic"; got != want {
		t.Fatalf("unexpected exchange kind: got %q want %q", got, want)
	}
	if got, want := channel.publishedKey, events.OrderCreatedType; got != want {
		t.Fatalf("unexpected routing key: got %q want %q", got, want)
	}
	if got, want := channel.publishedMessage.ContentType, "application/json"; got != want {
		t.Fatalf("unexpected content type: got %q want %q", got, want)
	}
	if got, want := channel.publishedMessage.DeliveryMode, uint8(amqp.Persistent); got != want {
		t.Fatalf("unexpected delivery mode: got %d want %d", got, want)
	}
	if got, want := channel.publishedMessage.MessageId, "evt-1"; got != want {
		t.Fatalf("unexpected message id: got %q want %q", got, want)
	}
	if got, want := channel.publishedMessage.Timestamp, fixedTime; !got.Equal(want) {
		t.Fatalf("unexpected timestamp: got %v want %v", got, want)
	}

	var decoded events.OrderCreatedEvent
	if err := json.Unmarshal(channel.publishedMessage.Body, &decoded); err != nil {
		t.Fatalf("failed to decode message body: %v", err)
	}
	if got, want := decoded.EventType, events.OrderCreatedType; got != want {
		t.Fatalf("unexpected event type: got %q want %q", got, want)
	}
	if got, want := decoded.OrderID, int64(10); got != want {
		t.Fatalf("unexpected order id: got %d want %d", got, want)
	}
}

func TestRabbitMQPublisherCopiesRequestIDToCorrelationID(t *testing.T) {
	channel := &fakeChannel{}
	publisher := NewRabbitMQPublisher(channel, "ecommerce.events")
	ctx := observability.WithRequestID(context.Background(), "req-789")

	if err := publisher.Publish(ctx, events.OrderCreatedType, events.OrderCreatedEvent{}); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if got, want := channel.publishedMessage.CorrelationId, "req-789"; got != want {
		t.Fatalf("unexpected correlation id: got %q want %q", got, want)
	}
}

func TestRabbitMQPublisherReturnsChannelErrors(t *testing.T) {
	t.Run("declare", func(t *testing.T) {
		publisher := NewRabbitMQPublisher(&fakeChannel{declareErr: errors.New("declare failed")}, "ecommerce.events")

		err := publisher.Publish(context.Background(), events.OrderCreatedType, events.OrderCreatedEvent{})
		if err == nil {
			t.Fatal("expected declare error")
		}
	})

	t.Run("publish", func(t *testing.T) {
		publisher := NewRabbitMQPublisher(&fakeChannel{publishErr: errors.New("publish failed")}, "ecommerce.events")

		err := publisher.Publish(context.Background(), events.OrderCreatedType, events.OrderCreatedEvent{})
		if err == nil {
			t.Fatal("expected publish error")
		}
	})
}

func TestRabbitMQPublisherPreservesRawOutboxPayload(t *testing.T) {
	channel := &fakeChannel{}
	publisher := NewRabbitMQPublisher(channel, "ecommerce.events")
	raw := RawEvent{
		EventID: "evt-raw-1",
		Body:    json.RawMessage(`{"event_id":"evt-raw-1","event_type":"order.created","order_id":88}`),
	}

	if err := publisher.Publish(context.Background(), events.OrderCreatedType, raw); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if got, want := channel.publishedMessage.MessageId, "evt-raw-1"; got != want {
		t.Fatalf("unexpected raw message id: got %q want %q", got, want)
	}
	if got, want := string(channel.publishedMessage.Body), string(raw.Body); got != want {
		t.Fatalf("unexpected raw body: got %s want %s", got, want)
	}
}
