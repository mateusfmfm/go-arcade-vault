package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/adapter/postgres/db"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/app"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/domain"
)

// OrderCreatedPayload defines the JSON contract for the outbox event payload
type OrderCreatedPayload struct {
	EventID     string `json:"event_id"`
	OrderID     string `json:"order_id"`
	UserID      string `json:"user_id"`
	AmountCents int64  `json:"amount_cents"`
	OccurredAt  string `json:"occurred_at"`
}

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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get cabinet: %w", err)
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

	cabinet, err := r.ReserveStockOnTx(ctx, tx, cabinetID, quantity, idempotencyKey)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return cabinet, nil
}

func (r *CabinetRepositoryImpl) ReserveStockOnTx(ctx context.Context, tx pgx.Tx, cabinetID string, quantity int, idempotencyKey string) (*domain.Cabinet, error) {
	qtx := db.New(tx)

	//1. Check stock idempotency: if the key was already used, returns the reservation
	existingRes, err := qtx.GetReservation(ctx, idempotencyKey)
	if err == nil {
		if existingRes.CabinetID != cabinetID || int(existingRes.Quantity) != quantity {
			return nil, domain.ErrDuplicateReservation
		}
		c, err := qtx.GetCabinet(ctx, existingRes.CabinetID)
		if err != nil {
			return nil, fmt.Errorf("failed to get cabinet for existing reservation: %w", err)
		}
		return mapCabinetToDomain(c), nil
	}

	//2. Try to decrement the stock (UPDATE ... WHERE quantity >= @quantity)
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

	//3. Register the stock idempotency key on the table catalog.reservations
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

	return mapCabinetToDomain(db.GetCabinetRow(c)), nil

}

