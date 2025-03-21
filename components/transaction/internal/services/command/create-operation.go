package command

import (
	"context"
	"reflect"
	"strconv"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"go.opentelemetry.io/otel/attribute"
)

// CreateOperation creates a new operation based on transaction id and persisting data in the repository.
func (uc *UseCase) CreateOperation(ctx context.Context, balances []*mmodel.Balance, transactionID string, dsl *goldModel.Transaction, validate goldModel.Responses, result chan []*operation.Operation, err chan error) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a batch operation telemetry entity
	op := uc.Telemetry.NewOperationOperation("create_batch", transactionID)

	// Add important attributes
	op.WithAttributes(
		attribute.String("transaction_id", transactionID),
		attribute.String("asset_code", dsl.Send.Asset),
		attribute.Int("balance_count", len(balances)),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	logger.Infof("Trying to create new operations")

	// Collect all from/to entries from the transaction
	fromTo := uc.collectFromToEntries(dsl)

	// Process operations for each balance
	operations, operationSuccessCount, operationErrorCount := uc.processBalanceOperations(
		ctx, balances, transactionID, dsl, validate, fromTo, logger, err,
	)

	// Record batch operation results
	uc.finalizeOperationBatch(ctx, op, operations, operationSuccessCount, operationErrorCount)

	result <- operations
}

// collectFromToEntries collects all FromTo entries from a transaction DSL
func (uc *UseCase) collectFromToEntries(dsl *goldModel.Transaction) []goldModel.FromTo {
	var fromTo []goldModel.FromTo
	fromTo = append(fromTo, dsl.Send.Source.From...)
	fromTo = append(fromTo, dsl.Send.Distribute.To...)

	return fromTo
}

// processBalanceOperations creates operations for each balance that matches a FromTo entry
func (uc *UseCase) processBalanceOperations(
	ctx context.Context,
	balances []*mmodel.Balance,
	transactionID string,
	dsl *goldModel.Transaction,
	validate goldModel.Responses,
	fromTo []goldModel.FromTo,
	logger mlog.Logger,
	err chan error,
) ([]*operation.Operation, int, int) {
	var operations []*operation.Operation

	operationSuccessCount := 0
	operationErrorCount := 0

	for _, blc := range balances {
		for i := range fromTo {
			// Skip if this FromTo entry doesn't match the current balance
			if fromTo[i].Account != blc.ID && fromTo[i].Account != blc.Alias {
				continue
			}

			// Create operation for this balance
			op, success := uc.createSingleBalanceOperation(
				ctx, blc, transactionID, dsl, validate, fromTo[i], logger, err,
			)

			if success {
				operations = append(operations, op)
				operationSuccessCount++
			} else {
				operationErrorCount++
			}

			// We found a matching FromTo entry for this balance, so move to next balance
			break
		}
	}

	return operations, operationSuccessCount, operationErrorCount
}

// createSingleBalanceOperation creates a single operation for a balance
func (uc *UseCase) createSingleBalanceOperation(
	ctx context.Context,
	blc *mmodel.Balance,
	transactionID string,
	dsl *goldModel.Transaction,
	validate goldModel.Responses,
	fromTo goldModel.FromTo,
	logger mlog.Logger,
	err chan error,
) (*operation.Operation, bool) {
	// Create an individual operation telemetry entity for this balance operation
	balanceOp := uc.Telemetry.NewOperationOperation("create", blc.ID)
	balanceOp.WithAttributes(
		attribute.String("transaction_id", transactionID),
		attribute.String("balance_id", blc.ID),
		attribute.String("account_alias", blc.Alias),
		attribute.String("asset_code", dsl.Send.Asset),
	)

	// Record systemic metric for this individual operation
	balanceOp.RecordSystemicMetric(ctx)

	logger.Infof("Creating operation for account id: %s", blc.ID)

	// Prepare current balance info
	balance := operation.Balance{
		Available: &blc.Available,
		OnHold:    &blc.OnHold,
		Scale:     &blc.Scale,
	}

	// Validate the operation
	amt, bat, er := goldModel.ValidateFromToOperation(fromTo, validate, blc)
	if er != nil {
		// Record error for this individual operation
		balanceOp.RecordError(ctx, "validation_error", er)
		balanceOp.End(ctx, "failed")
		err <- er

		return nil, false
	}

	// Prepare amount and balance after transaction
	amount := operation.Amount{
		Amount: &amt.Value,
		Scale:  &amt.Scale,
	}

	balanceAfter := operation.Balance{
		Available: &bat.Available,
		OnHold:    &bat.OnHold,
		Scale:     &bat.Scale,
	}

	// Get appropriate description
	description := fromTo.Description
	if pkg.IsNilOrEmpty(&fromTo.Description) {
		description = dsl.Description
	}

	// Determine operation type
	typeOperation := constant.CREDIT
	if fromTo.IsFrom {
		typeOperation = constant.DEBIT
	}

	// Create the operation entity
	save := &operation.Operation{
		ID:              pkg.GenerateUUIDv7().String(),
		TransactionID:   transactionID,
		Description:     description,
		Type:            typeOperation,
		AssetCode:       dsl.Send.Asset,
		ChartOfAccounts: fromTo.ChartOfAccounts,
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

	// Add operation ID to the telemetry
	balanceOp.WithAttribute("operation_id", save.ID)

	// Persist the operation
	op, er := uc.OperationRepo.Create(ctx, save)
	if er != nil {
		// Record error for this individual operation
		balanceOp.RecordError(ctx, "creation_error", er)
		balanceOp.End(ctx, "failed")
		logger.Errorf("Error creating operation: %v", er)
		err <- er

		return nil, false
	}

	// Add metadata if present
	er = uc.CreateMetadata(ctx, logger, fromTo.Metadata, op, balanceOp)
	if er != nil {
		// Error already recorded in CreateMetadata
		balanceOp.End(ctx, "partial_success")
		err <- er

		return nil, false
	}

	// Record business metrics if amount exists
	if amount.Amount != nil {
		balanceOp.RecordBusinessMetric(ctx, "amount", float64(*amount.Amount))
	}

	// Record successful operation
	balanceOp.End(ctx, "success")

	return op, true
}

// finalizeOperationBatch records batch results and ends the batch operation
func (uc *UseCase) finalizeOperationBatch(ctx context.Context, op *EntityOperation, operations []*operation.Operation, operationSuccessCount, operationErrorCount int) {
	if operationErrorCount > 0 {
		// Record partial success if some operations succeeded
		op.WithAttributes(
			attribute.Int("success_count", operationSuccessCount),
			attribute.Int("error_count", operationErrorCount),
		)
		op.End(ctx, "partial_success")
	} else {
		// Record full success
		op.WithAttribute("operation_count", strconv.Itoa(len(operations)))
		op.End(ctx, "success")
	}
}

// CreateMetadata func that create metadata into operations
func (uc *UseCase) CreateMetadata(ctx context.Context, logger mlog.Logger, metadata map[string]any, o *operation.Operation, op *EntityOperation) error {
	if metadata != nil {
		if err := pkg.CheckMetadataKeyAndValueLength(100, metadata); err != nil {
			// Record error if operation provided
			if op != nil {
				op.RecordError(ctx, "metadata_validation_error", err)
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

			// Record error if operation provided
			if op != nil {
				op.RecordError(ctx, "metadata_creation_error", err)
			}

			return err
		}

		o.Metadata = metadata
	}

	return nil
}
