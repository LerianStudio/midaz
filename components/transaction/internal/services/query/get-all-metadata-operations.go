package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataOperations fetches all Operations from the repository by metadata
func (uc *UseCase) GetAllMetadataOperations(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*operation.Operation, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_operations")
	defer span.End()

	logger.Infof("Retrieving operations by metadata")

	// First, query metadata to get operation IDs
	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(operation.Operation{}).Name(), filter)
	if err != nil || metadata == nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get operations on repo by metadata", err)
		return nil, pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	// Then, fetch operations by their IDs
	operations, err := uc.OperationRepo.ListByIDs(ctx, organizationID, ledgerID, uuids)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get operations on repo by IDs", err)
		logger.Errorf("Error getting operations on repo by IDs: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())
		}

		return nil, err
	}

	// Populate operations with their metadata
	for i := range operations {
		if data, ok := metadataMap[operations[i].ID]; ok {
			operations[i].Metadata = data
		}
	}

	return operations, nil
}
