package command

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
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

	// New order: create transaction and then atomically insert all operations and apply balance deltas

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

	logger.Infof("Creating operations and applying balance deltas atomically")

	if repo, ok := uc.OperationRepo.(*operation.OperationPostgreSQLRepository); ok {
		applyBalances := t.Transaction.Status.Code != constant.NOTED
		if err := repo.CreateManyAndApplyDeltasAtomic(ctxProcessOperation, data.OrganizationID, data.LedgerID, applyBalances, tran.Operations); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to create operations and apply deltas atomically", err)
			logger.Errorf("Failed to create operations and apply deltas atomically: %v", err)

			return err
		}
	} else {
		// Fallback (should not happen in production): preserve old behavior but warn lack of atomicity
		logger.Warnf("OperationRepo is not PostgreSQL repository; falling back to non-atomic path")

		// Track deltas only for operations actually inserted now
		deltas := make(map[string]mmodel.BalanceDelta)

		for _, oper := range tran.Operations {
			created := true

			if _, err = uc.OperationRepo.Create(ctxProcessOperation, oper); err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == constant.UniqueViolationCode {
					created = false
				} else {
					libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to create operation", err)
					logger.Errorf("Error creating operation: %v", err)

					return err
				}
			}

			if created {
				if oper.BalanceAffected && oper.Balance.Available != nil && oper.BalanceAfter.Available != nil && oper.Balance.OnHold != nil && oper.BalanceAfter.OnHold != nil {
					da := oper.BalanceAfter.Available.Sub(*oper.Balance.Available)
					dh := oper.BalanceAfter.OnHold.Sub(*oper.Balance.OnHold)
					bd := deltas[oper.BalanceID]
					bd.ID = oper.BalanceID
					bd.DeltaAvailable = bd.DeltaAvailable.Add(da)
					bd.DeltaOnHold = bd.DeltaOnHold.Add(dh)
					deltas[oper.BalanceID] = bd
				}
			}
		}

		if t.Transaction.Status.Code != constant.NOTED && len(deltas) > 0 {
			list := make([]mmodel.BalanceDelta, 0, len(deltas))
			for _, v := range deltas {
				list = append(list, v)
			}

			if err := uc.BalanceRepo.BalancesIncrement(ctx, data.OrganizationID, data.LedgerID, list); err != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateOperation, "Failed to increment balances", err)
				logger.Errorf("Failed to increment balances: %v", err)

				return err
			}
		}
	}

	// Operation metadata is idempotent and can be created after DB atomics
	for _, oper := range tran.Operations {
		if err := uc.CreateMetadataAsync(ctx, logger, oper.Metadata, oper.ID, reflect.TypeOf(operation.Operation{}).Name()); err != nil {
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
	tran.Body = libTransaction.Transaction{}

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
		if err := libCommons.CheckMetadataKeyAndValueLength(100, metadata); err != nil {
			logger.Errorf("Error checking metadata key and value length: %v", err)

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
	transactionKey := libCommons.TransactionInternalKey(organizationID, ledgerID, transactionID)

	if err := uc.RedisRepo.RemoveMessageFromQueue(ctx, transactionKey); err != nil {
		logger.Warnf("err to remove message on redis: %s", err.Error())
	} else {
		logger.Infof("message removed from redis successfully: %s", transactionKey)
	}
}

// SendTransactionToRedisQueue func that send transaction to redis queue
func (uc *UseCase) SendTransactionToRedisQueue(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL libTransaction.Transaction, validate *libTransaction.Responses, transactionStatus string, transactionDate time.Time) {
	logger, _, reqId, _ := libCommons.NewTrackingFromContext(ctx)
	transactionKey := libCommons.TransactionInternalKey(organizationID, ledgerID, transactionID.String())

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
		logger.Warnf("Failed to marshal transaction to json string: %s", err.Error())
	}

	err = uc.RedisRepo.AddMessageToQueue(ctx, transactionKey, raw)
	if err != nil {
		logger.Warnf("Failed to send transaction to redis queue: %s", err.Error())
	}
}
