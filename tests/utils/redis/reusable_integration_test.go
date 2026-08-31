//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"strings"
	"testing"

	redislib "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestReusableContainerIsolatesLogicalDatabases(t *testing.T) {
	first := SetupReusableContainer(t)
	second := SetupReusableContainer(t)

	require.Equal(t, first.Container.GetContainerID(), second.Container.GetContainerID())
	require.NotEqual(t, first.DB, second.DB)
	require.NoError(t, first.Client.Set(context.Background(), "owned", "first", 0).Err())

	value, err := second.Client.Get(context.Background(), "owned").Result()
	require.ErrorIs(t, err, redislib.Nil)
	require.Empty(t, value)
}

func TestReusableFinancialContainerHasDurableNoEvictionProfile(t *testing.T) {
	container := SetupReusableContainerWithConfig(t, FinancialContainerConfig())
	ctx := context.Background()

	policy, err := container.Client.ConfigGet(ctx, "maxmemory-policy").Result()
	require.NoError(t, err)
	require.Equal(t, "noeviction", policy["maxmemory-policy"])

	appendOnly, err := container.Client.ConfigGet(ctx, "appendonly").Result()
	require.NoError(t, err)
	require.Equal(t, "yes", appendOnly["appendonly"])

	appendFsync, err := container.Client.ConfigGet(ctx, "appendfsync").Result()
	require.NoError(t, err)
	require.Equal(t, "always", appendFsync["appendfsync"])

	persistence, err := container.Client.Info(ctx, "persistence").Result()
	require.NoError(t, err)
	require.Contains(t, strings.ReplaceAll(persistence, "\r", ""), "aof_enabled:1\n")
}

func TestReusableContainerFlushesTheOwningTestsDatabase(t *testing.T) {
	observer := SetupReusableContainer(t)

	var ownedDB int
	require.True(t, t.Run("owner", func(t *testing.T) {
		owned := SetupReusableContainer(t)
		ownedDB = owned.DB
		require.NoError(t, owned.Client.Set(context.Background(), "owned", "value", 0).Err())
	}))

	client := redislib.NewClient(&redislib.Options{Addr: observer.Addr, DB: ownedDB})
	t.Cleanup(func() { _ = client.Close() })
	size, err := client.DBSize(context.Background()).Result()
	require.NoError(t, err)
	require.Zero(t, size)
}
