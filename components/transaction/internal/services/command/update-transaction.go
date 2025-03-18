package command

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// UpdateTransaction update a transaction from the repository by given id.
func (uc *UseCase) UpdateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, uti *transaction.UpdateTransactionInput) (*transaction.Transaction, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.update_transaction")
	defer span.End()

	// Record operation metrics
	uc.recordBusinessMetrics(ctx, "transaction_update_attempt",
		attribute.String("transaction_id", transactionID.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))

	logger.Infof("Trying to update transaction: %v", uti)

	trans := &transaction.Transaction{
		Description: uti.Description,
	}

	transUpdated, err := uc.TransactionRepo.Update(ctx, organizationID, ledgerID, transactionID, trans)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update transaction on repo by id", err)

		logger.Errorf("Error updating transaction on repo by id: %v", err)

		// Record error metric
		uc.recordTransactionError(ctx, "update_error",
			attribute.String("transaction_id", transactionID.String()),
			attribute.String("error_detail", err.Error()))

		// Record transaction duration with error status
		uc.recordTransactionDuration(ctx, startTime, "update", "error",
			attribute.String("transaction_id", transactionID.String()),
			attribute.String("error", err.Error()))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, reflect.TypeOf(transaction.Transaction{}).Name())
		}

		return nil, err
	}

	if len(uti.Metadata) > 0 {
		if err := pkg.CheckMetadataKeyAndValueLength(100, uti.Metadata); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to check metadata key and value length", err)

			// Record error metric
			uc.recordTransactionError(ctx, "metadata_validation_error",
				attribute.String("transaction_id", transactionID.String()),
				attribute.String("error_detail", err.Error()))

			// Record transaction duration with error status
			uc.recordTransactionDuration(ctx, startTime, "update", "error",
				attribute.String("transaction_id", transactionID.String()),
				attribute.String("error", "metadata_validation_failed"))

			return nil, err
		}

		err := uc.MetadataRepo.Update(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), transactionID.String(), uti.Metadata)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to update metadata on mongodb transaction", err)

			// Record error metric
			uc.recordTransactionError(ctx, "metadata_update_error",
				attribute.String("transaction_id", transactionID.String()),
				attribute.String("error_detail", err.Error()))

			// Record transaction duration with error status
			uc.recordTransactionDuration(ctx, startTime, "update", "error",
				attribute.String("transaction_id", transactionID.String()),
				attribute.String("error", "metadata_update_failed"))

			return nil, err
		}

		transUpdated.Metadata = uti.Metadata
	}

	// Record transaction duration with success status
	uc.recordTransactionDuration(ctx, startTime, "update", "success",
		attribute.String("transaction_id", transactionID.String()))

	// Record business metric for transaction update success
	uc.recordBusinessMetrics(ctx, "transaction_update_success",
		attribute.String("transaction_id", transactionID.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))

	return transUpdated, nil
}

// UpdateTransactionStatus update a status transaction from the repository by given id.
func (uc *UseCase) UpdateTransactionStatus(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, description string) (*transaction.Transaction, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.update_transaction_status")
	defer span.End()

	// Record operation metrics
	uc.recordBusinessMetrics(ctx, "transaction_status_update_attempt",
		attribute.String("transaction_id", transactionID.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("status", description))

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
		mopentelemetry.HandleSpanError(&span, "Failed to update status transaction on repo by id", err)

		logger.Errorf("Error updating status transaction on repo by id: %v", err)

		// Record error metric
		uc.recordTransactionError(ctx, "status_update_error",
			attribute.String("transaction_id", transactionID.String()),
			attribute.String("status", description),
			attribute.String("error_detail", err.Error()))

		// Record transaction duration with error status
		uc.recordTransactionDuration(ctx, startTime, "status_update", "error",
			attribute.String("transaction_id", transactionID.String()),
			attribute.String("status", description),
			attribute.String("error", err.Error()))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, reflect.TypeOf(transaction.Transaction{}).Name())
		}

		return nil, err
	}

	// Record transaction duration with success status
	uc.recordTransactionDuration(ctx, startTime, "status_update", "success",
		attribute.String("transaction_id", transactionID.String()),
		attribute.String("status", description))

	// Record business metric for transaction status update success
	uc.recordBusinessMetrics(ctx, "transaction_status_update_success",
		attribute.String("transaction_id", transactionID.String()),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("status", description))

	return nil, nil
}
