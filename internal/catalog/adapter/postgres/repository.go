package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/adapter/postgres/db"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/domain"
)

type CabinetRepositoryImpl struct {
	pool *pgxpool.Pool
}

func NewRepositoryImpl(pool *pgxpool.Pool) *CabinetRepositoryImpl {
	return &CabinetRepositoryImpl{pool: pool}
}

func (r *CabinetRepositoryImpl) GetCabinet(ctx context.Context, id string) (*domain.Cabinet, error) {
	queries := db.New(r.pool)
	c, err := queries.GetCabinet(ctx, id)
	if err != nil {
		return nil, err
	}
	return mapCabinetToDomain(c), nil
}

func (r *CabinetRepositoryImpl) ListCabinets(ctx context.Context) ([]*domain.Cabinet, error) {
	queries := db.New(r.pool)
	listCabinets, err := queries.ListCabinets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list cabinets: %w", err)
	}

	cabinets := make([]*domain.Cabinet, len(listCabinets))
	for i, c := range listCabinets {
		cabinets[i] = mapCabinetToDomain(db.GetCabinetRow(c))
	}
	return cabinets, nil
}

func (r *CabinetRepositoryImpl) ReserveStockTx(ctx context.Context, cabinetID string, quantity int, idempotencyKey string) (*domain.Cabinet, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := db.New(tx)

	//1. Check idempotency: if the key was already used, returns the reservation
	existingRes, err := qtx.GetReservation(ctx, idempotencyKey)
	if err == nil {
		if existingRes.CabinetID != cabinetID || int(existingRes.Quantity) != quantity {
			return nil, domain.ErrDuplicateReservation
		}
		c, err := qtx.GetCabinet(ctx, existingRes.CabinetID)
		if err != nil {
			return nil, fmt.Errorf("failed to get cabinet for existing reservation: %w", err)
		}
		_ = tx.Commit(ctx)
		return mapCabinetToDomain(c), nil
	}
	//2. Try to decrement the stock
	c, err := qtx.DecrementStock(ctx, db.DecrementStockParams{
		ID:       cabinetID,
		Quantity: int32(quantity),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrInsufficientStock
		}
		return nil, fmt.Errorf("failed to decrement stock: %w", err)
	}

	//3. Save the idempotency key on the table catalog.reservations
	err = qtx.CreateReservation(ctx, db.CreateReservationParams{
		IdempotencyKey: idempotencyKey,
		CabinetID:      cabinetID,
		Quantity:       int32(quantity),
	})

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrDuplicateReservation
		}
		return nil, fmt.Errorf("failed to create reservation: %w", err)
	}

	//4. Commit the transaction
	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return mapCabinetToDomain(db.GetCabinetRow(c)), nil
}

func mapCabinetToDomain(c db.GetCabinetRow) *domain.Cabinet {
	return &domain.Cabinet{
		ID:           c.ID,
		Slug:         c.Slug,
		Model:        c.Model,
		Type:         c.Type,
		Display:      c.Display,
		Condition:    c.Condition,
		Quantity:     int(c.Quantity),
		Price:        c.Price,
		Manufacturer: c.Manufacturer,
		Year:         c.Year,
		Images:       mapImagesToDomain(c.Images),
		CreatedAt:    c.CreatedAt.Time,
		UpdatedAt:    c.UpdatedAt.Time,
	}
}

func mapImagesToDomain(images db.CabinetImages) []domain.CabinetImage {
	if len(images) == 0 {
		return []domain.CabinetImage{}
	}
	out := make([]domain.CabinetImage, len(images))
	for i, img := range images {
		out[i] = domain.CabinetImage{
			ID:        img.ID,
			URL:       img.URL,
			Alt:       img.Alt,
			SortOrder: int(img.SortOrder),
		}
	}
	return out
}
