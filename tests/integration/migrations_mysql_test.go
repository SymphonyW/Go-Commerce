//go:build integration

package integration_test

import (
	"database/sql"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"

	dbmigrations "go-commerce/db"
)

const latestMigrationVersion int64 = 5

func TestMySQLMigrationsCanRunUpDownUp(t *testing.T) {
	gormDB := openIntegrationDB(t)
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("failed to get sql db: %v", err)
	}

	if err := dbmigrations.Configure(); err != nil {
		t.Fatalf("failed to configure migrations: %v", err)
	}

	if err := goose.Up(sqlDB, dbmigrations.Dir); err != nil {
		t.Fatalf("migration up failed: %v", err)
	}
	if err := goose.Up(sqlDB, dbmigrations.Dir); err != nil {
		t.Fatalf("second migration up failed: %v", err)
	}
	if err := goose.Status(sqlDB, dbmigrations.Dir); err != nil {
		t.Fatalf("migration status failed: %v", err)
	}
	assertMigrationVersion(t, sqlDB, latestMigrationVersion)
	assertTablesExist(t, sqlDB, []string{
		"users",
		"products",
		"merchants",
		"orders",
		"order_items",
		"payments",
		"idempotency_records",
		"outbox_events",
		"consumed_events",
	})
	assertIndexExists(t, sqlDB, "users", "uni_users_username", true)
	assertIndexExists(t, sqlDB, "orders", "idx_orders_user_id", false)
	assertIndexExists(t, sqlDB, "orders", "idx_orders_created_at", false)
	assertIndexExists(t, sqlDB, "order_items", "idx_order_items_order_id", false)
	assertIndexExists(t, sqlDB, "order_items", "idx_order_items_merchant_id", false)
	assertIndexExists(t, sqlDB, "payments", "idx_payments_active_order", true)
	assertIndexExists(t, sqlDB, "idempotency_records", "idx_user_path_key", true)
	assertIndexExists(t, sqlDB, "outbox_events", "idx_outbox_pending_claim", false)
	assertIndexExists(t, sqlDB, "outbox_events", "idx_outbox_processing_claim", false)
	assertIndexExists(t, sqlDB, "consumed_events", "idx_consumed_events_consumer_event", true)

	if err := goose.Reset(sqlDB, dbmigrations.Dir); err != nil {
		t.Fatalf("migration reset failed: %v", err)
	}
	assertTablesAbsent(t, sqlDB, []string{
		"users",
		"products",
		"merchants",
		"orders",
		"order_items",
		"payments",
		"idempotency_records",
		"outbox_events",
		"consumed_events",
	})

	if err := goose.Up(sqlDB, dbmigrations.Dir); err != nil {
		t.Fatalf("migration up after reset failed: %v", err)
	}
	assertMigrationVersion(t, sqlDB, latestMigrationVersion)
}

func assertMigrationVersion(t *testing.T, db *sql.DB, want int64) {
	t.Helper()

	got, err := goose.GetDBVersion(db)
	if err != nil {
		t.Fatalf("failed to get migration version: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected migration version: got %d want %d", got, want)
	}
}

func assertTablesExist(t *testing.T, db *sql.DB, tables []string) {
	t.Helper()

	for _, table := range tables {
		if !tableExists(t, db, table) {
			t.Fatalf("expected table %s to exist", table)
		}
	}
}

func assertTablesAbsent(t *testing.T, db *sql.DB, tables []string) {
	t.Helper()

	for _, table := range tables {
		if tableExists(t, db, table) {
			t.Fatalf("expected table %s to be absent after reset", table)
		}
	}
}

func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()

	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name = ?
	`, table).Scan(&count); err != nil {
		t.Fatalf("failed to inspect table %s: %v", table, err)
	}
	return count > 0
}

func assertIndexExists(t *testing.T, db *sql.DB, table, index string, wantUnique bool) {
	t.Helper()

	var count int
	var nonUnique int
	if err := db.QueryRow(`
		SELECT COUNT(*), COALESCE(MIN(non_unique), 1)
		FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = ?
		  AND index_name = ?
	`, table, index).Scan(&count, &nonUnique); err != nil {
		t.Fatalf("failed to inspect index %s.%s: %v", table, index, err)
	}
	if count == 0 {
		t.Fatalf("expected index %s.%s to exist", table, index)
	}
	if wantUnique && nonUnique != 0 {
		t.Fatalf("expected index %s.%s to be unique", table, index)
	}
}
