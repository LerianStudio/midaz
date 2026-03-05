// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

var errTransactionNil = errors.New("transaction is nil")

// RELEASE NOTE: TransactionAsync and BalanceOperationsTopic are resolved once at startup (not per-request).
// Changing these env vars requires a pod restart to take effect.

// WriteTransaction routes the transaction to sync or async execution.
func (uc *UseCase) WriteTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	if tran == nil {
		return errTransactionNil
	}

	if uc.TransactionAsync {
		return uc.WriteTransactionAsync(ctx, organizationID, ledgerID, transactionInput, validate, blc, tran)
	}

	return uc.WriteTransactionSync(ctx, organizationID, ledgerID, transactionInput, validate, blc, tran)
}

// WriteTransactionAsync publishes the transaction payload to Redpanda
// for asynchronous processing. Falls back to direct DB write if queue fails.
func (uc *UseCase) WriteTransactionAsync(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) (retErr error) {
	if tran == nil {
		return errTransactionNil
	}

	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.write_transaction_async")
	defer span.End()

	decisionContract := pkgTransaction.DefaultDecisionContract()
	emitDecisionEvent := func(action pkgTransaction.DecisionLifecycleAction) {
		uc.dispatchDecisionLifecycleEvent(ctx, tran, decisionContract, action, defaultDispatchTimeout)
	}

	emitDecisionEvent(pkgTransaction.DecisionLifecycleActionAuthorizationRequested)

	authorizationApproved := false

	defer func() {
		if retErr != nil && !authorizationApproved {
			emitDecisionEvent(pkgTransaction.DecisionLifecycleActionAuthorizationDeclined)
		}
	}()

	queueMessage, message, err := marshalTransactionQueue(organizationID, ledgerID, decisionContract, transactionInput, validate, blc, tran)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal transaction queue payload", err)

		logger.Errorf("Failed to marshal transaction queue payload: %s", err)

		return err
	}

	topic := uc.BalanceOperationsTopic

	partitionKey := tran.IDtoUUID().String()
	if uc.ShardRouter != nil {
		primaryAlias, primaryBalanceKey := extractPrimaryDebitRoute(tran)
		if primaryAlias != "" {
			partitionKey = strconv.Itoa(uc.ShardRouter.ResolveBalance(primaryAlias, primaryBalanceKey))
		}
	}

	if uc.Authorizer != nil && uc.Authorizer.Enabled() {
		headers := map[string]string{}
		if reqID != "" {
			headers[libConstants.HeaderID] = reqID
		}

		publishErr := uc.Authorizer.PublishBalanceOperations(ctx, topic, partitionKey, message, headers)
		if publishErr == nil {
			logger.Infof("Transaction sent successfully via authorizer publisher: %s", tran.ID)

			authorizationApproved = true

			emitDecisionEvent(pkgTransaction.DecisionLifecycleActionAuthorizationApproved)

			return nil
		}

		logger.Warnf("Failed to publish via authorizer for transaction %s: %v. Falling back to local broker producer", tran.ID, publishErr)
	}

	// ProducerDefaultWithContext handles scoped timeout internally.
	// If it fails, we fall back to direct DB write using the original context
	// which still has remaining HTTP timeout.
	if _, err := uc.BrokerRepo.ProducerDefaultWithContext(
		ctx,
		topic,
		partitionKey,
		message,
	); err != nil {
		logger.Warnf("Failed to send message to queue: %s", err)

		logger.Infof("Trying to send message directly to database: %s", tran.ID)

		authorizationApproved = true

		emitDecisionEvent(pkgTransaction.DecisionLifecycleActionAuthorizationApproved)

		// Use original context for fallback - it still has remaining HTTP timeout
		err = uc.CreateBalanceTransactionOperationsAsync(ctx, queueMessage)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to send message directly to database", err)

			logger.Errorf("Failed to send message directly to database: %s", err)

			return err
		}

		logger.Infof("transaction updated successfully directly to database: %s", tran.ID)

		return nil
	}

	logger.Infof("Transaction sent successfully to broker topic=%s id=%s", topic, tran.ID)

	authorizationApproved = true

	emitDecisionEvent(pkgTransaction.DecisionLifecycleActionAuthorizationApproved)

	return nil
}

