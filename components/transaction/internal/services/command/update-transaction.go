package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// UpdateTransaction update a transaction from the repository by given id.
func (uc *UseCase) UpdateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, uti *transaction.UpdateTransactionInput) (*transaction.Transaction, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a transaction operation telemetry entity
	op := uc.Telemetry.NewTransactionOperation("update", transactionID.String())

	// Add important attributes
	op.WithAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	logger.Infof("Trying to update transaction: %v", uti)

	trans := &transaction.Transaction{
		Description: uti.Description,
	}

	transUpdated, err := uc.TransactionRepo.Update(ctx, organizationID, ledgerID, transactionID, trans)
	if err != nil {
		// Record error
		op.RecordError(ctx, "update_error", err)

		logger.Errorf("Error updating transaction on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			// Handle not found error specially
			notFoundErr := pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, reflect.TypeOf(transaction.Transaction{}).Name())

			op.End(ctx, "failed")

			return nil, notFoundErr
		}

		op.End(ctx, "failed")

		return nil, err
	}

	if len(uti.Metadata) > 0 {
		if err := pkg.CheckMetadataKeyAndValueLength(100, uti.Metadata); err != nil {
			// Record metadata validation error
			op.RecordError(ctx, "metadata_validation_error", err)
			op.End(ctx, "failed")

			logger.Errorf("Error validating metadata: %v", err)

			return nil, err
		}

		err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), transactionID.String(), uti.Metadata)
		if err != nil {
			// Record metadata update error
			op.RecordError(ctx, "metadata_update_error", err)
			op.End(ctx, "failed")

			logger.Errorf("Error updating metadata: %v", err)

			return nil, err
		}

		transUpdated.Metadata = uti.Metadata
	}

	// Mark operation as successful
	op.End(ctx, "success")

	return transUpdated, nil
}

// UpdateTransactionStatus update a status transaction from the repository by given id.
func (uc *UseCase) UpdateTransactionStatus(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, description string) (*transaction.Transaction, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a transaction operation telemetry entity for status update
	op := uc.Telemetry.NewTransactionOperation("status_update", transactionID.String())

	// Add important attributes
	op.WithAttributes(
		attribute.String("status", description),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	logger.Infof("Trying to update transaction using status: : %v", description)

	status := transaction.Status{
		Code:        description,
		Description: &description,
	}

	trans := &transaction.Transaction{
		Status: status,
	}

	_, err := uc.TransactionRepo.Update(ctx, organizationID, ledgerID, transactionID, trans)
	if err != nil {
		// Record error
		op.RecordError(ctx, "status_update_error", err)

		logger.Errorf("Error updating status transaction on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			// Handle not found error specially
			notFoundErr := pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, reflect.TypeOf(transaction.Transaction{}).Name())

			op.End(ctx, "failed")

			return nil, notFoundErr
		}

		op.End(ctx, "failed")

		return nil, err
	}

	// Mark operation as successful
	op.End(ctx, "success")

	return nil, nil
}
