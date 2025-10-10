// Package command implements write operations (commands) for the transaction service.
// This file contains commands for updating a transaction.
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

// UpdateTransaction updates the description and metadata of an existing transaction.
//
// This use case handles partial updates, allowing clients to modify only the fields
// they provide. Metadata is merged with the existing data.
//
// Business Rules:
//   - The transaction must exist to be updated.
//   - Only the description and metadata are mutable.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - transactionID: The UUID of the transaction to update.
//   - uti: The input data containing the fields to update.
//
// Returns:
//   - *transaction.Transaction: The updated transaction, including merged metadata.
//   - error: An error if the transaction is not found or if the update fails.
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

// UpdateTransactionStatus updates the status of a transaction.
//
// This internal use case is responsible for transitioning a transaction's status
// as it moves through its lifecycle (e.g., from PENDING to APPROVED).
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - tran: The transaction with the updated status.
//
// Returns:
//   - *transaction.Transaction: The updated transaction.
//   - error: An error if the transaction is not found or if the update fails.
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
