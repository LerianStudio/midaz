// Package query implements read operations (queries) for the transaction service.
// This file contains query implementation.

package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
)

// GetTransactionByID retrieves a single transaction by ID with metadata.
//
// Fetches transaction from PostgreSQL and enriches with MongoDB metadata.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionID: UUID of the transaction to retrieve
//
// Returns:
//   - *transaction.Transaction: Transaction with metadata
//   - error: Business error if not found or query fails
//
// OpenTelemetry: Creates span "query.get_transaction_by_id"
func (uc *UseCase) GetTransactionByID(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_by_id")
	defer span.End()

	logger.Infof("Trying to get transaction")

	tran, err := uc.TransactionRepo.Find(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get transaction on repo by id", err)

		logger.Errorf("Error getting transaction: %v", err)

		return nil, err
	}

	if tran != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), transactionID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb account", err)

			logger.Errorf("Error get metadata on mongodb account: %v", err)

			return nil, err
		}

		if metadata != nil {
			tran.Metadata = metadata.Data
		}
	}

	return tran, nil
}

// GetTransactionWithOperationsByID retrieves a transaction with its operations and metadata.
//
// Fetches transaction with operations from PostgreSQL and enriches with MongoDB metadata.
// This is more efficient than separate queries for transaction and operations.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionID: UUID of the transaction to retrieve
//
// Returns:
//   - *transaction.Transaction: Transaction with operations and metadata
//   - error: Business error if not found or query fails
//
// OpenTelemetry: Creates span "query.get_transaction_and_operations_by_id"
func (uc *UseCase) GetTransactionWithOperationsByID(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_and_operations_by_id")
	defer span.End()

	logger.Infof("Trying to get transaction")

	tran, err := uc.TransactionRepo.FindWithOperations(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction on repo by id", err)

		logger.Errorf("Error getting transaction: %v", err)

		return nil, err
	}

	if tran != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), transactionID.String())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb account", err)

			logger.Errorf("Error get metadata on mongodb account: %v", err)

			return nil, err
		}

		if metadata != nil {
			tran.Metadata = metadata.Data
		}
	}

	return tran, nil
}
