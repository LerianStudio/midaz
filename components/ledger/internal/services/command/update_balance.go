// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	// UpdateBalances persists balance updates to PostgreSQL.
	// When balancesAfter is non-empty, it uses the Lua-computed AFTER states directly (primary path).
	// When balancesAfter is empty (legacy payloads during rolling update), it falls back to
	// recalculating via OperateBalances for backward compatibility.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) UpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate mtransaction.Responses, balances []*mmodel.Balance, balancesAfter []*mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances_new")
	defer spanUpdateBalances.End()

	newBalances := make([]*mmodel.Balance, 0, len(balances))

	if len(balancesAfter) > 0 {
		// Primary path: use Lua's AFTER state directly — no recalculation
		afterMap := make(map[string]*mmodel.Balance, len(balancesAfter))
		for _, b := range balancesAfter {
			afterMap[b.Alias] = b
		}

		for _, balance := range balances {
			after, ok := afterMap[balance.Alias]
			if !ok {
				err := fmt.Errorf("missing AFTER state for alias %s", balance.Alias)
				spanUpdateBalances.SetAttributes(
					attribute.String("balances.missing_after_alias", balance.Alias),
					attribute.String("balances.failure_reason", "missing_after_state"),
				)
				libOpentelemetry.HandleSpanError(spanUpdateBalances, "Incomplete AFTER state payload", err)
				logger.Log(ctx, libLog.LevelError, err.Error())

				return err
			}

			newBalances = append(newBalances, &mmodel.Balance{
				ID:        balance.ID,
				Alias:     balance.Alias,
				Available: after.Available,
				OnHold:    after.OnHold,
				Version:   after.Version,
			})
		}
	} else {
		// Fallback path: recalculate via OperateBalances (rolling update compatibility)
		fromTo := make(map[string]mtransaction.Amount, len(validate.From)+len(validate.To))
		for k, v := range validate.From {
			fromTo[k] = v
		}

		for k, v := range validate.To {
			fromTo[k] = v
		}

		for _, balance := range balances {
			_, spanBalance := tracer.Start(ctx, "command.update_balances_new.balance")

			calculateBalances, err := mtransaction.OperateBalances(fromTo[balance.Alias], *balance.ToTransactionBalance())
			if err != nil {
				libOpentelemetry.HandleSpanError(spanUpdateBalances, "Failed to update balances on database", err)
				logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update balances on database: %v", err.Error()))
				spanBalance.End()

				return err
			}

			newBalances = append(newBalances, &mmodel.Balance{
				ID:        balance.ID,
				Alias:     balance.Alias,
				Available: calculateBalances.Available,
				OnHold:    calculateBalances.OnHold,
				Version:   balance.Version + 1,
			})

			spanBalance.End()
		}
	}

	if len(newBalances) == 0 {
		return nil
	}

	if err := uc.BalanceRepo.BalancesUpdate(ctxProcessBalances, organizationID, ledgerID, newBalances); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(spanUpdateBalances, "Failed to update balances on database", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update balances on database: %v", err.Error()))

		return err
	}

	return nil
}

// Update balance in the repository and returns the updated balance.
// Overlays Redis cached values for Available, OnHold, and Version to ensure freshest data.
//
// When update.Settings is non-nil, the use case:
//  1. Validates the Settings payload (HARD GATE — fails closed, no partial writes).
//  2. Loads the current balance to enforce overdraft transition invariants:
//     - disable-with-debt is rejected
//     - reducing limit below current usage is rejected
//  3. Auto-creates the system-managed "overdraft" balance on a false→true
//     transition (idempotent — existing overdraft balances are reused).
//
// When update.Settings is nil, legacy behaviour is preserved: no Find call,
// no settings validation, no auto-creation.
func (uc *UseCase) Update(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, update mmodel.UpdateBalance) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Trying to update balance")

	var current *mmodel.Balance

	if update.Settings != nil {
		if err := validateUpdateSettings(ctx, logger, span, update.Settings); err != nil {
			return nil, err
		}

		existing, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to fetch current balance for settings update", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error fetching current balance: %v", err))

			return nil, err
		}

		if err := enforceOverdraftTransition(ctx, logger, span, existing, update.Settings); err != nil {
			return nil, err
		}

		current = existing
	}

	// Auto-create the system-managed overdraft balance BEFORE persisting
	// the settings update. If auto-creation fails (e.g., Create returns a
	// database error), the parent balance's settings MUST NOT have been
	// mutated — this keeps the pair (parent.AllowOverdraft, overdraft
	// balance) consistent on failure. Guarded so the path only runs when
	// the caller actually provided new settings.
	if update.Settings != nil {
		if err := uc.ensureOverdraftBalance(ctx, logger, span, organizationID, ledgerID, current, update.Settings); err != nil {
			return nil, err
		}
	}

	balance, err := uc.BalanceRepo.Update(ctx, organizationID, ledgerID, balanceID, update)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update balance on repo", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error update balance: %v", err))

		return nil, err
	}

	// Overlay amounts from Redis cache when available to ensure freshest values
	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, balance.Alias+"#"+balance.Key)

	value, rerr := uc.TransactionRedisRepo.Get(ctx, internalKey)
	if rerr != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balance cache value on redis", rerr)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to get balance cache value on redis: %v", rerr))
	}

	if value != "" {
		cached := mmodel.BalanceRedis{}
		if uerr := json.Unmarshal([]byte(value), &cached); uerr != nil {
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Error unmarshalling balance cache value: %v", uerr))
		} else {
			balance.Available = cached.Available
			balance.OnHold = cached.OnHold
			balance.Version = cached.Version
		}
	}

	return balance, nil
}
