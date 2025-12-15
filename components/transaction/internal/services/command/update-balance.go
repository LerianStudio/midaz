package command

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// UpdateBalances func that is responsible to update balances without select for update.
func (uc *UseCase) UpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate libTransaction.Responses, balances []*mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances_new")
	defer spanUpdateBalances.End()

	fromTo := make(map[string]libTransaction.Amount, len(validate.From)+len(validate.To))
	for k, v := range validate.From {
		fromTo[k] = v
	}

	for k, v := range validate.To {
		fromTo[k] = v
	}

	newBalances := make([]*mmodel.Balance, 0, len(balances))

	for _, balance := range balances {
		_, spanBalance := tracer.Start(ctx, "command.update_balances_new.balance")

		calculateBalances, err := libTransaction.OperateBalances(fromTo[balance.Alias], *balance.ConvertToLibBalance())
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanUpdateBalances, "Failed to update balances on database", err)
			logger.Errorf("Failed to update balances on database: %v", err.Error())

			return fmt.Errorf("failed to update: %w", err)
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
		// CRITICAL: Do NOT return success when all balances are skipped!
		// This was the root cause of 82-97% data loss in chaos tests.
		// Transaction/operations are created but balance never persisted.
		logger.Errorf("CRITICAL: All %d balances are stale, returning error to trigger retry. "+
			"org=%s ledger=%s balance_count=%d",
			len(newBalances), organizationID, ledgerID, len(newBalances))

		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "All balances stale - data integrity risk", nil)

		return fmt.Errorf("all %d balance updates skipped due to stale versions: %w",
			len(newBalances), constant.ErrStaleBalanceUpdateSkipped)
	}

	if err := uc.BalanceRepo.BalancesUpdate(ctxProcessBalances, organizationID, ledgerID, balancesToUpdate); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdateBalances, "Failed to update balances on database", err)
		logger.Errorf("Failed to update balances on database: %v", err.Error())

		return fmt.Errorf("operation failed: %w", err)
	}

	return nil
}

// filterStaleBalances checks Redis cache and filters out balances where the cache version
// is greater than the version being persisted. This prevents unnecessary database updates
// and reduces Lock:tuple contention when multiple workers process the same balance.
func (uc *UseCase) filterStaleBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance, logger libLog.Logger) []*mmodel.Balance {
	loggerFromCtx, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	_ = loggerFromCtx // not used, logger is passed as parameter

	ctx, span := tracer.Start(ctx, "command.filter_stale_balances")
	defer span.End()

	result := make([]*mmodel.Balance, 0, len(balances))
	skippedCount := 0
	totalCount := len(balances)

	for _, balance := range balances {
		// Extract the balance key from alias format "0#@account1#default" -> "@account1#default"
		balanceKey := libTransaction.SplitAliasWithKey(balance.Alias)

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

	return result
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

		return fmt.Errorf("failed to update: %w", err)
	}

	return nil
}
