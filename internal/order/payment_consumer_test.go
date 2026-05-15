package order

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/streadway/amqp"

	"go-commerce/pkg/events"
)

type fakeAcknowledger struct {
	acked bool
}

func (f *fakeAcknowledger) Ack(uint64, bool) error {
	f.acked = true
	return nil
}

func (f *fakeAcknowledger) Nack(uint64, bool, bool) error {
	return nil
}

func (f *fakeAcknowledger) Reject(uint64, bool) error {
	return nil
}

func TestPaymentSucceededConsumerMarksOrderPaid(t *testing.T) {
	_, db := newTestService(t)
	order := Order{
		UserID:      1,
		TotalAmount: 99,
		Status:      OrderStatusPending,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	body, err := json.Marshal(events.PaymentSucceededEvent{
		BaseEvent: events.NewBaseEvent(events.PaymentSucceededType, time.Now()),
		OrderID:   int64(order.ID),
		UserID:    1,
		Amount:    99,
	})
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	ack := &fakeAcknowledger{}
	publisher := &recordingPublisher{}
	consumer := NewPaymentSucceededConsumer(db, publisher, nil)
	if err := consumer.HandleDelivery(amqp.Delivery{
		Acknowledger: ack,
		DeliveryTag:  1,
		Body:         body,
	}); err != nil {
		t.Fatalf("HandleDelivery returned error: %v", err)
	}

	var latest Order
	if err := db.First(&latest, order.ID).Error; err != nil {
		t.Fatalf("failed to reload order: %v", err)
	}
	if got, want := latest.Status, OrderStatusPaid; got != want {
		t.Fatalf("unexpected order status: got %q want %q", got, want)
	}
	if !ack.acked {
		t.Fatal("expected event to be acked")
	}
	if len(publisher.events) != 1 {
		t.Fatalf("unexpected published event count: got %d want 1", len(publisher.events))
	}
	if got, want := publisher.events[0].routingKey, events.OrderPaidType; got != want {
		t.Fatalf("unexpected routing key: got %q want %q", got, want)
	}
}

func TestMarkOrderPaidRejectsCancelledOrder(t *testing.T) {
	_, db := newTestService(t)
	order := Order{
		UserID:      1,
		TotalAmount: 99,
		Status:      OrderStatusCancelled,
		OrderDate:   time.Now(),
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	_, _, err := MarkOrderPaid(db, int64(order.ID), 1, 99)
	if err == nil {
		t.Fatal("expected cancelled order to reject payment")
	}
}
