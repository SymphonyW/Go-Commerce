package outbox

import (
	"context"
	"errors"
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
	if saved.LockedBy != "" || saved.LockedAt != nil || saved.LeaseExpiresAt != nil {
		t.Fatalf("expected new event lock fields to be clear, got locked_by=%q locked_at=%v lease_expires_at=%v", saved.LockedBy, saved.LockedAt, saved.LeaseExpiresAt)
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

func TestClaimDueEventsClaimsPendingEvent(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	saved := createPendingOrderEvent(t, repo, db, "evt-claim-1", now)

	claim, err := repo.ClaimDueEvents(context.Background(), now, 10, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimDueEvents returned error: %v", err)
	}
	if got, want := len(claim.Events), 1; got != want {
		t.Fatalf("unexpected claimed count: got %d want %d", got, want)
	}
	if got, want := claim.Events[0].ID, saved.ID; got != want {
		t.Fatalf("unexpected claimed event id: got %d want %d", got, want)
	}

	latest := loadEvent(t, db, saved.ID)
	if got, want := latest.Status, StatusProcessing; got != want {
		t.Fatalf("unexpected status after claim: got %q want %q", got, want)
	}
	if got, want := latest.LockedBy, "worker-a"; got != want {
		t.Fatalf("unexpected locked_by: got %q want %q", got, want)
	}
	if latest.LockedAt == nil || !latest.LockedAt.Equal(now) {
		t.Fatalf("unexpected locked_at: got %v want %v", latest.LockedAt, now)
	}
	if latest.LeaseExpiresAt == nil || !latest.LeaseExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("unexpected lease_expires_at: got %v want %v", latest.LeaseExpiresAt, now.Add(time.Minute))
	}
}

func TestClaimDueEventsDoesNotOverlapAcrossWorkers(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		createPendingOrderEvent(t, repo, db, fmt.Sprintf("evt-overlap-%d", i), now)
	}

	first, err := repo.ClaimDueEvents(context.Background(), now, 3, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("first ClaimDueEvents returned error: %v", err)
	}
	second, err := repo.ClaimDueEvents(context.Background(), now, 3, "worker-b", time.Minute)
	if err != nil {
		t.Fatalf("second ClaimDueEvents returned error: %v", err)
	}
	if got, want := len(first.Events), 3; got != want {
		t.Fatalf("unexpected first claim count: got %d want %d", got, want)
	}
	if got, want := len(second.Events), 2; got != want {
		t.Fatalf("unexpected second claim count: got %d want %d", got, want)
	}

	seen := map[uint]string{}
	for _, event := range first.Events {
		seen[event.ID] = "worker-a"
	}
	for _, event := range second.Events {
		if owner, exists := seen[event.ID]; exists {
			t.Fatalf("event %d claimed by both %s and worker-b", event.ID, owner)
		}
		seen[event.ID] = "worker-b"
	}
}

func TestMarkPublishedRequiresLeaseOwner(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	saved := createPendingOrderEvent(t, repo, db, "evt-owner-1", now)

	claim, err := repo.ClaimDueEvents(context.Background(), now, 1, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("ClaimDueEvents returned error: %v", err)
	}
	if len(claim.Events) != 1 {
		t.Fatalf("expected one claimed event, got %d", len(claim.Events))
	}

	err = repo.MarkPublished(context.Background(), saved.ID, "worker-b", now.Add(time.Second))
	if !errors.Is(err, ErrLeaseNotOwned) {
		t.Fatalf("expected ErrLeaseNotOwned for non-owner publish, got %v", err)
	}

	latest := loadEvent(t, db, saved.ID)
	if got, want := latest.Status, StatusProcessing; got != want {
		t.Fatalf("unexpected status after non-owner publish: got %q want %q", got, want)
	}
	if latest.PublishedAt != nil {
		t.Fatalf("expected published_at to remain nil, got %v", latest.PublishedAt)
	}
	if err := repo.MarkPublished(context.Background(), saved.ID, "worker-a", now.Add(time.Second)); err != nil {
		t.Fatalf("owner MarkPublished returned error: %v", err)
	}
}

func TestMarkRetryRequiresLeaseOwner(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	saved := createPendingOrderEvent(t, repo, db, "evt-retry-owner-1", now)

	if _, err := repo.ClaimDueEvents(context.Background(), now, 1, "worker-a", time.Minute); err != nil {
		t.Fatalf("ClaimDueEvents returned error: %v", err)
	}

	err := repo.MarkRetry(context.Background(), saved.ID, "worker-b", now.Add(time.Second), RetryUpdate{
		RetryCount:   1,
		NextRetryAt:  now.Add(time.Minute),
		LastError:    "late failure",
		MarkAsFailed: false,
	})
	if !errors.Is(err, ErrLeaseNotOwned) {
		t.Fatalf("expected ErrLeaseNotOwned for non-owner retry, got %v", err)
	}

	latest := loadEvent(t, db, saved.ID)
	if got, want := latest.Status, StatusProcessing; got != want {
		t.Fatalf("unexpected status after non-owner retry: got %q want %q", got, want)
	}
	if got, want := latest.RetryCount, 0; got != want {
		t.Fatalf("unexpected retry count after non-owner retry: got %d want %d", got, want)
	}
}

func TestClaimDueEventsRecoversExpiredLease(t *testing.T) {
	repo, db := newTestRepository(t)
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)
	saved := createPendingOrderEvent(t, repo, db, "evt-expired-lease-1", now)
	lockedAt := now.Add(-2 * time.Minute)
	leaseExpiresAt := now.Add(-time.Minute)
	if err := db.Model(&Event{}).Where("id = ?", saved.ID).Updates(map[string]interface{}{
		"status":           StatusProcessing,
		"locked_by":        "worker-a",
		"locked_at":        lockedAt,
		"lease_expires_at": leaseExpiresAt,
	}).Error; err != nil {
		t.Fatalf("failed to seed processing event: %v", err)
	}

	claim, err := repo.ClaimDueEvents(context.Background(), now, 1, "worker-b", time.Minute)
	if err != nil {
		t.Fatalf("ClaimDueEvents returned error: %v", err)
	}
	if got, want := len(claim.Events), 1; got != want {
		t.Fatalf("unexpected claimed count: got %d want %d", got, want)
	}
	if got, want := claim.LeaseRecoveredCount, 1; got != want {
		t.Fatalf("unexpected recovered count: got %d want %d", got, want)
	}

	latest := loadEvent(t, db, saved.ID)
	if got, want := latest.Status, StatusProcessing; got != want {
		t.Fatalf("unexpected status after lease recovery: got %q want %q", got, want)
	}
	if got, want := latest.LockedBy, "worker-b"; got != want {
		t.Fatalf("unexpected locked_by after lease recovery: got %q want %q", got, want)
	}
}
