// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package redis_test

import (
	"context"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	tracerRedis "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/redis"
)

// These tests run the dashboard cache against a REAL Valkey rather than
// miniredis. The unit tests pin the behaviour; this pins that the behaviour
// survives a real server — real TTL expiry on a real clock, real key
// namespacing, and a real SET/GET round trip of the encoded bodies.
func startValkey(t *testing.T) goredis.UniversalClient {
	t.Helper()

	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "valkey/valkey:8-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	require.NoError(t, err, "starting valkey container")

	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("cleanup: terminating valkey container: %v", err)
		}
	})

	endpoint, err := container.PortEndpoint(ctx, "6379/tcp", "")
	require.NoError(t, err)

	client := goredis.NewClient(&goredis.Options{Addr: endpoint})
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Ping(ctx).Err(), "valkey must answer PING")

	return client
}

func TestDashboardCache_RealValkey_HitAfterMiss_Integration(t *testing.T) {
	client := startValkey(t)

	repo := &countingRepo{processed: 8123}
	cache := tracerRedis.NewDashboardCache(repo, client, time.Minute, nil)
	ctx := context.Background()
	window := testWindow(t, "30d")

	miss, err := cache.Metrics(ctx, window)
	require.NoError(t, err)
	assert.EqualValues(t, 1, repo.metrics.Load(), "the first read is a miss and computes")

	hit, err := cache.Metrics(ctx, window)
	require.NoError(t, err)
	assert.EqualValues(t, 1, repo.metrics.Load(), "the second read is a hit and does NOT reach the database")

	assert.Equal(t, miss.TransactionsProcessed, hit.TransactionsProcessed,
		"a hit must carry the same figures as the miss that produced it")

	keys, err := client.Keys(ctx, "*").Result()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, "tracer:dashboard:metrics:"+window.CacheKey(), keys[0],
		"an untenanted context writes the bare key")

	ttl, err := client.TTL(ctx, keys[0]).Result()
	require.NoError(t, err)
	assert.Positive(t, ttl, "the entry must carry a TTL, never live forever")
	assert.LessOrEqual(t, ttl, time.Minute)
}

// Real expiry on a real server clock, not a fast-forwarded fake.
func TestDashboardCache_RealValkey_Expiry_Integration(t *testing.T) {
	client := startValkey(t)

	repo := &countingRepo{processed: 5}
	cache := tracerRedis.NewDashboardCache(repo, client, 2*time.Second, nil)
	ctx := context.Background()
	window := testWindow(t, "7d")

	_, err := cache.Metrics(ctx, window)
	require.NoError(t, err)

	_, err = cache.Metrics(ctx, window)
	require.NoError(t, err)
	require.EqualValues(t, 1, repo.metrics.Load(), "still inside the TTL")

	require.Eventually(t, func() bool {
		n, err := client.Exists(ctx, "tracer:dashboard:metrics:"+window.CacheKey()).Result()

		return err == nil && n == 0
	}, 10*time.Second, 250*time.Millisecond, "the entry must expire on its own")

	_, err = cache.Metrics(ctx, window)
	require.NoError(t, err)
	assert.EqualValues(t, 2, repo.metrics.Load(), "past the TTL the figures are recomputed")
}

func TestDashboardCache_RealValkey_TenantIsolation_Integration(t *testing.T) {
	client := startValkey(t)

	repo := &countingRepo{processed: 42}
	cache := tracerRedis.NewDashboardCache(repo, client, time.Minute, nil)
	window := testWindow(t, "30d")

	alpha := tmcore.ContextWithTenantID(context.Background(), "tenant-alpha")
	beta := tmcore.ContextWithTenantID(context.Background(), "tenant-beta")

	_, err := cache.Metrics(alpha, window)
	require.NoError(t, err)

	_, err = cache.Metrics(beta, window)
	require.NoError(t, err)

	assert.EqualValues(t, 2, repo.metrics.Load(), "tenant-beta must not read tenant-alpha's entry")

	keys, err := client.Keys(context.Background(), "*").Result()
	require.NoError(t, err)
	require.Len(t, keys, 2)

	suffix := "tracer:dashboard:metrics:" + window.CacheKey()
	assert.ElementsMatch(t,
		[]string{"tenant:tenant-alpha:" + suffix, "tenant:tenant-beta:" + suffix},
		keys,
		"each tenant's figures live under its own namespaced key")

	// And a repeat read as alpha hits, so the isolation is not bought by
	// making every read a miss.
	_, err = cache.Metrics(alpha, window)
	require.NoError(t, err)
	assert.EqualValues(t, 2, repo.metrics.Load())
}
