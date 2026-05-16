package idempotency

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/wrapperspb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := db.AutoMigrate(&Record{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}
	return NewService(db, 24*time.Hour), db
}

func TestBeginCreatesProcessingRecord(t *testing.T) {
	service, db := newTestService(t)

	result, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-a",
	})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if got, want := result.Action, ActionProceed; got != want {
		t.Fatalf("unexpected action: got %q want %q", got, want)
	}

	var record Record
	if err := db.First(&record, result.Record.ID).Error; err != nil {
		t.Fatalf("failed to reload record: %v", err)
	}
	if got, want := record.State, StateProcessing; got != want {
		t.Fatalf("unexpected state: got %q want %q", got, want)
	}
}

func TestBeginReplaysCompletedRecord(t *testing.T) {
	service, _ := newTestService(t)
	first, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-a",
	})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if err := service.Complete(context.Background(), first.Record.ID, 200, wrapperspb.String("ok")); err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	replay, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-a",
	})
	if err != nil {
		t.Fatalf("second Begin returned error: %v", err)
	}
	if got, want := replay.Action, ActionReplay; got != want {
		t.Fatalf("unexpected action: got %q want %q", got, want)
	}

	var restored wrapperspb.StringValue
	if err := ReplayInto(replay.Record, &restored); err != nil {
		t.Fatalf("ReplayInto returned error: %v", err)
	}
	if got, want := restored.Value, "ok"; got != want {
		t.Fatalf("unexpected restored value: got %q want %q", got, want)
	}
}

func TestBeginRejectsHashMismatch(t *testing.T) {
	service, _ := newTestService(t)
	if _, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-a",
	}); err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	_, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-b",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBeginRejectsProcessingDuplicate(t *testing.T) {
	service, _ := newTestService(t)
	if _, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-a",
	}); err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	_, err := service.Begin(context.Background(), BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-a",
	})
	if !errors.Is(err, ErrInProgress) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHashPayloadIsStable(t *testing.T) {
	first, err := HashPayload(map[string]any{
		"user_id": 1,
		"items": []map[string]any{
			{"product_id": 1, "quantity": 2},
		},
	})
	if err != nil {
		t.Fatalf("HashPayload returned error: %v", err)
	}
	second, err := HashPayload(map[string]any{
		"items": []map[string]any{
			{"quantity": 2, "product_id": 1},
		},
		"user_id": 1,
	})
	if err != nil {
		t.Fatalf("HashPayload returned error: %v", err)
	}
	if first != second {
		t.Fatalf("expected stable hash, got %q and %q", first, second)
	}
}
