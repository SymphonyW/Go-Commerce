package order

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/streadway/amqp"

	"go-commerce/internal/outbox"
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
	if got := len(publisher.events); got != 0 {
		t.Fatalf("unexpected direct publish count: got %d want 0", got)
	}
	var saved outbox.Event
	if err := db.Where("event_type = ?", events.OrderPaidType).First(&saved).Error; err != nil {
		t.Fatalf("failed to load outbox event: %v", err)
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

	_, _, err := MarkOrderPaid(db, int64(order.ID), 1, 99, nil)
	if err == nil {
		t.Fatal("expected cancelled order to reject payment")
	}
}
