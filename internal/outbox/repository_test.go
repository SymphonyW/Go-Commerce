package outbox

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go-commerce/pkg/events"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestRepository(t *testing.T) (*GormRepository, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&Event{}); err != nil {
		t.Fatalf("failed to migrate outbox table: %v", err)
	}
	return NewRepository(db), db
}

func TestCreateStoresPendingEventInsideTransaction(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	event := events.OrderCreatedEvent{
		BaseEvent: events.BaseEvent{
			EventID:    "evt-create-1",
			EventType:  events.OrderCreatedType,
			OccurredAt: now.Format(time.RFC3339Nano),
		},
		OrderID: 101,
		UserID:  202,
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		_, createErr := repo.Create(context.Background(), tx, NewEventInput{
			AggregateType: "order",
			AggregateID:   "101",
			EventType:     events.OrderCreatedType,
			Payload:       event,
			CreatedAt:     now,
		})
		return createErr
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	var saved Event
	if err := db.First(&saved).Error; err != nil {
		t.Fatalf("failed to load saved outbox event: %v", err)
	}
	if got, want := saved.EventID, "evt-create-1"; got != want {
		t.Fatalf("unexpected event id: got %q want %q", got, want)
	}
	if got, want := saved.Status, StatusPending; got != want {
		t.Fatalf("unexpected status: got %q want %q", got, want)
	}
	if !saved.NextRetryAt.Equal(now) {
		t.Fatalf("unexpected next retry time: got %v want %v", saved.NextRetryAt, now)
	}
}

func TestCreateRollsBackWithBusinessTransaction(t *testing.T) {
	repo, db := newTestRepository(t)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, createErr := repo.Create(context.Background(), tx, NewEventInput{
			AggregateType: "order",
			AggregateID:   "102",
			EventType:     events.OrderCreatedType,
			Payload: events.OrderCreatedEvent{
				BaseEvent: events.BaseEvent{
					EventID:    "evt-rollback-1",
					EventType:  events.OrderCreatedType,
					OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
				},
				OrderID: 102,
				UserID:  203,
			},
		})
		if createErr != nil {
			return createErr
		}
		return fmt.Errorf("force rollback")
	})
	if err == nil {
		t.Fatal("expected transaction to fail")
	}

	var count int64
	if err := db.Model(&Event{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count outbox events: %v", err)
	}
	if got, want := count, int64(0); got != want {
		t.Fatalf("unexpected outbox count after rollback: got %d want %d", got, want)
	}
}

func TestMarkPublishedDoesNotOverwriteTerminalState(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	saved, err := repo.Create(context.Background(), db, NewEventInput{
		AggregateType: "order",
		AggregateID:   "103",
		EventType:     events.OrderCreatedType,
		Payload: events.OrderCreatedEvent{
			BaseEvent: events.BaseEvent{
				EventID:    "evt-terminal-1",
				EventType:  events.OrderCreatedType,
				OccurredAt: now.Format(time.RFC3339Nano),
			},
			OrderID: 103,
			UserID:  204,
		},
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if err := repo.MarkPublished(context.Background(), saved.ID, now.Add(time.Second)); err != nil {
		t.Fatalf("MarkPublished returned error: %v", err)
	}
	if err := repo.MarkRetry(context.Background(), saved.ID, RetryUpdate{
		RetryCount:   1,
		NextRetryAt:  now.Add(5 * time.Second),
		LastError:    "late failure",
		MarkAsFailed: false,
	}); err != nil {
		t.Fatalf("MarkRetry returned error: %v", err)
	}

	var latest Event
	if err := db.First(&latest, saved.ID).Error; err != nil {
		t.Fatalf("failed to reload event: %v", err)
	}
	if got, want := latest.Status, StatusPublished; got != want {
		t.Fatalf("unexpected terminal status: got %q want %q", got, want)
	}
	if got, want := latest.RetryCount, 0; got != want {
		t.Fatalf("unexpected retry count after terminal update: got %d want %d", got, want)
	}
}
