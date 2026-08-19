// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const (
	balanceExecutionLeaseTTL = 300
)

func (handler *TransactionHandler) readTransactionLifecycleOutcome(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
) (*mmodel.TransactionRedisQueue, error) {
	backupKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	raw, err := handler.Command.TransactionRedisRepo.ReadMessageFromQueue(ctx, backupKey)
	if err != nil {
		if err == redislib.Nil {
			return nil, nil
		}

		return nil, fmt.Errorf("read transaction balance outcome: %w", err)
	}

	queued := &mmodel.TransactionRedisQueue{}
	if err := json.Unmarshal(raw, queued); err != nil {
		return nil, fmt.Errorf("decode transaction balance outcome: %w", err)
	}
	if queued.OrganizationID != organizationID || queued.LedgerID != ledgerID ||
		queued.TransactionID != transactionID {
		return nil, fmt.Errorf("transaction balance outcome scope mismatch")
	}
	if queued.Action != constant.ActionCommit && queued.Action != constant.ActionCancel {
		return nil, nil
	}
	if queued.TransactionStatus != constant.APPROVED && queued.TransactionStatus != constant.CANCELED {
		return nil, fmt.Errorf("transaction balance outcome status mismatch")
	}

	return queued, nil
}

// clearDurableLegacyHoldBackup closes the handoff between a synchronously
// persisted legacy hold and its next lifecycle stage. Legacy holds have no
// outcome owner, so their best-effort post-persistence cleanup may still be in
// flight when a client immediately commits or cancels the durable PENDING row.
// Remove only that exact unowned PENDING hold; owned economic evidence and a
// newer terminal-stage backup are never eligible.
func (handler *TransactionHandler) clearDurableLegacyHoldBackup(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
) error {
	backupKey := utils.TransactionInternalKey(organizationID, ledgerID, transactionID.String())
	raw, err := handler.Command.TransactionRedisRepo.ReadMessageFromQueue(ctx, backupKey)
	if err != nil {
		if err == redislib.Nil {
			return nil
		}

		return fmt.Errorf("read durable pending transaction backup: %w", err)
	}

	queued := &mmodel.TransactionRedisQueue{}
	if err := json.Unmarshal(raw, queued); err != nil {
		return fmt.Errorf("decode durable pending transaction backup: %w", err)
	}
	if queued.OrganizationID != organizationID || queued.LedgerID != ledgerID ||
		queued.TransactionID != transactionID {
		return fmt.Errorf("durable pending transaction backup scope mismatch")
	}
	if queued.TransactionStatus != constant.PENDING || queued.Action != constant.ActionHold {
		return nil
	}
	if queued.AttemptOwner != "" || queued.ExpectedOutcome != "" {
		return fmt.Errorf("durable pending transaction still has owned economic evidence")
	}

	_, err = handler.Command.TransactionRedisRepo.RemoveMessageFromQueueIfStatus(
		ctx, backupKey, constant.PENDING, "", "", false,
	)
	if err != nil {
		return fmt.Errorf("clear durable pending transaction backup: %w", err)
	}

	return nil
}

func lifecycleBalanceAtomicResult(queued *mmodel.TransactionRedisQueue) *mmodel.BalanceAtomicResult {
	if queued == nil || len(queued.BalancesAfter) == 0 {
		return nil
	}

	before := make([]*mmodel.Balance, 0, len(queued.Balances))
	for _, balance := range queued.Balances {
		before = append(before, lifecycleBalanceFromBackup(balance, queued.OrganizationID, queued.LedgerID))
	}
	after := make([]*mmodel.Balance, 0, len(queued.BalancesAfter))
	for _, balance := range queued.BalancesAfter {
		after = append(after, lifecycleBalanceFromBackup(balance, queued.OrganizationID, queued.LedgerID))
	}

	return &mmodel.BalanceAtomicResult{Before: before, After: after}
}

