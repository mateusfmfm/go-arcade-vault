-- name: CreateOrder :one
-- CreateOrder inserts a new order aggregate into the orders table.
INSERT INTO catalog.orders (id, user_id, status, total_cents, idempotency_key)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreateOrderItem :exec
-- CreateOrderItem inserts an individual item line belonging to an order.
INSERT INTO catalog.order_items (id, order_id, cabinet_id, quantity, unit_price_cents)
VALUES ($1, $2, $3, $4, $5);

-- name: GetOrder :one
-- GetOrder fetches a single order by its primary key ID.
SELECT * FROM catalog.orders WHERE id = $1 LIMIT 1;

-- name: GetOrderByItempotencyKey :one
-- GetOrderByIdempotencyKey retrieves an existing order using its order idempotency key.
SELECT * FROM catalog.orders WHERE idempotency_key = $1 LIMIT 1;

-- name: ListOrderItems :many
-- ListOrderItems fetches all item lines associated with a given order.
SELECT * FROM catalog.order_items WHERE order_id = $1;

-- name: InsertOutboxEvent :exec
-- InsertOutboxEvent records an event in the outbox table within the active transaction.
INSERT INTO catalog.outbox (id, aggregate_id, event_type, payload)
VALUES ($1, $2, $3, $4);

-- name: ListUnpublishedOutboxEvents :many
-- ListUnpublishedOutboxEvents fetches pending outbox events for background worker dispatch.
SELECT * FROM catalog.outbox WHERE published_at IS NULL ORDER BY created_at ASC LIMIT $1;

-- name: MarkOutboxEventPublished :exec
-- MarkOutboxEventPublished marks an outbox event as successfilly sent.
UPDATE catalog.outbox SET published_at = NOW() WHERE id = $1;