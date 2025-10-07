// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

package command

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

// UpdateOperation updates an existing operation in the repository.
//
// This method updates operation description and metadata. Only provided fields are updated.
//
// Business Rules:
//   - Only description and metadata can be updated
//   - Operation type, amount, and balance cannot be changed (immutable)
//   - Metadata is merged with existing (RFC 7396 JSON Merge Patch)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionID: UUID of the parent transaction
//   - operationID: UUID of the operation to update
//   - uoi: Update input with description and metadata
//
// Returns:
//   - *operation.Operation: Updated operation with merged metadata
//   - error: Business error if operation not found or update fails
//
// OpenTelemetry: Creates span "command.update_operation"
func (uc *UseCase) UpdateOperation(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID, uoi *operation.UpdateOperationInput) (*operation.Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_operation")
	defer span.End()

	logger.Infof("Trying to update operation: %v", uoi)

	op := &operation.Operation{
		Description: uoi.Description,
	}

	operationUpdated, err := uc.OperationRepo.Update(ctx, organizationID, ledgerID, transactionID, operationID, op)
	if err != nil {
		logger.Errorf("Error updating op on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrOperationIDNotFound, reflect.TypeOf(operation.Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation on repo by id", err)

			logger.Warnf("Error updating op on repo by id: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationID.String(), uoi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	operationUpdated.Metadata = metadataUpdated

	return operationUpdated, nil
}
