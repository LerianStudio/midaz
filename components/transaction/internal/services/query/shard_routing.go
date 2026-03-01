// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

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

	return shardrouting.ResolveBalanceShard(ctx, uc.ShardRouter, uc.ShardManager, organizationID, ledgerID, alias, balanceKey)
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
