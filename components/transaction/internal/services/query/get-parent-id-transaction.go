package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
)

// GetParentByTransactionID retrieves a transaction by its parent transaction ID.
//
// This method enables hierarchical transaction queries, finding transactions
// that are linked to a parent transaction. This is useful for transaction
// chains such as refunds, reversals, or multi-leg transactions.
//
// Query Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "query.get_parent_by_transaction_id"
//
//	Step 2: Transaction Retrieval
//	  - Query TransactionRepo.FindByParentID with organization, ledger, and parent ID
//	  - If retrieval fails: Return error with span event
//	  - Note: Returns nil transaction (not error) if no child transaction exists
//
//	Step 3: Metadata Enrichment
//	  - If transaction found: Query MongoDB for associated metadata
//	  - If metadata retrieval fails: Return error with span event
//	  - If metadata exists: Attach to transaction entity
//
//	Step 4: Response
//	  - Return enriched transaction with metadata (or nil if not found)
//
// Transaction Hierarchy:
//
// Transactions can form hierarchies through parent-child relationships:
//   - Original transaction (no parent)
//     - Refund transaction (parent = original)
//     - Reversal transaction (parent = original)
//     - Adjustment transaction (parent = original)
//
// This method finds the child transaction given a parent ID, enabling
// traversal down the transaction tree.
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the transaction
//   - parentID: UUID of the parent transaction to find children for
//
// Returns:
//   - *transaction.Transaction: Child transaction with metadata if found
//   - error: Infrastructure error (nil transaction is valid for no children)
//
// Error Scenarios:
//   - Database connection failure
//   - MongoDB metadata retrieval failure
func (uc *UseCase) GetParentByTransactionID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_parent_by_transaction_id")
	defer span.End()

	logger.Infof("Trying to get transaction")

	tran, err := uc.TransactionRepo.FindByParentID(ctx, organizationID, ledgerID, parentID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get parent transaction on repo by id", err)

		logger.Errorf("Error getting parent transaction: %v", err)

		return nil, err
	}

	if tran != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), tran.ID)
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
