package command

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
)

// CreateBalanceTransactionOperationsAsync func that is responsible to create all transactions at the same async.
func (uc *UseCase) CreateBalanceTransactionOperationsAsync(ctx context.Context, data mmodel.Queue) error {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a batch transaction operation telemetry entity
	op := uc.Telemetry.NewTransactionOperation("bto_async_process", "queue")

	// Add important attributes
	op.WithAttributes(
		attribute.String("organization_id", data.OrganizationID.String()),
		attribute.String("ledger_id", data.LedgerID.String()),
		attribute.Int("queue_data_count", len(data.QueueData)),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	// Parse the transaction queue data
	t, err := uc.parseTransactionQueue(ctx, data, op, logger)
	if err != nil {
		return err
	}

	// Extract transaction details for metrics and telemetry
	transactionID, assetCode := uc.extractTransactionDetails(t, op)

	// Update balances
	if err := uc.updateBalancesForTransaction(ctx, data, t, transactionID, op, logger); err != nil {
		return err
	}

	// Create the transaction
	if err := uc.createTransaction(ctx, t, transactionID, assetCode, data, op, logger); err != nil {
		return err
	}

	// Create operations
	operationResults, err := uc.createOperations(ctx, t, transactionID, op, logger)
	if err != nil {
		return err
	}

	// Record overall results in main operation
	op.WithAttributes(
		attribute.Int("balance_count", len(t.Balances)),
		attribute.Int("operation_count", len(t.Transaction.Operations)),
		attribute.Int("successful_operations", operationResults.successCount),
		attribute.Int("duplicate_operations", operationResults.duplicateCount),
		attribute.Int("failed_operations", operationResults.errorCount),
	)

	// End the main operation
	op.End(ctx, "success")

	return nil
}

// parseTransactionQueue unmarshal queue data into transaction queue
func (uc *UseCase) parseTransactionQueue(
	ctx context.Context,
	data mmodel.Queue,
	op *EntityOperation,
	logger mlog.Logger,
) (transaction.TransactionQueue, error) {
	var t transaction.TransactionQueue

	for _, item := range data.QueueData {
		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		err := json.Unmarshal(item.Value, &t)
		if err != nil {
			logger.Errorf("failed to unmarshal response: %v", err.Error())

			// Record error
			op.RecordError(ctx, "bto_unmarshal_error", err)
			op.WithAttribute("item_id", item.ID.String())
			op.End(ctx, "failed")

			return t, err
		}
	}

	return t, nil
}

// extractTransactionDetails extracts transaction ID and asset code for metrics and telemetry
func (uc *UseCase) extractTransactionDetails(t transaction.TransactionQueue, op *EntityOperation) (string, string) {
	transactionID := "unknown"
	assetCode := "unknown"

	if t.Transaction != nil {
		transactionID = t.Transaction.ID
		assetCode = t.Transaction.AssetCode

		// Add transaction details to telemetry
		op.WithAttributes(
			attribute.String("transaction_id", transactionID),
			attribute.String("asset_code", assetCode),
			attribute.Int("balance_count", len(t.Balances)),
		)
	}

	return transactionID, assetCode
}

// updateBalancesForTransaction updates balances for a transaction
func (uc *UseCase) updateBalancesForTransaction(
	ctx context.Context,
	data mmodel.Queue,
	t transaction.TransactionQueue,
	transactionID string,
	op *EntityOperation,
	logger mlog.Logger,
) error {
	// Start balances update sub-operation
	balanceOp := uc.Telemetry.NewBalanceOperation("bto_balance_update", transactionID)
	balanceOp.WithAttributes(
		attribute.String("organization_id", data.OrganizationID.String()),
		attribute.String("ledger_id", data.LedgerID.String()),
		attribute.Int("balance_count", len(t.Balances)),
	)

	// Start tracing for balance update
	balanceCtx := balanceOp.StartTrace(ctx)
	balanceOp.RecordSystemicMetric(balanceCtx)

	logger.Infof("Trying to update balances")

	validate := t.Validate
	balances := t.Balances

	err := uc.UpdateBalancesNew(balanceCtx, data.OrganizationID, data.LedgerID, *validate, balances)
	if err != nil {
		// Record error
		balanceOp.RecordError(balanceCtx, "bto_balance_update_error", err)
		balanceOp.End(balanceCtx, "failed")

		// Also record in parent operation
		op.RecordError(ctx, "bto_balance_update_error", err)
		op.End(ctx, "failed")

		logger.Errorf("Failed to update balances: %v", err.Error())

		return err
	}

	// Mark balance update as successful
	balanceOp.End(balanceCtx, "success")

	return nil
}

// createTransaction creates a transaction record
func (uc *UseCase) createTransaction(
	ctx context.Context,
	t transaction.TransactionQueue,
	transactionID string,
	assetCode string,
	data mmodel.Queue,
	op *EntityOperation,
	logger mlog.Logger,
) error {
	// Start transaction creation sub-operation
	transOp := uc.Telemetry.NewTransactionOperation("bto_transaction_creation", transactionID)
	transOp.WithAttributes(
		attribute.String("organization_id", data.OrganizationID.String()),
		attribute.String("ledger_id", data.LedgerID.String()),
		attribute.String("asset_code", assetCode),
	)

	// Start tracing for transaction creation
	transCtx := transOp.StartTrace(ctx)
	transOp.RecordSystemicMetric(transCtx)

	logger.Infof("Trying to create new transaction")

	tran := t.Transaction
	tran.Body = *t.ParseDSL

	description := constant.APPROVED
	status := transaction.Status{
		Code:        description,
		Description: &description,
	}

	tran.Status = status

	_, err := uc.TransactionRepo.Create(transCtx, tran)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			logger.Infof("Transaction already exists: %v", tran.ID)

			// Record duplicate but consider it a success
			transOp.WithAttribute("duplicate", "true")
			transOp.End(transCtx, "success")
		} else {
			logger.Errorf("Failed to create transaction on repo: %v", err.Error())

			// Record error
			transOp.RecordError(transCtx, "bto_transaction_creation_error", err)
			transOp.End(transCtx, "failed")

			// Also record in parent operation
			op.RecordError(ctx, "bto_transaction_creation_error", err)
			op.End(ctx, "failed")

			return err
		}
	} else {
		// Record business metrics
		if tran.Amount != nil {
			transOp.RecordBusinessMetric(transCtx, "amount", float64(*tran.Amount))
		}

		// Mark transaction creation as successful
		transOp.End(transCtx, "success")
	}

	// Try to add metadata to transaction
	metaOp := uc.Telemetry.NewTransactionOperation("bto_transaction_metadata", tran.ID)
	metaOp.StartTrace(ctx)
	metaOp.RecordSystemicMetric(ctx)

	err = uc.CreateMetadataAsync(ctx, logger, tran.Metadata, tran.ID, reflect.TypeOf(transaction.Transaction{}).Name(), metaOp)
	// We just record any errors in metaOp inside the CreateMetadataAsync function
	// but continue processing regardless of metadata errors
	if err != nil {
		logger.Errorf("Failed to create metadata on transaction: %v", err.Error())
	}

	return nil
}

