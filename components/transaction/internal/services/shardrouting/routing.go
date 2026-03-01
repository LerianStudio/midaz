// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package shardrouting

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

// ResolveBalanceShard determines which shard a balance belongs to using the manager (if available)
// or falling back to the static router.
func ResolveBalanceShard(
	ctx context.Context,
	router *shard.Router,
	manager *internalsharding.Manager,
	organizationID, ledgerID uuid.UUID,
	alias, balanceKey string,
) (int, error) {
	if router != nil && router.ShardCount() <= 0 {
		return 0, internalsharding.ErrInvalidShardCount
	}

	if manager != nil {
		shardID, err := manager.ResolveBalanceShard(ctx, organizationID, ledgerID, alias, balanceKey)
		if err != nil {
			if router != nil && shardID >= 0 && shardID < router.ShardCount() {
				logger, _, _, _ := libCommons.NewTrackingFromContext(ctx)
				logger.Warnf("shard resolution fell back to router default (shard %d) due to manager error: %v", shardID, err)

				return shardID, nil
			}

			return 0, fmt.Errorf("resolve balance shard: %w", err)
		}

		return shardID, nil
	}

	if router != nil {
		if router.ShardCount() <= 0 {
			return 0, internalsharding.ErrInvalidShardCount
		}

		return router.ResolveBalance(alias, balanceKey), nil
	}

	return 0, nil
}
