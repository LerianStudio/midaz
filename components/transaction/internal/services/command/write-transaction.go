// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"os"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

// WriteTransaction routes the transaction to sync or async execution
// based on the RABBITMQ_TRANSACTION_ASYNC environment variable.
func (uc *UseCase) WriteTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		return uc.WriteTransactionAsync(ctx, organizationID, ledgerID, transactionInput, validate, blc, tran)
	} else {
		return uc.WriteTransactionSync(ctx, organizationID, ledgerID, transactionInput, validate, blc, tran)
	}
}

// WriteTransactionAsync publishes the transaction payload to RabbitMQ
// for asynchronous processing. Falls back to direct DB write if queue fails.
func (uc *UseCase) WriteTransactionAsync(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.write_transaction_async")
	defer span.End()

	queueData := make([]mmodel.QueueData, 0, 1)

	value := transaction.TransactionProcessingPayload{
		Validate:    validate,
		Balances:    blc,
		Transaction: tran,
		Input:       transactionInput,
	}

	marshal, err := msgpack.Marshal(value)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal transaction to JSON string", err)

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

	message, err := msgpack.Marshal(queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal exchange message struct", err)

		logger.Errorf("Failed to marshal exchange message struct")

		return err
	}

	// ProducerDefaultWithContext handles scoped timeout internally.
	// If it fails, we fall back to direct DB write using the original context
	// which still has remaining HTTP timeout.
	if _, err := uc.RabbitMQRepo.ProducerDefaultWithContext(
		ctx,
		os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE"),
		os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY"),
		message,
	); err != nil {
		logger.Warnf("Failed to send message to queue: %s", err.Error())

		logger.Infof("Trying to send message directly to database: %s", tran.ID)

		// Use original context for fallback - it still has remaining HTTP timeout
		err = uc.CreateBalanceTransactionOperationsAsync(ctx, queueMessage)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to send message directly to database", err)

			logger.Errorf("Failed to send message directly to database: %s", err.Error())

			return err
		}

		logger.Infof("transaction updated successfully directly to database: %s", tran.ID)

		return nil
	}

	logger.Infof("Transaction send successfully to queue: %s", tran.ID)

	return nil
}

// WriteTransactionSync performs direct database writes for balance updates,
// transaction record creation, and operation records.
func (uc *UseCase) WriteTransactionSync(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.write_transaction_sync")
	defer span.End()

	queueData := make([]mmodel.QueueData, 0, 1)

	value := transaction.TransactionProcessingPayload{
		Validate:    validate,
		Balances:    blc,
		Transaction: tran,
		Input:       transactionInput,
	}

	marshal, err := msgpack.Marshal(value)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal transaction to JSON string", err)

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

	err = uc.CreateBalanceTransactionOperationsAsync(ctx, queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to send message directly to database", err)

		logger.Errorf("Failed to send message directly to database: %s", err.Error())

		return err
	}

	logger.Infof("Transaction updated successfully directly in database: %s", tran.ID)

	return nil
}
