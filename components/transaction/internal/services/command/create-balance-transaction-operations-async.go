package command

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/jackc/pgx/v5/pgconn"
	"reflect"
	"time"
)

// CreateBalanceTransactionOperationsAsync func that is responsible to create all transactions at the same async.
func (uc *UseCase) CreateBalanceTransactionOperationsAsync(ctx context.Context, data mmodel.Queue) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

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

	err := uc.UpdateBalances(ctxProcessBalances, data.OrganizationID, data.LedgerID, *validate, balances)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances", err)

		logger.Errorf("Failed to update balances: %v", err.Error())

		return err
	}

	ctxProcessTransaction, spanCreateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			logger.Infof("Transaction already exists: %v", tran.ID)
		} else {
			mopentelemetry.HandleSpanError(&spanCreateTransaction, "Failed to create transaction on repo", err)

			logger.Errorf("Failed to create transaction on repo: %v", err.Error())

			return err
		}
	}

	if tran.Metadata != nil {
		if err = pkg.CheckMetadataKeyAndValueLength(100, tran.Metadata); err != nil {
			mopentelemetry.HandleSpanError(&spanCreateTransaction, "Failed to check metadata key and value length", err)

			logger.Errorf("Failed to check metadata key and value length: %v", err.Error())

			return err
		}

		meta := mongodb.Metadata{
			EntityID:   tran.ID,
			EntityName: reflect.TypeOf(transaction.Transaction{}).Name(),
			Data:       tran.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err = uc.MetadataRepo.Create(ctxProcessTransaction, reflect.TypeOf(transaction.Transaction{}).Name(), &meta); err != nil {
			mopentelemetry.HandleSpanError(&spanCreateTransaction, "Failed to create transaction metadata", err)

			logger.Errorf("Error into creating transactiont metadata: %v", err)

			return err
		}
	}

	ctxProcessOperation, spanCreateOperation := tracer.Start(ctx, "command.create_balance_transaction_operations.create_operation")
	defer spanCreateOperation.End()

	logger.Infof("Trying to create new operations")

	for _, operation := range tran.Operations {
		op, er := uc.OperationRepo.Create(ctxProcessOperation, operation)
		if er != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				logger.Infof("Operation already exists: %v", operation.ID)
			} else {
				mopentelemetry.HandleSpanError(&spanCreateOperation, "Failed to create operation", er)

				logger.Errorf("Error creating operation: %v", er)

				return err
			}
		}

		er = uc.CreateMetadata(ctxProcessOperation, logger, tran.Metadata, op)
		if er != nil {
			mopentelemetry.HandleSpanError(&spanCreateOperation, "Failed to create metadata on operation", er)

			logger.Errorf("Error creating metadata: %v", er)

			return err
		}
	}

	return nil
}

func (uc *UseCase) CreateBTOAsync(ctx context.Context, data mmodel.Queue) {
	logger := pkg.NewLoggerFromContext(ctx)

	err := uc.CreateBalanceTransactionOperationsAsync(ctx, data)
	if err != nil {
		logger.Errorf("Failed to create balance transaction operations: %v", err)
	}
}
