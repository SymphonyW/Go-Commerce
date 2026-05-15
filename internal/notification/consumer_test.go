package notification

import (
	"bytes"
	"encoding/json"
	"log"
	"testing"
	"time"

	"github.com/streadway/amqp"

	"go-commerce/pkg/events"
)

type fakeAcknowledger struct {
	acked   bool
	nacked  bool
	requeue bool
}

func (f *fakeAcknowledger) Ack(tag uint64, multiple bool) error {
	f.acked = true
	return nil
}

func (f *fakeAcknowledger) Nack(tag uint64, multiple, requeue bool) error {
	f.nacked = true
	f.requeue = requeue
	return nil
}

func (f *fakeAcknowledger) Reject(tag uint64, requeue bool) error {
	return nil
}

func TestConsumerAcknowledgesValidOrderCreatedEvent(t *testing.T) {
	var logs bytes.Buffer
	consumer := NewConsumer(log.New(&logs, "", 0))
	ack := &fakeAcknowledger{}
	body, err := json.Marshal(events.OrderCreatedEvent{
		BaseEvent: events.BaseEvent{
			EventID:    "evt-1",
			EventType:  events.OrderCreatedType,
			OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
		OrderID: 88,
		UserID:  99,
	})
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	err = consumer.HandleDelivery(amqp.Delivery{
		Acknowledger: ack,
		DeliveryTag:  1,
		Body:         body,
	})
	if err != nil {
		t.Fatalf("HandleDelivery returned error: %v", err)
	}
	if !ack.acked {
		t.Fatal("expected delivery to be acked")
	}
	if ack.nacked {
		t.Fatal("did not expect delivery to be nacked")
	}
	if got := logs.String(); got == "" {
		t.Fatal("expected notification log output")
	}
}

func TestConsumerNacksMalformedMessageWithoutRequeue(t *testing.T) {
	consumer := NewConsumer(log.New(&bytes.Buffer{}, "", 0))
	ack := &fakeAcknowledger{}

	err := consumer.HandleDelivery(amqp.Delivery{
		Acknowledger: ack,
		DeliveryTag:  1,
		Body:         []byte("{bad json"),
	})
	if err == nil {
		t.Fatal("expected malformed message error")
	}
	if !ack.nacked {
		t.Fatal("expected malformed message to be nacked")
	}
	if ack.requeue {
		t.Fatal("expected malformed message not to be requeued")
	}
}
