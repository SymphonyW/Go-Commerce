package order

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/streadway/amqp"
	"gorm.io/gorm"

	"go-commerce/internal/inbox"
	"go-commerce/internal/outbox"
	"go-commerce/pkg/events"
)

type fakeAcknowledger struct {
	acked   bool
	nacked  bool
	requeue bool
}

func (f *fakeAcknowledger) Ack(uint64, bool) error {
	f.acked = true
	return nil
}

func (f *fakeAcknowledger) Nack(_ uint64, _ bool, requeue bool) error {
	f.nacked = true
	f.requeue = requeue
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
		BaseEvent: events.BaseEvent{
			EventID:    "evt-payment-paid-1",
			EventType:  events.PaymentSucceededType,
			OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
		OrderID: int64(order.ID),
		UserID:  1,
		Amount:  99,
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
	assertConsumedPaymentEventCount(t, db, "evt-payment-paid-1", 1)
}

func TestPaymentSucceededConsumerSkipsDuplicateEventAndAcks(t *testing.T) {
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
		BaseEvent: events.BaseEvent{
			EventID:    "evt-payment-duplicate-1",
			EventType:  events.PaymentSucceededType,
			OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
		OrderID: int64(order.ID),
		UserID:  1,
		Amount:  99,
	})
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	consumer := NewPaymentSucceededConsumer(db, nil, nil)
	firstAck := &fakeAcknowledger{}
	if err := consumer.HandleDelivery(amqp.Delivery{Acknowledger: firstAck, DeliveryTag: 1, Body: body}); err != nil {
		t.Fatalf("first HandleDelivery returned error: %v", err)
	}
	secondAck := &fakeAcknowledger{}
	if err := consumer.HandleDelivery(amqp.Delivery{Acknowledger: secondAck, DeliveryTag: 2, Body: body}); err != nil {
		t.Fatalf("second HandleDelivery returned error: %v", err)
	}
	if !secondAck.acked {
		t.Fatal("expected duplicate payment event to be acked")
	}
	if secondAck.nacked {
		t.Fatal("did not expect duplicate payment event to be nacked")
	}

	var eventCount int64
	if err := db.Model(&outbox.Event{}).Where("event_type = ?", events.OrderPaidType).Count(&eventCount).Error; err != nil {
		t.Fatalf("failed to count order paid outbox events: %v", err)
	}
	if got, want := eventCount, int64(1); got != want {
		t.Fatalf("unexpected order paid outbox count: got %d want %d", got, want)
	}
	assertConsumedPaymentEventCount(t, db, "evt-payment-duplicate-1", 1)
}

func TestPaymentSucceededConsumerNacksMalformedMessageWithoutRequeue(t *testing.T) {
	consumer := NewPaymentSucceededConsumer(nil, nil, nil)
	ack := &fakeAcknowledger{}

	err := consumer.HandleDelivery(amqp.Delivery{Acknowledger: ack, DeliveryTag: 1, Body: []byte("{bad json")})
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

func TestPaymentSucceededConsumerNacksMissingEventIDWithoutRequeue(t *testing.T) {
	consumer := NewPaymentSucceededConsumer(nil, nil, nil)
	ack := &fakeAcknowledger{}
	body, err := json.Marshal(events.PaymentSucceededEvent{
		BaseEvent: events.BaseEvent{EventType: events.PaymentSucceededType},
		OrderID:   1,
		UserID:    1,
		Amount:    99,
	})
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	err = consumer.HandleDelivery(amqp.Delivery{Acknowledger: ack, DeliveryTag: 1, Body: body})
	if err == nil {
		t.Fatal("expected missing event id error")
	}
	if !ack.nacked {
		t.Fatal("expected missing event id to be nacked")
	}
	if ack.requeue {
		t.Fatal("expected missing event id not to be requeued")
	}
}

func TestPaymentSucceededConsumerNacksTemporaryDatabaseErrorWithRequeue(t *testing.T) {
	consumer := NewPaymentSucceededConsumer(nil, nil, nil)
	ack := &fakeAcknowledger{}
	body, err := json.Marshal(events.PaymentSucceededEvent{
		BaseEvent: events.BaseEvent{
			EventID:    "evt-payment-db-error-1",
			EventType:  events.PaymentSucceededType,
			OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
		OrderID: 1,
		UserID:  1,
		Amount:  99,
	})
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	err = consumer.HandleDelivery(amqp.Delivery{Acknowledger: ack, DeliveryTag: 1, Body: body})
	if err == nil {
		t.Fatal("expected database error")
	}
	if !ack.nacked {
		t.Fatal("expected database error to be nacked")
	}
	if !ack.requeue {
		t.Fatal("expected temporary database error to be requeued")
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

func assertConsumedPaymentEventCount(t *testing.T, db *gorm.DB, eventID string, want int64) {
	t.Helper()

	var count int64
	if err := db.Model(&inbox.ConsumedEvent{}).
		Where("consumer_name = ? AND event_id = ?", paymentSucceededConsumerName, eventID).
		Count(&count).Error; err != nil {
		t.Fatalf("failed to count consumed payment events: %v", err)
	}
	if got := count; got != want {
		t.Fatalf("unexpected consumed payment event count: got %d want %d", got, want)
	}
}
