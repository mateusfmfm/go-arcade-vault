package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/app"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestCreateOrderTx(t *testing.T) {
	ctx, pool := setupPostgres(t)
	repo := NewRepositoryImpl(pool)

	t.Run("happy path persists pending order, decrements stock and writes outbox", func(t *testing.T) {
		cabMK := "cab-order-happy-mk"
		cabTMNT := "cab-order-happy-tmnt"
		seedCabinet(t, ctx, pool, cabMK, 5, 320000)
		seedCabinet(t, ctx, pool, cabTMNT, 3, 580000)

		items := []app.OrderItemInput{
			{CabinetID: cabMK, Quantity: 2},
			{CabinetID: cabTMNT, Quantity: 1},
		}

		order, err := repo.CreateOrderTx(ctx, "user-happy-1", items, "idem-happy-1")
		require.NoError(t, err)
		require.NotNil(t, order)

		assert.Equal(t, "user-happy-1", order.UserID)
		assert.Equal(t, domain.StatusPendingPayment, order.Status)
		assert.Equal(t, int64(320000*2+580000), order.TotalCents)
		assert.Equal(t, "idem-happy-1", order.IdempotencyKey)
		require.Len(t, order.Items, 2)

		gotItems := orderItemsByCabinet(order.Items)
		assert.Equal(t, 2, gotItems[cabMK].Quantity)
		assert.Equal(t, int64(320000), gotItems[cabMK].UnitPriceCents)
		assert.Equal(t, 1, gotItems[cabTMNT].Quantity)
		assert.Equal(t, int64(580000), gotItems[cabTMNT].UnitPriceCents)

		persisted, err := repo.GetOrder(ctx, order.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusPendingPayment, persisted.Status)
		assert.Equal(t, order.TotalCents, persisted.TotalCents)
		require.Len(t, persisted.Items, 2)
		persistedItems := orderItemsByCabinet(persisted.Items)
		assert.Equal(t, int64(320000), persistedItems[cabMK].UnitPriceCents)
		assert.Equal(t, int64(580000), persistedItems[cabTMNT].UnitPriceCents)

		assert.Equal(t, 3, cabinetQuantity(t, ctx, pool, cabMK))
		assert.Equal(t, 2, cabinetQuantity(t, ctx, pool, cabTMNT))

		assert.Equal(t, int64(320000), orderItemUnitPrice(t, ctx, pool, order.ID, cabMK))
		assert.Equal(t, int64(580000), orderItemUnitPrice(t, ctx, pool, order.ID, cabTMNT))

		assert.Equal(t, 1, countOutboxEvents(t, ctx, pool, order.ID))
		payload := outboxPayload(t, ctx, pool, order.ID)
		assert.Equal(t, order.ID, payload.OrderID)
		assert.Equal(t, "user-happy-1", payload.UserID)
		assert.Equal(t, order.TotalCents, payload.AmountCents)
		assert.NotEmpty(t, payload.EventID)
		assert.NotEmpty(t, payload.OccurredAt)
	})

	t.Run("insufficient stock rolls back the transaction", func(t *testing.T) {
		cabOK := "cab-order-stock-ok"
		cabLow := "cab-order-stock-low"
		seedCabinet(t, ctx, pool, cabOK, 4, 1000)
		seedCabinet(t, ctx, pool, cabLow, 1, 2500)

		outboxBefore := countAllOutboxEvents(t, ctx, pool)
		ordersBefore := countOrdersByIdempotencyKey(t, ctx, pool, "idem-stock-fail")

		items := []app.OrderItemInput{
			{CabinetID: cabOK, Quantity: 1},
			{CabinetID: cabLow, Quantity: 5},
		}

		order, err := repo.CreateOrderTx(ctx, "user-stock-fail", items, "idem-stock-fail")
		require.ErrorIs(t, err, domain.ErrInsufficientStock)
		assert.Nil(t, order)

		assert.Equal(t, 4, cabinetQuantity(t, ctx, pool, cabOK), "successful line must roll back")
		assert.Equal(t, 1, cabinetQuantity(t, ctx, pool, cabLow))
		assert.Equal(t, ordersBefore, countOrdersByIdempotencyKey(t, ctx, pool, "idem-stock-fail"))
		assert.Equal(t, outboxBefore, countAllOutboxEvents(t, ctx, pool))
		assert.Equal(t, 0, countReservationsForCabinet(t, ctx, pool, cabOK))
		assert.Equal(t, 0, countReservationsForCabinet(t, ctx, pool, cabLow))
	})

	t.Run("same idempotency key and payload returns existing order without side effects", func(t *testing.T) {
		cabinetID := "cab-order-idem"
		seedCabinet(t, ctx, pool, cabinetID, 5, 450000)

		items := []app.OrderItemInput{
			{CabinetID: cabinetID, Quantity: 2},
		}

		first, err := repo.CreateOrderTx(ctx, "user-idem-1", items, "idem-replay-1")
		require.NoError(t, err)
		require.NotNil(t, first)

		second, err := repo.CreateOrderTx(ctx, "user-idem-1", items, "idem-replay-1")
		require.NoError(t, err)
		require.NotNil(t, second)

		assert.Equal(t, first.ID, second.ID)
		assert.Equal(t, domain.StatusPendingPayment, second.Status)
		assert.Equal(t, first.TotalCents, second.TotalCents)
		assert.Equal(t, 1, countOrdersByIdempotencyKey(t, ctx, pool, "idem-replay-1"))
		assert.Equal(t, 3, cabinetQuantity(t, ctx, pool, cabinetID), "stock must be decremented only once")
		assert.Equal(t, 1, countOutboxEvents(t, ctx, pool, first.ID), "outbox must not duplicate order.created")
		assert.Equal(t, 1, countReservationsForCabinet(t, ctx, pool, cabinetID))
	})
}

