package command

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"strings"
)

// TransactionExecute func that send balances, transaction and operations to execute sync/async.
func (uc *UseCase) TransactionExecute(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *libTransaction.Transaction, validate *libTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION")) == "true" {
		return uc.SendBTOExecuteAsync(ctx, organizationID, ledgerID, parseDSL, validate, blc, tran)
	} else {
		return uc.CreateBTOExecuteSync(ctx, organizationID, ledgerID, parseDSL, validate, blc, tran)
	}
}

// SendBTOExecuteAsync func that send balances, transaction and operations to a queue to execute async.
func (uc *UseCase) SendBTOExecuteAsync(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *libTransaction.Transaction, validate *libTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctxSendBTOQueue, spanSendBTOQueue := tracer.Start(ctx, "command.send_bto_execute_async")
	defer spanSendBTOQueue.End()

	queueData := make([]mmodel.QueueData, 0)

	value := transaction.TransactionQueue{
		Validate:    validate,
		Balances:    blc,
		Transaction: tran,
		ParseDSL:    parseDSL,
	}

	marshal, err := msgpack.Marshal(value)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanSendBTOQueue, "Failed to marshal transaction to JSON string", err)

		logger.Errorf("Failed to marshal validate to JSON string: %s", err.Error())

		return err
	}

	queueData = append(queueData, mmodel.QueueData{
		ID:    tran.IDtoUUID(),
		Value: marshal,
	})

	queueMessage := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData:      queueData,
	}

	redisMessage := redis.RedisMessage{
		ID:        tran.ID,
		Payload:   queueMessage,
		Timestamp: tran.CreatedAt.Unix(),
		Status:    constant.PENDING,
	}

	err = uc.RedisRepo.AddMessageToQueue(ctx, redisMessage)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanSendBTOQueue, "Failed to send BTO to redis backup queue", err)

		logger.Errorf("Failed to send message to redis backup queue: %s", err.Error())

		return err
	}

	message, err := msgpack.Marshal(queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanSendBTOQueue, "Failed to marshal exchange message struct", err)

		logger.Errorf("Failed to marshal exchange message struct")

		return err
	}

	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctxSendBTOQueue,
		os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE"),
		os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY"),
		message,
	); err != nil {
		logger.Warnf("Failed to send message to queue: %s", err.Error())

		logger.Infof("Trying to send message directly to database: %s", tran.ID)

		err = uc.CreateBalanceTransactionOperationsAsync(ctxSendBTOQueue, queueMessage)
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanSendBTOQueue, "Failed to send message directly to database", err)

			logger.Errorf("Failed to send message directly to database: %s", err.Error())

			return err
		}

		logger.Infof("transaction updated successfully directly to database: %s", tran.ID)

		return nil
	}

	logger.Infof("Transaction send successfully to queue: %s", tran.ID)

	return nil
}

// CreateBTOExecuteSync func that send balances, transaction and operations to execute in database sync.
func (uc *UseCase) CreateBTOExecuteSync(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *libTransaction.Transaction, validate *libTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctxSendBTODirect, spanSendBTODirect := tracer.Start(ctx, "command.create_bto_execute_sync")
	defer spanSendBTODirect.End()

	queueData := make([]mmodel.QueueData, 0)

	value := transaction.TransactionQueue{
		Validate:    validate,
		Balances:    blc,
		Transaction: tran,
		ParseDSL:    parseDSL,
	}

	marshal, err := msgpack.Marshal(value)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanSendBTODirect, "Failed to marshal transaction to JSON string", err)

		logger.Errorf("Failed to marshal validate to JSON string: %s", err.Error())

		return err
	}

	queueData = append(queueData, mmodel.QueueData{
		ID:    tran.IDtoUUID(),
		Value: marshal,
	})

	queueMessage := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData:      queueData,
	}

	redisMessage := redis.RedisMessage{
		ID:        tran.ID,
		Payload:   queueMessage,
		Timestamp: tran.CreatedAt.Unix(),
		Status:    constant.PENDING,
	}

	err = uc.RedisRepo.AddMessageToQueue(ctx, redisMessage)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanSendBTODirect, "Failed to send BTO to redis backup queue", err)

		logger.Errorf("Failed to send message to redis backup queue: %s", err.Error())

		return err
	}

	err = uc.CreateBalanceTransactionOperationsAsync(ctxSendBTODirect, queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanSendBTODirect, "Failed to send message directly to database", err)

		logger.Errorf("Failed to send message directly to database: %s", err.Error())

		return err
	}

	logger.Infof("Transaction updated successfully directly in database: %s", tran.ID)

	return nil
}
