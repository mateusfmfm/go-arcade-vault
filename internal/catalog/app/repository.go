package app

import (
	"context"

	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/domain"
)

// OrderItemInput represents an item request input for creating an order
type OrderItemInput struct {
	CabinetID string
	Quantity  int
}

// CabinetRepository defines the database persistence port for catalog and order operations
type CabinetRepository interface {
	GetCabinet(ctx context.Context, id string) (*domain.Cabinet, error)
	ListCabinets(ctx context.Context) ([]*domain.Cabinet, error)
	ReserveStockTx(ctz context.Context, cabinetID string, quantity int, idempotencyKey string) (*domain.Cabinet, error)
	GetOrder(ctx context.Context, orderID string) (*domain.Order, error)
	CreateOrderTx(ctx context.Context, userID string, items []OrderItemInput, idempotencyKey string) (*domain.Order, error)
}
