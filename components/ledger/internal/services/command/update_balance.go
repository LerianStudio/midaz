// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
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
// The method always loads the current balance first to enforce scope
// protection: internal-scope balances (e.g. overdraft companions) are
// rejected with 0176 regardless of which fields the payload carries.
//
// When update.Settings is non-nil, the use case additionally:
//  1. Validates the Settings payload (HARD GATE — fails closed, no partial writes).
//  2. Enforces overdraft transition invariants:
//     - disable-with-debt is rejected
//     - reducing limit below current usage is rejected
//  3. Auto-creates the system-managed "overdraft" balance on a false→true
//     transition (idempotent — existing overdraft balances are reused).
func (uc *UseCase) Update(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, update mmodel.UpdateBalance) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Trying to update balance")

	// Always load the current balance so the scope guard can fire
	// regardless of which fields the payload contains (Settings,
	// AllowSending, AllowReceiving, or any combination). Without this
	// unconditional Find, a payload without Settings bypasses the guard.
	current, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to fetch current balance for update", err)
		logger.Log(ctx, libLog.LevelError, "Error fetching current balance", libLog.Err(err))

		return nil, err
	}

	// Scope protection: internal-scope balances are managed exclusively by
	// the system (e.g. overdraft reserves) and cannot be updated via the
	// public API. This guard runs AFTER Find (so a non-existent balance
	// returns 404, not 422) and BEFORE any Settings validation, overdraft
	// transition enforcement, or repo Update — no partial mutations.
	if current != nil && current.Settings != nil && current.Settings.BalanceScope == mmodel.BalanceScopeInternal {
		err = pkg.ValidateBusinessError(constant.ErrUpdateOfInternalBalance, constant.EntityBalance)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rejected update on internal-scope balance", err)
		logger.Log(ctx, libLog.LevelWarn, "Rejected update on internal-scope balance",
			libLog.String("alias", current.Alias),
			libLog.String("key", current.Key))

		return nil, err
	}

	if update.Settings != nil {
		if err := validateUpdateSettings(ctx, logger, span, update.Settings); err != nil {
			return nil, err
		}

		if err := enforceOverdraftTransition(ctx, logger, span, current, update.Settings); err != nil {
			return nil, err
		}
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

	// When the caller updates overdraft/scope Settings, rewrite ONLY those
	// fields in the cached JSON blob in place. Deleting the key would discard
	// live transactional state (Available, OnHold, Version, OverdraftUsed)
	// that the Lua atomic script may have mutated but not yet flushed to
	// PostgreSQL. An in-place settings rewrite preserves that state while
	// making the new overdraft configuration visible to the next transaction.
	//
	// The repo method composes its own BalanceInternalKey from (org, ledger,
	// alias#key) and applies the tenant prefix internally, mirroring the Get
	// above. A cache miss inside the repo is a no-op — the next transaction
	// will load the freshly-persisted settings via the Lua SETNX path.
	//
	// Non-settings updates (e.g. AllowSending, AllowReceiving) do not trigger
	// this rewrite: those fields are not part of the settings contract this
	// method guards, and leaving the cache alone avoids a useless round-trip.
	//
	// This is best-effort: the PostgreSQL write is already durable, so a
	// Redis-side failure here is logged and swallowed. A subsequent cache
	// miss or the sync worker will reconcile.
	if update.Settings != nil {
		cacheKey := balance.Alias + "#" + balance.Key
		if uerr := uc.TransactionRedisRepo.UpdateBalanceCacheSettings(ctx, organizationID, ledgerID, cacheKey, update.Settings); uerr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update balance cache settings on redis", uerr)
			logger.Log(ctx, libLog.LevelWarn, "Failed to update balance cache settings on Redis", libLog.Err(uerr))
		}
	}

	return balance, nil
}
