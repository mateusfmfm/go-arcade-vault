package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/app"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestReserveStock_RaceCondition(t *testing.T) {
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

	cabinetID := "cab-race-test-01"
	_, err = pool.Exec(ctx, `
		INSERT INTO catalog.cabinets (id, slug, model, type, display, condition, quantity, price, manufacturer, year)
		VALUES ($1, 'mortal-kombat-2-test', 'Mortal Kombat II', 'upright', 'crt', 'Restored Mint', 1, 320000, 'Midway', '1993')
	`, cabinetID)
	require.NoError(t, err)

	usecase := app.NewCabinetUsecase(NewRepositoryImpl(pool))

	var wg sync.WaitGroup
	var err1, err2 error

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err1 = usecase.ReserveStockTx(ctx, cabinetID, 1, "idempotency-key-user-1")
	}()
	go func() {
		defer wg.Done()
		_, err2 = usecase.ReserveStockTx(ctx, cabinetID, 1, "idempotency-key-user-2")
	}()
	wg.Wait()

	successes := 0
	insufficientErrors := 0
	for _, e := range []error{err1, err2} {
		switch {
		case e == nil:
			successes++
		case errors.Is(e, domain.ErrInsufficientStock):
			insufficientErrors++
		default:
			t.Errorf("unexpected error: %v", e)
		}
	}

	assert.Equal(t, 1, successes, "exactly one goroutine must reserve")
	assert.Equal(t, 1, insufficientErrors, "exactly one goroutine must get ErrInsufficientStock")

	c, err := usecase.GetCabinet(ctx, cabinetID)
	require.NoError(t, err)
	assert.Equal(t, 0, c.Quantity, "final stock must be zero")
}

func applyMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	migrationsDir := filepath.Join(filepath.Dir(file), "../../../../db/catalog/migrations")

	matches, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "no migration files in %s", migrationsDir)
	sort.Strings(matches)

	for _, path := range matches {
		sql, err := os.ReadFile(path)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, string(sql))
		require.NoError(t, err, "apply %s", path)
	}
}
