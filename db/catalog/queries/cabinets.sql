-- name: GetCabinet :one
SELECT
    c.id,
    c.slug,
    c.model,
    c.type,
    c.display,
    c.condition,
    c.quantity,
    c.price,
    c.manufacturer,
    c.year,
    c.created_at,
    c.updated_at,
    COALESCE((
        SELECT json_agg(
            json_build_object(
                'id', i.id,
                'url', i.url,
                'alt', i.alt,
                'sort_order', i.sort_order
            )
            ORDER BY i.sort_order ASC, i.created_at ASC
        )
        FROM catalog.cabinet_images i
        WHERE i.cabinet_id = c.id
    ), '[]'::jsonb)::jsonb AS images
FROM catalog.cabinets c
WHERE c.id = $1;

-- name: ListCabinets :many
SELECT
    c.id,
    c.slug,
    c.model,
    c.type,
    c.display,
    c.condition,
    c.quantity,
    c.price,
    c.manufacturer,
    c.year,
    c.created_at,
    c.updated_at,
    COALESCE((
        SELECT json_agg(
            json_build_object(
                'id', i.id,
                'url', i.url,
                'alt', i.alt,
                'sort_order', i.sort_order
            )
            ORDER BY i.sort_order ASC, i.created_at ASC
        )
        FROM catalog.cabinet_images i
        WHERE i.cabinet_id = c.id
    ), '[]'::jsonb)::jsonb AS images
FROM catalog.cabinets c
ORDER BY c.created_at DESC;

-- name: DecrementStock :one
UPDATE catalog.cabinets c
SET quantity = quantity - @quantity,
    updated_at = NOW()
WHERE c.id = @id AND c.quantity >= @quantity
RETURNING
    c.id,
    c.slug,
    c.model,
    c.type,
    c.display,
    c.condition,
    c.quantity,
    c.price,
    c.manufacturer,
    c.year,
    c.created_at,
    c.updated_at,
    COALESCE((
        SELECT json_agg(
            json_build_object(
                'id', i.id,
                'url', i.url,
                'alt', i.alt,
                'sort_order', i.sort_order
            )
            ORDER BY i.sort_order ASC, i.created_at ASC
        )
        FROM catalog.cabinet_images i
        WHERE i.cabinet_id = c.id
    ), '[]'::jsonb)::jsonb AS images;

-- name: CreateReservation :exec
INSERT INTO catalog.reservations (idempotency_key, cabinet_id, quantity)
VALUES ($1, $2, $3);

-- name: GetReservation :one
SELECT * FROM catalog.reservations
WHERE idempotency_key = $1;
