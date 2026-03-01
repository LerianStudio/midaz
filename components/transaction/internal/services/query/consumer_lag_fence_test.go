// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

func TestShouldEnforceConsumerLagFence(t *testing.T) {
	t.Parallel()

	var nilUC *UseCase
	assert.False(t, nilUC.shouldEnforceConsumerLagFence())

	uc := &UseCase{}
	assert.False(t, uc.shouldEnforceConsumerLagFence())

	uc.ConsumerLagFenceEnabled = true
	uc.LagChecker = &stubLagChecker{}
	uc.ShardRouter = shard.NewRouter(8)
	uc.BalanceOperationsTopic = "ledger.balance.operations"
	assert.True(t, uc.shouldEnforceConsumerLagFence())
}

func TestEnsureConsumerLagFenceForAliases(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	ledgerID := uuid.New()
	ctx := context.Background()

	t.Run("returns nil when disabled", func(t *testing.T) {
		t.Parallel()

		uc := &UseCase{}
		err := uc.ensureConsumerLagFenceForAliases(ctx, orgID, ledgerID, []string{"@alice#default"})
		require.NoError(t, err)
	})

	t.Run("returns stale error when partition lagging", func(t *testing.T) {
		t.Parallel()

		router := shard.NewRouter(8)
		partition := int32(router.ResolveBalance("@alice", "default"))

		lagChecker := &stubLagChecker{caughtUpByPartition: map[int32]bool{partition: false}}
		uc := &UseCase{
			ConsumerLagFenceEnabled: true,
			LagChecker:              lagChecker,
			ShardRouter:             router,
			BalanceOperationsTopic:  "ledger.balance.operations",
		}

		err := uc.ensureConsumerLagFenceForAliases(ctx, orgID, ledgerID, []string{"@alice#default"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "synchronized")
	})

	t.Run("deduplicates repeated partitions", func(t *testing.T) {
		t.Parallel()

		router := shard.NewRouter(8)
		lagChecker := &stubLagChecker{caughtUpByPartition: map[int32]bool{}}
		uc := &UseCase{
			ConsumerLagFenceEnabled: true,
			LagChecker:              lagChecker,
			ShardRouter:             router,
			BalanceOperationsTopic:  "ledger.balance.operations",
		}

		err := uc.ensureConsumerLagFenceForAliases(ctx, orgID, ledgerID, []string{"@alice#default", "@alice#default"})
		require.NoError(t, err)
		assert.Equal(t, 1, lagChecker.calls)
	})
}

func TestEnsureConsumerLagFenceForPartitions_DeduplicatesAndFailsOnLag(t *testing.T) {
	t.Parallel()

	lagChecker := &stubLagChecker{caughtUpByPartition: map[int32]bool{1: true, 2: false}}
	uc := &UseCase{
		ConsumerLagFenceEnabled: true,
		LagChecker:              lagChecker,
		ShardRouter:             shard.NewRouter(8),
		BalanceOperationsTopic:  "ledger.balance.operations",
	}

	err := uc.ensureConsumerLagFenceForPartitions(context.Background(), []int32{1, 1, 2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "synchronized")
	assert.Equal(t, 2, lagChecker.calls)
}
