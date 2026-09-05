// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// GetBalances retrieves balances for the given aliases using a cache-aside
// pattern: checks Redis first, falls back to PostgreSQL for cache misses.
// This is a pure read -- it does not mutate balances or execute any Lua scripts.
func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balances")
	defer span.End()

	balances, uncachedAliases := uc.getBalancesFromCache(ctx, organizationID, ledgerID, aliases)

	if len(uncachedAliases) > 0 {
		balancesDB, err := uc.BalanceRepo.ListByAliasesWithKeys(ctx, organizationID, ledgerID, uncachedAliases)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to get balances from database", err)
			logger.Log(ctx, libLog.LevelError, "Failed to get balances from database", libLog.Err(err))

			return nil, err
		}

		if err := uc.hydrateAccountBlocked(ctx, organizationID, ledgerID, balancesDB); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to hydrate account blocked state", err)
			logger.Log(ctx, libLog.LevelError, "Failed to hydrate account blocked state", libLog.Err(err))

			return nil, err
		}

		balances = append(balances, balancesDB...)
	}

	return balances, nil
}

// hydrateAccountBlocked stamps database-loaded balances with their owning
// account's blocked flag using ONE batched primary-key lookup over the
// distinct account IDs. Only the cache-miss path reaches here: on a hit the
// flag is already in the cached blob. Failing the lookup fails the read —
// the block state must never be silently assumed open. A balance whose
// account row is absent (e.g. soft-deleted) resolves to not blocked.
func (uc *UseCase) hydrateAccountBlocked(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	if len(balances) == 0 {
		return nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balances.hydrate_account_blocked")
	defer span.End()

	seen := make(map[uuid.UUID]struct{}, len(balances))
	accountIDs := make([]uuid.UUID, 0, len(balances))

	for _, b := range balances {
		accountID, err := uuid.Parse(b.AccountID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Invalid account ID on balance", err)
			logger.Log(ctx, libLog.LevelError, "Invalid account ID on balance", libLog.String("balance_id", b.ID), libLog.Err(err))

			return err
		}

		if _, ok := seen[accountID]; !ok {
			seen[accountID] = struct{}{}

			accountIDs = append(accountIDs, accountID)
		}
	}

	accounts, err := uc.AccountRepo.ListAccountsByIDs(ctx, organizationID, ledgerID, accountIDs)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list accounts for blocked hydration", err)
		logger.Log(ctx, libLog.LevelError, "Failed to list accounts for blocked hydration", libLog.Err(err))

		return err
	}

	blockedByAccount := make(map[string]bool, len(accounts))
	for _, acc := range accounts {
		blockedByAccount[acc.ID] = acc.Blocked != nil && *acc.Blocked
	}

	for _, b := range balances {
		b.Blocked = blockedByAccount[b.AccountID]
	}

	return nil
}

// getBalancesFromCache checks Redis for cached balances. Returns two slices:
// the balances found in cache, and the aliases that were not found (cache misses)
// which need to be fetched from the database.
func (uc *UseCase) getBalancesFromCache(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []string) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balances.cache_read")
	defer span.End()

	cached := make([]*mmodel.Balance, 0, len(aliases))
	misses := make([]string, 0, len(aliases))

	for _, alias := range aliases {
		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, alias)

		value, err := uc.TransactionRedisRepo.Get(ctx, internalKey)
		if err != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to read balance from cache, falling back to database", libLog.String("alias", alias), libLog.Err(err))

			misses = append(misses, alias)

			continue
		}

		if libCommons.IsNilOrEmpty(&value) {
			misses = append(misses, alias)

			continue
		}

		snapshot, err := balancecache.DecodeForRead([]byte(value))
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to deserialize cached balance", err)
			logger.Log(ctx, libLog.LevelWarn, "Failed to deserialize cached balance, falling back to database", libLog.String("alias", alias), libLog.Err(err))

			misses = append(misses, alias)

			continue
		}

		balanceAlias, balanceKey, _ := strings.Cut(alias, "#")

		balanceKey = strings.TrimSpace(balanceKey)
		if balanceKey == "" {
			balanceKey = constant.DefaultBalanceKey
		}

		if snapshot.Key != balanceKey || (snapshot.Alias != "" && snapshot.Alias != balanceAlias) {
			logger.Log(ctx, libLog.LevelWarn, "Cached balance identity does not match requested balance, falling back to database", libLog.String("alias", alias))

			misses = append(misses, alias)

			continue
		}

		// Synthesize Settings only when at least one field diverges from the
		// defaults. This preserves nil Settings for legacy balances that never
		// had custom configuration. Mirrors the materialization logic used by
		// consumer.redis.go:balanceRedisToBalance so transaction flows observe
		// the same overdraft configuration whether the balance is read from
		// the cache path (here) or returned from the Lua script.
		var settings *mmodel.BalanceSettings
		if snapshot.AllowOverdraft || snapshot.OverdraftLimitEnabled ||
			(snapshot.BalanceScope != "" && snapshot.BalanceScope != mmodel.BalanceScopeTransactional) ||
			!snapshot.OverdraftLimit.IsZero() {
			settings = &mmodel.BalanceSettings{
				BalanceScope:          snapshot.BalanceScope,
				AllowOverdraft:        snapshot.AllowOverdraft,
				OverdraftLimitEnabled: snapshot.OverdraftLimitEnabled,
			}
			// Only expose OverdraftLimit when the limit is actively enforced.
			// BalanceSettings.Validate() requires OverdraftLimit to be nil
			// whenever OverdraftLimitEnabled is false.
			if snapshot.OverdraftLimitEnabled {
				limit := snapshot.OverdraftLimit.String()
				settings.OverdraftLimit = &limit
			}
		}

		cached = append(cached, &mmodel.Balance{
			ID:             snapshot.ID.String(),
			AccountID:      snapshot.AccountID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          balanceAlias,
			Key:            balanceKey,
			Available:      snapshot.Available,
			OnHold:         snapshot.OnHold,
			Version:        snapshot.Version,
			AccountType:    snapshot.AccountType,
			AllowSending:   snapshot.AllowSending,
			AllowReceiving: snapshot.AllowReceiving,
			Blocked:        snapshot.Blocked,
			AssetCode:      snapshot.AssetCode,
			Direction:      snapshot.Direction,
			OverdraftUsed:  snapshot.OverdraftUsed,
			Settings:       settings,
		})
	}

	logger.Log(ctx, libLog.LevelDebug, "Balance cache lookup complete",
		libLog.Int("cached", len(cached)),
		libLog.Int("misses", len(misses)))

	return cached, misses
}
