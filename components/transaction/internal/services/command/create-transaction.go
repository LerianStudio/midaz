// Package command implements write operations (commands) for the transaction service.
// This file contains the command for creating a new transaction.
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
// This use case is responsible for:
// 1. Generating a UUIDv7 for the new transaction.
// 2. Setting the transaction status to "APPROVED" by default.
// 3. Persisting the transaction in the PostgreSQL database.
// 4. Storing any associated metadata in MongoDB.
// 5. Returning the newly created transaction, including its metadata.
//
// Business Rules:
//   - Transactions are created with an "APPROVED" status, as validation is
//     expected to occur before this step.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - transactionID: The UUID of the parent transaction, if any.
//   - t: The transaction data from the lib-commons library.
//
// Returns:
//   - *transaction.Transaction: The newly created transaction.
//   - error: An error if the creation fails.
func (uc *UseCase) CreateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, t *libTransaction.Transaction) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction")
	defer span.End()

	logger.Infof("Trying to create new transaction")

	// FIXME: The 'description' variable is redundant. The status can be
	// initialized directly using constant.APPROVED for both Code and Description.
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
