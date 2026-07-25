package idempotency

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"google.golang.org/protobuf/types/known/wrapperspb"
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

func newConcurrentTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "idempotency.db") + "?_busy_timeout=5000&_journal_mode=WAL"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open concurrent test database: %v", err)
	}
	if err := db.AutoMigrate(&Record{}); err != nil {
		t.Fatalf("failed to migrate concurrent test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to open sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	return NewService(db, time.Hour), db
}

func beginRequest(hash string) BeginRequest {
	return BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    hash,
	}
}

func TestBeginCreatesProcessingRecord(t *testing.T) {
	service, db := newTestService(t)

	result, err := service.Begin(context.Background(), beginRequest("hash-a"))
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
	first, err := service.Begin(context.Background(), beginRequest("hash-a"))
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if err := service.Complete(context.Background(), first.Record.ID, 200, wrapperspb.String("ok")); err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	replay, err := service.Begin(context.Background(), beginRequest("hash-a"))
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
	if _, err := service.Begin(context.Background(), beginRequest("hash-a")); err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	_, err := service.Begin(context.Background(), beginRequest("hash-b"))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBeginRejectsProcessingDuplicate(t *testing.T) {
	service, _ := newTestService(t)
	if _, err := service.Begin(context.Background(), beginRequest("hash-a")); err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	_, err := service.Begin(context.Background(), beginRequest("hash-a"))
	if !errors.Is(err, ErrInProgress) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBeginTakesOverExpiredProcessingRecord(t *testing.T) {
	service, db := newTestService(t)
	firstNow := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	secondNow := firstNow.Add(2 * time.Hour)
	service.ttl = time.Hour
	service.now = func() time.Time { return firstNow }

	first, err := service.Begin(context.Background(), beginRequest("hash-a"))
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if err := service.Complete(context.Background(), first.Record.ID, 200, wrapperspb.String("old")); err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if err := db.Model(&Record{}).Where("id = ?", first.Record.ID).Updates(map[string]any{
		"state":         StateProcessing,
		"expired_at":    firstNow.Add(-time.Minute),
		"response_body": `{"value":"old"}`,
		"status_code":   200,
	}).Error; err != nil {
		t.Fatalf("failed to prepare expired processing record: %v", err)
	}

	service.now = func() time.Time { return secondNow }
	taken, err := service.Begin(context.Background(), beginRequest("hash-b"))
	if err != nil {
		t.Fatalf("Begin returned error after expiry: %v", err)
	}
	if got, want := taken.Action, ActionProceed; got != want {
		t.Fatalf("unexpected action after expiry: got %q want %q", got, want)
	}
	if got, want := taken.Record.ID, first.Record.ID; got != want {
		t.Fatalf("expected takeover to reuse record id: got %d want %d", got, want)
	}

	var latest Record
	if err := db.First(&latest, first.Record.ID).Error; err != nil {
		t.Fatalf("failed to reload record: %v", err)
	}
	if got, want := latest.RequestHash, "hash-b"; got != want {
		t.Fatalf("unexpected request hash: got %q want %q", got, want)
	}
	if got := latest.ResponseBody; got != "" {
		t.Fatalf("expected old response to be cleared, got %q", got)
	}
	if got := latest.StatusCode; got != 0 {
		t.Fatalf("expected status code to be reset, got %d", got)
	}
	if got, want := latest.ExpiredAt, secondNow.Add(time.Hour); !got.Equal(want) {
		t.Fatalf("unexpected expiration: got %s want %s", got, want)
	}
}

func TestBeginStartsNewRequestAfterCompletedRecordExpires(t *testing.T) {
	service, db := newTestService(t)
	firstNow := time.Date(2026, 7, 24, 11, 0, 0, 0, time.UTC)
	secondNow := firstNow.Add(25 * time.Hour)
	service.now = func() time.Time { return firstNow }

	first, err := service.Begin(context.Background(), beginRequest("hash-a"))
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if err := service.Complete(context.Background(), first.Record.ID, 200, wrapperspb.String("ok")); err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	service.now = func() time.Time { return secondNow }
	result, err := service.Begin(context.Background(), beginRequest("hash-b"))
	if err != nil {
		t.Fatalf("Begin returned error after completed expiry: %v", err)
	}
	if got, want := result.Action, ActionProceed; got != want {
		t.Fatalf("unexpected action: got %q want %q", got, want)
	}

	var latest Record
	if err := db.First(&latest, first.Record.ID).Error; err != nil {
		t.Fatalf("failed to reload record: %v", err)
	}
	if got, want := latest.State, StateProcessing; got != want {
		t.Fatalf("unexpected state: got %q want %q", got, want)
	}
	if got, want := latest.RequestHash, "hash-b"; got != want {
		t.Fatalf("unexpected hash: got %q want %q", got, want)
	}
	if latest.ResponseBody != "" || latest.StatusCode != 0 {
		t.Fatalf("expected previous replay payload to be cleared, body=%q status=%d", latest.ResponseBody, latest.StatusCode)
	}
}

func TestBeginRejectsDifferentHashForUnexpiredCompletedRecord(t *testing.T) {
	service, _ := newTestService(t)
	first, err := service.Begin(context.Background(), beginRequest("hash-a"))
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if err := service.Complete(context.Background(), first.Record.ID, 200, wrapperspb.String("ok")); err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	_, err = service.Begin(context.Background(), beginRequest("hash-b"))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("unexpected error: got %v want %v", err, ErrConflict)
	}
}

func TestBeginOnlyOneConcurrentRequestTakesOverExpiredRecord(t *testing.T) {
	service, db := newConcurrentTestService(t)
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	req := beginRequest("hash-a")
	if err := db.Create(&Record{
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		RequestPath:    req.RequestPath,
		RequestHash:    req.RequestHash,
		State:          StateProcessing,
		ExpiredAt:      now.Add(-time.Minute),
	}).Error; err != nil {
		t.Fatalf("failed to create expired processing record: %v", err)
	}

	const requestCount = 20
	start := make(chan struct{})
	var wg sync.WaitGroup
	var proceedCount int32
	errCh := make(chan error, requestCount)

	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := service.Begin(context.Background(), req)
			if err != nil {
				if errors.Is(err, ErrInProgress) {
					return
				}
				errCh <- err
				return
			}
			if result.Action == ActionProceed {
				atomic.AddInt32(&proceedCount, 1)
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("unexpected concurrent Begin error: %v", err)
	}
	if got, want := atomic.LoadInt32(&proceedCount), int32(1); got != want {
		t.Fatalf("unexpected proceed count: got %d want %d", got, want)
	}
}

func TestDeleteExpiredBeforePreservesUnexpiredRecords(t *testing.T) {
	service, db := newTestService(t)
	now := time.Date(2026, 7, 24, 13, 0, 0, 0, time.UTC)
	expired := Record{
		IdempotencyKey: "expired-key",
		UserID:         1,
		RequestPath:    "POST /api/orders",
		RequestHash:    "hash-a",
		State:          StateCompleted,
		ExpiredAt:      now.Add(-time.Hour),
	}
	unexpired := Record{
		IdempotencyKey: "unexpired-key",
		UserID:         1,
		RequestPath:    "POST /api/orders",
		RequestHash:    "hash-a",
		State:          StateCompleted,
		ExpiredAt:      now.Add(time.Hour),
	}
	if err := db.Create(&expired).Error; err != nil {
		t.Fatalf("failed to create expired record: %v", err)
	}
	if err := db.Create(&unexpired).Error; err != nil {
		t.Fatalf("failed to create unexpired record: %v", err)
	}

	deleted, err := service.DeleteExpiredBefore(context.Background(), now)
	if err != nil {
		t.Fatalf("DeleteExpiredBefore returned error: %v", err)
	}
	if got, want := deleted, int64(1); got != want {
		t.Fatalf("unexpected deleted count: got %d want %d", got, want)
	}

	var remaining []Record
	if err := db.Find(&remaining).Error; err != nil {
		t.Fatalf("failed to list remaining records: %v", err)
	}
	if len(remaining) != 1 || remaining[0].IdempotencyKey != "unexpired-key" {
		t.Fatalf("unexpected remaining records: %+v", remaining)
	}
}

func TestAbortReleasesProcessingRecord(t *testing.T) {
	service, db := newTestService(t)
	req := BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "order-key",
		RequestHash:    "hash-a",
	}

	first, err := service.Begin(context.Background(), req)
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if err := service.Abort(context.Background(), first.Record.ID); err != nil {
		t.Fatalf("Abort returned error: %v", err)
	}

	var count int64
	if err := db.Unscoped().Model(&Record{}).Where("id = ?", first.Record.ID).Count(&count).Error; err != nil {
		t.Fatalf("failed to count aborted record: %v", err)
	}
	if got, want := count, int64(0); got != want {
		t.Fatalf("unexpected aborted record count: got %d want %d", got, want)
	}

	second, err := service.Begin(context.Background(), req)
	if err != nil {
		t.Fatalf("second Begin returned error: %v", err)
	}
	if got, want := second.Action, ActionProceed; got != want {
		t.Fatalf("unexpected action after abort: got %q want %q", got, want)
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
