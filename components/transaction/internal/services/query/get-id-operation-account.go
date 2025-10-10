// Package query implements read operations (queries) for the transaction service.
// This file contains the query for retrieving a single operation for a specific account.
package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// GetOperationByAccount retrieves a single operation for a specific account, enriched with metadata.
//
// This use case fetches an operation from PostgreSQL, filtering by both account ID
// and operation ID, and then enriches it with metadata from MongoDB.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - accountID: The UUID of the account.
//   - operationID: The UUID of the operation to retrieve.
//
// Returns:
//   - *operation.Operation: The operation with its metadata.
//   - error: An error if the operation is not found or if the retrieval fails.
func (uc *UseCase) GetOperationByAccount(ctx context.Context, organizationID, ledgerID, accountID, operationID uuid.UUID) (*operation.Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_by_account")
	defer span.End()

	logger.Infof("Retrieving operation by account")

	op, err := uc.OperationRepo.FindByAccount(ctx, organizationID, ledgerID, accountID, operationID)
	if err != nil {
		logger.Errorf("Error getting operation on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation on repo by account", err)

			logger.Warnf("Error getting operation on repo: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation on repo by account", err)

		return nil, err
	}

	if op != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationID.String())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb operation", err)

			logger.Errorf("Error get metadata on mongodb operation: %v", err)

			return nil, err
		}

		if metadata != nil {
			op.Metadata = metadata.Data
		}
	}

	return op, nil
}
