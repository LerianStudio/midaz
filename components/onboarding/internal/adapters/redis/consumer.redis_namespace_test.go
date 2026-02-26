// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"
	"time"

	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingRedisClient is a stub that records the keys passed to Redis operations.
// It embeds redis.UniversalClient (nil) and overrides the methods we need to capture.
type recordingRedisClient struct {
	t        *testing.T
	setCalls []recordedSetCall
	getCalls []string
	delCalls []string
	redis.UniversalClient
}

type recordedSetCall struct {
	Key   string
	Value any
	TTL   time.Duration
}

func (r *recordingRedisClient) Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	r.setCalls = append(r.setCalls, recordedSetCall{Key: key, Value: value, TTL: expiration})

	cmd := redis.NewStatusCmd(ctx)
	cmd.SetVal("OK")

	return cmd
}

func (r *recordingRedisClient) Get(ctx context.Context, key string) *redis.StringCmd {
	r.getCalls = append(r.getCalls, key)

	cmd := redis.NewStringCmd(ctx)
	cmd.SetVal("test-value")

	return cmd
}

func (r *recordingRedisClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	r.delCalls = append(r.delCalls, keys...)

	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(1)

	return cmd
}

func newRecordingConnection(t *testing.T) (*libRedis.RedisConnection, *recordingRedisClient) {
	t.Helper()

	client := &recordingRedisClient{t: t}

	return &libRedis.RedisConnection{
		Client:    client,
		Connected: true,
	}, client
}

// =============================================================================
// UNIT TESTS — Redis Key Namespacing (T-002)
// =============================================================================

// TestKeyNamespacing_SimpleKeyMethods verifies that Set, Get, and Del namespace
// their key parameter when a tenantId is present in the context, and leave keys
// unchanged when no tenantId is present.
func TestKeyNamespacing_SimpleKeyMethods(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		tenantID    string
		originalKey string
		expectedKey string
	}{
		{
			name:        "with_tenant_id_key_is_namespaced",
			tenantID:    "test-tenant",
			originalKey: "my:key",
			expectedKey: "tenant:test-tenant:my:key",
		},
		{
			name:        "without_tenant_id_key_is_unchanged",
			tenantID:    "",
			originalKey: "my:key",
			expectedKey: "my:key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn, recorder := newRecordingConnection(t)
			repo := &RedisConsumerRepository{conn: conn}

			ctx := context.Background()
			if tc.tenantID != "" {
				ctx = tmcore.SetTenantIDInContext(ctx, tc.tenantID)
			}

			// Test Set
			err := repo.Set(ctx, tc.originalKey, "val", time.Minute)
			require.NoError(t, err)
			require.Len(t, recorder.setCalls, 1, "Set should record one call")
			assert.Equal(t, tc.expectedKey, recorder.setCalls[0].Key,
				"Set: Redis should receive the namespaced key")

			// Test Get
			_, err = repo.Get(ctx, tc.originalKey)
			require.NoError(t, err)
			require.Len(t, recorder.getCalls, 1, "Get should record one call")
			assert.Equal(t, tc.expectedKey, recorder.getCalls[0],
				"Get: Redis should receive the namespaced key")

			// Test Del
			err = repo.Del(ctx, tc.originalKey)
			require.NoError(t, err)
			require.Len(t, recorder.delCalls, 1, "Del should record one call")
			assert.Equal(t, tc.expectedKey, recorder.delCalls[0],
				"Del: Redis should receive the namespaced key")
		})
	}
}

// TestKeyNamespacing_BackwardsCompatible_NoTenantInContext is a focused regression test
// confirming that all namespaced methods are backwards compatible: when no tenantID
// is present in the context, every Redis key is IDENTICAL to the key that was used
// before the namespacing change.
func TestKeyNamespacing_BackwardsCompatible_NoTenantInContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	conn, recorder := newRecordingConnection(t)
	repo := &RedisConsumerRepository{conn: conn}

	originalKey := "my:original:key"

	_ = repo.Set(ctx, originalKey, "v", time.Minute)
	assert.Equal(t, originalKey, recorder.setCalls[0].Key, "Set: key must be unchanged without tenant")

	_, _ = repo.Get(ctx, originalKey)
	assert.Equal(t, originalKey, recorder.getCalls[0], "Get: key must be unchanged without tenant")

	_ = repo.Del(ctx, originalKey)
	assert.Equal(t, originalKey, recorder.delCalls[0], "Del: key must be unchanged without tenant")
}
