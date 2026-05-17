//go:build integration

package integration_test

import (
	"testing"

	"go-commerce/internal/payment"
)

func TestMySQLPaymentAutoMigrateCanRunRepeatedly(t *testing.T) {
	db := openIntegrationDB(t)

	if err := db.AutoMigrate(&payment.Payment{}); err != nil {
		t.Fatalf("first payment migration failed: %v", err)
	}
	if err := db.AutoMigrate(&payment.Payment{}); err != nil {
		t.Fatalf("second payment migration failed: %v", err)
	}
}
