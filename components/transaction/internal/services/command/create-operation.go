package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"
)

// CreateOperation creates a new operation based on transaction id and persisting data in the repository.
func (uc *UseCase) CreateOperation(ctx context.Context, balances []*mmodel.Balance, transactionID string, dsl *goldModel.Transaction, validate goldModel.Responses, result chan []*operation.Operation, err chan error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.create_operation")
	defer span.End()

	// Record operation metrics
	uc.recordBusinessMetrics(ctx, "operation_creation_attempt",
		attribute.String("transaction_id", transactionID),
		attribute.String("asset_code", dsl.Send.Asset),
		attribute.Int("balance_count", len(balances)))

	logger.Infof("Trying to create new operations")

	var operations []*operation.Operation

	var fromTo []goldModel.FromTo
	fromTo = append(fromTo, dsl.Send.Source.From...)
	fromTo = append(fromTo, dsl.Send.Distribute.To...)

	operationSuccessCount := 0
	operationErrorCount := 0

	for _, blc := range balances {
		for i := range fromTo {
			if fromTo[i].Account == blc.ID || fromTo[i].Account == blc.Alias {
				logger.Infof("Creating operation for account id: %s", blc.ID)

				balance := operation.Balance{
					Available: &blc.Available,
					OnHold:    &blc.OnHold,
					Scale:     &blc.Scale,
				}

				amt, bat, er := goldModel.ValidateFromToOperation(fromTo[i], validate, blc)
				if er != nil {
					mopentelemetry.HandleSpanError(&span, "Failed to validate operation", er)

					// Record error
					uc.recordTransactionError(ctx, "operation_validation_error",
						attribute.String("transaction_id", transactionID),
						attribute.String("balance_id", blc.ID),
						attribute.String("account_alias", blc.Alias),
						attribute.String("error_detail", er.Error()))

					operationErrorCount++
					err <- er
					continue
				}

				amount := operation.Amount{
					Amount: &amt.Value,
					Scale:  &amt.Scale,
				}

				balanceAfter := operation.Balance{
					Available: &bat.Available,
					OnHold:    &bat.OnHold,
					Scale:     &bat.Scale,
				}

				description := fromTo[i].Description
				if pkg.IsNilOrEmpty(&fromTo[i].Description) {
					description = dsl.Description
				}

				var typeOperation string
				if fromTo[i].IsFrom {
					typeOperation = constant.DEBIT
				} else {
					typeOperation = constant.CREDIT
				}

				save := &operation.Operation{
					ID:              pkg.GenerateUUIDv7().String(),
					TransactionID:   transactionID,
					Description:     description,
					Type:            typeOperation,
					AssetCode:       dsl.Send.Asset,
					ChartOfAccounts: fromTo[i].ChartOfAccounts,
					Amount:          amount,
					Balance:         balance,
					BalanceAfter:    balanceAfter,
					BalanceID:       blc.ID,
					AccountID:       blc.AccountID,
					AccountAlias:    blc.Alias,
					OrganizationID:  blc.OrganizationID,
					LedgerID:        blc.LedgerID,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
				}

				op, er := uc.OperationRepo.Create(ctx, save)
				if er != nil {
					mopentelemetry.HandleSpanError(&span, "Failed to create operation", er)

					logger.Errorf("Error creating operation: %v", er)

					// Record error
					uc.recordTransactionError(ctx, "operation_creation_error",
						attribute.String("transaction_id", transactionID),
						attribute.String("balance_id", blc.ID),
						attribute.String("account_alias", blc.Alias),
						attribute.String("error_detail", er.Error()))

					operationErrorCount++
					err <- er
					continue
				}

				er = uc.CreateMetadata(ctx, logger, fromTo[i].Metadata, op)
				if er != nil {
					mopentelemetry.HandleSpanError(&span, "Failed to create metadata on operation", er)

					// Record error
					uc.recordTransactionError(ctx, "operation_metadata_error",
						attribute.String("transaction_id", transactionID),
						attribute.String("operation_id", op.ID),
						attribute.String("error_detail", er.Error()))

					operationErrorCount++
					err <- er
					continue
				}

				operations = append(operations, op)
				operationSuccessCount++

				break
			}
		}
	}

	// Record transaction duration and results
	if operationErrorCount > 0 {
		// Record transaction duration with partial success (some operations failed)
		uc.recordTransactionDuration(ctx, startTime, "operation_creation", "partial",
			attribute.String("transaction_id", transactionID),
			attribute.Int("success_count", operationSuccessCount),
			attribute.Int("error_count", operationErrorCount))

		// Record business metrics for partial success
		uc.recordBusinessMetrics(ctx, "operation_creation_partial",
			attribute.String("transaction_id", transactionID),
			attribute.String("asset_code", dsl.Send.Asset),
			attribute.Int("success_count", operationSuccessCount),
			attribute.Int("error_count", operationErrorCount))
	} else {
		// Record transaction duration with full success
		uc.recordTransactionDuration(ctx, startTime, "operation_creation", "success",
			attribute.String("transaction_id", transactionID),
			attribute.Int("operation_count", len(operations)))

		// Record business metrics for full success
		uc.recordBusinessMetrics(ctx, "operation_creation_success",
			attribute.String("transaction_id", transactionID),
			attribute.String("asset_code", dsl.Send.Asset),
			attribute.Int("operation_count", len(operations)))
	}

	result <- operations
}

// CreateMetadata func that create metadata into operations
func (uc *UseCase) CreateMetadata(ctx context.Context, logger mlog.Logger, metadata map[string]any, o *operation.Operation) error {
	// We don't need to start/end a separate timer here since this is usually called within a parent function
	// that already has timing instrumentation

	if metadata != nil {
		if err := pkg.CheckMetadataKeyAndValueLength(100, metadata); err != nil {
			// Record error only if we have a context and valid operation ID
			if ctx != nil && o != nil && o.ID != "" {
				// Record error
				uc.recordTransactionError(ctx, "metadata_validation_error",
					attribute.String("operation_id", o.ID),
					attribute.String("entity_type", reflect.TypeOf(operation.Operation{}).Name()),
					attribute.String("error_detail", err.Error()))
			}
			return err
		}

		meta := mongodb.Metadata{
			EntityID:   o.ID,
			EntityName: reflect.TypeOf(operation.Operation{}).Name(),
			Data:       metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(operation.Operation{}).Name(), &meta); err != nil {
			logger.Errorf("Error into creating operation metadata: %v", err)

			// Record error only if we have a context and valid operation ID
			if ctx != nil && o != nil && o.ID != "" {
				// Record error
				uc.recordTransactionError(ctx, "metadata_creation_error",
					attribute.String("operation_id", o.ID),
					attribute.String("entity_type", reflect.TypeOf(operation.Operation{}).Name()),
					attribute.String("error_detail", err.Error()))
			}
			return err
		}

		o.Metadata = metadata
	}

	return nil
}
