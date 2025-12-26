package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	balanceRepo "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// UpdateBalances func that is responsible to update balances without select for update.
func (uc *UseCase) UpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate pkgTransaction.Responses, balances []*mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances_new")
	defer spanUpdateBalances.End()

	fromTo := make(map[string]pkgTransaction.Amount, len(validate.From)+len(validate.To))
	for k, v := range validate.From {
		fromTo[k] = pkgTransaction.Amount{
			Asset:           v.Asset,
			Value:           v.Value,
			Operation:       v.Operation,
			TransactionType: v.TransactionType,
		}
	}

	for k, v := range validate.To {
		fromTo[k] = pkgTransaction.Amount{
			Asset:           v.Asset,
			Value:           v.Value,
			Operation:       v.Operation,
			TransactionType: v.TransactionType,
		}
	}

	newBalances := make([]*mmodel.Balance, 0, len(balances))

	for _, balance := range balances {
		_, spanBalance := tracer.Start(ctx, "command.update_balances_new.balance")

		calculateBalances, err := pkgTransaction.OperateBalances(fromTo[balance.Alias], *balance.ToTransactionBalance())
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)
			logger.Errorf("Failed to update balances on database: %v", err.Error())

			return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
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
	balancesToUpdate, skippedCount := uc.filterStaleBalances(ctxProcessBalances, organizationID, ledgerID, newBalances, logger)
	usedCache := false

	if skippedCount > 0 {
		logger.Warnf("STALE_BALANCE_DETECTED: %d stale balances found, refreshing from cache before DB update (org=%s, ledger=%s)",
			skippedCount, organizationID, ledgerID)

		refreshedBalances, err := uc.refreshBalancesFromCache(ctxProcessBalances, organizationID, ledgerID, newBalances, logger)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to refresh balances from cache", err)
			logger.Errorf("Failed to refresh balances from cache: %v", err)

			return pkg.ValidateBusinessError(constant.ErrStaleBalanceUpdateSkipped, reflect.TypeOf(mmodel.Balance{}).Name())
		}

		balancesToUpdate = refreshedBalances
		usedCache = true
	}

	if len(balancesToUpdate) == 0 {
		// CRITICAL: Do NOT return success when all balances are skipped!
		// This was the root cause of 82-97% data loss in chaos tests.
		// Transaction/operations are created but balance never persisted.
		logger.Errorf("CRITICAL: All %d balances are stale, returning error to trigger retry. "+
			"org=%s ledger=%s balance_count=%d",
			len(newBalances), organizationID, ledgerID, len(newBalances))

		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "All balances stale - data integrity risk", nil)

		return pkg.ValidateBusinessError(constant.ErrStaleBalanceUpdateSkipped, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	logger.Infof("DB_UPDATE_START: Updating %d balances in PostgreSQL (org=%s, ledger=%s)",
		len(balancesToUpdate), organizationID, ledgerID)

	updateStart := time.Now()

	if err := uc.BalanceRepo.BalancesUpdate(ctxProcessBalances, organizationID, ledgerID, balancesToUpdate); err != nil {
		if errors.Is(err, balanceRepo.ErrNoBalancesUpdated) {
			if usedCache {
				logger.Warnf("BALANCE_UPDATE_SKIPPED: balances already up to date after cache refresh (org=%s, ledger=%s)", organizationID, ledgerID)
				return nil
			}

			refreshedBalances, refreshErr := uc.refreshBalancesFromCache(ctxProcessBalances, organizationID, ledgerID, newBalances, logger)
			if refreshErr != nil {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to refresh balances after no-op update", refreshErr)
				logger.Errorf("Failed to refresh balances after no-op update: %v", refreshErr)

				return pkg.ValidateBusinessError(constant.ErrStaleBalanceUpdateSkipped, reflect.TypeOf(mmodel.Balance{}).Name())
			}

			if len(refreshedBalances) == 0 {
				libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Cache refresh returned empty balances", nil)
				logger.Errorf("Cache refresh returned empty balances after no-op update, aborting")

				return pkg.ValidateBusinessError(constant.ErrStaleBalanceUpdateSkipped, reflect.TypeOf(mmodel.Balance{}).Name())
			}

			if err = uc.BalanceRepo.BalancesUpdate(ctxProcessBalances, organizationID, ledgerID, refreshedBalances); err != nil {
				if errors.Is(err, balanceRepo.ErrNoBalancesUpdated) {
					logger.Warnf("BALANCE_UPDATE_SKIPPED: balances already current after cache refresh (org=%s, ledger=%s)", organizationID, ledgerID)
					return nil
				}

				updateDuration := time.Since(updateStart)
				logger.Errorf("DB_UPDATE_FAILED: Balance update failed after refresh in %v: %v", updateDuration, err)
				libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to update balances on database after refresh", err)
				logger.Errorf("Failed to update balances on database after refresh: %v", err.Error())

				return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
			}

			updateDuration := time.Since(updateStart)
			logger.Infof("DB_UPDATE_SUCCESS: Balance update completed after refresh in %v for %d balances", updateDuration, len(refreshedBalances))

			return nil
		}

		updateDuration := time.Since(updateStart)
		logger.Errorf("DB_UPDATE_FAILED: Balance update failed after %v: %v", updateDuration, err)
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to update balances on database", err)
		logger.Errorf("Failed to update balances on database: %v", err.Error())

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	updateDuration := time.Since(updateStart)
	logger.Infof("DB_UPDATE_SUCCESS: Balance update completed in %v for %d balances", updateDuration, len(balancesToUpdate))

	return nil
}

// filterStaleBalances checks Redis cache and filters out balances where the cache version
// is greater than the version being persisted. This prevents unnecessary database updates
// and reduces Lock:tuple contention when multiple workers process the same balance.
func (uc *UseCase) filterStaleBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance, logger libLog.Logger) ([]*mmodel.Balance, int) {
	loggerFromCtx, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	_ = loggerFromCtx // not used, logger is passed as parameter

	ctx, span := tracer.Start(ctx, "command.filter_stale_balances")
	defer span.End()

	result := make([]*mmodel.Balance, 0, len(balances))
	skippedCount := 0
	totalCount := len(balances)

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
			skippedCount++

			logger.Warnf("STALE_BALANCE_SKIP: balance_id=%s, alias=%s, key=%s, msg_version=%d, cache_version=%d, gap=%d",
				balance.ID, balance.Alias, balanceKey, balance.Version, cachedBalance.Version, cachedBalance.Version-balance.Version)

			continue
		}

		result = append(result, balance)
	}

	// Record metrics via span attributes for observability
	span.SetAttributes(
		attribute.Int("balance.total_count", totalCount),
		attribute.Int("balance.skipped_count", skippedCount),
		attribute.Int("balance.updated_count", len(result)),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	if skippedCount > 0 {
		logger.Warnf("BALANCE_FILTER_SUMMARY: total=%d, skipped=%d, updating=%d, org=%s, ledger=%s",
			totalCount, skippedCount, len(result), organizationID, ledgerID)
	}

	return result, skippedCount
}

