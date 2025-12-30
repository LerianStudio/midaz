package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// GetTransactionByID gets data in the repository.
func (uc *UseCase) GetTransactionByID(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(transactionID != uuid.Nil, "transactionID must not be nil UUID",
		"transactionID", transactionID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_by_id")
	defer span.End()

	logger.Infof("Trying to get transaction")

	tran, err := uc.TransactionRepo.Find(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get transaction on repo by id", err)
		logger.Errorf("Error getting transaction: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	if tran == nil {
		return nil, nil
	}

	if err := uc.enrichTransactionMetadata(ctx, &span, logger, tran, transactionID); err != nil {
		return nil, err
	}

	if len(tran.Operations) > 0 {
		if err := uc.enrichOperationsWithMetadata(ctx, &span, tran.Operations); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation metadata", err)
			logger.Warnf("Error getting operation metadata: %v", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(operation.Operation{}).Name())
		}
	}

	return tran, nil
}

// GetTransactionWithOperationsByID gets data in the repository.
func (uc *UseCase) GetTransactionWithOperationsByID(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(transactionID != uuid.Nil, "transactionID must not be nil UUID",
		"transactionID", transactionID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_and_operations_by_id")
	defer span.End()

	logger.Infof("Trying to get transaction")

	tran, err := uc.TransactionRepo.FindWithOperations(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction on repo by id", err)
		logger.Errorf("Error getting transaction: %v", err)

		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	if tran == nil {
		return nil, nil
	}

	if err := uc.enrichTransactionMetadata(ctx, &span, logger, tran, transactionID); err != nil {
		return nil, err
	}

	return tran, nil
}

// enrichTransactionMetadata fetches and applies metadata to a transaction.
func (uc *UseCase) enrichTransactionMetadata(ctx context.Context, span *trace.Span, logger libLog.Logger, tran *transaction.Transaction, transactionID uuid.UUID) error {
	metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), transactionID.String())
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get metadata on mongodb account", err)
		logger.Errorf("Error get metadata on mongodb account: %v", err)

		return pkg.ValidateInternalError(err, "Transaction")
	}

	tran.Metadata = extractMetadataData(metadata)
	if len(tran.Metadata) == 0 {
		if outboxMetadata, ok, outboxErr := uc.fetchMetadataFromOutbox(ctx, outbox.EntityTypeTransaction, transactionID.String()); outboxErr != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to fetch transaction metadata from outbox", outboxErr)
			logger.Warnf("Error fetching transaction metadata from outbox: %v", outboxErr)
		} else if ok {
			tran.Metadata = outboxMetadata
		}
	}
	backfillTransactionMetadataFromBody(tran)

	return nil
}

// extractMetadataData extracts metadata data or returns an empty map.
// Postcondition: always returns a non-nil map for safe iteration by callers.
func extractMetadataData(metadata *mongodb.Metadata) map[string]any {
	if metadata != nil && metadata.Data != nil {
		return metadata.Data
	}

	// Postcondition: ensure Metadata is never nil for safe iteration
	return map[string]any{}
}
