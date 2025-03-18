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
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
)

// CreateBalanceTransactionOperationsAsync func that is responsible to create all transactions at the same async.
func (uc *UseCase) CreateBalanceTransactionOperationsAsync(ctx context.Context, data mmodel.Queue) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	// Record operation metrics
	uc.recordBusinessMetrics(ctx, "bto_async_process_attempt",
		attribute.String("organization_id", data.OrganizationID.String()),
		attribute.String("ledger_id", data.LedgerID.String()),
		attribute.Int("queue_data_count", len(data.QueueData)))

	var t transaction.TransactionQueue

	for _, item := range data.QueueData {
		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		err := json.Unmarshal(item.Value, &t)
		if err != nil {
			logger.Errorf("failed to unmarshal response: %v", err.Error())

			// Record error
			uc.recordTransactionError(ctx, "bto_unmarshal_error",
				attribute.String("queue_data_id", item.ID.String()),
				attribute.String("error_detail", err.Error()))

			// Record duration with error status
			uc.recordTransactionDuration(ctx, startTime, "bto_async_process", "error",
				attribute.String("error", "unmarshal_error"))

			return err
		}
	}

	// Record transaction details for metrics
	transactionID := "unknown"
	assetCode := "unknown"
	if t.Transaction != nil {
		transactionID = t.Transaction.ID
		assetCode = t.Transaction.AssetCode
	}

	// Add transaction info to metrics
	uc.recordBusinessMetrics(ctx, "bto_async_process_transaction",
		attribute.String("transaction_id", transactionID),
		attribute.String("organization_id", data.OrganizationID.String()),
		attribute.String("ledger_id", data.LedgerID.String()),
		attribute.String("asset_code", assetCode),
		attribute.Int("balance_count", len(t.Balances)))

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.create_balance_transaction_operations.update_balances")
	defer spanUpdateBalances.End()

	logger.Infof("Trying to update balances")

	validate := t.Validate
	balances := t.Balances

	balancesStartTime := time.Now()

	err := uc.UpdateBalancesNew(ctxProcessBalances, data.OrganizationID, data.LedgerID, *validate, balances)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances", err)

		logger.Errorf("Failed to update balances: %v", err.Error())

		// Record error
		uc.recordTransactionError(ctx, "bto_balance_update_error",
			attribute.String("transaction_id", transactionID),
			attribute.String("error_detail", err.Error()))

		// Record duration with error status
		uc.recordTransactionDuration(ctx, startTime, "bto_async_process", "error",
			attribute.String("transaction_id", transactionID),
			attribute.String("error", "balance_update_error"))

		return err
	}

	// Record balance update duration
	uc.recordTransactionDuration(ctx, balancesStartTime, "bto_balance_update", "success",
		attribute.String("transaction_id", transactionID),
		attribute.Int("balance_count", len(balances)))

	transactionStartTime := time.Now()
	_, spanCreateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanCreateTransaction.End()

	logger.Infof("Trying to create new transaction")

	tran := t.Transaction
	tran.Body = *t.ParseDSL

	description := constant.APPROVED
	status := transaction.Status{
		Code:        description,
		Description: &description,
	}

	tran.Status = status

	_, err = uc.TransactionRepo.Create(ctx, tran)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanCreateTransaction, "Failed to create transaction on repo", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			logger.Infof("Transaction already exists: %v", tran.ID)

			// Record duplicate transaction
			uc.recordBusinessMetrics(ctx, "bto_transaction_already_exists",
				attribute.String("transaction_id", tran.ID),
				attribute.String("organization_id", data.OrganizationID.String()),
				attribute.String("ledger_id", data.LedgerID.String()))
		} else {
			logger.Errorf("Failed to create transaction on repo: %v", err.Error())

			// Record error
			uc.recordTransactionError(ctx, "bto_transaction_creation_error",
				attribute.String("transaction_id", tran.ID),
				attribute.String("error_detail", err.Error()))

			// Record duration with error status
			uc.recordTransactionDuration(ctx, startTime, "bto_async_process", "error",
				attribute.String("transaction_id", tran.ID),
				attribute.String("error", "transaction_creation_error"))

			return err
		}
	} else {
		// Record transaction creation success
		uc.recordBusinessMetrics(ctx, "bto_transaction_created",
			attribute.String("transaction_id", tran.ID),
			attribute.String("organization_id", data.OrganizationID.String()),
			attribute.String("ledger_id", data.LedgerID.String()),
			attribute.String("asset_code", tran.AssetCode))
	}

	// Record transaction creation duration
	uc.recordTransactionDuration(ctx, transactionStartTime, "bto_transaction_creation", "success",
		attribute.String("transaction_id", tran.ID))

	err = uc.CreateMetadataAsync(ctx, logger, tran.Metadata, tran.ID, reflect.TypeOf(transaction.Transaction{}).Name())
	if err != nil {
		mopentelemetry.HandleSpanError(&spanCreateTransaction, "Failed to create metadata on transaction", err)

		logger.Errorf("Failed to create metadata on transaction: %v", err.Error())

		// Record error
		uc.recordTransactionError(ctx, "bto_transaction_metadata_error",
			attribute.String("transaction_id", tran.ID),
			attribute.String("error_detail", err.Error()))

		// Record duration with error status
		uc.recordTransactionDuration(ctx, startTime, "bto_async_process", "error",
			attribute.String("transaction_id", tran.ID),
			attribute.String("error", "transaction_metadata_error"))

		return err
	}

	operationsStartTime := time.Now()
	operationSuccessCount := 0
	operationErrorCount := 0
	operationDuplicateCount := 0

	ctxProcessOperation, spanCreateOperation := tracer.Start(ctx, "command.create_balance_transaction_operations.create_operation")
	defer spanCreateOperation.End()

	logger.Infof("Trying to create new operations")

	for _, oper := range tran.Operations {
		_, err = uc.OperationRepo.Create(ctxProcessOperation, oper)
		if err != nil {
			mopentelemetry.HandleSpanError(&spanCreateOperation, "Failed to create operation", err)

			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				logger.Infof("Operation already exists: %v", oper.ID)
				operationDuplicateCount++
				continue
			} else {
				logger.Errorf("Error creating operation: %v", err)

				// Record error
				uc.recordTransactionError(ctx, "bto_operation_creation_error",
					attribute.String("transaction_id", tran.ID),
					attribute.String("operation_id", oper.ID),
					attribute.String("error_detail", err.Error()))

				operationErrorCount++

				// If all operations fail, consider it a total failure
				if operationErrorCount == len(tran.Operations) {
					// Record duration with error status
					uc.recordTransactionDuration(ctx, startTime, "bto_async_process", "error",
						attribute.String("transaction_id", tran.ID),
						attribute.String("error", "all_operations_failed"))

					return err
				}

				continue
			}
		}

		err = uc.CreateMetadataAsync(ctx, logger, oper.Metadata, oper.ID, reflect.TypeOf(operation.Operation{}).Name())
		if err != nil {
			mopentelemetry.HandleSpanError(&spanCreateOperation, "Failed to create metadata on operation", err)

			logger.Errorf("Failed to create metadata on operation: %v", err)

			// Record error but continue with other operations
			uc.recordTransactionError(ctx, "bto_operation_metadata_error",
				attribute.String("transaction_id", tran.ID),
				attribute.String("operation_id", oper.ID),
				attribute.String("error_detail", err.Error()))

			continue
		}

		operationSuccessCount++
	}

	// Record operations metrics
	uc.recordBusinessMetrics(ctx, "bto_operations_created",
		attribute.String("transaction_id", tran.ID),
		attribute.Int("successful_operations", operationSuccessCount),
		attribute.Int("duplicate_operations", operationDuplicateCount),
		attribute.Int("failed_operations", operationErrorCount))

	// Record operations duration
	operationStatus := "success"
	if operationErrorCount > 0 {
		operationStatus = "partial"
	}
	uc.recordTransactionDuration(ctx, operationsStartTime, "bto_operations_creation",
		operationStatus,
		attribute.String("transaction_id", tran.ID),
		attribute.Int("operation_count", len(tran.Operations)),
		attribute.Int("success_count", operationSuccessCount))

	// Record overall success metrics
	uc.recordBusinessMetrics(ctx, "bto_async_process_success",
		attribute.String("transaction_id", tran.ID),
		attribute.String("organization_id", data.OrganizationID.String()),
		attribute.String("ledger_id", data.LedgerID.String()),
		attribute.String("asset_code", assetCode),
		attribute.Int("balance_count", len(balances)),
		attribute.Int("operation_count", len(tran.Operations)))

	// Record overall duration
	uc.recordTransactionDuration(ctx, startTime, "bto_async_process", "success",
		attribute.String("transaction_id", tran.ID),
		attribute.Int("balance_count", len(balances)),
		attribute.Int("operation_count", len(tran.Operations)))

	return nil
}

