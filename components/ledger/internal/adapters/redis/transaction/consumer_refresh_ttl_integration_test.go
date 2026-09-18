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
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// refreshTTLFixture isolates one scenario in its own tenant namespace, so the two
// schedule ZSETs it plants cannot collide with a concurrent scenario on the shared
// container.
type refreshTTLFixture struct {
	ctx                 context.Context
	client              goredis.UniversalClient
	scheduleKey         string
	legacyKey           string
	keyPrefix           string
	keepaliveTTL        time.Duration
	shortTTL            time.Duration
	minRefreshedSeconds float64
}

func newRefreshTTLFixture(t *testing.T, infra *integrationTestInfra) *refreshTTLFixture {
	t.Helper()

	tenantID := "refresh-ttl-" + uuid.NewString()
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
	prefix := "tenant:" + tenantID + ":"

	return &refreshTTLFixture{
		ctx:                 ctx,
		client:              infra.redisContainer.Client,
		scheduleKey:         prefix + utils.BalanceSyncScheduleKey,
		legacyKey:           prefix + utils.BalanceSyncScheduleKeyLegacy,
		keyPrefix:           prefix + "balance:" + cachepolicy.HashTag + ":",
		keepaliveTTL:        cachepolicy.BalanceTTL,
		shortTTL:            60 * time.Second,
		minRefreshedSeconds: (cachepolicy.BalanceTTL - 10*time.Minute).Seconds(),
	}
}

// scheduleBalance plants a balance key with a short TTL and schedules it at score.
func (f *refreshTTLFixture) scheduleBalance(t *testing.T, zsetKey, name string, score float64) string {
	t.Helper()

	key := f.keyPrefix + name

	require.NoError(t, f.client.Set(context.Background(), key, "{}", f.shortTTL).Err())
	f.scheduleMember(t, zsetKey, key, score)

	return key
}

// scheduleMember adds a member to a schedule ZSET without creating its key.
func (f *refreshTTLFixture) scheduleMember(t *testing.T, zsetKey, member string, score float64) {
	t.Helper()

	require.NoError(t, f.client.ZAdd(context.Background(), zsetKey, goredis.Z{Score: score, Member: member}).Err())
}

func (f *refreshTTLFixture) ttlSeconds(t *testing.T, key string) float64 {
	t.Helper()

	ttl, err := f.client.TTL(context.Background(), key).Result()
	require.NoError(t, err)

	return ttl.Seconds()
}

func TestIntegration_RefreshBalanceSyncKeyTTLs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)

	t.Run("scheduled keys in both ZSETs are refreshed", func(t *testing.T) {
		f := newRefreshTTLFixture(t, infra)

		current := []string{
			f.scheduleBalance(t, f.scheduleKey, "current-a", 1000),
			f.scheduleBalance(t, f.scheduleKey, "current-b", 2000),
		}
		legacy := f.scheduleBalance(t, f.legacyKey, "legacy-a", 3000)

		refreshed, _, err := infra.repo.RefreshBalanceSyncKeyTTLs(f.ctx, f.keepaliveTTL)

		require.NoError(t, err)
		assert.Equal(t, int64(3), refreshed, "both schedules must be covered")

		for _, key := range append(current, legacy) {
			assert.GreaterOrEqual(t, f.ttlSeconds(t, key), f.minRefreshedSeconds,
				"a scheduled key must carry the full balance TTL again")
		}
	})

	t.Run("unscheduled balance key keeps its original TTL", func(t *testing.T) {
		f := newRefreshTTLFixture(t, infra)

		scheduled := f.scheduleBalance(t, f.scheduleKey, "scheduled", 1000)

		untouched := f.keyPrefix + "unscheduled"
		require.NoError(t, f.client.Set(context.Background(), untouched, "{}", f.shortTTL).Err())

		refreshed, _, err := infra.repo.RefreshBalanceSyncKeyTTLs(f.ctx, f.keepaliveTTL)

		require.NoError(t, err)
		assert.Equal(t, int64(1), refreshed)
		assert.GreaterOrEqual(t, f.ttlSeconds(t, scheduled), f.minRefreshedSeconds)
		assert.LessOrEqual(t, f.ttlSeconds(t, untouched), f.shortTTL.Seconds(),
			"a key outside the schedule must not be touched")
	})

	t.Run("orphan member without a key is a no-op", func(t *testing.T) {
		f := newRefreshTTLFixture(t, infra)

		scheduled := f.scheduleBalance(t, f.scheduleKey, "alive", 1000)
		f.scheduleMember(t, f.scheduleKey, f.keyPrefix+"orphan", 2000)

		refreshed, _, err := infra.repo.RefreshBalanceSyncKeyTTLs(f.ctx, f.keepaliveTTL)

		require.NoError(t, err)
		assert.Equal(t, int64(1), refreshed, "a member with no key must not be counted as refreshed")
		assert.GreaterOrEqual(t, f.ttlSeconds(t, scheduled), f.minRefreshedSeconds)
	})

	t.Run("oldest score is the smallest across both schedules", func(t *testing.T) {
		f := newRefreshTTLFixture(t, infra)

		f.scheduleBalance(t, f.scheduleKey, "newer", 5000)
		f.scheduleBalance(t, f.scheduleKey, "newest", 9000)
		f.scheduleBalance(t, f.legacyKey, "oldest", 1500)

		refreshed, oldestScore, err := infra.repo.RefreshBalanceSyncKeyTTLs(f.ctx, f.keepaliveTTL)

		require.NoError(t, err)
		assert.Equal(t, int64(3), refreshed)
		assert.InDelta(t, 1500.0, oldestScore, 0.001, "the oldest due time drives the pending-age gauge")
	})

	t.Run("empty schedules report nothing pending", func(t *testing.T) {
		f := newRefreshTTLFixture(t, infra)

		refreshed, oldestScore, err := infra.repo.RefreshBalanceSyncKeyTTLs(f.ctx, f.keepaliveTTL)

		require.NoError(t, err)
		assert.Zero(t, refreshed)
		assert.Zero(t, oldestScore)
	})
}
