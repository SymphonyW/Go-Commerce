-- +goose Up
CREATE TABLE IF NOT EXISTS idempotency_records (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at DATETIME(3) NULL,
    updated_at DATETIME(3) NULL,
    deleted_at DATETIME(3) NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    request_path VARCHAR(128) NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    response_body LONGTEXT NULL,
    status_code INT NOT NULL DEFAULT 0,
    state VARCHAR(16) NOT NULL,
    expired_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY idx_user_path_key (user_id, request_path, idempotency_key),
    KEY idx_idempotency_records_deleted_at (deleted_at),
    KEY idx_idempotency_records_state (state),
    KEY idx_idempotency_records_expired_at (expired_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS idempotency_records;
