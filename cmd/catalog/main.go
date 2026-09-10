package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	catalogv1 "github.com/mateusfmfm/go-arcade-vault/api/proto/catalog/v1"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/adapter/grpc"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/adapter/postgres"
	"github.com/mateusfmfm/go-arcade-vault/internal/catalog/app"
	"github.com/mateusfmfm/go-arcade-vault/pkg/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	grpcserver "google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

const (
	defaultGRPCPort  = ":50051"
	defaultCollector = "localhost:4317"
	serviceName      = "catalog"
	defaultDbURL     = "postgres://arcade_user:arcade_password@localhost:5432/arcade_db?sslmode=disable&search_path=catalog"
)

func main() {
	//1. Config Logger JSON (slog)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	grpcPort := os.Getenv("CATALOG_GRPC_ADDR")
	if grpcPort == "" {
		grpcPort = defaultGRPCPort
	}
	collectAddr := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if collectAddr == "" {
		collectAddr = defaultCollector
	}

	slog.Info("starting catalog service", "port", grpcPort)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	//2, Tracing initialization (OpenTelemetry -> Jaeger)
	shutdownTracer, err := telemetry.InitTracer(ctx, serviceName, collectAddr)
	if err != nil {
		slog.Warn("failed to initialize tracer exporter, continuing without tracing", "error", err)
	} else {
		defer func() {
			if err := shutdownTracer(context.Background()); err != nil {
				slog.Error("failed to shutdown tracer provider", "error", err)
			}
		}()
	}

	//3. Postgres connection pool (pgx/v5)
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = defaultDbURL
	}

	if err := postgres.RunMigrations(dbURL); err != nil {
		slog.Error("failed to run database migrations", "error", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("unable to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	seedCabinets(ctx, pool)

	//4. Dependency Injection (GRPC)
	repo := postgres.NewRepositoryImpl(pool)
	useCase := app.NewCabinetUsecase(repo)
	catalogHandler := grpc.NewCabinetHandler(useCase)

	//5. Listener TCP
	lis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		slog.Error("failed to listen", "port", grpcPort, "error", err)
		os.Exit(1)
	}

	//6. gRPC Server with OTel interceptors
	server := grpcserver.NewServer(
		grpcserver.StatsHandler(otelgrpc.NewServerHandler()),
	)

	// Service Registration
	catalogv1.RegisterCatalogServiceServer(server, catalogHandler)

	healthServer := health.NewServer()
	healthServer.SetServingStatus(serviceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, healthServer)

	// Enable Reflection for grpcurl
	reflection.Register(server)

	// 7. Shutdown
	go func() {
		slog.Info("gRPC server listening", "address", lis.Addr().String())
		if err := server.Serve(lis); err != nil {
			slog.Error("failed to serve gRPC", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down catalog service...")
	server.GracefulStop()
	slog.Info("catalog service stopped")

}

// Seed com máquinas lendárias de arcade para testes e demonstrações
func seedCabinets(ctx context.Context, pool *pgxpool.Pool) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		slog.Warn("failed to begin seed transaction", "error", err)
		return
	}
	defer tx.Rollback(ctx)

	cabinetQuery := `
		INSERT INTO catalog.cabinets (id, slug, model, type, display, condition, quantity, price, manufacturer, year)
		VALUES
			('cab-mk2-01', 'mortal-kombat-2', 'Mortal Kombat II', 'upright', 'crt', 'Restored Mint', 1, 320000, 'Midway', '1993'),
			('cab-tmnt-02', 'turtles-in-time', 'Teenage Mutant Ninja Turtles: Turtles in Time', 'upright', 'crt', 'Restored Mint', 1, 580000, 'Konami', '1991'),
			('cab-cruisn-03', 'cruisn-usa', 'Cruis''n USA', 'sit-down', 'crt', 'Fully Serviced', 1, 450000, 'Midway', '1994'),
			('cab-cftbl-04', 'creature-from-the-black-lagoon', 'Creature from the Black Lagoon', 'pinball', 'dmd', 'Fully Serviced', 1, 760000, 'Bally', '1992')
		ON CONFLICT (id) DO NOTHING;
	`
	_, err = tx.Exec(ctx, cabinetQuery)
	if err != nil {
		slog.Warn("failed to seed initial cabinets", "error", err)
		return
	}

	imageQuery := `
		INSERT INTO catalog.cabinet_images (id, cabinet_id, url, alt, sort_order)
		VALUES
			('img-mk2-1', 'cab-mk2-01', 'https://m.media-amazon.com/images/M/MV5BMWYzMmU3MmEtZDk0YS00NDBhLWIyNTItM2EwOGU5NWJkZDRjXkEyXkFqcGc@._V1_FMjpg_UX1000_.jpg', 'Mortal Kombat II dedicated upright cabinet with Raiden side art', 1),
			('img-mk2-2', 'cab-mk2-01', 'https://www.classicarcadecabinets.com/uploads/4/9/8/2/49822065/mk2_2_orig.png', 'Mortal Kombat II control panel with Midway branding', 2),
			('img-tmnt-1', 'cab-tmnt-02', 'https://i.ebayimg.com/images/g/wPwAAOSw33Bm2N19/s-l1200.png', 'Teenage Mutant Ninja Turtles Turtles in Time 4-player cabinet', 1),
			('img-tmnt-2', 'cab-tmnt-02', 'https://www.szabosarcades.com/cdn/shop/products/old_side_art_mockup_1080x.jpg?v=1595674856', 'Turtles in Time full side art with lime green T-molding', 2),
			('img-cruisn-1', 'cab-cruisn-03', 'https://freebie.games/wp-content/uploads/2023/08/Cruisn-USA.png', 'Cruis''n USA marquee artwork with red sports car', 1),
			('img-cruisn-2', 'cab-cruisn-03', 'https://encrypted-tbn0.gstatic.com/images?q=tbn:ANd9GcTRdky7oL-7n5KNwsQ5Djv_qPea42yj8yF8GUtbgyHN3lTcsNqaqjEUfAU&s=10', 'Cruis''n USA sit-down racing cabinet with steering wheel', 2),
			('img-cftbl-1', 'cab-cftbl-04', 'https://www.cometpinball.com/cdn/shop/products/cftbl2-2.jpg?v=1585940020', 'Creature from the Black Lagoon backglass by Bally', 1),
			('img-cftbl-2', 'cab-cftbl-04', 'https://i.redd.it/creature-from-the-black-lagoon-pinball-machine-i-think-im-v0-1qs5ljaw8zhg1.jpg?width=2304&format=pjpg&auto=webp&s=bac1134663c5ac489fb89b83cc9939c588c42afb', 'Creature from the Black Lagoon playfield with hologram window', 2)
		ON CONFLICT (id) DO NOTHING;
	`
	_, _ = tx.Exec(ctx, imageQuery)

	_ = tx.Commit(ctx)
}
