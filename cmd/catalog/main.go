package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mateusfmfm/go-arcade-vault/pkg/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

const serviceName = "catalog"

func main() {
	slog.SetDefault(telemetry.NewLogger())

	grpcAddr := envOrDefault("CATALOG_GRPC_ADDR", ":50051")
	collectorAddr := envOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")

	slog.Info("initializing catalog service", "addr", grpcAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdownTracer, err := telemetry.InitTracer(ctx, serviceName, collectorAddr)
	if err != nil {
		slog.Warn("failed to initialize tracer exporter (Jaeger down?), continuing without tracing", "error", err)
	} else {
		defer func() {
			if err := shutdownTracer(context.Background()); err != nil {
				slog.Error("failed to shutdown tracer provider", "error", err)
			}
		}()
	}

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		slog.Error("failed to listen", "addr", grpcAddr, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(telemetry.UnarySlogInterceptor()),
	)

	healthServer := health.NewServer()
	healthServer.SetServingStatus(serviceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	reflection.Register(grpcServer)

	go func() {
		slog.Info("gRPC server listening", "address", lis.Addr().String())
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("failed to serve gRPC", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down catalog service gracefully")
	grpcServer.GracefulStop()
	slog.Info("catalog service stopped")
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
