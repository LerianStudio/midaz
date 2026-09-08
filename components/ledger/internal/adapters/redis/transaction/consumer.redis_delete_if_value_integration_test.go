//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_DeleteIfValue_AtomicOwnershipAndExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)

	t.Run("matching token deletes marker", func(t *testing.T) {
		ctx := context.Background()
		key := "delete-if-value:" + uuid.NewString()
		owner := "owner-" + uuid.NewString()

		require.NoError(t, infra.repo.Set(ctx, key, owner, 30))
		deleted, err := infra.repo.DeleteIfValue(ctx, key, owner)

		require.NoError(t, err)
		assert.True(t, deleted)
		exists, existsErr := infra.redisContainer.Client.Exists(ctx, key).Result()
		require.NoError(t, existsErr)
		assert.Zero(t, exists)
	})

	t.Run("mismatched token preserves marker", func(t *testing.T) {
		ctx := context.Background()
		key := "delete-if-value:" + uuid.NewString()
		owner := "owner-" + uuid.NewString()

		require.NoError(t, infra.repo.Set(ctx, key, owner, 30))
		deleted, err := infra.repo.DeleteIfValue(ctx, key, "different-owner")

		require.NoError(t, err)
		assert.False(t, deleted)
		value, getErr := infra.repo.Get(ctx, key)
		require.NoError(t, getErr)
		assert.Equal(t, owner, value)
	})

	t.Run("tenant namespacing is preserved while deleting", func(t *testing.T) {
		tenantID := "delete-if-value-tenant-" + uuid.NewString()
		ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
		key := "delete-if-value:" + uuid.NewString()
		owner := "owner-" + uuid.NewString()

		require.NoError(t, infra.repo.Set(ctx, key, owner, 30))
		deleted, err := infra.repo.DeleteIfValue(ctx, key, owner)

		require.NoError(t, err)
		assert.True(t, deleted)
		physicalKey := "tenant:" + tenantID + ":" + key
		exists, existsErr := infra.redisContainer.Client.Exists(context.Background(), physicalKey).Result()
		require.NoError(t, existsErr)
		assert.Zero(t, exists)
	})

	t.Run("expired marker is not deleted", func(t *testing.T) {
		ctx := context.Background()
		key := "delete-if-value:" + uuid.NewString()
		owner := "owner-" + uuid.NewString()

		require.NoError(t, infra.repo.Set(ctx, key, owner, 1))
		require.Eventually(t, func() bool {
			exists, err := infra.redisContainer.Client.Exists(ctx, key).Result()

			return err == nil && exists == 0
		}, 3*time.Second, 100*time.Millisecond, "marker should expire before the ownership check")

		deleted, err := infra.repo.DeleteIfValue(ctx, key, owner)
		require.NoError(t, err)
		assert.False(t, deleted, "an expired marker must report that no owned key was deleted")
	})

	t.Run("owned marker can be shortened and recreated after the short window", func(t *testing.T) {
		ctx := context.Background()
		key := "delete-if-value-recreate:" + uuid.NewString()
		owner := "owner-" + uuid.NewString()

		require.NoError(t, infra.repo.Set(ctx, key, owner, 30))
		shortened, err := infra.repo.ExpireIfValue(ctx, key, owner, 1)
		require.NoError(t, err)
		assert.True(t, shortened)
		require.Eventually(t, func() bool {
			exists, existsErr := infra.redisContainer.Client.Exists(ctx, key).Result()

			return existsErr == nil && exists == 0
		}, 3*time.Second, 100*time.Millisecond, "shortened marker should expire")

		recreated, recreateErr := infra.repo.SetNX(ctx, key, "new-owner", 30)
		require.NoError(t, recreateErr)
		assert.True(t, recreated, "a balance can be recreated after the short marker window")
	})
}
