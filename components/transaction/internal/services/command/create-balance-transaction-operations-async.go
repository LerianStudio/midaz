package command

import (
	"context"
	"encoding/json"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libLog "github.com/LerianStudio/lib-commons/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/jackc/pgx/v5/pgconn"
	"reflect"
	"time"
)

// CreateBalanceTransactionOperationsAsync func that is responsible to create all transactions at the same async.
func (uc *UseCase) CreateBalanceTransactionOperationsAsync(ctx context.Context, data mmodel.Queue) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	var t transaction.TransactionQueue

	for _, item := range data.QueueData {
		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		err := json.Unmarshal(item.Value, &t)
		if err != nil {
			logger.Errorf("failed to unmarshal response: %v", err.Error())

			return err
		}
	}

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.create_balance_transaction_operations.update_balances")
	defer spanUpdateBalances.End()

	logger.Infof("Trying to update balances")

	validate := t.Validate
	balances := t.Balances

	err := uc.UpdateBalancesNew(ctxProcessBalances, data.OrganizationID, data.LedgerID, *validate, balances)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances", err)

		logger.Errorf("Failed to update balances: %v", err.Error())

		return err
	}

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
		libOpentelemetry.HandleSpanError(&spanCreateTransaction, "Failed to create transaction on repo", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			logger.Infof("Transaction already exists: %v", tran.ID)
		} else {
			logger.Errorf("Failed to create transaction on repo: %v", err.Error())

			return err
		}
	}

	err = uc.CreateMetadataAsync(ctx, logger, tran.Metadata, tran.ID, reflect.TypeOf(transaction.Transaction{}).Name())
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanCreateTransaction, "Failed to create metadata on transaction", err)

		logger.Errorf("Failed to create metadata on transaction: %v", err.Error())

		return err
	}

	ctxProcessOperation, spanCreateOperation := tracer.Start(ctx, "command.create_balance_transaction_operations.create_operation")
	defer spanCreateOperation.End()

	logger.Infof("Trying to create new operations")

	for _, oper := range tran.Operations {
		_, err = uc.OperationRepo.Create(ctxProcessOperation, oper)
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanCreateOperation, "Failed to create operation", err)

			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				logger.Infof("Operation already exists: %v", oper.ID)

				continue
			} else {
				logger.Errorf("Error creating operation: %v", err)

				return err
			}
		}

		err = uc.CreateMetadataAsync(ctx, logger, oper.Metadata, oper.ID, reflect.TypeOf(operation.Operation{}).Name())
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanCreateOperation, "Failed to create metadata on operation", err)

			logger.Errorf("Failed to create metadata on operation: %v", err)

			return err
		}
	}

	return nil
}

// CreateMetadataAsync func that create metadata into operations
func (uc *UseCase) CreateMetadataAsync(ctx context.Context, logger libLog.Logger, metadata map[string]any, ID string, collection string) error {
	if metadata != nil {
		if err := libCommons.CheckMetadataKeyAndValueLength(100, metadata); err != nil {
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

			return err
		}
	}

	return nil
}

func (uc *UseCase) CreateBTOAsync(ctx context.Context, data mmodel.Queue) {
	logger := libCommons.NewLoggerFromContext(ctx)

	err := uc.CreateBalanceTransactionOperationsAsync(ctx, data)
	if err != nil {
		logger.Errorf("Failed to create balance transaction operations: %v", err)
	}
}
