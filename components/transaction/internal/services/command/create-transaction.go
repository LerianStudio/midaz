// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

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

// CreateTransaction creates a new transaction and persists it to the repository.
//
// This method implements the create transaction use case, which:
// 1. Generates a UUIDv7 for the transaction ID
// 2. Sets status to APPROVED (transactions are pre-validated)
// 3. Extracts transaction details from lib-commons Transaction struct
// 4. Persists transaction to PostgreSQL
// 5. Creates associated metadata in MongoDB
// 6. Returns the complete transaction with metadata
//
// Business Rules:
//   - Transactions are created in APPROVED status (validation happens before this)
//   - Parent transaction ID is optional (for multi-step transactions)
//   - Amount and asset code are extracted from the send specification
//   - Chart of accounts group name is required
//   - Metadata is optional (max 100 characters per key)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionID: UUID of parent transaction (uuid.Nil if none)
//   - t: lib-commons Transaction struct with send/distribute specifications
//
// Returns:
//   - *transaction.Transaction: Created transaction with metadata
//   - error: Business error if creation fails
//
// OpenTelemetry: Creates span "command.create_transaction"
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
