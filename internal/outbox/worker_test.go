package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"go-commerce/pkg/events"

	"gorm.io/gorm"
)

type recordingPublisher struct {
	routingKeys []string
	payloads    []interface{}
	err         error
}

func (p *recordingPublisher) Publish(ctx context.Context, routingKey string, event interface{}) error {
	p.routingKeys = append(p.routingKeys, routingKey)
	p.payloads = append(p.payloads, event)
	return p.err
}

func TestWorkerKeepsEventPendingWhenPublishFails(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	saved := createPendingOrderEvent(t, repo, db, "evt-retry-1", now)

	publisher := &recordingPublisher{err: errors.New("rabbitmq unavailable")}
	worker := NewWorker(repo, publisher, Config{
		BatchSize:      10,
		MaxRetry:       5,
		RetryBaseDelay: time.Second,
	}, nil)
	worker.now = func() time.Time { return now }

	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce returned error: %v", err)
	}

	latest := loadEvent(t, db, saved.ID)
	if got, want := latest.Status, StatusPending; got != want {
		t.Fatalf("unexpected status after failed publish: got %q want %q", got, want)
	}
	if got, want := latest.RetryCount, 1; got != want {
		t.Fatalf("unexpected retry count: got %d want %d", got, want)
	}
	if got, want := latest.NextRetryAt, now.Add(time.Second); !got.Equal(want) {
		t.Fatalf("unexpected next retry time: got %v want %v", got, want)
	}
}

func TestWorkerPublishesRecoveredEventAndMarksPublished(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	saved := createPendingOrderEvent(t, repo, db, "evt-recover-1", now)

	failingPublisher := &recordingPublisher{err: errors.New("rabbitmq unavailable")}
	worker := NewWorker(repo, failingPublisher, Config{
		BatchSize:      10,
		MaxRetry:       5,
		RetryBaseDelay: time.Second,
	}, nil)
	worker.now = func() time.Time { return now }
	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("first ProcessOnce returned error: %v", err)
	}

	healthyPublisher := &recordingPublisher{}
	worker.publisher = healthyPublisher
	worker.now = func() time.Time { return now.Add(time.Second) }
	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("second ProcessOnce returned error: %v", err)
	}

	latest := loadEvent(t, db, saved.ID)
	if got, want := latest.Status, StatusPublished; got != want {
		t.Fatalf("unexpected status after recovery: got %q want %q", got, want)
	}
	if latest.PublishedAt == nil {
		t.Fatal("expected published_at to be set")
	}
	if got, want := healthyPublisher.routingKeys, []string{events.OrderCreatedType}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("unexpected routing keys after recovery: got %v want %v", got, want)
	}
}

func TestWorkerMarksEventFailedAfterMaxRetry(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	saved := createPendingOrderEvent(t, repo, db, "evt-failed-1", now)

	publisher := &recordingPublisher{err: errors.New("rabbitmq unavailable")}
	worker := NewWorker(repo, publisher, Config{
		BatchSize:      10,
		MaxRetry:       1,
		RetryBaseDelay: time.Second,
	}, nil)
	worker.now = func() time.Time { return now }

	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce returned error: %v", err)
	}

	latest := loadEvent(t, db, saved.ID)
	if got, want := latest.Status, StatusFailed; got != want {
		t.Fatalf("unexpected terminal status: got %q want %q", got, want)
	}
}

func createPendingOrderEvent(t *testing.T, repo *GormRepository, db *gorm.DB, eventID string, now time.Time) *Event {
	t.Helper()

	saved, err := repo.Create(context.Background(), db, NewEventInput{
		AggregateType: "order",
		AggregateID:   "200",
		EventType:     events.OrderCreatedType,
		Payload: events.OrderCreatedEvent{
			BaseEvent: events.BaseEvent{
				EventID:    eventID,
				EventType:  events.OrderCreatedType,
				OccurredAt: now.Format(time.RFC3339Nano),
			},
			OrderID: 200,
			UserID:  300,
		},
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("failed to create outbox event: %v", err)
	}
	return saved
}

func loadEvent(t *testing.T, db *gorm.DB, id uint) Event {
	t.Helper()

	var event Event
	if err := db.First(&event, id).Error; err != nil {
		t.Fatalf("failed to reload outbox event: %v", err)
	}
	return event
}
