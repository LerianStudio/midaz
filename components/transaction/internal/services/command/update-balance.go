// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// UpdateBalances func that is responsible to update balances without select for update.
func (uc *UseCase) UpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate pkgTransaction.Responses, balances []*mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances_new")
	defer spanUpdateBalances.End()

	fromTo := make(map[string]pkgTransaction.Amount, len(validate.From)+len(validate.To))
	for k, v := range validate.From {
		fromTo[k] = v
	}

	for k, v := range validate.To {
		fromTo[k] = v
	}

	newBalances := make([]*mmodel.Balance, 0, len(balances))

	for _, balance := range balances {
		_, spanBalance := tracer.Start(ctx, "command.update_balances_new.balance")

		calculateBalances, err := pkgTransaction.OperateBalances(fromTo[balance.Alias], *balance.ToTransactionBalance())
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)
			logger.Errorf("Failed to update balances on database: %v", err.Error())

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

	// Filter out stale balances by checking Redis cache version
	balancesToUpdate := uc.filterStaleBalances(ctxProcessBalances, organizationID, ledgerID, newBalances, logger)

	if len(balancesToUpdate) == 0 {
		libOpentelemetry.HandleSpanEvent(&spanUpdateBalances, "All balances are stale, skipping database update")

		logger.Info("All balances are stale, skipping database update")

		return nil
	}

	if err := uc.BalanceRepo.BalancesUpdate(ctxProcessBalances, organizationID, ledgerID, balancesToUpdate); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to update balances on database", err)
		logger.Errorf("Failed to update balances on database: %v", err.Error())

		return err
	}

	return nil
}

// filterStaleBalances checks Redis cache and filters out balances where the cache version
// is greater than the version being persisted. This prevents unnecessary database updates
// and reduces Lock:tuple contention when multiple workers process the same balance.
func (uc *UseCase) filterStaleBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance, logger libLog.Logger) []*mmodel.Balance {
	result := make([]*mmodel.Balance, 0, len(balances))

	for _, balance := range balances {
		// Extract the balance key from alias format "0#@account1#default" -> "@account1#default"
		balanceKey := pkgTransaction.SplitAliasWithKey(balance.Alias)

		cachedBalance, err := uc.RedisRepo.ListBalanceByKey(ctx, organizationID, ledgerID, balanceKey)
		if err != nil {
			// If we can't get from cache, proceed with update (fail-open)
			logger.Warnf("Failed to get balance from cache for key %s (alias: %s), proceeding with update: %v", balanceKey, balance.Alias, err)
			result = append(result, balance)

			continue
		}

		if cachedBalance != nil && cachedBalance.Version > balance.Version {
			// Cache has a newer version, skip this update
			logger.Infof("Skipping stale balance update: balance_id=%s, alias=%s, key=%s, update_version=%d, cache_version=%d",
				balance.ID, balance.Alias, balanceKey, balance.Version, cachedBalance.Version)

			continue
		}

		result = append(result, balance)
	}

	return result
}

// Update balance in the repository and returns the updated balance.
// Overlays Redis cached values for Available, OnHold, and Version to ensure freshest data.
func (uc *UseCase) Update(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, update mmodel.UpdateBalance) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance")
	defer span.End()

	logger.Infof("Trying to update balance")

	balance, err := uc.BalanceRepo.Update(ctx, organizationID, ledgerID, balanceID, update)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update balance on repo", err)
		logger.Errorf("Error update balance: %v", err)

		return nil, err
	}

	// Overlay amounts from Redis cache when available to ensure freshest values
	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, balance.Alias+"#"+balance.Key)

	value, rerr := uc.RedisRepo.Get(ctx, internalKey)
	if rerr != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance cache value on redis", rerr)
		logger.Warnf("Failed to get balance cache value on redis: %v", rerr)
	}

	if value != "" {
		cached := mmodel.BalanceRedis{}
		if uerr := json.Unmarshal([]byte(value), &cached); uerr != nil {
			logger.Warnf("Error unmarshalling balance cache value: %v", uerr)
		} else {
			balance.Available = cached.Available
			balance.OnHold = cached.OnHold
			balance.Version = cached.Version
		}
	}

	return balance, nil
}
