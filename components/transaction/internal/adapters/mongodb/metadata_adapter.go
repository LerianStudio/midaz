package mongodb

import (
	"context"

	"github.com/google/uuid"
)

// MetadataRepositoryWrapper wraps a MongoDB Repository to implement the MetadataRepository interface
type MetadataRepositoryWrapper struct {
	repo       *MockRepository
	collection string
}

// NewMetadataRepositoryAdapter creates a new adapter for the MongoDB Repository
func NewMetadataRepositoryAdapter(repo *MockRepository, collection string) *MetadataRepositoryWrapper {
	return &MetadataRepositoryWrapper{
		repo:       repo,
		collection: collection,
	}
}

// Create implements the MetadataRepository interface
func (w *MetadataRepositoryWrapper) Create(ctx context.Context, metadata *Metadata) error {
	return w.repo.Create(ctx, w.collection, metadata)
}

// FindList implements the MetadataRepository interface
func (w *MetadataRepositoryWrapper) FindList(ctx context.Context, organizationID, ledgerID string, entityIDs []string) ([]*Metadata, error) {
	// This is a simplified implementation for the test
	// In a real implementation, you would create a proper filter
	return nil, nil
}

// Update implements the MetadataRepository interface
func (w *MetadataRepositoryWrapper) Update(ctx context.Context, organizationID, ledgerID, entityID string, metadata map[string]any) error {
	return w.repo.Update(ctx, w.collection, entityID, metadata)
}

// Delete implements the MetadataRepository interface
func (w *MetadataRepositoryWrapper) Delete(ctx context.Context, organizationID, ledgerID, entityID string) error {
	return w.repo.Delete(ctx, w.collection, entityID)
}

// FindByIdempotencyKey implements the MetadataRepository interface
func (w *MetadataRepositoryWrapper) FindByIdempotencyKey(ctx context.Context, organizationID, ledgerID, key string) (uuid.UUID, error) {
	// Simplified implementation for testing
	return uuid.Nil, nil
}
