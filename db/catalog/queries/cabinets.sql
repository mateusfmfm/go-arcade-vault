-- name: GetCabinet :one
SELECT * FROM catalog.cabinets
WHERE id = $1;

-- name: ListCabinets :many
SELECT * FROM catalog.cabinets
ORDER BY created_at DESC;

-- name: DecrementStock :one
UPDATE catalog.cabinets
SET quantity = quantity - @quantity,
    updated_at = NOW()
WHERE id = @id AND quantity >= @quantity
RETURNING *;

-- name: CreateReservation :exec
INSERT INTO catalog.reservations (idempotency_key, cabinet_id, quantity)
VALUES ($1, $2, $3);

-- name: GetReservation :one
SELECT * FROM catalog.reservations
WHERE idempotency_key = $1;