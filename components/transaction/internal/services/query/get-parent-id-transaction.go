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

// GetParentByTransactionID retrieves a parent transaction by parent ID with metadata.
//
// Fetches transaction from PostgreSQL using parent_transaction_id, then enriches with
// MongoDB metadata. Used for retrieving the original transaction when querying child transactions.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - parentID: UUID of the parent transaction
//
// Returns:
//   - *transaction.Transaction: Parent transaction with metadata
//   - error: Error if not found or query fails
//
// OpenTelemetry: Creates span "query.get_parent_by_transaction_id"
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
