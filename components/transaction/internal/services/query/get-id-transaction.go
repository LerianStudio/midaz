package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
)

// GetTransactionByID retrieves a transaction by its unique identifier.
//
// This method fetches a single transaction without its associated operations.
// For scenarios requiring the full transaction with operations, use
// GetTransactionWithOperationsByID instead.
//
// Query Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "query.get_transaction_by_id"
//
//	Step 2: Transaction Retrieval
//	  - Query TransactionRepo.Find with organization, ledger, and transaction IDs
//	  - If retrieval fails: Return error with span event
//	  - Note: Returns nil transaction (not error) if not found
//
//	Step 3: Metadata Enrichment
//	  - If transaction found: Query MongoDB for associated metadata
//	  - If metadata retrieval fails: Return error with span event
//	  - If metadata exists: Attach to transaction entity
//
//	Step 4: Response
//	  - Return enriched transaction with metadata (or nil if not found)
//
// Transaction Structure:
//
// A transaction represents a complete ledger entry that must balance:
//   - Total debits must equal total credits
//   - Contains one or more operations (retrieved separately)
//   - Has status lifecycle: PENDING -> COMMITTED or FAILED
//   - May reference a parent transaction for chains
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the transaction
//   - transactionID: UUID of the transaction to retrieve
//
// Returns:
//   - *transaction.Transaction: Transaction with metadata if found
//   - error: Infrastructure error (nil transaction is valid for not found)
//
// Error Scenarios:
//   - Database connection failure
//   - MongoDB metadata retrieval failure
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

// GetTransactionWithOperationsByID retrieves a transaction with all its operations.
//
// This method fetches a complete transaction including all associated operations
// in a single query. This is more efficient than fetching the transaction and
// operations separately when both are needed.
//
// Query Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "query.get_transaction_and_operations_by_id"
//
//	Step 2: Transaction with Operations Retrieval
//	  - Query TransactionRepo.FindWithOperations with organization, ledger, and transaction IDs
//	  - Returns transaction with Operations slice populated
//	  - If retrieval fails: Return error with span event
//
//	Step 3: Metadata Enrichment
//	  - If transaction found: Query MongoDB for associated metadata
//	  - If metadata retrieval fails: Return error with span event
//	  - If metadata exists: Attach to transaction entity
//	  - Note: Operation metadata is NOT fetched (use GetAllOperations for that)
//
//	Step 4: Response
//	  - Return enriched transaction with operations and metadata
//
// Use Cases:
//
// Use this method when you need:
//   - Transaction details display with operation breakdown
//   - Transaction verification (checking all operations)
//   - Audit trail queries
//   - Transaction reversal preparation
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the transaction
//   - transactionID: UUID of the transaction to retrieve
//
// Returns:
//   - *transaction.Transaction: Transaction with operations and metadata
//   - error: Infrastructure error
//
// Error Scenarios:
//   - Database connection failure
//   - MongoDB metadata retrieval failure
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