func (uc *UseCase) refreshBalancesFromCache(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance, logger libLog.Logger) ([]*mmodel.Balance, error) {
	refreshed := make([]*mmodel.Balance, 0, len(balances))

	for _, balance := range balances {
		balanceKey := pkgTransaction.SplitAliasWithKey(balance.Alias)

		cachedBalance, err := uc.RedisRepo.ListBalanceByKey(ctx, organizationID, ledgerID, balanceKey)
		if err != nil {
			logger.Errorf("Failed to refresh balance from cache for key %s (alias: %s): %v", balanceKey, balance.Alias, err)
			return nil, fmt.Errorf("refresh balance from cache: %w", err)
		}

		if cachedBalance == nil {
			return nil, fmt.Errorf("cache miss for balance key %s (alias: %s)", balanceKey, balance.Alias)
		}

		balanceID := cachedBalance.ID
		if balanceID == "" {
			balanceID = balance.ID
		}

		refreshAlias := cachedBalance.Alias
		if refreshAlias == "" {
			refreshAlias = balance.Alias
		}

		refreshed = append(refreshed, &mmodel.Balance{
			ID:        balanceID,
			Alias:     refreshAlias,
			Available: cachedBalance.Available,
			OnHold:    cachedBalance.OnHold,
			Version:   cachedBalance.Version,
		})
	}

	return refreshed, nil
}

// Update balance in the repository.
func (uc *UseCase) Update(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, update mmodel.UpdateBalance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance")
	defer span.End()

	logger.Infof("Trying to update balance")

	err := uc.BalanceRepo.Update(ctx, organizationID, ledgerID, balanceID, update)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update balance on repo", err)
		logger.Errorf("Error update balance: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	return nil
}
