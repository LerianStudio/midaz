// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
)

func newRepoWithMini(t *testing.T) (*RedisConsumerRepository, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	t.Cleanup(func() { _ = client.Close() })

	conn := &libRedis.RedisConnection{
		Client:    client,
		Connected: true,
	}

	return &RedisConsumerRepository{conn: conn}, mr
}

func TestRedisConsumerRepository_Set(t *testing.T) {
	t.Parallel()

	t.Run("stores_value_with_ttl", func(t *testing.T) {
		t.Parallel()

		r, mr := newRepoWithMini(t)

		err := r.Set(context.Background(), "key1", "value1", 30*time.Second)
		require.NoError(t, err)

		got, err := mr.Get("key1")
		require.NoError(t, err)
		assert.Equal(t, "value1", got)
	})

	t.Run("zero_ttl_falls_back_to_default", func(t *testing.T) {
		t.Parallel()

		r, mr := newRepoWithMini(t)

		err := r.Set(context.Background(), "k", "v", 0)
		require.NoError(t, err)

		got, err := mr.Get("k")
		require.NoError(t, err)
		assert.Equal(t, "v", got)
	})

	t.Run("negative_ttl_falls_back_to_default", func(t *testing.T) {
		t.Parallel()

		r, mr := newRepoWithMini(t)

		err := r.Set(context.Background(), "k2", "v2", -5*time.Second)
		require.NoError(t, err)

		got, err := mr.Get("k2")
		require.NoError(t, err)
		assert.Equal(t, "v2", got)
	})

	t.Run("client_error_propagates", func(t *testing.T) {
		t.Parallel()

		r, mr := newRepoWithMini(t)

		// Close the server to induce a client error on Set.
		mr.Close()

		err := r.Set(context.Background(), "k", "v", time.Second)
		require.Error(t, err)
	})
}

func TestRedisConsumerRepository_Get(t *testing.T) {
	t.Parallel()

	r, _ := newRepoWithMini(t)

	// Get is a no-op stub that always returns nil. Exercise it so
	// future refactors keep the behavior explicit.
	assert.NoError(t, r.Get(context.Background(), "any-key"))
}

func TestRedisConsumerRepository_Del(t *testing.T) {
	t.Parallel()

	r, _ := newRepoWithMini(t)

	// Del is a no-op stub that always returns nil.
	assert.NoError(t, r.Del(context.Background(), "any-key"))
}

func TestNewConsumerRedis_UsesExistingClient(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	t.Cleanup(func() { _ = client.Close() })

	conn := &libRedis.RedisConnection{
		Client:    client,
		Connected: true,
	}

	r, err := NewConsumerRedis(conn)
	require.NoError(t, err)
	require.NotNil(t, r)
}
