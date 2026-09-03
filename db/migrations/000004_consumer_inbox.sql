-- +goose Up
CREATE TABLE IF NOT EXISTS consumed_events (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    consumer_name VARCHAR(128) NOT NULL,
    event_id VARCHAR(128) NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    consumed_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY idx_consumed_events_consumer_event (consumer_name, event_id),
    KEY idx_consumed_events_event_type (event_type),
    KEY idx_consumed_events_consumed_at (consumed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS consumed_events;
