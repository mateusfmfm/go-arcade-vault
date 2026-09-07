package app

import (
	"context"

	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/domain"
)

type CabinetRepository interface {
	GetCabinet(ctx context.Context, id string) (*domain.Cabinet, error)
	ListCabinets(ctx context.Context) ([]*domain.Cabinet, error)
	ReserveStockTx(ctz context.Context, cabinetID string, quantity int, idempotencyKey string) (*domain.Cabinet, error)
}
