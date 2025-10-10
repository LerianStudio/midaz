// Package query implements read operations (queries) for the transaction service.
// This file contains the query for retrieving an operation by its ID.
package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/google/uuid"
)

// GetOperationByID retrieves a single operation by its ID, enriched with metadata.
//
// This use case fetches an operation from PostgreSQL and its corresponding metadata
// from MongoDB.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - transactionID: The UUID of the parent transaction.
//   - operationID: The UUID of the operation to retrieve.
//
// Returns:
//   - *operation.Operation: The operation with its metadata.
//   - error: An error if the operation is not found or if the retrieval fails.
func (uc *UseCase) GetOperationByID(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID) (*operation.Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_by_id")
	defer span.End()

	logger.Infof("Trying to get operation")

	o, err := uc.OperationRepo.Find(ctx, organizationID, ledgerID, transactionID, operationID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get operation on repo by id", err)

		logger.Errorf("Error getting operation: %v", err)

		return nil, err
	}

	if o != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb operation", err)

			logger.Errorf("Error get metadata on mongodb operation: %v", err)

			return nil, err
		}

		if metadata != nil {
			o.Metadata = metadata.Data
		}
	}

	return o, nil
}
