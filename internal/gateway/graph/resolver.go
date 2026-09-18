package graph

import catalogv1 "github.com/mateusfmfm/go-arcade-vault/api/proto/catalog/v1"

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

type Resolver struct {
	Catalog catalogv1.CatalogServiceClient
}
