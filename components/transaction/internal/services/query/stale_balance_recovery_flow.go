// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"math"

	"github.com/google/uuid"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

func (uc *UseCase) recoverLaggedBalancesForAliases( //nolint:gocyclo,cyclop
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	aliases []string,
) ([]*mmodel.Balance, []string, error) {
	if !uc.shouldEnforceConsumerLagFence() || uc.StaleBalanceRecoverer == nil || len(aliases) == 0 {
		return nil, aliases, nil
	}

	logger, _, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // only logger is needed; tracing is handled by the parent caller

	partitionAliases := make(map[int32][]string)
	seen := make(map[int32]struct{}, len(aliases))
	laggingPartitions := make(map[int32]bool)

	for _, aliasWithKey := range aliases {
		alias, balanceKey := shard.SplitAliasAndBalanceKey(aliasWithKey)

		shardID, err := uc.resolveBalanceShard(ctx, organizationID, ledgerID, alias, balanceKey)
		if err != nil {
			logger.Warnf("Stale-balance recovery skipped alias %s due to shard resolution error: %v", aliasWithKey, err)
			continue
		}

		if shardID > math.MaxInt32 {
			shardID = 0
		}

		partition := int32(shardID)
		partitionAliases[partition] = append(partitionAliases[partition], aliasWithKey)

		if _, checked := seen[partition]; checked {
			continue
		}

		seen[partition] = struct{}{}
		laggingPartitions[partition] = !uc.LagChecker.IsPartitionCaughtUp(ctx, uc.BalanceOperationsTopic, partition)
	}

	laggedAliasesByPartition := make(map[int32][]string)

	for partition, partitionAliasList := range partitionAliases {
		if !laggingPartitions[partition] {
			continue
		}

		laggedAliasesByPartition[partition] = partitionAliasList
	}

	if len(laggedAliasesByPartition) == 0 {
		return nil, aliases, nil
	}

	recoveredByAlias, err := uc.StaleBalanceRecoverer.RecoverLaggedAliases(
		ctx,
		uc.BalanceOperationsTopic,
		organizationID,
		ledgerID,
		laggedAliasesByPartition,
	)
	if err != nil {
		return nil, aliases, err
	}

	recovered := make([]*mmodel.Balance, 0, len(recoveredByAlias))
	remaining := make([]string, 0, len(aliases))

	for _, aliasWithKey := range aliases {
		balance, ok := recoveredByAlias[aliasWithKey]
		if !ok || balance == nil {
			remaining = append(remaining, aliasWithKey)
			continue
		}

		recovered = append(recovered, balance)

		if cacheErr := uc.cacheRecoveredBalance(ctx, organizationID, ledgerID, aliasWithKey, balance); cacheErr != nil {
			logger.Warnf("Failed to cache recovered stale balance for %s: %v", aliasWithKey, cacheErr)
		}
	}

	return recovered, remaining, nil
}

func (uc *UseCase) cacheRecoveredBalance(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	aliasWithKey string,
	balance *mmodel.Balance,
) error {
	if uc == nil || uc.RedisRepo == nil || balance == nil {
		return nil
	}

	internalKey, err := uc.resolveBalanceCacheKey(ctx, organizationID, ledgerID, balance.Alias, balance.Key)
	if err != nil {
		internalKey = utils.BalanceInternalKey(organizationID, ledgerID, aliasWithKey)
	}

	payload, err := json.Marshal(balanceToRedis(balance))
	if err != nil {
		return err
	}

	return uc.RedisRepo.Set(ctx, internalKey, string(payload), uc.balanceCacheTTL())
}
