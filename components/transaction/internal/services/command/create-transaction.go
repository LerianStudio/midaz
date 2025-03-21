package command

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// CreateTransaction creates a new transaction persisting data in the repository.
func (uc *UseCase) CreateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, t *goldModel.Transaction) (*transaction.Transaction, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a transaction operation telemetry entity
	newTransactionID := pkg.GenerateUUIDv7().String()
	op := uc.Telemetry.NewTransactionOperation("create", newTransactionID)

	// Add organization and ledger attributes
	op.WithAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

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

		// Add parent transaction as an attribute
		op.WithAttribute("parent_transaction_id", value)
	}

	trans := &transaction.Transaction{
		ID:                  newTransactionID,
		OrganizationID:      organizationID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: parentTransactionID,
		Status:              status,
	}

	logger.Infof("Creating transaction %v for account %v", newTransactionID, organizationID.String())

	// Persist transaction to database
	createdTrans, err := uc.TransactionRepo.Create(ctx, trans)
	if err != nil {
		// Record error metric
		op.RecordError(ctx, "database_error", err)

		// End operation with failed status
		op.End(ctx, "failed")

		logger.Errorf("Error creating transaction: %v", err)

		return nil, err
	}

	// Add metadata if available
	if t.Metadata != nil {
		metadata := &mongodb.Metadata{
			EntityID:   newTransactionID,
			EntityName: reflect.TypeOf(transaction.Transaction{}).Name(),
			Data:       t.Metadata,
		}

		// Persist metadata
		err = uc.MetadataRepo.Create(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), metadata)
		if err != nil {
			// Record error metric for metadata creation
			op.RecordError(ctx, "metadata_error", err)

			// Still end with partial success since the transaction was created
			op.End(ctx, "partial_success")

			logger.Errorf("Error creating transaction metadata: %v", err)

			return createdTrans, err
		}

		createdTrans.Metadata = t.Metadata
	}

	// Record business metrics for the transaction amount if available
	if t.Send.Value != 0 {
		op.RecordBusinessMetric(ctx, "amount", float64(t.Send.Value))
	}

	// End operation with success status
	op.End(ctx, "success")

	return createdTrans, nil
}
