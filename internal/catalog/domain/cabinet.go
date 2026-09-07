package domain

import (
	"errors"
	"time"
)

// Domain errors
var (
	ErrNotFound             = errors.New("cabinet not found")
	ErrInsufficientStock    = errors.New("insufficient stock available")
	ErrDuplicateReservation = errors.New("duplicate reservationkey with different parameters")
)

// Cabinet entity
type Cabinet struct {
	ID           string
	Slug         string
	Model        string
	Type         string
	Display      string
	Condition    string
	Quantity     int
	Price        int64
	Manufacturer string
	Year         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Reservation entity represents the reservation of a cabinet
type Reservation struct {
	IdempotencyKey string
	CabinetID      string
	Quantity       int
	CreatedAt      time.Time
}
