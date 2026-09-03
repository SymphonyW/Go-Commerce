# Database Migrations

This project uses Goose for versioned MySQL schema migrations.

Why Goose:

- It is a Go library and CLI-friendly package, so the migration runner can live in this repo.
- SQL files can be embedded with `go:embed`, which makes the `cmd/migrate` container self-contained.
- It supports `Up`, `Down`, `Status`, and version tracking through the `goose_db_version` table.

Current topology: all services share the same `ecommerce` MySQL database. Migrations are centralized here; business services do not run schema changes by default.

## Commands

Set `DB_DSN` first when not using the default local Compose DSN.

```bash
go run ./cmd/migrate up
go run ./cmd/migrate down
go run ./cmd/migrate status
go run ./cmd/migrate version
```

Makefile shortcuts:

```bash
make migrate-up
make migrate-down
make migrate-status
```

`down` rolls back one migration version. Integration tests use Goose `Reset` to validate a full down/up cycle on a temporary MySQL schema.

## Local Compose

`docker compose up -d --build` starts a one-shot `migrate` service after MySQL is healthy. Database-backed services depend on that service completing successfully, so a migration failure prevents those services from starting.

## AutoMigrate

Production and Compose defaults do not run GORM `AutoMigrate`. A local developer can temporarily set:

```bash
AUTO_MIGRATE_ENABLED=true
```

This is only a development escape hatch for tests or experiments. Prefer `cmd/migrate` for the shared MySQL database.

SQLite unit tests may continue using `AutoMigrate` for isolated in-memory schemas.

## Existing Databases

For a current local demo database, the simplest path is to rebuild it:

```bash
docker compose down -v
docker compose up -d --build
go run ./cmd/seed-data
```

For a database that already has the exact current schema and must be kept, create a backup first and baseline Goose by marking the latest version in `goose_db_version`. Do not baseline an older schema that is missing columns or indexes.

Current latest version: `5`.

```sql
CREATE TABLE IF NOT EXISTS goose_db_version (
    id SERIAL NOT NULL,
    version_id BIGINT NOT NULL,
    is_applied BOOLEAN NOT NULL,
    tstamp TIMESTAMP NULL DEFAULT NOW(),
    PRIMARY KEY (id)
);

INSERT INTO goose_db_version (version_id, is_applied)
SELECT 5, TRUE
WHERE NOT EXISTS (
    SELECT 1
    FROM goose_db_version
    WHERE version_id = 5
      AND is_applied = TRUE
);
```

Verify afterwards:

```bash
go run ./cmd/migrate status
go run ./cmd/migrate version
```

Historical floating-point monetary data is not preserved by this demo schema. If migrating old amount columns manually, write cents with `ROUND(old_amount * 100)` into the matching `*_cents` columns before cutting over.

## Indexed Constraints

The migrations create the current required constraints and indexes, including:

| Table | Index / constraint |
| --- | --- |
| `users` | unique `username`, unique `email` |
| `orders` | `user_id`, `created_at` |
| `order_items` | `order_id`, `merchant_id` |
| `payments` | unique `payment_no`, unique `active_order_id` |
| `idempotency_records` | unique `user_id + request_path + idempotency_key` |
| `outbox_events` | unique `event_id`, claim indexes for pending and expired processing leases |
| `consumed_events` | unique `consumer_name + event_id` |
