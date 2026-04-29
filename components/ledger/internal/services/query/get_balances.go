// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// GetBalances retrieves balances for the given aliases using a cache-aside
// pattern: checks Redis first, falls back to PostgreSQL for cache misses.
// This is a pure read -- it does not mutate balances or execute any Lua scripts.
func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

		balances = append(balances, balancesDB...)
	}

	return balances, nil
}

// getBalancesFromCache checks Redis for cached balances. Returns two slices:
// the balances found in cache, and the aliases that were not found (cache misses)
// which need to be fetched from the database.
func (uc *UseCase) getBalancesFromCache(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []string) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

		var b mmodel.BalanceRedis
		if err = json.Unmarshal([]byte(value), &b); err != nil {
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

		// OverdraftUsed is stored as a decimal string in the Lua/Redis layer.
		// An unparseable value is treated as zero to match the Lua fallback
		// rather than corrupting the domain model with an arbitrary number.
		overdraftUsed, derr := decimal.NewFromString(b.OverdraftUsed)
		if derr != nil {
			overdraftUsed = decimal.Zero
		}

		// Synthesize Settings only when at least one field diverges from the
		// defaults. This preserves nil Settings for legacy balances that never
		// had custom configuration. Mirrors the materialization logic used by
		// consumer.redis.go:balanceRedisToBalance so transaction flows observe
		// the same overdraft configuration whether the balance is read from
		// the cache path (here) or returned from the Lua script.
		var settings *mmodel.BalanceSettings
		if b.AllowOverdraft != 0 || b.OverdraftLimitEnabled != 0 ||
			(b.BalanceScope != "" && b.BalanceScope != mmodel.BalanceScopeTransactional) ||
			(b.OverdraftLimit != "" && b.OverdraftLimit != "0") {
			settings = &mmodel.BalanceSettings{
				BalanceScope:          b.BalanceScope,
				AllowOverdraft:        b.AllowOverdraft == 1,
				OverdraftLimitEnabled: b.OverdraftLimitEnabled == 1,
			}
			// Only expose OverdraftLimit when the limit is actively enforced.
			// BalanceSettings.Validate() requires OverdraftLimit to be nil
			// whenever OverdraftLimitEnabled is false.
			if b.OverdraftLimitEnabled == 1 && b.OverdraftLimit != "" {
				limit := b.OverdraftLimit
				settings.OverdraftLimit = &limit
			}
		}

		cached = append(cached, &mmodel.Balance{
			ID:             b.ID,
			AccountID:      b.AccountID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          balanceAlias,
			Key:            balanceKey,
			Available:      b.Available,
			OnHold:         b.OnHold,
			Version:        b.Version,
			AccountType:    b.AccountType,
			AllowSending:   b.AllowSending == 1,
			AllowReceiving: b.AllowReceiving == 1,
			AssetCode:      b.AssetCode,
			Direction:      b.Direction,
			OverdraftUsed:  overdraftUsed,
			Settings:       settings,
		})
	}

	logger.Log(ctx, libLog.LevelDebug, "Balance cache lookup complete",
		libLog.Int("cached", len(cached)),
		libLog.Int("misses", len(misses)))

	return cached, misses
}
