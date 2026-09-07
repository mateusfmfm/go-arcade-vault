CREATE SCHEMA IF NOT EXISTS catalog;

CREATE TABLE catalog.cabinets (
    id VARCHAR(36) PRIMARY KEY,
    slug VARCHAR(100) UNIQUE NOT NULL,
    model VARCHAR(100) NOT NULL,
    type VARCHAR(50) NOT NULL,
    display VARCHAR(50) NOT NULL,
    condition TEXT NOT NULL,
    quantity INT NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    price BIGINT NOT NULL,
    manufacturer VARCHAR(100) NOT NULL,
    year VARCHAR(10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE catalog.reservations (
    idempotency_key VARCHAR(255) PRIMARY KEY,
    cabinet_id VARCHAR(36) NOT NULL REFERENCES catalog.cabinets(id),
    quantity INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cabinets_slug ON catalog.cabinets(slug);