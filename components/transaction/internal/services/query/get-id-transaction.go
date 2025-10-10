// Package query implements read operations (queries) for the transaction service.
// This file contains queries for retrieving a transaction by its ID.
package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
)

// GetTransactionByID retrieves a single transaction by its ID, enriched with metadata.
//
// This use case fetches a transaction from PostgreSQL and its corresponding metadata
// from MongoDB.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - transactionID: The UUID of the transaction to retrieve.
//
// Returns:
//   - *transaction.Transaction: The transaction with its metadata.
//   - error: An error if the transaction is not found or if the retrieval fails.
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

// GetTransactionWithOperationsByID retrieves a transaction and its associated operations.
//
// This use case fetches a transaction and all of its operations in a single query
// from PostgreSQL, and then enriches the transaction with metadata from MongoDB.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - transactionID: The UUID of the transaction to retrieve.
//
// Returns:
//   - *transaction.Transaction: The transaction with its operations and metadata.
//   - error: An error if the transaction is not found or if the retrieval fails.
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
