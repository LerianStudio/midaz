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

// CreateTransaction creates a new transaction record from a parsed Gold DSL transaction.
//
// This function creates the transaction header record in PostgreSQL along with optional
// metadata in MongoDB. It does NOT update balances or create operations - those are
// handled by the async processing pipeline (CreateBalanceTransactionOperationsAsync).
//
// The transaction is created in APPROVED status and stores:
// - The complete Gold DSL body for audit and replay purposes
// - Core transaction attributes (amount, asset code, description)
// - Parent transaction reference (for reversals/child transactions)
// - Chart of accounts group reference
//
// This function is typically called as part of a larger transaction creation flow
// where balance validation and updates happen separately.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: Organization UUID owning the transaction
//   - ledgerID: Ledger UUID containing the transaction
//   - transactionID: Parent transaction UUID (or uuid.Nil if no parent)
//   - t: Parsed Gold DSL transaction structure with all transaction details
//
// Returns:
//   - *transaction.Transaction: The created transaction header record
//   - error: Persistence or metadata validation errors
func (uc *UseCase) CreateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, t *libTransaction.Transaction) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction")
	defer span.End()

	logger.Infof("Trying to create new transaction")

	// All transactions start in APPROVED status (balance updates handled elsewhere)
	description := constant.APPROVED
	status := transaction.Status{
		Code:        description,
		Description: &description,
	}

	// Link to parent transaction if provided (used for reversals, corrections, etc.)
	var parentTransactionID *string

	if transactionID != uuid.Nil {
		value := transactionID.String()
		parentTransactionID = &value
	}

	// Construct transaction header with Gold DSL body preserved for audit
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

	// Persist transaction header to PostgreSQL
	tran, err := uc.TransactionRepo.Create(ctx, save)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction on repo", err)

		logger.Errorf("Error creating t: %v", err)

		return nil, err
	}

	// Store custom metadata in MongoDB if provided
	if t.Metadata != nil {
		if err := libCommons.CheckMetadataKeyAndValueLength(100, t.Metadata); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to check metadata key and value length", err)

			return nil, err
		}

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
