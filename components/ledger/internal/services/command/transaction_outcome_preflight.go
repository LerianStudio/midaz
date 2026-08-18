// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
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
	attempt, outcomeBacked, err := outcomeBackedAttempt(organizationID, ledgerID, payload)
	if err != nil || !outcomeBacked {
		return outcomeBacked, false, err
	}
	if uc.TransactionRedisRepo == nil {
		return true, false, fmt.Errorf("transaction economic evidence repository is required")
	}

	transactionID := payload.Transaction.IDtoUUID()
	canonicalOperations, terminal, err := uc.UpdateTransactionBackupOperations(
		ctx,
		organizationID,
		ledgerID,
		transactionID,
		payload.Transaction.Operations,
		mmodel.BalancesToRedis(payload.BalancesAfter),
		actionForTransactionPayload(*payload),
		attempt,
	)
	if err != nil {
		return true, false, err
	}
	payload.Transaction.Operations = canonicalOperations

	return true, terminal, nil
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

	return &mmodel.BalanceExecutionAttempt{
		ExecutionKey:    utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID),
		OutcomeKey:      utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID),
		Owner:           payload.AttemptOwner,
		Outcome:         payload.ExpectedOutcome,
		Identity:        transactionID,
		RedisGeneration: payload.RedisGeneration,
	}, true, nil
}