// operationResults collects statistics about operation creation
type operationResults struct {
	successCount   int
	errorCount     int
	duplicateCount int
}

// createOperations creates all operations for a transaction
func (uc *UseCase) createOperations(
	ctx context.Context,
	t transaction.TransactionQueue,
	transactionID string,
	op *EntityOperation,
	logger mlog.Logger,
) (operationResults, error) {
	tran := t.Transaction

	// Start operations creation sub-operation
	operationsOp := uc.Telemetry.NewOperationOperation("bto_operations_creation", transactionID)
	operationsOp.WithAttributes(
		attribute.String("transaction_id", transactionID),
		attribute.Int("operation_count", len(tran.Operations)),
	)

	// Start tracing for operations creation
	opsCtx := operationsOp.StartTrace(ctx)
	operationsOp.RecordSystemicMetric(opsCtx)

	logger.Infof("Trying to create new operations")

	results := operationResults{}

	for _, oper := range tran.Operations {
		err := uc.createSingleAsyncOperation(opsCtx, oper, transactionID, &results, logger)

		// If all operations fail, consider it a total failure
		if results.errorCount == len(tran.Operations) {
			// Record in parent operations operation
			operationsOp.RecordError(opsCtx, "all_operations_failed", err)
			operationsOp.End(opsCtx, "failed")

			// Also record in main operation
			op.RecordError(ctx, "all_operations_failed", err)
			op.End(ctx, "failed")

			return results, err
		}
	}

	// Record operations batch results
	operationsOp.WithAttributes(
		attribute.Int("successful_operations", results.successCount),
		attribute.Int("duplicate_operations", results.duplicateCount),
		attribute.Int("failed_operations", results.errorCount),
	)

	// End the operations batch operation
	if results.errorCount > 0 {
		operationsOp.End(opsCtx, "partial_success")
	} else {
		operationsOp.End(opsCtx, "success")
	}

	return results, nil
}