func (r *CabinetRepositoryImpl) GetOrder(ctx context.Context, id string) (*domain.Order, error) {
	queries := db.New(r.pool)
	o, err := queries.GetOrder(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	items, err := queries.ListOrderItems(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list order items: %w", err)
	}

	return mapOrderToDomain(o, items), nil
}

func (r *CabinetRepositoryImpl) CreateOrderTx(ctx context.Context, userID string, items []app.OrderItemInput, idempotencyKey string) (*domain.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := db.New(tx)

	// 1. Check if the order already exists using the order idempotency key
	existingOrder, err := qtx.GetOrderByItempotencyKey(ctx, idempotencyKey)
	if err == nil {
		// 2. Fetch existing items if order exists
		existingItems, err := qtx.ListOrderItems(ctx, existingOrder.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list existing order items: %w", err)
		}

		// 3. Check if userID and items count match
		if existingOrder.UserID != userID || len(existingItems) != len(items) {
			return nil, domain.ErrDuplicateOrder
		}

		// 4. Map existing items for O(1) comparison
		itemMap := make(map[string]int, len(existingItems))
		for _, item := range existingItems {
			itemMap[item.CabinetID] = int(item.Quantity)
		}

		// 5. Verify if each item in the new request matches cabinet_id and quantity exactly
		for _, newItem := range items {
			quantity, exists := itemMap[newItem.CabinetID]
			if !exists || quantity != newItem.Quantity {
				return nil, domain.ErrDuplicateOrder
			}
		}

		// 6. Idempotency match: return existing order without altering stock or writing new outbox
		return mapOrderToDomain(existingOrder, existingItems), nil
	}

	// 7. Fetch real cabinet price snapshots directly from database inside active transaction
	type itemSnapshot struct {
		cabinetID      string
		quantity       int
		unitPriceCents int64
	}

	snapshots := make([]itemSnapshot, len(items))
	var totalCents int64

	for i, item := range items {
		cabinetRow, err := qtx.GetCabinet(ctx, item.CabinetID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, domain.ErrNotFound
			}
			return nil, fmt.Errorf("failed to get cabinet snapshot while creating order: %w", err)
		}

		snapshots[i] = itemSnapshot{
			cabinetID:      item.CabinetID,
			quantity:       item.Quantity,
			unitPriceCents: cabinetRow.Price,
		}

		totalCents += cabinetRow.Price * int64(item.Quantity)
	}

	// 8. Generate domain IDs for the new Order Aggregate
	orderID := uuid.New().String()

	// 9. Reserve stock for each item within the SAME transaction (ReserveStockOnTx)
	for _, item := range snapshots {
		// Isolated stock idempotency key per order item line
		stockKey := fmt.Sprintf("order:%s:%s", orderID, item.cabinetID)
		_, err := r.ReserveStockOnTx(ctx, tx, item.cabinetID, item.quantity, stockKey)
		if err != nil {
			return nil, err // ErrInsufficientStock, etc.
		}
	}

	// 10. Persist order header (catalog.orders) with status "pending_payment"
	createdOrderRow, err := qtx.CreateOrder(ctx, db.CreateOrderParams{
		ID:             orderID,
		UserID:         userID,
		Status:         string(domain.StatusPendingPayment),
		TotalCents:     totalCents,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return nil, domain.ErrDuplicateOrder
		}
		return nil, fmt.Errorf("failed to insert order: %w", err)
	}

	// 11. Persist order item lines (catalog.order_items)
	dbOrderItems := make([]db.CatalogOrderItem, len(snapshots))
	for i, item := range snapshots {
		itemID := uuid.New().String()
		err := qtx.CreateOrderItem(ctx, db.CreateOrderItemParams{
			ID:             itemID,
			OrderID:        orderID,
			CabinetID:      item.cabinetID,
			Quantity:       int32(item.quantity),
			UnitPriceCents: item.unitPriceCents,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to insert order item line: %w", err)
		}

		dbOrderItems[i] = db.CatalogOrderItem{
			ID:             itemID,
			OrderID:        orderID,
			CabinetID:      item.cabinetID,
			Quantity:       int32(item.quantity),
			UnitPriceCents: item.unitPriceCents,
		}
	}

	// 12. Write order.created event payload to the Outbox table (catalog.outbox)
	eventID := uuid.New().String()
	now := time.Now().UTC()

	payloadStruct := OrderCreatedPayload{
		EventID:     eventID,
		OrderID:     orderID,
		UserID:      userID,
		AmountCents: totalCents,
		OccurredAt:  now.Format(time.RFC3339),
	}

	payloadBytes, err := json.Marshal(payloadStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal outbox event payload: %w", err)
	}

	err = qtx.InsertOutboxEvent(ctx, db.InsertOutboxEventParams{
		ID:          eventID,
		AggregateID: orderID,
		EventType:   "order.created",
		Payload:     payloadBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to insert outbox event: %w", err)
	}

	// 13. Commit the single PostgreSQL transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit order creation transaction: %w", err)
	}

	return mapOrderToDomain(createdOrderRow, dbOrderItems), nil
}

func (r *CabinetRepositoryImpl) ListUnpublishedOutboxEvents(ctx context.Context, limit int32) ([]*domain.OutboxEvent, error) {
	return nil, nil
}

func (r *CabinetRepositoryImpl) MarkOutboxEventPublished(ctx context.Context, id string) error {
	return nil
}

func mapOrderToDomain(o db.CatalogOrder, items []db.CatalogOrderItem) *domain.Order {
	orderLines := make([]domain.OrderLine, len(items))
	for i, item := range items {
		orderLines[i] = domain.OrderLine{
			CabinetID: item.CabinetID,
			Quantity:  int(item.Quantity),
		}
	}
	return &domain.Order{
		ID:             o.ID,
		UserID:         o.UserID,
		Status:         domain.OrderStatus(o.Status),
		TotalCents:     o.TotalCents,
		IdempotencyKey: o.IdempotencyKey,
		Items:          orderLines,
		CreatedAt:      o.CreatedAt.Time,
		UpdatedAt:      o.UpdatedAt.Time,
	}
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
