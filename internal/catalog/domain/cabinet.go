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
	Images       []CabinetImage
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CabinetImage is a photo of a cabinet, ordered for display.
type CabinetImage struct {
	ID        string
	URL       string
	Alt       string
	SortOrder int
}

// Reservation entity represents the reservation of a cabinet
type Reservation struct {
	IdempotencyKey string
	CabinetID      string
	Quantity       int
	CreatedAt      time.Time
}