// createSingleAsyncOperation creates a single operation for async processing
func (uc *UseCase) createSingleAsyncOperation(
	ctx context.Context,
	oper *operation.Operation,
	transactionID string,
	results *operationResults,
	logger mlog.Logger,
) error {
	// Create an individual operation telemetry entity for each operation
	operOp := uc.Telemetry.NewOperationOperation("create", oper.ID)
	operOp.WithAttributes(
		attribute.String("transaction_id", transactionID),
		attribute.String("balance_id", oper.BalanceID),
		attribute.String("account_id", oper.AccountID),
	)

	// Start tracing for this operation
	operCtx := operOp.StartTrace(ctx)
	operOp.RecordSystemicMetric(operCtx)

	_, err := uc.OperationRepo.Create(operCtx, oper)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			logger.Infof("Operation already exists: %v", oper.ID)

			results.duplicateCount++

			// Record duplicate but consider it a success
			operOp.WithAttribute("duplicate", "true")
			operOp.End(operCtx, "success")

			return nil
		}

		logger.Errorf("Error creating operation: %v", err)

		// Record error
		operOp.RecordError(operCtx, "bto_operation_creation_error", err)
		operOp.End(operCtx, "failed")

		results.errorCount++

		return err
	}

	// Try to add metadata to the operation
	err = uc.CreateMetadataAsync(operCtx, logger, oper.Metadata, oper.ID, reflect.TypeOf(operation.Operation{}).Name(), operOp)
	// We just record any errors in operOp inside CreateMetadataAsync
	// but continue processing regardless of metadata errors
	if err != nil {
		logger.Errorf("Failed to create metadata on operation: %v", err)
		// Consider it a partial success
		operOp.End(operCtx, "partial_success")

		return nil
	}

	// Record business metrics if amount exists
	if oper.Amount.Amount != nil {
		operOp.RecordBusinessMetric(operCtx, "amount", float64(*oper.Amount.Amount))
	}

	// Mark operation as successful
	operOp.End(operCtx, "success")

	results.successCount++

	return nil
}

// CreateMetadataAsync func that create metadata into operations
func (uc *UseCase) CreateMetadataAsync(ctx context.Context, logger mlog.Logger, metadata map[string]any, ID string, collection string, op *EntityOperation) error {
	if metadata != nil {
		if err := pkg.CheckMetadataKeyAndValueLength(100, metadata); err != nil {
			// Record error in provided operation
			if op != nil {
				op.RecordError(ctx, "metadata_validation_error", err)
				op.WithAttributes(
					attribute.String("entity_type", collection),
					attribute.String("entity_id", ID),
				)
			}

			return err
		}

		meta := mongodb.Metadata{
			EntityID:   ID,
			EntityName: collection,
			Data:       metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, collection, &meta); err != nil {
			logger.Errorf("Error into creating %s metadata: %v", collection, err)

			// Record error in provided operation
			if op != nil {
				op.RecordError(ctx, "metadata_creation_error", err)
				op.WithAttributes(
					attribute.String("entity_type", collection),
					attribute.String("entity_id", ID),
				)
			}

			return err
		}
	}

	return nil
}

func (uc *UseCase) CreateBTOAsync(ctx context.Context, data mmodel.Queue) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a wrapper operation telemetry entity
	op := uc.Telemetry.NewTransactionOperation("bto_async_wrapper", data.OrganizationID.String())

	// Add important attributes
	op.WithAttributes(
		attribute.String("organization_id", data.OrganizationID.String()),
		attribute.String("ledger_id", data.LedgerID.String()),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	err := uc.CreateBalanceTransactionOperationsAsync(ctx, data)
	if err != nil {
		logger.Errorf("Failed to create balance transaction operations: %v", err)

		// Record error
		op.RecordError(ctx, "bto_async_wrapper_error", err)
		op.End(ctx, "failed")
	} else {
		// Mark operation as successful
		op.End(ctx, "success")
	}
}
