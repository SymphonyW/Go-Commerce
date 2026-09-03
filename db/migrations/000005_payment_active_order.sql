-- +goose Up
UPDATE payments
SET active_order_id = order_id
WHERE active_order_id IS NULL
  AND status IN ('created', 'succeeded');

CREATE UNIQUE INDEX idx_payments_active_order ON payments (active_order_id);

-- +goose Down
DROP INDEX idx_payments_active_order ON payments;
