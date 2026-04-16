// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

func (uc *UseCase) shouldEnforceConsumerLagFence() bool {
	return uc != nil &&
		uc.ConsumerLagFenceEnabled &&
		uc.LagChecker != nil &&
		uc.ShardRouter != nil &&
		uc.BalanceOperationsTopic != ""
}

func (uc *UseCase) ensureConsumerLagFenceForAliases(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	aliases []string,
) error {
	if !uc.shouldEnforceConsumerLagFence() || len(aliases) == 0 {
		return nil
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.ensure_consumer_lag_fence_for_aliases")
	defer span.End()

	checkedPartitions := make(map[int32]struct{}, len(aliases))

	for _, aliasWithKey := range aliases {
		alias, balanceKey := shard.SplitAliasAndBalanceKey(aliasWithKey)

		shardID, err := uc.resolveBalanceShard(ctx, organizationID, ledgerID, alias, balanceKey)
		if err != nil {
			logger.Warnf("Consumer lag fence skipped for alias %s due to shard resolution error: %v", aliasWithKey, err)
			continue
		}

		if shardID > math.MaxInt32 {
			shardID = 0
		}

		partition := int32(shardID)
		if _, alreadyChecked := checkedPartitions[partition]; alreadyChecked {
			continue
		}

		checkedPartitions[partition] = struct{}{}

		if !uc.LagChecker.IsPartitionCaughtUp(ctx, uc.BalanceOperationsTopic, partition) {
			err := fmt.Errorf("consumer lag fence: %w", pkg.ValidateBusinessError(constant.ErrConsumerLagStaleBalance, "GetAccountAndLock"))
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "consumer lag fence triggered for partition", err)

			return err
		}
	}

	return nil
}

func (uc *UseCase) ensureConsumerLagFenceForPartitions(ctx context.Context, partitions []int32) error {
	if !uc.shouldEnforceConsumerLagFence() || len(partitions) == 0 {
		return nil
	}

	checkedPartitions := make(map[int32]struct{}, len(partitions))

	for _, partition := range partitions {
		if _, alreadyChecked := checkedPartitions[partition]; alreadyChecked {
			continue
		}

		checkedPartitions[partition] = struct{}{}

		if !uc.LagChecker.IsPartitionCaughtUp(ctx, uc.BalanceOperationsTopic, partition) {
			return fmt.Errorf("consumer lag fence: %w", pkg.ValidateBusinessError(constant.ErrConsumerLagStaleBalance, "loadAuthorizerBalancesForOperations"))
		}
	}

	return nil
}
