package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// UpdateOperation update an operation from the repository by given id.
func (uc *UseCase) UpdateOperation(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID, uoi *operation.UpdateOperationInput) (*operation.Operation, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create an operation telemetry entity
	op := uc.Telemetry.NewOperationOperation("update", operationID.String())

	// Add important attributes
	op.WithAttributes(
		attribute.String("transaction_id", transactionID.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	logger.Infof("Trying to update operation: %v", uoi)

	oper := &operation.Operation{
		Description: uoi.Description,
	}

	operationUpdated, err := uc.OperationRepo.Update(ctx, organizationID, ledgerID, transactionID, operationID, oper)
	if err != nil {
		// Record error
		op.RecordError(ctx, "operation_update_error", err)

		logger.Errorf("Error updating op on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			// Handle not found error specially
			notFoundErr := pkg.ValidateBusinessError(constant.ErrOperationIDNotFound, reflect.TypeOf(operation.Operation{}).Name())

			op.End(ctx, "failed")

			return nil, notFoundErr
		}

		op.End(ctx, "failed")

		return nil, err
	}

	if len(uoi.Metadata) > 0 {
		if err := pkg.CheckMetadataKeyAndValueLength(100, uoi.Metadata); err != nil {
			// Record metadata validation error
			op.RecordError(ctx, "metadata_validation_error", err)
			op.End(ctx, "failed")

			logger.Errorf("Error validating metadata: %v", err)

			return nil, err
		}

		err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationID.String(), uoi.Metadata)
		if err != nil {
			// Record metadata update error
			op.RecordError(ctx, "metadata_update_error", err)
			op.End(ctx, "failed")

			logger.Errorf("Error updating metadata: %v", err)

			return nil, err
		}

		operationUpdated.Metadata = uoi.Metadata
	}

	// Mark operation as successful
	op.End(ctx, "success")

	return operationUpdated, nil
}
