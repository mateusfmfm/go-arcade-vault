package grpc

import (
	"context"
	"errors"

	catalogv1 "github.com/mateusfmfm/go-arcade-vault/api/proto/catalog/v1"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/app"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Handler connects gRPC requests with application layer
type CabinetHandler struct {
	catalogv1.UnimplementedCatalogServiceServer
	usecase *app.CabinetUsecase
}

// NewCabinetHandler build the gRPC handler, injecting the usecase
func NewCabinetHandler(usecase *app.CabinetUsecase) *CabinetHandler {
	return &CabinetHandler{
		usecase: usecase,
	}
}

// ListCabinets lists all cabinets
func (h *CabinetHandler) ListCabinets(ctx context.Context, req *catalogv1.ListCabinetsRequest) (*catalogv1.ListCabinetsResponse, error) {
	cabinets, err := h.usecase.ListCabinets(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list cabinets: %v", err)
	}
	pbCabinets := make([]*catalogv1.Cabinet, len(cabinets))
	for i, c := range cabinets {
		pbCabinets[i] = mapCabinetToProto(c)
	}
	return &catalogv1.ListCabinetsResponse{
		Cabinets: pbCabinets,
	}, nil
}

// GetCabinet gets a cabinet by ID
func (h *CabinetHandler) GetCabinet(ctx context.Context, req *catalogv1.GetCabinetRequest) (*catalogv1.GetCabinetResponse, error) {
	cabinet, err := h.usecase.GetCabinet(ctx, req.Id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "cabinet not found: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to get cabinet: %v", err)
	}
	return &catalogv1.GetCabinetResponse{
		Cabinet: mapCabinetToProto(cabinet),
	}, nil
}

// ReserveStock executes a stock reservation
func (h *CabinetHandler) ReserveStock(ctx context.Context, req *catalogv1.ReserveStockRequest) (*catalogv1.ReserveStockResponse, error) {
	c, err := h.usecase.ReserveStockTx(ctx, req.CabinetId, int(req.Quantity), req.IdempotencyKey)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInsufficientStock):
			return nil, status.Error(codes.FailedPrecondition, "insufficient stock")
		case errors.Is(err, domain.ErrDuplicateReservation):
			return nil, status.Error(codes.AlreadyExists, "duplicate reservation")
		case errors.Is(err, domain.ErrNotFound):
			return nil, status.Error(codes.NotFound, "cabinet not found")
		default:
			return nil, status.Errorf(codes.Internal, "failed to reserve stock: %v", err)
		}
	}
	return &catalogv1.ReserveStockResponse{
		IdempotencyKey: req.IdempotencyKey,
		Cabinet:        mapCabinetToProto(c),
	}, nil
}

func mapCabinetToProto(c *domain.Cabinet) *catalogv1.Cabinet {
	pbImages := make([]*catalogv1.CabinetImage, len(c.Images))
	for i, img := range c.Images {
		pbImages[i] = &catalogv1.CabinetImage{
			Id:        img.ID,
			Url:       img.URL,
			Alt:       img.Alt,
			SortOrder: int32(img.SortOrder),
		}
	}

	return &catalogv1.Cabinet{
		Id:           c.ID,
		Slug:         c.Slug,
		Model:        c.Model,
		Type:         c.Type,
		Display:      c.Display,
		Condition:    c.Condition,
		Quantity:     int32(c.Quantity),
		Price:        c.Price,
		Manufacturer: c.Manufacturer,
		Year:         c.Year,
		Images:       pbImages,
	}
}
