package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
)

// GetTransactionByID gets data in the repository.
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

// GetTransactionWithOperationsByID gets data in the repository.
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

// GetTransactionByIDWithFallback retrieves a transaction from replica with primary fallback.
// This method is provided for future operations (e.g., account balance queries) that may need
// to read-then-write with replica lag tolerance. Currently used by metadata enrichment only.
// See the design document: docs/plans/2026-02-08-primary-read-fallback-pattern-design.md
func (uc *UseCase) GetTransactionByIDWithFallback(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_by_id_with_fallback")
	defer span.End()

	logger.Infof("Trying to get transaction with fallback")

	// Use repository fallback method
	tran, err := uc.TransactionRepo.FindWithFallback(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction with fallback", err)
		logger.Errorf("Error getting transaction with fallback: %v", err)
		return nil, err
	}

	// Fetch metadata (same as GetTransactionByID)
	if tran != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), transactionID.String())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb", err)
			logger.Errorf("Error getting metadata: %v", err)
			return nil, err
		}

		if metadata != nil {
			tran.Metadata = metadata.Data
		}
	}

	return tran, nil
}

// GetTransactionWithOperationsByIDWithFallback gets transaction with operations from replica with primary fallback.
func (uc *UseCase) GetTransactionWithOperationsByIDWithFallback(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_and_operations_by_id_with_fallback")
	defer span.End()

	logger.Infof("Trying to get transaction with operations with fallback")

	// Use repository fallback method
	tran, err := uc.TransactionRepo.FindWithOperationsWithFallback(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction with operations with fallback", err)
		logger.Errorf("Error getting transaction with operations with fallback: %v", err)
		return nil, err
	}

	// Fetch metadata (same as GetTransactionWithOperationsByID)
	if tran != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), transactionID.String())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb", err)
			logger.Errorf("Error getting metadata: %v", err)
			return nil, err
		}

		if metadata != nil {
			tran.Metadata = metadata.Data
		}
	}

	return tran, nil
}
