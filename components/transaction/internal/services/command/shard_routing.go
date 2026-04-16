// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/shardrouting"
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

// waitForMigrationUnlock mirrors the query-side guard so write paths can block
// briefly while a migration is in progress for any alias they touch.
// Safe to call with an empty alias slice or with a nil ShardManager (no-op).
func (uc *UseCase) waitForMigrationUnlock(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) error {
	if uc == nil || uc.ShardManager == nil || len(aliases) == 0 {
		return nil
	}

	if err := uc.ShardManager.WaitForAliasesUnlocked(ctx, organizationID, ledgerID, aliases); err != nil {
		return fmt.Errorf("wait for aliases to be unlocked: %w", err)
	}

	return nil
}

// collectTransactionAliases extracts the set of canonical account aliases
// referenced by a Transaction payload (source.from + distribute.to). Aliases
// are deduplicated and returned in insertion order; empty aliases are dropped.
// Used by the write path to feed WaitForAliasesUnlocked.
func collectTransactionAliases(t *pkgTransaction.Transaction) []string {
	if t == nil {
		return nil
	}

	seen := make(map[string]struct{}, len(t.Send.Source.From)+len(t.Send.Distribute.To))
	aliases := make([]string, 0, len(t.Send.Source.From)+len(t.Send.Distribute.To))

	appendAlias := func(alias string) {
		if alias == "" {
			return
		}

		if _, exists := seen[alias]; exists {
			return
		}

		seen[alias] = struct{}{}

		aliases = append(aliases, alias)
	}

	for _, ft := range t.Send.Source.From {
		appendAlias(ft.AccountAlias)
	}

	for _, ft := range t.Send.Distribute.To {
		appendAlias(ft.AccountAlias)
	}

	return aliases
}
