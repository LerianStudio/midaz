package mbootstrap

import (
	"context"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

//go:generate mockgen --destination=metadata_index_mock.go --package=mbootstrap . MetadataIndexPort

// MetadataIndexPort defines the interface for metadata index operations.
// This is a transport-agnostic "port" that abstracts how the ledger module
// communicates with the transaction module for metadata index management.
//
// This interface is implemented by:
//   - transaction.UseCase: Direct implementation (unified ledger mode)
//
// The ledger module's HTTP handler depends on this port, allowing it to work
// with the transaction module's implementation without knowing the underlying details.
type MetadataIndexPort interface {
	// CreateMetadataIndex creates a new metadata index on a MongoDB collection.
	CreateMetadataIndex(ctx context.Context, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error)

	// GetAllMetadataIndexes retrieves all metadata indexes, optionally filtered by entity name.
	GetAllMetadataIndexes(ctx context.Context, filter http.QueryHeader) ([]*mmodel.MetadataIndex, error)

	// DeleteMetadataIndex removes a metadata index from a specific entity collection.
	DeleteMetadataIndex(ctx context.Context, entityName, indexName string) error
}
