// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMGetClient is a mock Redis client for testing GetBalancesByKeys.
type mockMGetClient struct {
	redis.UniversalClient
	mGetFunc func(ctx context.Context, keys ...string) *redis.SliceCmd
}

func (m *mockMGetClient) MGet(ctx context.Context, keys ...string) *redis.SliceCmd {
	if m.mGetFunc != nil {
		return m.mGetFunc(ctx, keys...)
	}

	return redis.NewSliceCmd(ctx)
}

func newMockMGetConnection(client *mockMGetClient) *staticRedisProvider {
	return &staticRedisProvider{client: client}
}

func TestGetBalancesByKeys_EmptyInput(t *testing.T) {
	// Create a repository with nil connection to test early return
	repo := &RedisConsumerRepository{
		conn: nil,
	}

	// Empty input should return empty map without any Redis call
	result, err := repo.GetBalancesByKeys(context.Background(), []string{})

	assert.NoError(t, err, "Empty keys should return nil error")
	assert.NotNil(t, result, "Result should not be nil")
	assert.Empty(t, result, "Empty keys should return empty map")
}

func TestGetBalancesByKeys_EmptyInput_NoRedisCall(t *testing.T) {
	// Create mock that fails if called
	mockClient := &mockMGetClient{
		mGetFunc: func(_ context.Context, _ ...string) *redis.SliceCmd {
			t.Fatal("MGet should not be called for empty input")

			return nil
		},
	}

	repo := &RedisConsumerRepository{
		conn: newMockMGetConnection(mockClient),
	}

	result, err := repo.GetBalancesByKeys(context.Background(), []string{})

	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestGetBalancesByKeys_SingleKey_Found(t *testing.T) {
	validJSON := `{"id":"uuid-123","alias":"@sender","key":"default","accountId":"acc-1","assetCode":"USD","available":"100.00","onHold":"10.00","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1}`

	mockClient := &mockMGetClient{
		mGetFunc: func(_ context.Context, _ ...string) *redis.SliceCmd {
			cmd := redis.NewSliceCmd(context.Background())
			cmd.SetVal([]any{validJSON})

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn: newMockMGetConnection(mockClient),
	}

	result, err := repo.GetBalancesByKeys(context.Background(), []string{"balance:key1"})

	require.NoError(t, err)
	require.Len(t, result, 1)
	require.NotNil(t, result["balance:key1"])
	assert.Equal(t, "uuid-123", result["balance:key1"].ID)
	assert.Equal(t, "@sender", result["balance:key1"].Alias)
	assert.Equal(t, "USD", result["balance:key1"].AssetCode)
	assert.True(t, result["balance:key1"].Available.Equal(decimal.NewFromFloat(100.00)))
}

func TestGetBalancesByKeys_SingleKey_NotFound(t *testing.T) {
	mockClient := &mockMGetClient{
		mGetFunc: func(_ context.Context, _ ...string) *redis.SliceCmd {
			cmd := redis.NewSliceCmd(context.Background())
			cmd.SetVal([]any{nil}) // Key not found

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn: newMockMGetConnection(mockClient),
	}

	result, err := repo.GetBalancesByKeys(context.Background(), []string{"balance:key1"})

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Nil(t, result["balance:key1"], "Missing key should have nil value in map")
}

func TestGetBalancesByKeys_MultipleKeys_MixedResults(t *testing.T) {
	validJSON1 := `{"id":"uuid-1","alias":"@sender","key":"default","accountId":"acc-1","assetCode":"USD","available":"100.00","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1}`
	validJSON2 := `{"id":"uuid-2","alias":"@receiver","key":"default","accountId":"acc-2","assetCode":"USD","available":"500.00","onHold":"50.00","version":2,"accountType":"deposit","allowSending":1,"allowReceiving":1}`

	mockClient := &mockMGetClient{
		mGetFunc: func(_ context.Context, _ ...string) *redis.SliceCmd {
			cmd := redis.NewSliceCmd(context.Background())
			// key1 found, key2 not found, key3 found
			cmd.SetVal([]any{validJSON1, nil, validJSON2})

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn: newMockMGetConnection(mockClient),
	}

	result, err := repo.GetBalancesByKeys(context.Background(), []string{"key1", "key2", "key3"})

	require.NoError(t, err)
	require.Len(t, result, 3)

	// key1 - found
	require.NotNil(t, result["key1"])
	assert.Equal(t, "uuid-1", result["key1"].ID)

	// key2 - not found
	assert.Nil(t, result["key2"])

	// key3 - found
	require.NotNil(t, result["key3"])
	assert.Equal(t, "uuid-2", result["key3"].ID)
}

func TestGetBalancesByKeys_MalformedJSON(t *testing.T) {
	validJSON := `{"id":"uuid-1","alias":"@sender","key":"default","accountId":"acc-1","assetCode":"USD","available":"100.00","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1}`
	malformedJSON := `{invalid json`

	mockClient := &mockMGetClient{
		mGetFunc: func(_ context.Context, _ ...string) *redis.SliceCmd {
			cmd := redis.NewSliceCmd(context.Background())
			// key1 valid, key2 malformed, key3 valid
			cmd.SetVal([]any{validJSON, malformedJSON, validJSON})

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn: newMockMGetConnection(mockClient),
	}

	result, err := repo.GetBalancesByKeys(context.Background(), []string{"key1", "key2", "key3"})

	// Should not return error - malformed keys logged and set to nil
	require.NoError(t, err)
	require.Len(t, result, 3)

	// key1 - valid
	require.NotNil(t, result["key1"])

	// key2 - malformed JSON should result in nil (logged, not fatal)
	assert.Nil(t, result["key2"], "Malformed JSON should result in nil value")

	// key3 - valid
	require.NotNil(t, result["key3"])
}

func TestGetBalancesByKeys_ByteSliceValue(t *testing.T) {
	// Test that []byte values are handled correctly
	validJSON := []byte(`{"id":"uuid-bytes","alias":"@test","key":"default","accountId":"acc-1","assetCode":"BRL","available":"200.00","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1}`)

	mockClient := &mockMGetClient{
		mGetFunc: func(_ context.Context, _ ...string) *redis.SliceCmd {
			cmd := redis.NewSliceCmd(context.Background())
			cmd.SetVal([]any{validJSON})

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn: newMockMGetConnection(mockClient),
	}

	result, err := repo.GetBalancesByKeys(context.Background(), []string{"balance:key1"})

	require.NoError(t, err)
	require.Len(t, result, 1)
	require.NotNil(t, result["balance:key1"])
	assert.Equal(t, "uuid-bytes", result["balance:key1"].ID)
	assert.Equal(t, "BRL", result["balance:key1"].AssetCode)
}

func TestGetBalancesByKeys_UnexpectedValueType(t *testing.T) {
	mockClient := &mockMGetClient{
		mGetFunc: func(_ context.Context, _ ...string) *redis.SliceCmd {
			cmd := redis.NewSliceCmd(context.Background())
			// Return an integer instead of string - unexpected type
			cmd.SetVal([]any{12345})

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn: newMockMGetConnection(mockClient),
	}

	result, err := repo.GetBalancesByKeys(context.Background(), []string{"balance:key1"})

	// Should not error - unexpected type logged and set to nil
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Nil(t, result["balance:key1"], "Unexpected type should result in nil value")
}

func TestGetBalancesByKeys_RedisError(t *testing.T) {
	expectedError := errors.New("redis connection timeout")

	mockClient := &mockMGetClient{
		mGetFunc: func(_ context.Context, _ ...string) *redis.SliceCmd {
			cmd := redis.NewSliceCmd(context.Background())
			cmd.SetErr(expectedError)

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn: newMockMGetConnection(mockClient),
	}

	result, err := repo.GetBalancesByKeys(context.Background(), []string{"balance:key1"})

	require.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, result, "Result should be nil on error")
}

func TestGetBalancesByKeys_AllKeysNotFound(t *testing.T) {
	mockClient := &mockMGetClient{
		mGetFunc: func(_ context.Context, _ ...string) *redis.SliceCmd {
			cmd := redis.NewSliceCmd(context.Background())
			cmd.SetVal([]any{nil, nil, nil})

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn: newMockMGetConnection(mockClient),
	}

	result, err := repo.GetBalancesByKeys(context.Background(), []string{"key1", "key2", "key3"})

	require.NoError(t, err)
	require.Len(t, result, 3)

	// All keys should be present in map with nil values
	assert.Nil(t, result["key1"])
	assert.Nil(t, result["key2"])
	assert.Nil(t, result["key3"])
}

func TestGetBalancesByKeys_InterfaceCompliance(t *testing.T) {
	// Type assertion to verify method exists with correct signature
	type BalanceGetter interface {
		GetBalancesByKeys(ctx context.Context, keys []string) (map[string]*mmodel.BalanceRedis, error)
	}

	// This line will fail to compile if method does not exist or has wrong signature
	var _ BalanceGetter = (*RedisConsumerRepository)(nil)
}
