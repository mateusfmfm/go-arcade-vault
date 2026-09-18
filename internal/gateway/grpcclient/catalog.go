package grpcclient

import (
	"fmt"

	catalogv1 "github.com/mateusfmfm/go-arcade-vault/api/proto/catalog/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type CatalogClient struct {
	conn   *grpc.ClientConn
	Client catalogv1.CatalogServiceClient
}

func NewCatalogClient(addr string) (*CatalogClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	if err != nil {
		return nil, fmt.Errorf("dial catalog client: %w", err)
	}
	return &CatalogClient{
		conn:   conn,
		Client: catalogv1.NewCatalogServiceClient(conn),
	}, nil
}

func (c *CatalogClient) Close() error {
	return c.conn.Close()
}
