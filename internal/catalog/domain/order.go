package domain

import (
	"errors"
	"time"
)

// Sentinel errors specific to order domain validation and state transitions
var (
	ErrInvalidQuantity = errors.New("order item quantity must be greater than zero")
	ErrEmptyOrder      = errors.New("order must contain at least one item")
	ErrDuplicateOrder  = errors.New("duplicate order request with different parameters with this idempotency key")
)

// OrderStatus represents the current state of an order in the lifecycle
type OrderStatus string

const (
	StatusPendingPayment OrderStatus = "pending_payment"
	StatusPaid           OrderStatus = "paid"
	StatusCancelled      OrderStatus = "cancelled"
	StatusFailed         OrderStatus = "failed"
)

// OrderLine represents a snapshot of an individual item within an order
type OrderLine struct {
	ID             string
	OrderID        string
	CabinetID      string
	Quantity       int
	UnitPriceCents int64
}

// Order represents the complete order aggregate root
type Order struct {
	ID             string
	UserID         string
	Status         OrderStatus
	TotalCents     int64
	IdempotencyKey string
	Items          []OrderLine
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// OutboxEvent represents a transactional outbox event to be dispatched asynchronously
type OutboxEvent struct {
	ID          string
	AggregateID string
	EventType   string
	Payload     []byte
	UpdatedAt   *time.Time
}
