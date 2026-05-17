//go:build integration

package integration_test

import (
	"testing"

	"go-commerce/internal/outbox"
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
