package command

import (
	"context"
	"os"
	"reflect"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

// TransactionExecute func that send balances, transaction and operations to execute sync/async.
func (uc *UseCase) TransactionExecute(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *pkgTransaction.Transaction, validate *pkgTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		return uc.SendBTOExecuteAsync(ctx, organizationID, ledgerID, parseDSL, validate, blc, tran)
	}

	return uc.CreateBTOExecuteSync(ctx, organizationID, ledgerID, parseDSL, validate, blc, tran)
}

// SendBTOExecuteAsync func that send balances, transaction and operations to a queue to execute async.
func (uc *UseCase) SendBTOExecuteAsync(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *pkgTransaction.Transaction, validate *pkgTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

		return pkg.ValidateInternalError(err, reflect.TypeOf(transaction.Transaction{}).Name())
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

	message, err := msgpack.Marshal(queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanSendBTOQueue, "Failed to marshal exchange message struct", err)

		logger.Errorf("Failed to marshal exchange message struct")

		return pkg.ValidateInternalError(err, reflect.TypeOf(transaction.Transaction{}).Name())
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
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanSendBTOQueue, "Failed to send message directly to database", err)

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
func (uc *UseCase) CreateBTOExecuteSync(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *pkgTransaction.Transaction, validate *pkgTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

		return pkg.ValidateInternalError(err, reflect.TypeOf(transaction.Transaction{}).Name())
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

	err = uc.CreateBalanceTransactionOperationsAsync(ctxSendBTODirect, queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanSendBTODirect, "Failed to send message directly to database", err)

		logger.Errorf("Failed to send message directly to database: %s", err.Error())

		return err
	}

	logger.Infof("Transaction updated successfully directly in database: %s", tran.ID)

	return nil
}
