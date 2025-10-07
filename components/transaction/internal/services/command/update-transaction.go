// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

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

// UpdateTransaction updates an existing transaction in the repository.
//
// This method updates transaction description and metadata. Only provided fields are updated.
//
// Business Rules:
//   - Only description and metadata can be updated
//   - Transaction status, amount, and operations cannot be changed
//   - Metadata is merged with existing (RFC 7396 JSON Merge Patch)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionID: UUID of the transaction to update
//   - uti: Update input with description and metadata
//
// Returns:
//   - *transaction.Transaction: Updated transaction with merged metadata
//   - error: Business error if transaction not found or update fails
//
// OpenTelemetry: Creates span "command.update_transaction"
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
// This method is used internally to change transaction status during processing.
// It updates only the status field, leaving other fields unchanged.
//
// Transaction Status Flow:
//   - CREATED → APPROVED (after validation)
//   - CREATED → CANCELED (if validation fails)
//   - PENDING → APPROVED (after async processing)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - tran: Transaction with updated status
//
// Returns:
//   - *transaction.Transaction: Updated transaction
//   - error: Business error if transaction not found or update fails
//
// OpenTelemetry: Creates span "command.update_transaction_status"
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