func setupPostgres(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("arcade_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, testcontainers.TerminateContainer(pgContainer))
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	applyMigrations(t, ctx, pool)
	return ctx, pool
}

func seedCabinet(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string, quantity int, price int64) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		INSERT INTO catalog.cabinets (id, slug, model, type, display, condition, quantity, price, manufacturer, year)
		VALUES ($1, $2, 'Test Cabinet', 'upright', 'crt', 'Restored Mint', $3, $4, 'Midway', '1993')
	`, id, id+"-slug", quantity, price)
	require.NoError(t, err)
}

func cabinetQuantity(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string) int {
	t.Helper()
	var quantity int
	err := pool.QueryRow(ctx, `SELECT quantity FROM catalog.cabinets WHERE id = $1`, id).Scan(&quantity)
	require.NoError(t, err)
	return quantity
}

func orderItemUnitPrice(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderID, cabinetID string) int64 {
	t.Helper()
	var price int64
	err := pool.QueryRow(ctx, `
		SELECT unit_price_cents FROM catalog.order_items WHERE order_id = $1 AND cabinet_id = $2
	`, orderID, cabinetID).Scan(&price)
	require.NoError(t, err)
	return price
}

func countOutboxEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderID string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM catalog.outbox WHERE aggregate_id = $1 AND event_type = 'order.created'
	`, orderID).Scan(&n)
	require.NoError(t, err)
	return n
}

func countAllOutboxEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM catalog.outbox`).Scan(&n)
	require.NoError(t, err)
	return n
}

func countOrdersByIdempotencyKey(t *testing.T, ctx context.Context, pool *pgxpool.Pool, key string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM catalog.orders WHERE idempotency_key = $1`, key).Scan(&n)
	require.NoError(t, err)
	return n
}

func countReservationsForCabinet(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cabinetID string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM catalog.reservations WHERE cabinet_id = $1`, cabinetID).Scan(&n)
	require.NoError(t, err)
	return n
}

func outboxPayload(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderID string) OrderCreatedPayload {
	t.Helper()
	var raw []byte
	err := pool.QueryRow(ctx, `
		SELECT payload FROM catalog.outbox WHERE aggregate_id = $1 AND event_type = 'order.created'
	`, orderID).Scan(&raw)
	require.NoError(t, err)

	var payload OrderCreatedPayload
	require.NoError(t, json.Unmarshal(raw, &payload))
	return payload
}

func orderItemsByCabinet(items []domain.OrderLine) map[string]domain.OrderLine {
	out := make(map[string]domain.OrderLine, len(items))
	for _, item := range items {
		out[item.CabinetID] = item
	}
	return out
}
