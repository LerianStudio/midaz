// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strconv"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

// RELEASE NOTE: TransactionAsync, RabbitMQBalanceOperationExchange, and
// RabbitMQBalanceOperationKey are now resolved once at startup (not per-request).
// Changing these env vars requires a pod restart to take effect.

// WriteTransaction routes the transaction to sync or async execution
// based on the TransactionAsync field (resolved once at startup from RABBITMQ_TRANSACTION_ASYNC).
func (uc *UseCase) WriteTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	if uc.TransactionAsync {
		return uc.WriteTransactionAsync(ctx, organizationID, ledgerID, transactionInput, validate, blc, tran)
	}

	return uc.WriteTransactionSync(ctx, organizationID, ledgerID, transactionInput, validate, blc, tran)
}

// WriteTransactionAsync publishes the transaction payload to RabbitMQ
// for asynchronous processing. Falls back to direct DB write if queue fails.
func (uc *UseCase) WriteTransactionAsync(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

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

	exchange := uc.RabbitMQBalanceOperationExchange
	routingKey := resolveBTORoutingKey(uc.RabbitMQBalanceOperationKey, tran.IDtoUUID(), uc.ShardRouter, uc.ShardedBTOQueuesEnabled)

	if uc.Authorizer != nil && uc.Authorizer.Enabled() {
		headers := map[string]string{}
		if reqID != "" {
			headers[libConstants.HeaderID] = reqID
		}

		publishErr := uc.Authorizer.PublishBalanceOperations(ctx, exchange, routingKey, message, headers)
		if publishErr == nil {
			logger.Infof("Transaction sent successfully via authorizer publisher: %s", tran.ID)
			return nil
		}

		logger.Warnf("Failed to publish via authorizer for transaction %s: %v. Falling back to local RabbitMQ producer", tran.ID, publishErr)
	}

	// ProducerDefaultWithContext handles scoped timeout internally.
	// If it fails, we fall back to direct DB write using the original context
	// which still has remaining HTTP timeout.
	if _, err := uc.RabbitMQRepo.ProducerDefaultWithContext(
		ctx,
		exchange,
		routingKey,
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

func resolveBTORoutingKey(baseKey string, transactionID uuid.UUID, shardRouter *shard.Router, shardedEnabled bool) string {
	if !IsShardedBTOQueueEnabled(shardRouter, shardedEnabled) {
		return baseKey
	}

	shardID := shardRouter.Resolve(transactionID.String())

	return baseKey + ".shard_" + strconv.Itoa(shardID)
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
