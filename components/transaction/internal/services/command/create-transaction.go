package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// CreateTransaction creates a new transaction persisting data in the repository.
func (uc *UseCase) CreateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, t *goldModel.Transaction) (*transaction.Transaction, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start span for transaction creation
	ctx, span := tracer.Start(ctx, "command.create_transaction")
	defer span.End()

	// Record start time for duration metrics
	startTime := time.Now()

	logger.Infof("Trying to create new transaction")

	// Add detailed tracing for transaction initialization phase
	initPhaseStartTime := time.Now()

	description := constant.APPROVED
	status := transaction.Status{
		Code:        description,
		Description: &description,
	}

	var parentTransactionID *string

	if transactionID != uuid.Nil {
		value := transactionID.String()
		parentTransactionID = &value

		// Record parent transaction relationship
		uc.recordBusinessMetrics(ctx, "transaction_with_parent",
			attribute.String("parent_transaction_id", value),
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()))
	}

	newTransactionID := pkg.GenerateUUIDv7().String()

	save := &transaction.Transaction{
		ID:                       newTransactionID,
		ParentTransactionID:      parentTransactionID,
		OrganizationID:           organizationID.String(),
		LedgerID:                 ledgerID.String(),
		Description:              t.Description,
		Template:                 t.ChartOfAccountsGroupName,
		Status:                   status,
		Amount:                   &t.Send.Value,
		AmountScale:              &t.Send.Scale,
		AssetCode:                t.Send.Asset,
		ChartOfAccountsGroupName: t.ChartOfAccountsGroupName,
		Body:                     *t,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}

	// Record transaction data preparation duration
	uc.recordTransactionDuration(ctx, initPhaseStartTime, "transaction_preparation", "success",
		attribute.String("transaction_id", newTransactionID),
		attribute.String("asset_code", t.Send.Asset))

	// Expanded record business metric for transaction creation attempt
	uc.recordBusinessMetrics(ctx, "transaction_create_attempt",
		attribute.String("transaction_id", newTransactionID),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("asset_code", t.Send.Asset),
		attribute.String("template", t.ChartOfAccountsGroupName),
		attribute.Int64("amount", t.Send.Value),
		attribute.Int64("scale", t.Send.Scale),
		attribute.Bool("has_metadata", t.Metadata != nil),
		attribute.Bool("has_parent", parentTransactionID != nil),
	)

	// Start database operation phase timing
	dbPhaseStartTime := time.Now()

	tran, err := uc.TransactionRepo.Create(ctx, save)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create transaction on repo", err)
		logger.Errorf("Error creating t: %v", err)

		// Record error with more details
		uc.recordTransactionError(ctx, "transaction_creation_error",
			attribute.String("transaction_id", newTransactionID),
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
			attribute.String("asset_code", t.Send.Asset),
			attribute.String("error_detail", err.Error()))

		// Record transaction duration with error status
		uc.recordTransactionDuration(ctx, startTime, "create", "error",
			attribute.String("transaction_id", newTransactionID),
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
			attribute.String("error", err.Error()),
		)

		// Record business metric for transaction creation failure
		uc.recordBusinessMetrics(ctx, "transaction_create_failure",
			attribute.String("transaction_id", newTransactionID),
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
			attribute.String("asset_code", t.Send.Asset),
			attribute.String("error", err.Error()),
		)

		return nil, err
	}

	// Record database operation duration
	uc.recordTransactionDuration(ctx, dbPhaseStartTime, "transaction_database_write", "success",
		attribute.String("transaction_id", tran.ID),
		attribute.String("asset_code", t.Send.Asset))

	// Start metadata phase timing
	metadataPhaseStartTime := time.Now()
	metadataProcessed := false

	// Process metadata if present
	if t.Metadata != nil {
		if err := pkg.CheckMetadataKeyAndValueLength(100, t.Metadata); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to check metadata key and value length", err)

			// Record detailed error for metadata validation
			uc.recordTransactionError(ctx, "metadata_validation_error",
				attribute.String("transaction_id", tran.ID),
				attribute.String("organization_id", organizationID.String()),
				attribute.String("ledger_id", ledgerID.String()),
				attribute.Int("metadata_field_count", len(t.Metadata)),
				attribute.String("error_detail", err.Error()))

			// Record transaction duration with error status
			uc.recordTransactionDuration(ctx, startTime, "create", "error",
				attribute.String("transaction_id", tran.ID),
				attribute.String("organization_id", organizationID.String()),
				attribute.String("ledger_id", ledgerID.String()),
				attribute.String("error", "metadata_validation_failed"),
			)

			return nil, err
		}

		meta := mongodb.Metadata{
			EntityID:   tran.ID,
			EntityName: reflect.TypeOf(transaction.Transaction{}).Name(),
			Data:       t.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		// Record metadata size metrics
		uc.recordBusinessMetrics(ctx, "transaction_metadata_size",
			attribute.String("transaction_id", tran.ID),
			attribute.Int("field_count", len(t.Metadata)))

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), &meta); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to create transaction metadata", err)
			logger.Errorf("Error into creating transactiont metadata: %v", err)

			// Record detailed error for metadata creation
			uc.recordTransactionError(ctx, "metadata_creation_error",
				attribute.String("transaction_id", tran.ID),
				attribute.String("organization_id", organizationID.String()),
				attribute.String("ledger_id", ledgerID.String()),
				attribute.Int("metadata_field_count", len(t.Metadata)),
				attribute.String("error_detail", err.Error()))

			// Record transaction duration with error status
			uc.recordTransactionDuration(ctx, startTime, "create", "error",
				attribute.String("transaction_id", tran.ID),
				attribute.String("organization_id", organizationID.String()),
				attribute.String("ledger_id", ledgerID.String()),
				attribute.String("error", "metadata_creation_failed"),
			)

			return nil, err
		}

		tran.Metadata = t.Metadata
		metadataProcessed = true
	}

	// Record metadata phase duration
	metadataStatus := "skipped"
	if metadataProcessed {
		metadataStatus = "success"
	}
	uc.recordTransactionDuration(ctx, metadataPhaseStartTime, "transaction_metadata_processing",
		metadataStatus,
		attribute.String("transaction_id", tran.ID),
		attribute.Bool("metadata_processed", metadataProcessed))

	// Record transaction value metrics
	amount := *save.Amount
	uc.recordBalanceUpdates(ctx, save.AssetCode, float64(amount))

	// Record transaction duration with success status
	uc.recordTransactionDuration(ctx, startTime, "create", "success",
		attribute.String("transaction_id", tran.ID),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("asset_code", t.Send.Asset),
		attribute.Int64("amount", amount),
		attribute.Bool("metadata_processed", metadataProcessed),
	)

	// Expanded record business metric for transaction creation success
	uc.recordBusinessMetrics(ctx, "transaction_create_success",
		attribute.String("transaction_id", tran.ID),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("asset_code", t.Send.Asset),
		attribute.String("template", t.ChartOfAccountsGroupName),
		attribute.Int64("amount", amount),
		attribute.Int64("scale", *save.AmountScale),
		attribute.Bool("has_metadata", metadataProcessed),
		attribute.Bool("has_parent", parentTransactionID != nil),
		attribute.Float64("duration_ms", float64(time.Since(startTime).Milliseconds())),
	)

	return tran, nil
}
