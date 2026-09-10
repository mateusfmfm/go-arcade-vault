package app

import (
	"context"

	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/domain"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
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
	ctx, span := otel.Tracer("catalog").Start(ctx, "ReserveCabinet")
	defer span.End()
	span.SetAttributes(
		attribute.String("cabinet_id", cabinetID),
		attribute.Int("quantity", quantity),
		attribute.String("idempotency_key", idempotencyKey),
	)

	if quantity <= 0 {
		span.SetStatus(otelcodes.Error, domain.ErrInsufficientStock.Error())
		return nil, domain.ErrInsufficientStock
	}

	cabinet, err := u.repository.ReserveStockTx(ctx, cabinetID, quantity, idempotencyKey)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	return cabinet, nil
}
