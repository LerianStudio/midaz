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

func TestIntegration_Del_RemovesOnlyTheAddressedKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	client := infra.redisContainer.Client

	seed := func(t *testing.T, physicalKey string) {
		t.Helper()

		require.NoError(t, client.Set(context.Background(), physicalKey, "value", time.Minute).Err())
		t.Cleanup(func() { _ = client.Del(context.Background(), physicalKey).Err() })
	}

	exists := func(t *testing.T, physicalKey string) bool {
		t.Helper()

		n, err := client.Exists(context.Background(), physicalKey).Result()
		require.NoError(t, err)

		return n == 1
	}

	t.Run("deletes an existing key without tenant", func(t *testing.T) {
		key := "del:" + uuid.NewString()
		seed(t, key)

		require.NoError(t, infra.repo.Del(context.Background(), key))

		assert.False(t, exists(t, key))
	})

	t.Run("deletes the tenant-namespaced key and leaves the raw key", func(t *testing.T) {
		tenantID := "del-tenant-" + uuid.NewString()
		ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
		key := "del:" + uuid.NewString()
		physicalKey := "tenant:" + tenantID + ":" + key
		seed(t, physicalKey)
		seed(t, key)

		require.NoError(t, infra.repo.Del(ctx, key))

		assert.False(t, exists(t, physicalKey), "the namespaced key must be deleted")
		assert.True(t, exists(t, key), "the raw key outside the tenant must stay")
	})

	t.Run("absent key is not an error", func(t *testing.T) {
		key := "del:" + uuid.NewString()

		require.NoError(t, infra.repo.Del(context.Background(), key))

		assert.False(t, exists(t, key))
	})

	t.Run("leaves neighbouring keys untouched", func(t *testing.T) {
		prefix := "del:" + uuid.NewString()
		key := prefix + ":target"
		neighbours := []string{prefix, prefix + ":other", prefix + ":target:suffix"}

		seed(t, key)

		for _, neighbour := range neighbours {
			seed(t, neighbour)
		}

		require.NoError(t, infra.repo.Del(context.Background(), key))

		assert.False(t, exists(t, key))

		for _, neighbour := range neighbours {
			assert.True(t, exists(t, neighbour), "neighbour %s must stay", neighbour)
		}
	})
}
