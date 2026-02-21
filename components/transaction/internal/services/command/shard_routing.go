// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/shardrouting"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
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

// IsShardedBTOQueueEnabled checks if sharded balance-transaction-operation queues are enabled.
// Requires an active shard router with shard count > 0 and the shardedEnabled flag to be true.
// The shardedEnabled flag is resolved once at startup from RABBITMQ_TRANSACTION_BALANCE_OPERATION_SHARDED
// and stored in UseCase.ShardedBTOQueuesEnabled to avoid per-request os.Getenv overhead.
func IsShardedBTOQueueEnabled(router *shard.Router, shardedEnabled bool) bool {
	if router == nil || router.ShardCount() <= 0 {
		return false
	}

	return shardedEnabled
}

