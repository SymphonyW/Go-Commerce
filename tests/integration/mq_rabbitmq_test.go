//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go-commerce/pkg/events"
	"go-commerce/pkg/mq"
)

func TestRabbitMQPublisherDeliversDomainEvent(t *testing.T) {
	conn := openIntegrationRabbitMQ(t)
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("failed to open rabbitmq channel: %v", err)
	}
	defer ch.Close()

	exchange := "integration.events." + uniqueSuffix(t)
	queue, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ch.QueueDelete(queue.Name, false, false, false)
		_ = ch.ExchangeDelete(exchange, false, false)
	})

	publisher := mq.NewRabbitMQPublisher(ch, exchange)
	event := events.OrderCreatedEvent{
		BaseEvent: events.NewBaseEvent(events.OrderCreatedType, time.Now()),
		OrderID:   9001,
		UserID:    77,
	}
	if err := publisher.Publish(context.Background(), events.OrderCreatedType, event); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	if err := ch.QueueBind(queue.Name, events.OrderCreatedType, exchange, false, nil); err != nil {
		t.Fatalf("failed to bind queue: %v", err)
	}
	if err := publisher.Publish(context.Background(), events.OrderCreatedType, event); err != nil {
		t.Fatalf("Publish returned error after bind: %v", err)
	}

	deadline := time.After(5 * time.Second)
	for {
		msg, ok, err := ch.Get(queue.Name, true)
		if err != nil {
			t.Fatalf("failed to get message: %v", err)
		}
		if ok {
			var decoded events.OrderCreatedEvent
			if err := json.Unmarshal(msg.Body, &decoded); err != nil {
				t.Fatalf("failed to decode message body: %v", err)
			}
			if got, want := decoded.OrderID, int64(9001); got != want {
				t.Fatalf("unexpected order id: got %d want %d", got, want)
			}
			return
		}

		select {
		case <-deadline:
			t.Fatal("timed out waiting for rabbitmq event")
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}
