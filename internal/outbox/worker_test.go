package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go-commerce/pkg/events"

	"gorm.io/gorm"
)

type recordingPublisher struct {
	mu          sync.Mutex
	routingKeys []string
	payloads    []interface{}
	err         error
}

func (p *recordingPublisher) Publish(ctx context.Context, routingKey string, event interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.routingKeys = append(p.routingKeys, routingKey)
	p.payloads = append(p.payloads, event)
	return p.err
}

func TestWorkerKeepsEventPendingWhenPublishFails(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	saved := createPendingOrderEvent(t, repo, db, "evt-retry-1", now)

	publisher := &recordingPublisher{err: errors.New("rabbitmq unavailable")}
	worker := NewWorker(repo, publisher, testWorkerConfig("worker-retry"), nil)
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
	if latest.LockedBy != "" || latest.LockedAt != nil || latest.LeaseExpiresAt != nil {
		t.Fatalf("expected lock fields to be clear after retry, got locked_by=%q locked_at=%v lease_expires_at=%v", latest.LockedBy, latest.LockedAt, latest.LeaseExpiresAt)
	}
}

func TestWorkerPublishesRecoveredEventAndMarksPublished(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	saved := createPendingOrderEvent(t, repo, db, "evt-recover-1", now)

	failingPublisher := &recordingPublisher{err: errors.New("rabbitmq unavailable")}
	worker := NewWorker(repo, failingPublisher, testWorkerConfig("worker-recover"), nil)
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
	if latest.LockedBy != "" || latest.LockedAt != nil || latest.LeaseExpiresAt != nil {
		t.Fatalf("expected lock fields to be clear after publish, got locked_by=%q locked_at=%v lease_expires_at=%v", latest.LockedBy, latest.LockedAt, latest.LeaseExpiresAt)
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
		LeaseDuration:  time.Minute,
		WorkerID:       "worker-failed",
	}, nil)
	worker.now = func() time.Time { return now }

	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce returned error: %v", err)
	}

	latest := loadEvent(t, db, saved.ID)
	if got, want := latest.Status, StatusFailed; got != want {
		t.Fatalf("unexpected terminal status: got %q want %q", got, want)
	}
	if latest.LockedBy != "" || latest.LockedAt != nil || latest.LeaseExpiresAt != nil {
		t.Fatalf("expected lock fields to be clear after failure, got locked_by=%q locked_at=%v lease_expires_at=%v", latest.LockedBy, latest.LockedAt, latest.LeaseExpiresAt)
	}
}

func TestWorkerCheckPollingReportsRunState(t *testing.T) {
	repo, _ := newTestRepository(t)
	worker := NewWorker(repo, nil, testWorkerConfig("worker-health"), nil)

	if err := worker.CheckPolling(context.Background()); err == nil {
		t.Fatal("expected CheckPolling to fail before Run marks worker polling")
	}

	worker.polling.Store(true)
	if err := worker.CheckPolling(context.Background()); err != nil {
		t.Fatalf("expected CheckPolling to pass while polling, got %v", err)
	}
}

func TestWorkerMarksPublishedOnlyOnce(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	saved := createPendingOrderEvent(t, repo, db, "evt-published-once-1", now)

	publisher := &recordingPublisher{}
	first := NewWorker(repo, publisher, testWorkerConfig("worker-a"), nil)
	first.now = func() time.Time { return now }
	if err := first.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("first ProcessOnce returned error: %v", err)
	}

	second := NewWorker(repo, publisher, testWorkerConfig("worker-b"), nil)
	second.now = func() time.Time { return now.Add(time.Second) }
	if err := second.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("second ProcessOnce returned error: %v", err)
	}

	latest := loadEvent(t, db, saved.ID)
	if got, want := latest.Status, StatusPublished; got != want {
		t.Fatalf("unexpected status: got %q want %q", got, want)
	}
	if got, want := len(publisher.routingKeys), 1; got != want {
		t.Fatalf("unexpected publish count: got %d want %d", got, want)
	}
}

func TestMultipleWorkersProcessHundredEventsWithoutOmissions(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 100; i++ {
		createPendingOrderEvent(t, repo, db, "evt-bulk-"+time.Duration(i).String(), now)
	}

	publisher := &recordingPublisher{}
	for i := 0; i < 4; i++ {
		worker := NewWorker(repo, publisher, Config{
			BatchSize:      25,
			MaxRetry:       5,
			RetryBaseDelay: time.Second,
			LeaseDuration:  time.Minute,
			WorkerID:       "worker-bulk-" + time.Duration(i).String(),
		}, nil)
		worker.now = func() time.Time { return now }
		if err := worker.ProcessOnce(context.Background()); err != nil {
			t.Fatalf("ProcessOnce for worker %d returned error: %v", i, err)
		}
	}

	var publishedCount int64
	if err := db.Model(&Event{}).Where("status = ?", StatusPublished).Count(&publishedCount).Error; err != nil {
		t.Fatalf("failed to count published events: %v", err)
	}
	if got, want := publishedCount, int64(100); got != want {
		t.Fatalf("unexpected published count: got %d want %d", got, want)
	}
	if got, want := len(publisher.routingKeys), 100; got != want {
		t.Fatalf("unexpected publish count: got %d want %d", got, want)
	}
}

func testWorkerConfig(workerID string) Config {
	return Config{
		BatchSize:      10,
		MaxRetry:       5,
		RetryBaseDelay: time.Second,
		LeaseDuration:  time.Minute,
		WorkerID:       workerID,
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
