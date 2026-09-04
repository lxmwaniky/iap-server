CREATE TABLE IF NOT EXISTS purchases (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL,
    product_id TEXT NOT NULL,
    purchase_token TEXT NOT NULL UNIQUE,
    purchase_type TEXT NOT NULL,
    status TEXT NOT NULL,
    order_id TEXT,
    obfuscated_account_id TEXT,
    expires_at TIMESTAMPTZ,
    acknowledged_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS entitlements (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL,
    product_id TEXT NOT NULL,
    status TEXT NOT NULL,
    source TEXT NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, product_id)
);

CREATE TABLE IF NOT EXISTS play_notifications (
    id BIGSERIAL PRIMARY KEY,
    message_id TEXT NOT NULL UNIQUE,
    package_name TEXT NOT NULL,
    event_type TEXT NOT NULL,
    product_id TEXT,
    purchase_token TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_purchases_user_product ON purchases(user_id, product_id);
CREATE INDEX IF NOT EXISTS idx_purchases_token ON purchases(purchase_token);
CREATE INDEX IF NOT EXISTS idx_entitlements_user_product ON entitlements(user_id, product_id);
CREATE INDEX IF NOT EXISTS idx_play_notifications_token ON play_notifications(purchase_token);

