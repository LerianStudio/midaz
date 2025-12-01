package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// UpdateTransaction updates a transaction's description and metadata.
//
// This function allows modification of transaction properties after creation.
// Only non-financial properties can be updated - the transaction amount,
// accounts, and status cannot be changed through this endpoint.
//
// Update Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Build Update Model
//	  - Map input description to Transaction model
//	  - Only description field is updatable
//
//	Step 3: Repository Update
//	  - Call TransactionRepo.Update with scoped IDs
//	  - Handle not-found scenarios with business error
//
//	Step 4: Metadata Update
//	  - Update associated metadata in MongoDB
//	  - Merge new metadata with existing data
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - transactionID: UUID of the transaction to update
//   - uti: Update payload with Description and optional Metadata
//
// Returns:
//   - *transaction.Transaction: Updated transaction with refreshed metadata
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrTransactionIDNotFound: Transaction with given ID does not exist
//   - Database errors: PostgreSQL or MongoDB unavailable
func (uc *UseCase) UpdateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, uti *transaction.UpdateTransactionInput) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction")
	defer span.End()

	logger.Infof("Trying to update transaction: %v", uti)

	trans := &transaction.Transaction{
		Description: uti.Description,
	}

	transUpdated, err := uc.TransactionRepo.Update(ctx, organizationID, ledgerID, transactionID, trans)
	if err != nil {
		logger.Errorf("Error updating transaction on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, reflect.TypeOf(transaction.Transaction{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update transaction on repo by id", err)

			logger.Warnf("Error updating transaction on repo by id: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update transaction on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), transactionID.String(), uti.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	transUpdated.Metadata = metadataUpdated

	return transUpdated, nil
}

// UpdateTransactionStatus updates only the status of a transaction.
//
// This internal function is used by the async processing pipeline to update
// transaction status after balance operations complete. It handles status
// transitions like CREATED -> APPROVED or PENDING -> CANCELED.
//
// Status Update Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Parse UUIDs from transaction string fields
//
//	Step 2: Repository Update
//	  - Call TransactionRepo.Update with status change
//	  - Handle not-found scenarios (shouldn't happen in normal flow)
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - tran: Transaction with updated status to persist
//
// Returns:
//   - *transaction.Transaction: Updated transaction
//   - error: Database or validation error
//
// Error Scenarios:
//   - ErrTransactionIDNotFound: Transaction doesn't exist (data integrity issue)
//   - Database errors: PostgreSQL unavailable
func (uc *UseCase) UpdateTransactionStatus(ctx context.Context, tran *transaction.Transaction) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_transaction_status")
	defer span.End()

	organizationID := uuid.MustParse(tran.OrganizationID)
	ledgerID := uuid.MustParse(tran.LedgerID)
	transactionID := uuid.MustParse(tran.ID)

	logger.Infof("Trying to update transaction using status: : %v", tran.Status.Description)

	updateTran, err := uc.TransactionRepo.Update(ctx, organizationID, ledgerID, transactionID, tran)
	if err != nil {
		logger.Errorf("Error updating status transaction on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrTransactionIDNotFound, reflect.TypeOf(transaction.Transaction{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update status transaction on repo by id", err)

			logger.Warnf("Error updating status transaction on repo by id: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update status transaction on repo by id", err)

		return nil, err
	}

	return updateTran, nil
}
