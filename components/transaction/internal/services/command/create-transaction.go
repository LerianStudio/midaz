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
		ID:                       pkg.GenerateUUIDv7().String(),
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

	// Record business metric for transaction creation attempt
	uc.recordBusinessMetrics(ctx, "transaction_create_attempt", 
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("asset_code", t.Send.Asset),
		attribute.String("template", t.ChartOfAccountsGroupName),
	)

	tran, err := uc.TransactionRepo.Create(ctx, save)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create transaction on repo", err)
		logger.Errorf("Error creating t: %v", err)

		// Record transaction duration with error status
		uc.recordTransactionDuration(ctx, startTime, "create", "error",
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
			attribute.String("error", err.Error()),
		)

		// Record business metric for transaction creation failure
		uc.recordBusinessMetrics(ctx, "transaction_create_failure", 
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
			attribute.String("asset_code", t.Send.Asset),
			attribute.String("error", err.Error()),
		)

		return nil, err
	}

	// Process metadata if present
	if t.Metadata != nil {
		if err := pkg.CheckMetadataKeyAndValueLength(100, t.Metadata); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to check metadata key and value length", err)

			// Record transaction duration with error status
			uc.recordTransactionDuration(ctx, startTime, "create", "error",
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

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), &meta); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to create transaction metadata", err)
			logger.Errorf("Error into creating transactiont metadata: %v", err)

			// Record transaction duration with error status
			uc.recordTransactionDuration(ctx, startTime, "create", "error",
				attribute.String("organization_id", organizationID.String()),
				attribute.String("ledger_id", ledgerID.String()),
				attribute.String("error", "metadata_creation_failed"),
			)

			return nil, err
		}

		tran.Metadata = t.Metadata
	}

	// Record transaction value metrics
	amount := *save.Amount
	uc.recordBalanceUpdates(ctx, save.AssetCode, float64(amount))

	// Record transaction duration with success status
	uc.recordTransactionDuration(ctx, startTime, "create", "success",
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("asset_code", t.Send.Asset),
		attribute.String("transaction_id", tran.ID),
	)

	// Record business metric for transaction creation success
	uc.recordBusinessMetrics(ctx, "transaction_create_success", 
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("asset_code", t.Send.Asset),
		attribute.String("transaction_id", tran.ID),
		attribute.String("template", t.ChartOfAccountsGroupName),
	)

	return tran, nil
}
