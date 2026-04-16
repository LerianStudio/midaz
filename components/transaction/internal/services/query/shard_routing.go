// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/shardrouting"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// resolveBalanceCacheKey returns the appropriate Redis cache key for a balance,
// using shard-aware keys when sharding is enabled and internal keys otherwise.
func (uc *UseCase) resolveBalanceCacheKey(ctx context.Context, organizationID, ledgerID uuid.UUID, alias, key string) (string, error) {
	if uc.ShardRouter != nil {
		shardID, err := uc.resolveBalanceShard(ctx, organizationID, ledgerID, alias, key)
		if err != nil {
			return "", err
		}

		return utils.BalanceShardKey(shardID, organizationID, ledgerID, alias+"#"+key), nil
	}

	return utils.BalanceInternalKey(organizationID, ledgerID, alias+"#"+key), nil
}

func (uc *UseCase) resolveBalanceShard(ctx context.Context, organizationID, ledgerID uuid.UUID, alias, balanceKey string) (int, error) {
	if uc == nil {
		return 0, nil
	}

	return shardrouting.ResolveBalanceShardWithOptions(
		ctx,
		uc.ShardRouter,
		uc.ShardManager,
		organizationID, ledgerID,
		alias, balanceKey,
		shardrouting.Options{AllowFallback: uc.AllowShardRoutingFallback},
	)
}

// trackInFlightWrites increments the per-alias in-flight counter in Redis for
// each alias and returns a release function that decrements them all. Aliases
// are deduplicated so a transaction that touches the same alias twice
// increments exactly once. Errors during increment are logged (via warnf) and
// suppressed so a transient Redis hiccup doesn't fail authorization.
//
// The release function uses context.Background() intentionally: if the request
// context was cancelled we still must decrement, otherwise the counter stays
// inflated until the ShardInFlightCounter TTL (1m) elapses, blocking migrations.
func (uc *UseCase) trackInFlightWrites(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) func() {
	if uc == nil || uc.ShardManager == nil || len(aliases) == 0 {
		return func() {}
	}

	logger, _, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // tracking tuple discards are idiomatic here

	seen := make(map[string]struct{}, len(aliases))
	tracked := make([]string, 0, len(aliases))

	for _, alias := range aliases {
		if alias == "" {
			continue
		}

		if _, ok := seen[alias]; ok {
			continue
		}

		seen[alias] = struct{}{}

		if _, err := uc.ShardManager.IncrementInFlight(ctx, organizationID, ledgerID, alias); err != nil {
			if logger != nil {
				logger.Warnf("track in-flight writes: increment for %s: %v", alias, err)
			}

			continue
		}

		tracked = append(tracked, alias)
	}

	return func() { //nolint:contextcheck // context.Background() is intentional; see below
		if len(tracked) == 0 {
			return
		}

		// context.Background() is intentional: if the request context was
		// cancelled we still must decrement, otherwise the counter stays
		// inflated until its TTL expires and blocks migrations.
		releaseCtx := context.Background()

		for _, alias := range tracked {
			if _, err := uc.ShardManager.DecrementInFlight(releaseCtx, organizationID, ledgerID, alias); err != nil && logger != nil {
				logger.Warnf("track in-flight writes: decrement for %s: %v", alias, err)
			}
		}
	}
}

func (uc *UseCase) waitForMigrationUnlock(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) error {
	if uc == nil || uc.ShardManager == nil {
		return nil
	}

	if err := uc.ShardManager.WaitForAliasesUnlocked(ctx, organizationID, ledgerID, aliases); err != nil {
		return fmt.Errorf("failed to wait for aliases to be unlocked: %w", err)
	}

	return nil
}

func (uc *UseCase) recordShardLoad(ctx context.Context, organizationID, ledgerID uuid.UUID, operations []mmodel.BalanceOperation) {
	if uc == nil || uc.ShardManager == nil || len(operations) == 0 {
		return
	}

	for _, op := range operations {
		if op.ShardID < 0 {
			continue
		}

		aliasWithKey := pkgTransaction.SplitAliasWithKey(op.Alias)
		alias, _ := shard.SplitAliasAndBalanceKey(aliasWithKey)
		_ = uc.ShardManager.RecordShardAliasLoad(ctx, organizationID, ledgerID, alias, op.ShardID, 1)
	}
}
