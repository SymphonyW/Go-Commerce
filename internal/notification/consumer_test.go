package notification

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/streadway/amqp"
	"gorm.io/gorm"

	"go-commerce/internal/inbox"
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
	db := newNotificationTestDB(t)
	consumer := NewConsumer(db, log.New(&logs, "", 0))
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
	assertNotificationConsumedEventCount(t, db, "evt-1", 1)
}

func TestConsumerSkipsDuplicateOrderCreatedEventAndAcks(t *testing.T) {
	var logs bytes.Buffer
	db := newNotificationTestDB(t)
	consumer := NewConsumer(db, log.New(&logs, "", 0))
	body, err := json.Marshal(events.OrderCreatedEvent{
		BaseEvent: events.BaseEvent{
			EventID:    "evt-duplicate-1",
			EventType:  events.OrderCreatedType,
			OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
		OrderID: 88,
		UserID:  99,
	})
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	if err := consumer.HandleDelivery(amqp.Delivery{Acknowledger: &fakeAcknowledger{}, DeliveryTag: 1, Body: body}); err != nil {
		t.Fatalf("first HandleDelivery returned error: %v", err)
	}
	secondAck := &fakeAcknowledger{}
	if err := consumer.HandleDelivery(amqp.Delivery{Acknowledger: secondAck, DeliveryTag: 2, Body: body}); err != nil {
		t.Fatalf("second HandleDelivery returned error: %v", err)
	}
	if !secondAck.acked {
		t.Fatal("expected duplicate notification event to be acked")
	}
	if secondAck.nacked {
		t.Fatal("did not expect duplicate notification event to be nacked")
	}
	if got, want := strings.Count(logs.String(), "notification_event_received"), 1; got != want {
		t.Fatalf("unexpected notification received log count: got %d want %d; logs=%s", got, want, logs.String())
	}
	assertNotificationConsumedEventCount(t, db, "evt-duplicate-1", 1)
}

func TestConsumerNacksMalformedMessageWithoutRequeue(t *testing.T) {
	consumer := NewConsumer(nil, log.New(&bytes.Buffer{}, "", 0))
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

func TestConsumerNacksMissingEventIDWithoutRequeue(t *testing.T) {
	consumer := NewConsumer(nil, log.New(&bytes.Buffer{}, "", 0))
	ack := &fakeAcknowledger{}
	body, err := json.Marshal(events.OrderCreatedEvent{
		BaseEvent: events.BaseEvent{EventType: events.OrderCreatedType},
		OrderID:   88,
		UserID:    99,
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

func TestConsumerNacksTemporaryDatabaseErrorWithRequeue(t *testing.T) {
	consumer := NewConsumer(nil, log.New(&bytes.Buffer{}, "", 0))
	ack := &fakeAcknowledger{}
	body, err := json.Marshal(events.OrderCreatedEvent{
		BaseEvent: events.BaseEvent{
			EventID:    "evt-db-error-1",
			EventType:  events.OrderCreatedType,
			OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
		OrderID: 88,
		UserID:  99,
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

func newNotificationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := "file:" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open notification test database: %v", err)
	}
	if err := db.AutoMigrate(&inbox.ConsumedEvent{}); err != nil {
		t.Fatalf("failed to migrate notification test database: %v", err)
	}
	return db
}

func assertNotificationConsumedEventCount(t *testing.T, db *gorm.DB, eventID string, want int64) {
	t.Helper()

	var count int64
	if err := db.Model(&inbox.ConsumedEvent{}).
		Where("consumer_name = ? AND event_id = ?", orderCreatedConsumerName, eventID).
		Count(&count).Error; err != nil {
		t.Fatalf("failed to count consumed notification events: %v", err)
	}
	if got := count; got != want {
		t.Fatalf("unexpected consumed notification event count: got %d want %d", got, want)
	}
}
