package command

import (
	"context"
	"errors"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	balanceRepo "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ErrCacheMiss indicates a balance was not found in the cache during refresh.
var ErrCacheMiss = errors.New("cache miss for balance")

const (
	attrBalanceTotalCount   = "balance.total_count"
	attrBalanceSkippedCount = "balance.skipped_count"
	attrBalanceUpdatedCount = "balance.updated_count"
	attrBalanceOrgID        = "organization_id"
	attrBalanceLedgerID     = "ledger_id"
)

// UpdateBalances func that is responsible to update balances without select for update.
func (uc *UseCase) UpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionID string, validate pkgTransaction.Responses, balances []*mmodel.Balance) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances_new")
	defer spanUpdateBalances.End()

	fromTo := buildFromToMap(validate)

	newBalances, err := uc.calculateNewBalances(ctx, tracer, transactionID, fromTo, balances, &spanUpdateBalances, logger)
	if err != nil {
		return err
	}

	// Filter out stale balances by checking Redis cache version.
	// If stale balances are detected, we refresh from cache to align DB with Redis state (no amount re-application).
	balancesToUpdate, usedCache, err := uc.prepareBalancesForUpdate(ctxProcessBalances, organizationID, ledgerID, transactionID, newBalances, &spanUpdateBalances, logger)
	if err != nil {
		return err
	}

	if len(balancesToUpdate) == 0 {
		// CRITICAL: Do NOT return success when all balances are skipped!
		// This was the root cause of 82-97% data loss in chaos tests.
		// Transaction/operations are created but balance never persisted.
		//
		// NOTE: This panic is intentional and expected to be caught by worker-level
		// recovery (see mruntime.SafeGoWithContextAndComponent). The panic ensures
		// this data integrity violation is visible and not silently swallowed.
		assert.Never("all balances stale - data integrity violation",
			"organization_id", organizationID.String(),
			"ledger_id", ledgerID.String(),
			"original_balance_count", len(newBalances),
			"balances_to_update", len(balancesToUpdate))
	}

	logger.Infof("DB_UPDATE_START: Updating %d balances in PostgreSQL (org=%s, ledger=%s)",
		len(balancesToUpdate), organizationID, ledgerID)

	updateStart := time.Now()

	if err := uc.BalanceRepo.BalancesUpdate(ctxProcessBalances, organizationID, ledgerID, balancesToUpdate); err != nil {
		return uc.handleBalanceUpdateError(ctxProcessBalances, err, organizationID, ledgerID, transactionID, newBalances, usedCache, updateStart, &spanUpdateBalances, logger)
	}

	updateDuration := time.Since(updateStart)
	logger.Infof("DB_UPDATE_SUCCESS: Balance update completed in %v for %d balances", updateDuration, len(balancesToUpdate))

	return nil
}

// buildFromToMap creates a map of balance aliases to their transaction amounts.
func buildFromToMap(validate pkgTransaction.Responses) map[string]pkgTransaction.Amount {
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

	return fromTo
}

// calculateNewBalances computes new balance values after applying transaction amounts.
func (uc *UseCase) calculateNewBalances(ctx context.Context, tracer trace.Tracer, transactionID string, fromTo map[string]pkgTransaction.Amount, balances []*mmodel.Balance, span *trace.Span, logger libLog.Logger) ([]*mmodel.Balance, error) {
	newBalances := make([]*mmodel.Balance, 0, len(balances))

	for _, balance := range balances {
		_, spanBalance := tracer.Start(ctx, "command.update_balances_new.balance")

		calculateBalances, err := pkgTransaction.OperateBalancesWithContext(ctx, transactionID, fromTo[balance.Alias], *balance.ToTransactionBalance())
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to update balances on database", err)
			logger.Errorf("Failed to update balances on database: %v", err.Error())

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
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

	return newBalances, nil
}

// prepareBalancesForUpdate filters stale balances and refreshes from cache if needed.
// During cache refresh we do NOT re-apply transaction amounts because Redis already contains the effects
// from the Lua script executed during the HTTP request.
func (uc *UseCase) prepareBalancesForUpdate(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionID string, newBalances []*mmodel.Balance, span *trace.Span, logger libLog.Logger) ([]*mmodel.Balance, bool, error) {
	balancesToUpdate, skippedCount := uc.filterStaleBalances(ctx, organizationID, ledgerID, newBalances, logger)
	usedCache := false

	if skippedCount > 0 {
		// Stale balances are expected during concurrent processing - part of normal optimistic locking flow
		logger.Debugf("STALE_BALANCE_DETECTED: %d stale balances found, refreshing from cache before DB update (org=%s, ledger=%s)",
			skippedCount, organizationID, ledgerID)

		refreshedBalances, err := uc.refreshBalancesFromCache(ctx, organizationID, ledgerID, transactionID, newBalances, logger)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to refresh balances from cache", err)
			logger.Errorf("Failed to refresh balances from cache: %v", err)

			return nil, false, pkg.ValidateBusinessError(constant.ErrStaleBalanceUpdateSkipped, reflect.TypeOf(mmodel.Balance{}).Name())
		}

		balancesToUpdate = refreshedBalances
		usedCache = true
	}

	return balancesToUpdate, usedCache, nil
}

