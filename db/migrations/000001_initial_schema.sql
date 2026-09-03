-- +goose Up
CREATE TABLE IF NOT EXISTS users (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at DATETIME(3) NULL,
    updated_at DATETIME(3) NULL,
    deleted_at DATETIME(3) NULL,
    username VARCHAR(191) NOT NULL,
    password VARCHAR(255) NOT NULL,
    email VARCHAR(191) NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'customer',
    PRIMARY KEY (id),
    UNIQUE KEY uni_users_username (username),
    UNIQUE KEY uni_users_email (email),
    KEY idx_users_deleted_at (deleted_at),
    KEY idx_users_role (role)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS merchants (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at DATETIME(3) NULL,
    updated_at DATETIME(3) NULL,
    deleted_at DATETIME(3) NULL,
    name VARCHAR(255) NOT NULL,
    contact_info VARCHAR(255) NOT NULL,
    owner_user_id BIGINT UNSIGNED NULL,
    PRIMARY KEY (id),
    KEY idx_merchants_deleted_at (deleted_at),
    KEY idx_merchants_owner_user_id (owner_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS products (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at DATETIME(3) NULL,
    updated_at DATETIME(3) NULL,
    deleted_at DATETIME(3) NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT NULL,
    price_cents BIGINT NOT NULL,
    stock INT NOT NULL DEFAULT 0,
    category VARCHAR(128) NULL,
    image_url VARCHAR(512) NULL,
    merchant_id BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (id),
    KEY idx_products_deleted_at (deleted_at),
    KEY idx_products_category (category),
    KEY idx_products_merchant_id (merchant_id),
    KEY idx_products_price_cents (price_cents),
    KEY idx_products_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS orders (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at DATETIME(3) NULL,
    updated_at DATETIME(3) NULL,
    deleted_at DATETIME(3) NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    total_amount_cents BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    cancel_reason VARCHAR(64) NULL,
    order_date DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_orders_deleted_at (deleted_at),
    KEY idx_orders_user_id (user_id),
    KEY idx_orders_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS order_items (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at DATETIME(3) NULL,
    updated_at DATETIME(3) NULL,
    deleted_at DATETIME(3) NULL,
    order_id BIGINT UNSIGNED NOT NULL,
    product_id BIGINT NOT NULL,
    merchant_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
    product_name VARCHAR(255) NOT NULL,
    price_cents BIGINT NOT NULL,
    quantity INT NOT NULL,
    PRIMARY KEY (id),
    KEY idx_order_items_deleted_at (deleted_at),
    KEY idx_order_items_order_id (order_id),
    KEY idx_order_items_merchant_id (merchant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS payments (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at DATETIME(3) NULL,
    updated_at DATETIME(3) NULL,
    deleted_at DATETIME(3) NULL,
    payment_no VARCHAR(64) NOT NULL,
    order_id BIGINT UNSIGNED NOT NULL,
    active_order_id BIGINT UNSIGNED NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    amount_cents BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'created',
    payment_method VARCHAR(64) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uni_payments_payment_no (payment_no),
    KEY idx_payments_deleted_at (deleted_at),
    KEY idx_payments_order_id (order_id),
    KEY idx_payments_user_id (user_id),
    KEY idx_payments_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS merchants;
DROP TABLE IF EXISTS users;
