// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// preflightOutcomeBackedTransaction binds every transported money outcome to
// the immutable Redis envelope before any PostgreSQL or balance write. The
// deployment generation strengthens this proof when present; it never decides
// whether the proof is required.
func (uc *UseCase) preflightOutcomeBackedTransaction(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	payload *transaction.TransactionProcessingPayload,
) (outcomeBacked, terminal bool, err error) {
	effectMode, err := validateProcessingPayloadEffectMode(organizationID, ledgerID, payload)
	if err != nil {
		return false, false, err
	}
	attempt, outcomeBacked, err := outcomeBackedAttempt(organizationID, ledgerID, payload)
	if err != nil {
		return outcomeBacked, false, err
	}
	reverseBacked := payload != nil && payload.Transaction != nil && payload.Transaction.ParentTransactionID != nil
	annotationBacked := effectMode == mmodel.TransactionEffectAnnotationOnly
	if !outcomeBacked && !reverseBacked && !annotationBacked {
		return false, false, nil
	}
	if uc.TransactionRedisRepo == nil {
		return outcomeBacked, false, fmt.Errorf("transaction economic evidence repository is required")
	}

	transactionID := payload.Transaction.IDtoUUID()
	var parentTransactionID *uuid.UUID
	if payload.Transaction.ParentTransactionID != nil {
		parsedParent, parseErr := uuid.Parse(*payload.Transaction.ParentTransactionID)
		if parseErr != nil || parsedParent == uuid.Nil {
			return outcomeBacked, false, fmt.Errorf("transaction economic parent identity is invalid")
		}
		parentTransactionID = &parsedParent
	}
	canonicalOperations, terminal, err := uc.UpdateTransactionBackupOperations(
		ctx,
		organizationID,
		ledgerID,
		transactionID,
		payload.Transaction.Operations,
		mmodel.BalancesToRedis(payload.BalancesAfter),
		actionForTransactionPayload(*payload),
		attempt,
		mmodel.TransactionEconomicContext{
			ParentTransactionID:  parentTransactionID,
			TransactionStatus:    utils.ExpectedBackupStatusForCleanup(payload.Transaction.Status.Code, payload.Validate),
			TransactionAmount:    payload.Input.Send.Value.String(),
			TransactionAssetCode: payload.Input.Send.Asset,
		},
	)
	if err != nil {
		return outcomeBacked, false, err
	}
	payload.Transaction.Operations = canonicalOperations

	return outcomeBacked, terminal, nil
}

