CREATE TABLE catalog.orders (
    id VARCHAR(36) PRIMARY KEY,
    user_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending_payment', 'paid', 'cancelled', 'failed')),
    total_cents BIGINT NOT NULL,
    idempotency_key VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE catalog.order_items (
    id VARCHAR(36) PRIMARY KEY,
    order_id VARCHAR(36) NOT NULL REFERENCES catalog.orders(id) ON DELETE CASCADE,
    cabinet_id VARCHAR(36) NOT NULL REFERENCES catalog.cabinets(id),
    quantity INT NOT NULL CHECK (quantity > 0),
    unit_price_cents BIGINT NOT NULL
);

CREATE TABLE catalog.outbox (
    id VARCHAR(36) PRIMARY KEY,
    aggregate_id VARCHAR(36) NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_orders_user_id ON catalog.orders(user_id);
CREATE INDEX idx_order_items_order_id ON catalog.order_items(order_id);
CREATE INDEX idx_outbox_unpublished ON catalog.outbox(published_at) WHERE published_at IS NULL;