// CreateMetadataAsync func that create metadata into operations
func (uc *UseCase) CreateMetadataAsync(ctx context.Context, logger mlog.Logger, metadata map[string]any, ID string, collection string) error {
	if metadata != nil {
		metadataStartTime := time.Now()

		// We don't start a new span here since this is usually called from parent functions with spans

		if err := pkg.CheckMetadataKeyAndValueLength(100, metadata); err != nil {
			// Record error
			if ctx != nil {
				uc.recordTransactionError(ctx, "metadata_validation_error",
					attribute.String("entity_id", ID),
					attribute.String("entity_type", collection),
					attribute.String("error_detail", err.Error()))

				// Record duration
				uc.recordTransactionDuration(ctx, metadataStartTime, "metadata_creation", "error",
					attribute.String("entity_id", ID),
					attribute.String("entity_type", collection),
					attribute.String("error", "validation_error"))
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

			// Record error
			if ctx != nil {
				uc.recordTransactionError(ctx, "metadata_creation_error",
					attribute.String("entity_id", ID),
					attribute.String("entity_type", collection),
					attribute.String("error_detail", err.Error()))

				// Record duration
				uc.recordTransactionDuration(ctx, metadataStartTime, "metadata_creation", "error",
					attribute.String("entity_id", ID),
					attribute.String("entity_type", collection),
					attribute.String("error", "database_error"))
			}
			return err
		}

		// Record duration for successful metadata creation
		if ctx != nil {
			uc.recordTransactionDuration(ctx, metadataStartTime, "metadata_creation", "success",
				attribute.String("entity_id", ID),
				attribute.String("entity_type", collection))
		}
	}

	return nil
}

func (uc *UseCase) CreateBTOAsync(ctx context.Context, data mmodel.Queue) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Start time for operation
	startTime := time.Now()

	// Record attempt
	uc.recordBusinessMetrics(ctx, "bto_async_wrapper_attempt",
		attribute.String("organization_id", data.OrganizationID.String()),
		attribute.String("ledger_id", data.LedgerID.String()))

	err := uc.CreateBalanceTransactionOperationsAsync(ctx, data)
	if err != nil {
		logger.Errorf("Failed to create balance transaction operations: %v", err)

		// Record error
		uc.recordTransactionError(ctx, "bto_async_wrapper_error",
			attribute.String("organization_id", data.OrganizationID.String()),
			attribute.String("ledger_id", data.LedgerID.String()),
			attribute.String("error_detail", err.Error()))

		// Record duration
		uc.recordTransactionDuration(ctx, startTime, "bto_async_wrapper", "error",
			attribute.String("organization_id", data.OrganizationID.String()),
			attribute.String("ledger_id", data.LedgerID.String()),
			attribute.String("error", "processing_error"))
	} else {
		// Record success
		uc.recordBusinessMetrics(ctx, "bto_async_wrapper_success",
			attribute.String("organization_id", data.OrganizationID.String()),
			attribute.String("ledger_id", data.LedgerID.String()))

		// Record duration
		uc.recordTransactionDuration(ctx, startTime, "bto_async_wrapper", "success",
			attribute.String("organization_id", data.OrganizationID.String()),
			attribute.String("ledger_id", data.LedgerID.String()))
	}
}
