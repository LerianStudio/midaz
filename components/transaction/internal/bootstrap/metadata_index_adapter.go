package bootstrap

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// Compile-time interface verification.
var _ mbootstrap.MetadataIndexPort = (*MetadataIndexAdapter)(nil)

// MetadataIndexAdapter combines command and query UseCases to implement MetadataIndexPort.
// This adapter is necessary because metadata index operations are split between
// command (Create, Delete) and query (GetAll) UseCases.
type MetadataIndexAdapter struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// NewMetadataIndexAdapter creates a new MetadataIndexAdapter.
func NewMetadataIndexAdapter(cmd *command.UseCase, qry *query.UseCase) *MetadataIndexAdapter {
	return &MetadataIndexAdapter{
		Command: cmd,
		Query:   qry,
	}
}

// CreateMetadataIndex creates a new metadata index on a MongoDB collection.
func (a *MetadataIndexAdapter) CreateMetadataIndex(ctx context.Context, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
	result, err := a.Command.CreateMetadataIndex(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata index: %w", err)
	}

	return result, nil
}

// GetAllMetadataIndexes retrieves all metadata indexes, optionally filtered by entity name.
func (a *MetadataIndexAdapter) GetAllMetadataIndexes(ctx context.Context, filter http.QueryHeader) ([]*mmodel.MetadataIndex, error) {
	result, err := a.Query.GetAllMetadataIndexes(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get all metadata indexes: %w", err)
	}

	return result, nil
}

// DeleteMetadataIndex removes a metadata index from a specific entity collection.
func (a *MetadataIndexAdapter) DeleteMetadataIndex(ctx context.Context, entityName, indexName string) error {
	if err := a.Command.DeleteMetadataIndex(ctx, entityName, indexName); err != nil {
		return fmt.Errorf("failed to delete metadata index: %w", err)
	}

	return nil
}
