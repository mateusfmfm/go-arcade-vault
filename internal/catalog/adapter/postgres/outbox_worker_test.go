package postgres

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePublisher struct {
	err   error
	calls []publishedCall
}

type publishedCall struct {
	eventType string
	payload   []byte
}

func (p *fakePublisher) Publish(_ context.Context, eventType string, payload []byte) error {
	p.calls = append(p.calls, publishedCall{eventType: eventType, payload: payload})
	return p.err
}

func TestOutboxWorker_ProcessBatch(t *testing.T) {
	ctx, pool := setupPostgres(t)
	repo := NewRepositoryImpl(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("successful publish marks outbox row", func(t *testing.T) {
		cabinetID := "cab-outbox-ok"
		seedCabinet(t, ctx, pool, cabinetID, 2, 1000)

		order, err := repo.CreateOrderTx(ctx, "user-outbox-ok", []app.OrderItemInput{
			{CabinetID: cabinetID, Quantity: 1},
		}, "idem-outbox-ok")
		require.NoError(t, err)
		assert.Equal(t, 1, countUnpublishedOutbox(t, ctx, pool, order.ID))

		pub := &fakePublisher{}
		worker := app.NewOutboxWorker(repo, pub, logger)
		worker.ProcessBatch(ctx)

		require.Len(t, pub.calls, 1)
		assert.Equal(t, "order.created", pub.calls[0].eventType)
		assert.NotEmpty(t, pub.calls[0].payload)
		assert.Equal(t, 0, countUnpublishedOutbox(t, ctx, pool, order.ID))
		assert.Equal(t, 1, countPublishedOutbox(t, ctx, pool, order.ID))
	})

	t.Run("failed publish leaves row unpublished", func(t *testing.T) {
		cabinetID := "cab-outbox-fail"
		seedCabinet(t, ctx, pool, cabinetID, 2, 1000)

		order, err := repo.CreateOrderTx(ctx, "user-outbox-fail", []app.OrderItemInput{
			{CabinetID: cabinetID, Quantity: 1},
		}, "idem-outbox-fail")
		require.NoError(t, err)

		pub := &fakePublisher{err: assert.AnError}
		worker := app.NewOutboxWorker(repo, pub, logger)
		worker.ProcessBatch(ctx)

		require.Len(t, pub.calls, 1, "publish is attempted even when it fails")
		assert.Equal(t, 1, countUnpublishedOutbox(t, ctx, pool, order.ID))
		assert.Equal(t, 0, countPublishedOutbox(t, ctx, pool, order.ID))
	})
}

func countUnpublishedOutbox(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderID string) int {
	t.Helper()
	return countOutboxByPublished(t, ctx, pool, orderID, true)
}

func countPublishedOutbox(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderID string) int {
	t.Helper()
	return countOutboxByPublished(t, ctx, pool, orderID, false)
}

func countOutboxByPublished(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderID string, unpublished bool) int {
	t.Helper()
	predicate := "published_at IS NOT NULL"
	if unpublished {
		predicate = "published_at IS NULL"
	}
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM catalog.outbox
		WHERE aggregate_id = $1 AND event_type = 'order.created' AND `+predicate, orderID).Scan(&n)
	require.NoError(t, err)
	return n
}