// handleBalanceUpdateError handles errors from BalancesUpdate, including retry logic for ErrNoBalancesUpdated.
func (uc *UseCase) handleBalanceUpdateError(ctx context.Context, err error, organizationID, ledgerID uuid.UUID, transactionID string, newBalances []*mmodel.Balance, usedCache bool, updateStart time.Time, span *trace.Span, logger libLog.Logger) error {
	if !errors.Is(err, balanceRepo.ErrNoBalancesUpdated) {
		updateDuration := time.Since(updateStart)
		logger.Errorf("DB_UPDATE_FAILED: Balance update failed after %v: %v", updateDuration, err)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update balances on database", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	if usedCache {
		// ErrNoBalancesUpdated after cache refresh means DB already has a version >= what we tried to persist.
		// This is expected during concurrent processing: another worker already persisted a newer state.
		// Treat as success (idempotent) - the balance is already up-to-date.
		updateDuration := time.Since(updateStart)
		logger.Infof("DB_UPDATE_IDEMPOTENT: Balance already up-to-date after cache refresh in %v (org=%s, ledger=%s, txn=%s)",
			updateDuration, organizationID, ledgerID, transactionID)

		return nil
	}

	return uc.retryBalanceUpdateWithCacheRefresh(ctx, organizationID, ledgerID, transactionID, newBalances, updateStart, span, logger)
}

// retryBalanceUpdateWithCacheRefresh refreshes balances from cache and retries the database update.
// Transaction amounts are NOT re-applied during refresh because Redis already contains the effects (Lua script).
func (uc *UseCase) retryBalanceUpdateWithCacheRefresh(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionID string, newBalances []*mmodel.Balance, updateStart time.Time, span *trace.Span, logger libLog.Logger) error {
	refreshedBalances, refreshErr := uc.refreshBalancesFromCache(ctx, organizationID, ledgerID, transactionID, newBalances, logger)
	if refreshErr != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to refresh balances after no-op update", refreshErr)
		logger.Errorf("Failed to refresh balances after no-op update: %v", refreshErr)

		return pkg.ValidateBusinessError(constant.ErrStaleBalanceUpdateSkipped, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	if len(refreshedBalances) == 0 {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Cache refresh returned empty balances", nil)
		logger.Errorf("Cache refresh returned empty balances after no-op update, aborting")

		return pkg.ValidateBusinessError(constant.ErrStaleBalanceUpdateSkipped, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	if err := uc.BalanceRepo.BalancesUpdate(ctx, organizationID, ledgerID, refreshedBalances); err != nil {
		if errors.Is(err, balanceRepo.ErrNoBalancesUpdated) {
			// ErrNoBalancesUpdated after cache refresh means DB already has a version >= what we tried to persist.
			// This is expected during concurrent processing: another worker already persisted a newer state.
			// Treat as success (idempotent) - the balance is already up-to-date.
			updateDuration := time.Since(updateStart)
			logger.Infof("DB_UPDATE_IDEMPOTENT: Balance already up-to-date after retry refresh in %v (org=%s, ledger=%s, txn=%s)",
				updateDuration, organizationID, ledgerID, transactionID)

			return nil
		}

		updateDuration := time.Since(updateStart)
		logger.Errorf("DB_UPDATE_FAILED: Balance update failed after refresh in %v: %v", updateDuration, err)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update balances on database after refresh", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	updateDuration := time.Since(updateStart)
	logger.Infof("DB_UPDATE_SUCCESS: Balance update completed after refresh in %v for %d balances", updateDuration, len(refreshedBalances))

	return nil
}

// filterStaleBalances checks Redis cache and filters out balances where the cache version
// is greater than the version being persisted. This prevents unnecessary database updates
// and reduces Lock:tuple contention when multiple workers process the same balance.
func (uc *UseCase) filterStaleBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance, logger libLog.Logger) ([]*mmodel.Balance, int) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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
			// Cache has a newer version, skip this update - expected during concurrent processing
			skippedCount++

			logger.Debugf("STALE_BALANCE_SKIP: balance_id=%s, alias=%s, key=%s, msg_version=%d, cache_version=%d, gap=%d",
				balance.ID, balance.Alias, balanceKey, balance.Version, cachedBalance.Version, cachedBalance.Version-balance.Version)

			continue
		}

		result = append(result, balance)
	}

	// Record metrics via span attributes for observability
	span.SetAttributes(
		attribute.Int(attrBalanceTotalCount, totalCount),
		attribute.Int(attrBalanceSkippedCount, skippedCount),
		attribute.Int(attrBalanceUpdatedCount, len(result)),
		attribute.String(attrBalanceOrgID, organizationID.String()),
		attribute.String(attrBalanceLedgerID, ledgerID.String()),
	)

	if skippedCount > 0 {
		// Informational summary of filtering - expected during concurrent processing
		logger.Debugf("BALANCE_FILTER_SUMMARY: total=%d, skipped=%d, updating=%d, org=%s, ledger=%s",
			totalCount, skippedCount, len(result), organizationID, ledgerID)
	}

	return result, skippedCount
}