// marshalTransactionQueue builds the transaction processing payload and queue
// message, returning both the structured queue and its msgpack-serialized form.
func marshalTransactionQueue(
	organizationID, ledgerID uuid.UUID,
	decisionContract pkgTransaction.DecisionContract,
	transactionInput *pkgTransaction.Transaction,
	validate *pkgTransaction.Responses,
	blc []*mmodel.Balance,
	tran *transaction.Transaction,
) (mmodel.Queue, []byte, error) {
	value := transaction.TransactionProcessingPayload{
		DecisionContract: decisionContract,
		Validate:         validate,
		Balances:         blc,
		Transaction:      tran,
		Input:            transactionInput,
	}

	marshal, err := msgpack.Marshal(value)
	if err != nil {
		return mmodel.Queue{}, nil, fmt.Errorf("failed to marshal transaction payload: %w", err)
	}

	queueData := []mmodel.QueueData{{
		ID:    tran.IDtoUUID(),
		Value: marshal,
	}}

	queueMessage := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData:      queueData,
	}

	message, err := msgpack.Marshal(queueMessage)
	if err != nil {
		return mmodel.Queue{}, nil, fmt.Errorf("failed to marshal queue message: %w", err)
	}

	return queueMessage, message, nil
}

func extractPrimaryDebitRoute(tran *transaction.Transaction) (string, string) {
	if tran == nil || len(tran.Operations) == 0 {
		return "", ""
	}

	for _, op := range tran.Operations {
		if op == nil {
			continue
		}

		alias := shard.ExtractAccountAlias(op.AccountAlias)
		if alias == "" {
			continue
		}

		balanceKey := op.BalanceKey
		if balanceKey == "" {
			_, balanceKey = shard.SplitAliasAndBalanceKey(op.AccountAlias)
		}

		if op.Type == constant.DEBIT && !shard.IsExternal(alias) {
			return alias, balanceKey
		}
	}

	for _, op := range tran.Operations {
		if op == nil {
			continue
		}

		alias := shard.ExtractAccountAlias(op.AccountAlias)
		if alias != "" {
			balanceKey := op.BalanceKey
			if balanceKey == "" {
				_, balanceKey = shard.SplitAliasAndBalanceKey(op.AccountAlias)
			}

			return alias, balanceKey
		}
	}

	return "", ""
}

// WriteTransactionSync performs direct database writes for balance updates,
// transaction record creation, and operation records.
func (uc *UseCase) WriteTransactionSync(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionInput *pkgTransaction.Transaction, validate *pkgTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) (retErr error) {
	if tran == nil {
		return errTransactionNil
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.write_transaction_sync")
	defer span.End()

	decisionContract := pkgTransaction.DefaultDecisionContract()

	emitDecisionEvent := func(action pkgTransaction.DecisionLifecycleAction) {
		uc.dispatchDecisionLifecycleEvent(ctx, tran, decisionContract, action, defaultDispatchTimeout)
	}

	emitDecisionEvent(pkgTransaction.DecisionLifecycleActionAuthorizationRequested)

	authorizationApproved := false

	defer func() {
		if retErr != nil && !authorizationApproved {
			emitDecisionEvent(pkgTransaction.DecisionLifecycleActionAuthorizationDeclined)
		}
	}()

	queueMessage, _, err := marshalTransactionQueue(organizationID, ledgerID, decisionContract, transactionInput, validate, blc, tran)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal transaction queue payload", err)

		logger.Errorf("Failed to marshal transaction queue payload: %s", err)

		return err
	}

	authorizationApproved = true

	emitDecisionEvent(pkgTransaction.DecisionLifecycleActionAuthorizationApproved)

	err = uc.CreateBalanceTransactionOperationsAsync(ctx, queueMessage)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to send message directly to database", err)

		logger.Errorf("Failed to send message directly to database: %s", err)

		return err
	}

	logger.Infof("Transaction updated successfully directly in database: %s", tran.ID)

	return nil
}
