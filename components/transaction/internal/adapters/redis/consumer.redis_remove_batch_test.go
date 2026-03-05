// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"strings"
	"testing"

	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEvalClient is a mock Redis client for testing RemoveBalanceSyncKeysBatch.
type mockEvalClient struct {
	redis.UniversalClient
	evalFunc func(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd
}

func (m *mockEvalClient) Eval(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd {
	if m.evalFunc != nil {
		return m.evalFunc(ctx, script, keys, args...)
	}

	return redis.NewCmd(ctx)
}

func newMockEvalConnection(client *mockEvalClient) *libRedis.RedisConnection {
	return &libRedis.RedisConnection{
		Client:    client,
		Connected: true,
	}
}

func TestRemoveBalanceSyncKeysBatch_EmptyInput(t *testing.T) {
	// Create a repository with nil connection to test early return
	repo := &RedisConsumerRepository{
		conn:               nil,
		balanceSyncEnabled: true,
	}

	// Empty input should return 0 without any Redis call
	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{})

	assert.NoError(t, err, "Empty keys should return nil error")
	assert.Equal(t, int64(0), count, "Empty keys should return 0 count")
}

func TestRemoveBalanceSyncKeysBatch_EmptyInput_NoRedisCall(t *testing.T) {
	// Create mock that fails if called
	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			t.Fatal("Eval should not be called for empty input")

			return nil
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{})

	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestRemoveBalanceSyncKeysBatch_SingleKey(t *testing.T) {
	var capturedScript string

	var capturedKeys []string

	var capturedArgs []any

	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, script string, keys []string, args ...any) *redis.Cmd {
			capturedScript = script
			capturedKeys = keys
			capturedArgs = args

			cmd := redis.NewCmd(context.Background())
			cmd.SetVal(int64(1)) // 1 key removed

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"balance:key1"})

	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.NotEmpty(t, capturedScript, "Lua script should be passed")
	assert.Len(t, capturedKeys, 1, "Should have 1 key (schedule key)")
	assert.Len(t, capturedArgs, 2, "Should have lock prefix + 1 member")
}

func TestRemoveBalanceSyncKeysBatch_MultipleKeys(t *testing.T) {
	var capturedArgs []any

	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, _ string, _ []string, args ...any) *redis.Cmd {
			capturedArgs = args

			cmd := redis.NewCmd(context.Background())
			cmd.SetVal(int64(3)) // All 3 keys removed

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"key1", "key2", "key3"})

	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
	// Args should be: [lockPrefix, key1, key2, key3]
	assert.Len(t, capturedArgs, 4, "Should have lock prefix + 3 members")
}

func TestRemoveBalanceSyncKeysBatch_PartialRemoval(t *testing.T) {
	// Test when some keys did not exist in the schedule
	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			cmd := redis.NewCmd(context.Background())
			// Only 2 out of 3 keys existed and were removed
			cmd.SetVal(int64(2))

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"key1", "key2", "key3"})

	require.NoError(t, err)
	assert.Equal(t, int64(2), count, "Should return actual count of removed keys")
}

func TestRemoveBalanceSyncKeysBatch_RedisError(t *testing.T) {
	expectedError := errors.New("redis script error")

	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			cmd := redis.NewCmd(context.Background())
			cmd.SetErr(expectedError)

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"balance:key1"})

	require.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Equal(t, int64(0), count)
}

func TestRemoveBalanceSyncKeysBatch_UnexpectedResultType(t *testing.T) {
	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			cmd := redis.NewCmd(context.Background())
			// Return string instead of int64 - unexpected type
			cmd.SetVal("not an int64")

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"balance:key1"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected result type")
	assert.Equal(t, int64(0), count)
}

func TestRemoveBalanceSyncKeysBatch_LuaScriptContent(t *testing.T) {
	// Verify the Lua script is embedded and contains expected commands
	require.NotEmpty(t, removeBalanceSyncKeysBatchScript, "Lua script should be embedded and not empty")

	// Verify script contains expected commands
	assert.True(t, strings.Contains(removeBalanceSyncKeysBatchScript, "ZREM"),
		"Lua script should contain ZREM command for schedule removal")

	assert.True(t, strings.Contains(removeBalanceSyncKeysBatchScript, "DEL"),
		"Lua script should contain DEL command for lock cleanup")

	assert.True(t, strings.Contains(removeBalanceSyncKeysBatchScript, "KEYS[1]"),
		"Lua script should reference KEYS[1] for schedule key")

	assert.True(t, strings.Contains(removeBalanceSyncKeysBatchScript, "ARGV"),
		"Lua script should reference ARGV for lock prefix and members")
}

func TestRemoveBalanceSyncKeysBatch_ScriptUsesCorrectPattern(t *testing.T) {
	var capturedScript string

	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, script string, _ []string, _ ...any) *redis.Cmd {
			capturedScript = script

			cmd := redis.NewCmd(context.Background())
			cmd.SetVal(int64(1))

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	_, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"balance:key1"})

	require.NoError(t, err)

	// Verify the embedded script is passed to Eval
	assert.Equal(t, removeBalanceSyncKeysBatchScript, capturedScript,
		"Should pass the embedded Lua script to Eval")
}

func TestRemoveBalanceSyncKeysBatch_ZeroKeysRemoved(t *testing.T) {
	// Test when keys don't exist in the schedule
	mockClient := &mockEvalClient{
		evalFunc: func(_ context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
			cmd := redis.NewCmd(context.Background())
			cmd.SetVal(int64(0)) // No keys removed (they didn't exist)

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockEvalConnection(mockClient),
		balanceSyncEnabled: true,
	}

	count, err := repo.RemoveBalanceSyncKeysBatch(context.Background(), []string{"nonexistent:key1", "nonexistent:key2"})

	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "Should return 0 when no keys were removed")
}

func TestRemoveBalanceSyncKeysBatch_InterfaceCompliance(t *testing.T) {
	type BatchRemover interface {
		RemoveBalanceSyncKeysBatch(ctx context.Context, keys []string) (int64, error)
	}

	var _ BatchRemover = (*RedisConsumerRepository)(nil)
}
