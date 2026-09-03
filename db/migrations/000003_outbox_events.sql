-- +goose Up
CREATE TABLE IF NOT EXISTS outbox_events (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    event_id VARCHAR(64) NOT NULL,
    aggregate_type VARCHAR(64) NOT NULL,
    aggregate_id VARCHAR(64) NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    payload LONGTEXT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    locked_by VARCHAR(128) NOT NULL DEFAULT '',
    locked_at DATETIME(3) NULL,
    lease_expires_at DATETIME(3) NULL,
    retry_count INT NOT NULL DEFAULT 0,
    next_retry_at DATETIME(3) NOT NULL,
    last_error TEXT NULL,
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NULL,
    published_at DATETIME(3) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uni_outbox_events_event_id (event_id),
    KEY idx_outbox_aggregate (aggregate_type, aggregate_id),
    KEY idx_outbox_events_event_type (event_type),
    KEY idx_outbox_events_status (status),
    KEY idx_outbox_events_locked_by (locked_by),
    KEY idx_outbox_events_locked_at (locked_at),
    KEY idx_outbox_events_lease_expires_at (lease_expires_at),
    KEY idx_outbox_events_next_retry_at (next_retry_at),
    KEY idx_outbox_events_created_at (created_at),
    KEY idx_outbox_events_published_at (published_at),
    KEY idx_outbox_pending_claim (status, next_retry_at),
    KEY idx_outbox_processing_claim (status, lease_expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS outbox_events;
