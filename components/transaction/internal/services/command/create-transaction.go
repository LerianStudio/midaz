package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// CreateTransaction creates a new transaction persisting data in the repository.
//
// This function processes a parsed Gold DSL transaction and persists it to the database.
// Transactions represent the fundamental unit of value movement in the ledger, implementing
// double-entry accounting principles where every debit has a corresponding credit.
//
// Process:
//  1. Extract tracing context for observability
//  2. Build transaction entity with APPROVED status
//  3. Link to parent transaction if this is a child (chained transactions)
//  4. Persist transaction to PostgreSQL via repository
//  5. Store optional metadata in MongoDB for extensibility
//  6. Return the created transaction with generated ID
//
// Transaction Hierarchy:
// Transactions can form parent-child relationships for complex financial operations:
//   - Parent transactions: Standalone or root of a chain
//   - Child transactions: Reference parent via ParentTransactionID
//   - Use cases: Split payments, refunds, reversals, compound operations
//
// Parameters:
//   - ctx: Request context with tracing, logging, and tenant information
//   - organizationID: Organization UUID for multi-tenant isolation
//   - ledgerID: Ledger UUID where the transaction is recorded
//   - transactionID: Parent transaction UUID (uuid.Nil for standalone transactions)
//   - t: Parsed transaction from Gold DSL containing send/receive operations
//
// Returns:
//   - *transaction.Transaction: Created transaction with generated UUIDv7 ID
//   - error: Repository errors, metadata persistence failures
//
// Note: The transaction is created with APPROVED status. The actual balance
// mutations are handled separately by the balance processing pipeline.
func (uc *UseCase) CreateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, t *libTransaction.Transaction) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction")
	defer span.End()

	logger.Infof("Trying to create new transaction")

	description := constant.APPROVED
	status := transaction.Status{
		Code:        description,
		Description: &description,
	}

	var parentTransactionID *string

	if transactionID != uuid.Nil {
		value := transactionID.String()
		parentTransactionID = &value
	}

	save := &transaction.Transaction{
		ID:                       libCommons.GenerateUUIDv7().String(),
		ParentTransactionID:      parentTransactionID,
		OrganizationID:           organizationID.String(),
		LedgerID:                 ledgerID.String(),
		Description:              t.Description,
		Status:                   status,
		Amount:                   &t.Send.Value,
		AssetCode:                t.Send.Asset,
		ChartOfAccountsGroupName: t.ChartOfAccountsGroupName,
		Body:                     *t,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}

	tran, err := uc.TransactionRepo.Create(ctx, save)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction on repo", err)

		logger.Errorf("Error creating t: %v", err)

		return nil, err
	}

	if t.Metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   tran.ID,
			EntityName: reflect.TypeOf(transaction.Transaction{}).Name(),
			Data:       t.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), &meta); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction metadata", err)

			logger.Errorf("Error into creating transactiont metadata: %v", err)

			return nil, err
		}

		tran.Metadata = t.Metadata
	}

	return tran, nil
}