func validateProcessingPayloadEffectMode(
	organizationID, ledgerID uuid.UUID,
	payload *transaction.TransactionProcessingPayload,
) (mmodel.TransactionEffectMode, error) {
	if payload == nil || payload.Transaction == nil {
		return "", fmt.Errorf("transaction persistence payload is required")
	}
	transactionID, err := uuid.Parse(payload.Transaction.ID)
	if err != nil || transactionID == uuid.Nil {
		return "", fmt.Errorf("transaction persistence identity is invalid")
	}
	queue := mmodel.TransactionRedisQueue{
		TransactionID:         transactionID,
		OrganizationID:        organizationID,
		LedgerID:              ledgerID,
		TransactionStatus:     payload.Transaction.Status.Code,
		EffectModeVersion:     payload.EffectModeVersion,
		EffectMode:            payload.EffectMode,
		OperationTypeOverride: payload.OperationTypeOverride,
		Validate:              payload.Validate,
		AttemptOwner:          payload.AttemptOwner,
		ExpectedOutcome:       payload.ExpectedOutcome,
		Balances:              mmodel.BalancesToRedis(payload.Balances),
		BalancesAfter:         mmodel.BalancesToRedis(payload.BalancesAfter),
	}
	if payload.Input != nil {
		queue.TransactionInput = *payload.Input
	}
	mode, err := mmodel.ResolveTransactionEffectMode(&queue)
	if err != nil {
		return "", fmt.Errorf("resolve transaction persistence effect mode: %w", err)
	}
	requiresEconomicIdentity := mode == mmodel.TransactionEffectAnnotationOnly ||
		payload.EffectModeVersion != 0 || payload.EffectMode != "" ||
		payload.AttemptOwner != "" || payload.ExpectedOutcome != "" ||
		payload.RedisGeneration != "" || payload.Transaction.ParentTransactionID != nil
	if !requiresEconomicIdentity {
		return mode, nil
	}
	if payload.Input == nil || payload.Transaction.Amount == nil {
		return "", fmt.Errorf("transaction persistence amount and immutable input are required")
	}
	if !payload.Input.Send.Value.IsPositive() || !payload.Transaction.Amount.IsPositive() {
		return "", fmt.Errorf("transaction persistence amount must be positive")
	}
	if !payload.Transaction.Amount.Equal(payload.Input.Send.Value) {
		return "", fmt.Errorf("transaction persistence amount differs from immutable input")
	}
	if payload.Input.Send.Asset == "" || payload.Transaction.AssetCode != payload.Input.Send.Asset {
		return "", fmt.Errorf("transaction persistence asset differs from immutable input")
	}
	if mode != mmodel.TransactionEffectAnnotationOnly {
		return mode, nil
	}
	operations := make([]mmodel.OperationRedis, 0, len(payload.Transaction.Operations))
	for _, candidate := range payload.Transaction.Operations {
		if candidate == nil {
			return "", fmt.Errorf("transaction annotation operation is required")
		}
		operations = append(operations, candidate.ToRedis())
	}
	if err := mmodel.ValidateRedisTransactionAnnotationEffect(&queue, operations); err != nil {
		return "", fmt.Errorf("prove transaction annotation event: %w", err)
	}

	return mode, nil
}

func outcomeBackedAttempt(
	organizationID, ledgerID uuid.UUID,
	payload *transaction.TransactionProcessingPayload,
) (*mmodel.BalanceExecutionAttempt, bool, error) {
	if payload == nil {
		return nil, false, fmt.Errorf("transaction persistence payload is required")
	}
	hasOwner := payload.AttemptOwner != ""
	hasOutcome := payload.ExpectedOutcome != ""
	if !hasOwner && !hasOutcome {
		if payload.RedisGeneration != "" {
			return nil, false, fmt.Errorf("transaction Redis generation has no economic outcome owner")
		}

		return nil, false, nil
	}
	if !hasOwner || !hasOutcome {
		return nil, true, fmt.Errorf("incomplete transaction balance execution identity")
	}
	if payload.ExpectedOutcome != mmodel.TransactionOutcomeCommitted &&
		payload.ExpectedOutcome != mmodel.TransactionOutcomeAborted {
		return nil, true, fmt.Errorf("transaction balance execution outcome is not terminal")
	}
	if payload.Transaction == nil {
		return nil, true, fmt.Errorf("outcome-backed transaction envelope is required")
	}
	transactionID, err := uuid.Parse(payload.Transaction.ID)
	if err != nil || transactionID == uuid.Nil {
		return nil, true, fmt.Errorf("outcome-backed transaction identity is invalid")
	}

	executionKey := utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID)

	outcomeKey := utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID)
	if payload.Action == constant.ActionHold {
		executionKey = utils.TransactionPendingBalanceExecutionKey(organizationID, ledgerID, transactionID)
		outcomeKey = utils.TransactionPendingBalanceOutcomeKey(organizationID, ledgerID, transactionID)
	}

	return &mmodel.BalanceExecutionAttempt{
		ExecutionKey:    executionKey,
		OutcomeKey:      outcomeKey,
		Owner:           payload.AttemptOwner,
		Outcome:         payload.ExpectedOutcome,
		Identity:        transactionID,
		RedisGeneration: payload.RedisGeneration,
		Action:          payload.Action,
	}, true, nil
}
