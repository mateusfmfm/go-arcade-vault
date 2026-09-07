package app

import (
	"context"

	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/domain"
)

type CabinetUsecase struct {
	repository CabinetRepository
}

func NewCabinetUsecase(repository CabinetRepository) *CabinetUsecase {
	return &CabinetUsecase{
		repository: repository,
	}
}

func (u *CabinetUsecase) GetCabinet(ctx context.Context, id string) (*domain.Cabinet, error) {
	return u.repository.GetCabinet(ctx, id)
}

func (u *CabinetUsecase) ListCabinets(ctx context.Context) ([]*domain.Cabinet, error) {
	return u.repository.ListCabinets(ctx)
}

func (u *CabinetUsecase) ReserveStockTx(ctx context.Context, cabinetID string, quantity int, idempotencyKey string) (*domain.Cabinet, error) {
	if quantity <= 0 {
		return nil, domain.ErrInsufficientStock
	}
	return u.repository.ReserveStockTx(ctx, cabinetID, quantity, idempotencyKey)
}
