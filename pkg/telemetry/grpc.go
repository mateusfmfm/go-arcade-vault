package telemetry

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
)

func UnarySlogInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		slog.InfoContext(ctx, "grpc request", "grpc.method", info.FullMethod)
		resp, err := handler(ctx, req)
		if err != nil {
			slog.ErrorContext(ctx, "grpc request failed", "grpc.method", info.FullMethod, "error", err)
		}
		return resp, err
	}
}