func lifecycleBalanceFromBackup(balance mmodel.BalanceRedis, organizationID, ledgerID uuid.UUID) *mmodel.Balance {
	balanceKey := balance.Key
	if balanceKey == "" {
		balanceKey = constant.DefaultBalanceKey
	}
	overdraftUsed, err := decimal.NewFromString(balance.OverdraftUsed)
	if err != nil {
		overdraftUsed = decimal.Zero
	}

	var settings *mmodel.BalanceSettings
	if balance.AllowOverdraft != 0 || balance.OverdraftLimitEnabled != 0 ||
		(balance.BalanceScope != "" && balance.BalanceScope != mmodel.BalanceScopeTransactional) ||
		(balance.OverdraftLimit != "" && balance.OverdraftLimit != "0") {
		scope := balance.BalanceScope
		if scope == "" {
			scope = mmodel.BalanceScopeTransactional
		}
		settings = &mmodel.BalanceSettings{
			BalanceScope:          scope,
			AllowOverdraft:        balance.AllowOverdraft == 1,
			OverdraftLimitEnabled: balance.OverdraftLimitEnabled == 1,
		}
		if balance.OverdraftLimitEnabled == 1 && balance.OverdraftLimit != "" {
			limit := balance.OverdraftLimit
			settings.OverdraftLimit = &limit
		}
	}

	return &mmodel.Balance{
		Alias:          balance.Alias,
		ID:             balance.ID,
		AccountID:      balance.AccountID,
		Key:            balanceKey,
		Available:      balance.Available,
		OnHold:         balance.OnHold,
		Version:        balance.Version,
		AccountType:    balance.AccountType,
		AllowSending:   balance.AllowSending == 1,
		AllowReceiving: balance.AllowReceiving == 1,
		AssetCode:      balance.AssetCode,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Direction:      balance.Direction,
		OverdraftUsed:  overdraftUsed,
		Settings:       settings,
	}
}

func lifecycleOutcomeMatchesTarget(queued *mmodel.TransactionRedisQueue, status string) bool {
	if queued == nil || queued.TransactionStatus != status {
		return false
	}

	return (status == constant.APPROVED && queued.Action == constant.ActionCommit) ||
		(status == constant.CANCELED && queued.Action == constant.ActionCancel)
}

func (handler *TransactionHandler) readBalanceExecutionOutcome(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
) (*mmodel.BalanceExecutionOutcome, error) {
	raw, err := handler.Command.TransactionRedisRepo.Get(ctx,
		utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID))
	if err != nil {
		return nil, fmt.Errorf("read balance execution outcome: %w", err)
	}
	if raw == "" {
		return nil, nil
	}

	outcome := &mmodel.BalanceExecutionOutcome{}
	if err := json.Unmarshal([]byte(raw), outcome); err != nil {
		return nil, fmt.Errorf("decode balance execution outcome: %w", err)
	}
	if outcome.Identity != transactionID || outcome.Owner == "" ||
		(outcome.Outcome != mmodel.TransactionOutcomeCommitted && outcome.Outcome != mmodel.TransactionOutcomeAborted) {
		return nil, fmt.Errorf("balance execution outcome identity mismatch")
	}

	return outcome, nil
}

func balanceAtomicResultFromOutcome(outcome *mmodel.BalanceExecutionOutcome, organizationID, ledgerID uuid.UUID) *mmodel.BalanceAtomicResult {
	if outcome == nil || len(outcome.After) == 0 {
		return nil
	}

	queued := &mmodel.TransactionRedisQueue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Balances:       outcome.Before,
		BalancesAfter:  outcome.After,
	}

	return lifecycleBalanceAtomicResult(queued)
}

func economicOutcomeForStatus(status string) string {
	if status == constant.CANCELED {
		return mmodel.TransactionOutcomeAborted
	}

	return mmodel.TransactionOutcomeCommitted
}

func transactionLifecycleReconciliationError() error {
	return pkg.ValidateBusinessError(constant.ErrTransactionOutcomeReconciliationRequired, constant.EntityTransaction)
}
