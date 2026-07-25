//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"go-commerce/internal/idempotency"
)

func TestMySQLIdempotencyBeginReclaimsExpiredRecord(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	if err := db.AutoMigrate(&idempotency.Record{}); err != nil {
		t.Fatalf("failed to migrate integration schema: %v", err)
	}

	service := idempotency.NewService(db, time.Hour)
	expired := idempotency.Record{
		IdempotencyKey: "mysql-expired-key",
		UserID:         1,
		RequestPath:    "POST /api/orders",
		RequestHash:    "hash-old",
		ResponseBody:   `{"value":"old"}`,
		StatusCode:     200,
		State:          idempotency.StateCompleted,
		ExpiredAt:      time.Now().Add(-time.Hour),
	}
	if err := db.Create(&expired).Error; err != nil {
		t.Fatalf("failed to create expired record: %v", err)
	}

	result, err := service.Begin(ctx, idempotency.BeginRequest{
		UserID:         1,
		RequestPath:    "POST /api/orders",
		IdempotencyKey: "mysql-expired-key",
		RequestHash:    "hash-new",
	})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if got, want := result.Action, idempotency.ActionProceed; got != want {
		t.Fatalf("unexpected action: got %q want %q", got, want)
	}
	if got, want := result.Record.ID, expired.ID; got != want {
		t.Fatalf("expected record takeover, got id %d want %d", got, want)
	}

	var latest idempotency.Record
	if err := db.First(&latest, expired.ID).Error; err != nil {
		t.Fatalf("failed to reload record: %v", err)
	}
	if got, want := latest.State, idempotency.StateProcessing; got != want {
		t.Fatalf("unexpected state: got %q want %q", got, want)
	}
	if got, want := latest.RequestHash, "hash-new"; got != want {
		t.Fatalf("unexpected hash: got %q want %q", got, want)
	}
	if latest.ResponseBody != "" || latest.StatusCode != 0 {
		t.Fatalf("expected old response to be cleared, body=%q status=%d", latest.ResponseBody, latest.StatusCode)
	}
	if !latest.ExpiredAt.After(time.Now().Add(30 * time.Minute)) {
		t.Fatalf("expected expiration to be refreshed, got %s", latest.ExpiredAt)
	}
}
