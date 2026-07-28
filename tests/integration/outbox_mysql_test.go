//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go-commerce/internal/outbox"
	"go-commerce/pkg/events"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestMySQLOutboxAutoMigrateCanRunRepeatedly(t *testing.T) {
	db := openIntegrationDB(t)

	if err := db.AutoMigrate(&outbox.Event{}); err != nil {
		t.Fatalf("first outbox migration failed: %v", err)
	}
	if err := db.AutoMigrate(&outbox.Event{}); err != nil {
		t.Fatalf("second outbox migration failed: %v", err)
	}
}

func TestMySQLOutboxClaimDueEventsUsesSkipLocked(t *testing.T) {
	db := openMigratedOutboxDB(t)
	repo := outbox.NewRepository(db)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	first := createIntegrationOutboxEvent(t, repo, db, "evt-skip-locked-1", now)
	second := createIntegrationOutboxEvent(t, repo, db, "evt-skip-locked-2", now)

	lockTx := db.Begin()
	if lockTx.Error != nil {
		t.Fatalf("failed to begin lock transaction: %v", lockTx.Error)
	}
	defer lockTx.Rollback()

	var locked outbox.Event
	if err := lockTx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, first.ID).Error; err != nil {
		t.Fatalf("failed to lock first event: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	claim, err := repo.ClaimDueEvents(ctx, now, 2, "worker-skip-locked", time.Minute)
	if err != nil {
		t.Fatalf("ClaimDueEvents returned error: %v", err)
	}
	if got, want := len(claim.Events), 1; got != want {
		t.Fatalf("unexpected claimed count with first row locked: got %d want %d", got, want)
	}
	if got, want := claim.Events[0].ID, second.ID; got != want {
		t.Fatalf("expected SKIP LOCKED to claim second event, got id=%d want id=%d", got, want)
	}
}

func TestMySQLOutboxConcurrentClaimsDoNotOverlap(t *testing.T) {
	db := openMigratedOutboxDB(t)
	repo := outbox.NewRepository(db)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		createIntegrationOutboxEvent(t, repo, db, fmt.Sprintf("evt-concurrent-claim-%02d", i), now)
	}

	type outcome struct {
		workerID string
		events   []outbox.Event
		err      error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	for _, workerID := range []string{"worker-a", "worker-b"} {
		go func(workerID string) {
			<-start
			claim, err := repo.ClaimDueEvents(context.Background(), now, 10, workerID, time.Minute)
			results <- outcome{workerID: workerID, events: claim.Events, err: err}
		}(workerID)
	}
	close(start)

	first := <-results
	second := <-results
	if first.err != nil {
		t.Fatalf("%s ClaimDueEvents returned error: %v", first.workerID, first.err)
	}
	if second.err != nil {
		t.Fatalf("%s ClaimDueEvents returned error: %v", second.workerID, second.err)
	}
	if got, want := len(first.events)+len(second.events), 20; got != want {
		t.Fatalf("unexpected total claimed count: got %d want %d", got, want)
	}

	seen := map[uint]string{}
	for _, event := range first.events {
		seen[event.ID] = first.workerID
	}
	for _, event := range second.events {
		if owner, exists := seen[event.ID]; exists {
			t.Fatalf("event %d claimed by both %s and %s", event.ID, owner, second.workerID)
		}
		seen[event.ID] = second.workerID
	}
}

func openMigratedOutboxDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := openIntegrationDB(t)
	if err := db.AutoMigrate(&outbox.Event{}); err != nil {
		t.Fatalf("outbox migration failed: %v", err)
	}
	return db
}

func createIntegrationOutboxEvent(t *testing.T, repo *outbox.GormRepository, db *gorm.DB, eventID string, now time.Time) *outbox.Event {
	t.Helper()

	saved, err := repo.Create(context.Background(), db, outbox.NewEventInput{
		AggregateType: "order",
		AggregateID:   "900",
		EventType:     events.OrderCreatedType,
		Payload: events.OrderCreatedEvent{
			BaseEvent: events.BaseEvent{
				EventID:    eventID,
				EventType:  events.OrderCreatedType,
				OccurredAt: now.Format(time.RFC3339Nano),
			},
			OrderID: 900,
			UserID:  901,
		},
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("failed to create integration outbox event: %v", err)
	}
	return saved
}