// refreshBalancesFromCache fetches latest balance values from cache and returns them as-is.
// The cached balance already includes this transaction's effects (applied by the Lua script during
// the HTTP request), so we must NOT re-apply the transaction amounts - doing so would double-count.
//
// This function is called when filterStaleBalances detects that Redis has a newer version than
// what the RabbitMQ message contains, indicating concurrent processing already updated Redis.
func (uc *UseCase) refreshBalancesFromCache(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionID string, balances []*mmodel.Balance, logger libLog.Logger) ([]*mmodel.Balance, error) {
	trackingLogger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	if logger == nil {
		logger = trackingLogger
	}

	ctx, span := tracer.Start(ctx, "command.refresh_balances_from_cache")
	defer span.End()

	span.SetAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("transaction_id", transactionID),
		attribute.Int("balance.refresh_count", len(balances)),
	)

	refreshed := make([]*mmodel.Balance, 0, len(balances))

	for _, balance := range balances {
		balanceKey := pkgTransaction.SplitAliasWithKey(balance.Alias)

		cachedBalance, err := uc.RedisRepo.ListBalanceByKey(ctx, organizationID, ledgerID, balanceKey)
		if err != nil {
			logger.Errorf("Failed to refresh balance from cache for key %s (alias: %s): %v", balanceKey, balance.Alias, err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
		}

		if cachedBalance == nil {
			logger.Errorf("Cache miss for balance key %s (alias: %s)", balanceKey, balance.Alias)

			return nil, pkg.ValidateBusinessError(ErrCacheMiss, reflect.TypeOf(mmodel.Balance{}).Name())
		}

		balanceID := cachedBalance.ID
		if balanceID == "" {
			balanceID = balance.ID
		}

		refreshAlias := cachedBalance.Alias
		if refreshAlias == "" {
			refreshAlias = balance.Alias
		}

		// Use cached values directly - the Lua script already applied this transaction's effects.
		// Version is kept as-is (Option A: DB aligns with cache).
		// If cache version < message version (unexpected), we still use cache to avoid regression.
		if cachedBalance.Version < balance.Version {
			delta := balance.Version - cachedBalance.Version
			logger.Warnf("UNEXPECTED: cache version %d < message version %d for alias %s (delta=%d); using cache anyway to avoid regression (txn=%s)",
				cachedBalance.Version, balance.Version, balance.Alias, delta, transactionID)

			span.SetAttributes(
				attribute.Bool("balance.cache_version_regression", true),
				attribute.String("balance.alias", balance.Alias),
				attribute.Int64("balance.cache_version", cachedBalance.Version),
				attribute.Int64("balance.message_version", balance.Version),
				attribute.Int64("balance.version_regression_delta", delta),
			)
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
// Returns the updated balance directly from the primary database to avoid stale reads from replicas.
func (uc *UseCase) Update(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, update mmodel.UpdateBalance) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance")
	defer span.End()

	logger.Infof("Trying to update balance")

	balance, err := uc.BalanceRepo.Update(ctx, organizationID, ledgerID, balanceID, update)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update balance on repo", err)
		logger.Errorf("Error update balance: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Balance{}).Name())
	}

	return balance, nil
}
