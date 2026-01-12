package mbootstrap

import (
	"context"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

//go:generate mockgen --destination=metadata-index-repo_mock.go --package=mbootstrap . MetadataIndexRepository

// MetadataIndexRepository defines the interface for metadata index operations on MongoDB.
// This interface is implemented by the MongoDB metadata repositories in both
// onboarding and transaction components, allowing direct access in unified ledger mode.
//
// Implementations:
//   - onboarding/internal/adapters/mongodb.MetadataMongoDBRepository
//   - transaction/internal/adapters/mongodb.MetadataMongoDBRepository
type MetadataIndexRepository interface {
	// CreateIndex creates a new index on a metadata field in the specified collection.
	// The collection parameter is the entity name (e.g., "transaction", "account").
	CreateIndex(ctx context.Context, collection string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error)

	// FindAllIndexes retrieves all indexes from the specified collection.
	// Returns metadata indexes with their configuration (unique, sparse, etc.).
	FindAllIndexes(ctx context.Context, collection string) ([]*mmodel.MetadataIndex, error)

	// DeleteIndex removes an index from the specified collection.
	// The indexName should be the full MongoDB index name (e.g., "metadata.tier_1").
	DeleteIndex(ctx context.Context, collection, indexName string) error
}
