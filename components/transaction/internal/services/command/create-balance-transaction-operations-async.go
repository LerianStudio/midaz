package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel/trace"
)

// CreateBalanceTransactionOperationsAsync func that is responsible to create all transactions at the same async.
func (uc *UseCase) CreateBalanceTransactionOperationsAsync(ctx context.Context, data mmodel.Queue) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	var t transaction.TransactionQueue

	for _, item := range data.QueueData {
		logger.Infof("Unmarshal account ID: %v", item.ID.String())

		err := msgpack.Unmarshal(item.Value, &t)
		if err != nil {
			logger.Errorf("failed to unmarshal response: %v", err.Error())

			return err
		}
	}

	if t.Transaction.Status.Code != constant.NOTED {
		ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.create_balance_transaction_operations.update_balances")
		defer spanUpdateBalances.End()

		logger.Infof("Trying to update balances")

		err := uc.UpdateBalances(ctxProcessBalances, data.OrganizationID, data.LedgerID, *t.Validate, t.Balances)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to update balances", err)

			logger.Errorf("Failed to update balances: %v", err.Error())

			return err
		}
	}

	ctxProcessTransaction, spanUpdateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanUpdateTransaction.End()

	tran, err := uc.CreateOrUpdateTransaction(ctxProcessTransaction, logger, tracer, t)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateTransaction, "Failed to create or update transaction", err)

		logger.Errorf("Failed to create or update transaction: %v", err.Error())

		return err
	}

	ctxProcessMetadata, spanCreateMetadata := tracer.Start(ctx, "command.create_balance_transaction_operations.create_metadata")
	defer spanCreateMetadata.End()

	err = uc.CreateMetadataAsync(ctxProcessMetadata, logger, tran.Metadata, tran.ID, reflect.TypeOf(transaction.Transaction{}).Name())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateMetadata, "Failed to create metadata on transaction", err)

		logger.Errorf("Failed to create metadata on transaction: %v", err.Error())

		return err
	}

	ctxProcessOperation, spanCreateOperation := tracer.Start(ctx, "command.create_balance_transaction_operations.create_operation")
	defer spanCreateOperation.End()

	logger.Infof("Trying to create new operations")

	for _, oper := range tran.Operations {
		_, err = uc.OperationRepo.Create(ctxProcessOperation, oper)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
				msg := fmt.Sprintf("Skipping to create operation, operation already exists: %v", oper.ID)

				libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, msg, err)

				logger.Warnf(msg)

				continue
			} else {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to create operation", err)

				logger.Errorf("Error creating operation: %v", err)

				return err
			}
		}

		err = uc.CreateMetadataAsync(ctx, logger, oper.Metadata, oper.ID, reflect.TypeOf(operation.Operation{}).Name())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to create metadata on operation", err)

			logger.Errorf("Failed to create metadata on operation: %v", err)

			return err
		}
	}

	go uc.SendTransactionEvents(ctx, tran)

	go uc.RemoveTransactionFromRedisQueue(ctx, logger, data.OrganizationID, data.LedgerID, tran.ID)

	return nil
}

// CreateOrUpdateTransaction func that is responsible to create or update a transaction.
func (uc *UseCase) CreateOrUpdateTransaction(ctx context.Context, logger libLog.Logger, tracer trace.Tracer, t transaction.TransactionQueue) (*transaction.Transaction, error) {
	_, spanCreateTransaction := tracer.Start(ctx, "command.create_balance_transaction_operations.create_transaction")
	defer spanCreateTransaction.End()

	logger.Infof("Trying to create new transaction")

	tran := t.Transaction
	tran.Body = pkgTransaction.Transaction{}

	switch tran.Status.Code {
	case constant.CREATED:
		description := constant.APPROVED
		status := transaction.Status{
			Code:        description,
			Description: &description,
		}

		tran.Status = status
	case constant.PENDING:
		tran.Body = *t.ParseDSL
	}

	_, err := uc.TransactionRepo.Create(ctx, tran)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
			if t.Validate.Pending && (tran.Status.Code == constant.APPROVED || tran.Status.Code == constant.CANCELED) {
				_, err = uc.UpdateTransactionStatus(ctx, tran)
				if err != nil {
					libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateTransaction, "Failed to update transaction", err)

					logger.Warnf("Failed to update transaction with STATUS: %v by ID: %v", tran.Status.Code, tran.ID)

					return nil, err
				}
			}

			logger.Infof("skipping to create transaction, transaction already exists: %v", tran.ID)
		} else {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateTransaction, "Failed to create transaction on repo", err)

			logger.Errorf("Failed to create transaction on repo: %v", err.Error())

			return nil, err
		}
	}

	return tran, nil
}

// CreateMetadataAsync func that create metadata into operations
func (uc *UseCase) CreateMetadataAsync(ctx context.Context, logger libLog.Logger, metadata map[string]any, ID string, collection string) error {
	if metadata != nil {
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

// CreateBTOSync func that create balance transaction operations synchronously
func (uc *UseCase) CreateBTOSync(ctx context.Context, data mmodel.Queue) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_balance_transaction_operations.create_bto_sync")
	defer span.End()

	err := uc.CreateBalanceTransactionOperationsAsync(ctx, data)
	if err != nil {
		logger.Errorf("Failed to create balance transaction operations: %v", err)

		return err
	}

	return nil
}

// RemoveTransactionFromRedisQueue func that remove transaction from redis queue
func (uc *UseCase) RemoveTransactionFromRedisQueue(ctx context.Context, logger libLog.Logger, organizationID, ledgerID uuid.UUID, transactionID string) {
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID)

	if err := uc.RedisRepo.RemoveMessageFromQueue(ctx, transactionKey); err != nil {
		logger.Warnf("err to remove message on redis: %s", err.Error())
	} else {
		logger.Infof("message removed from redis successfully: %s", transactionKey)
	}
}

// SendTransactionToRedisQueue func that send transaction to redis queue
func (uc *UseCase) SendTransactionToRedisQueue(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL pkgTransaction.Transaction, validate *pkgTransaction.Responses, transactionStatus string, transactionDate time.Time) error {
	logger, _, reqId, _ := libCommons.NewTrackingFromContext(ctx)
	transactionKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

	utils.SanitizeAccountAliases(&parserDSL)

	queue := mmodel.TransactionRedisQueue{
		HeaderID:          reqId,
		OrganizationID:    organizationID,
		LedgerID:          ledgerID,
		TransactionID:     transactionID,
		ParserDSL:         parserDSL,
		TTL:               time.Now(),
		Validate:          validate,
		TransactionStatus: transactionStatus,
		TransactionDate:   transactionDate,
	}

	raw, err := json.Marshal(queue)
	if err != nil {
		logger.Errorf("Failed to marshal transaction to json string: %s", err.Error())

		return constant.ErrTransactionBackupCacheMarshalFailed
	}

	err = uc.RedisRepo.AddMessageToQueue(ctx, transactionKey, raw)
	if err != nil {
		logger.Errorf("Failed to send transaction to redis queue: %s", err.Error())

		return constant.ErrTransactionBackupCacheFailed
	}

	return nil
}
