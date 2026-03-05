// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockZAddNXClient is a mock Redis client for testing ScheduleBalanceSyncBatch.
type mockZAddNXClient struct {
	redis.UniversalClient
	zAddNXFunc func(ctx context.Context, key string, members ...redis.Z) *redis.IntCmd
}

func (m *mockZAddNXClient) ZAddNX(ctx context.Context, key string, members ...redis.Z) *redis.IntCmd {
	if m.zAddNXFunc != nil {
		return m.zAddNXFunc(ctx, key, members...)
	}

	return redis.NewIntCmd(ctx)
}

func newMockZAddNXConnection(client *mockZAddNXClient) *libRedis.RedisConnection {
	return &libRedis.RedisConnection{
		Client:    client,
		Connected: true,
	}
}

func TestScheduleBalanceSyncBatch_EmptyInput(t *testing.T) {
	// Create a repository with nil connection to test early return
	repo := &RedisConsumerRepository{
		conn:               nil,
		balanceSyncEnabled: true,
	}

	// Empty input should return nil without any Redis call
	err := repo.ScheduleBalanceSyncBatch(context.Background(), []redis.Z{})

	assert.NoError(t, err, "Empty batch should return nil without error")
}

func TestScheduleBalanceSyncBatch_EmptyInput_NoRedisCall(t *testing.T) {
	// Create mock that fails if called
	mockClient := &mockZAddNXClient{
		zAddNXFunc: func(_ context.Context, _ string, _ ...redis.Z) *redis.IntCmd {
			t.Fatal("ZAddNX should not be called for empty input")

			return nil
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockZAddNXConnection(mockClient),
		balanceSyncEnabled: true,
	}

	err := repo.ScheduleBalanceSyncBatch(context.Background(), []redis.Z{})

	assert.NoError(t, err, "Empty batch should return nil without error")
}

func TestScheduleBalanceSyncBatch_SingleMember(t *testing.T) {
	var capturedKey string

	var capturedMembers []redis.Z

	mockClient := &mockZAddNXClient{
		zAddNXFunc: func(_ context.Context, key string, members ...redis.Z) *redis.IntCmd {
			capturedKey = key
			capturedMembers = members

			cmd := redis.NewIntCmd(context.Background())
			cmd.SetVal(1) // 1 member added

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockZAddNXConnection(mockClient),
		balanceSyncEnabled: true,
	}

	members := []redis.Z{
		{Score: float64(time.Now().Unix()), Member: "balance:key1"},
	}

	err := repo.ScheduleBalanceSyncBatch(context.Background(), members)

	require.NoError(t, err)
	assert.NotEmpty(t, capturedKey, "Schedule key should be set")
	assert.Len(t, capturedMembers, 1, "Should have 1 member")
	assert.Equal(t, "balance:key1", capturedMembers[0].Member)
}

func TestScheduleBalanceSyncBatch_MultipleMembers(t *testing.T) {
	var capturedMembers []redis.Z

	mockClient := &mockZAddNXClient{
		zAddNXFunc: func(_ context.Context, _ string, members ...redis.Z) *redis.IntCmd {
			capturedMembers = members

			cmd := redis.NewIntCmd(context.Background())
			cmd.SetVal(int64(len(members))) // All members added

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockZAddNXConnection(mockClient),
		balanceSyncEnabled: true,
	}

	now := time.Now().Unix()
	members := []redis.Z{
		{Score: float64(now + 100), Member: "balance:key1"},
		{Score: float64(now + 200), Member: "balance:key2"},
		{Score: float64(now + 300), Member: "balance:key3"},
	}

	err := repo.ScheduleBalanceSyncBatch(context.Background(), members)

	require.NoError(t, err)
	assert.Len(t, capturedMembers, 3, "Should have 3 members")
}

func TestScheduleBalanceSyncBatch_RedisError(t *testing.T) {
	expectedError := errors.New("redis connection refused")

	mockClient := &mockZAddNXClient{
		zAddNXFunc: func(_ context.Context, _ string, _ ...redis.Z) *redis.IntCmd {
			cmd := redis.NewIntCmd(context.Background())
			cmd.SetErr(expectedError)

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockZAddNXConnection(mockClient),
		balanceSyncEnabled: true,
	}

	members := []redis.Z{
		{Score: float64(time.Now().Unix()), Member: "balance:key1"},
	}

	err := repo.ScheduleBalanceSyncBatch(context.Background(), members)

	require.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestScheduleBalanceSyncBatch_PartialAdd(t *testing.T) {
	// Test that ZAddNX returns count of NEW members (existing members not counted)
	mockClient := &mockZAddNXClient{
		zAddNXFunc: func(_ context.Context, _ string, members ...redis.Z) *redis.IntCmd {
			cmd := redis.NewIntCmd(context.Background())
			// Simulate 2 out of 3 members already existing
			cmd.SetVal(1) // Only 1 new member added

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockZAddNXConnection(mockClient),
		balanceSyncEnabled: true,
	}

	members := []redis.Z{
		{Score: float64(time.Now().Unix()), Member: "balance:key1"},
		{Score: float64(time.Now().Unix()), Member: "balance:key2"},
		{Score: float64(time.Now().Unix()), Member: "balance:key3"},
	}

	// Should not error even if not all members were added (NX behavior)
	err := repo.ScheduleBalanceSyncBatch(context.Background(), members)

	require.NoError(t, err)
}

func TestScheduleBalanceSyncBatch_DeduplicatesWithMinScore(t *testing.T) {
	var capturedMembers []redis.Z

	mockClient := &mockZAddNXClient{
		zAddNXFunc: func(_ context.Context, _ string, members ...redis.Z) *redis.IntCmd {
			capturedMembers = members

			cmd := redis.NewIntCmd(context.Background())
			cmd.SetVal(int64(len(members)))

			return cmd
		},
	}

	repo := &RedisConsumerRepository{
		conn:               newMockZAddNXConnection(mockClient),
		balanceSyncEnabled: true,
	}

	// Input has duplicate members with different scores
	// key1 appears twice: score 200 and score 100 (100 should win as minimum)
	// key2 appears twice: score 50 and score 150 (50 should win as minimum)
	members := []redis.Z{
		{Score: 200, Member: "balance:key1"},
		{Score: 100, Member: "balance:key1"}, // duplicate with lower score
		{Score: 50, Member: "balance:key2"},
		{Score: 150, Member: "balance:key2"}, // duplicate with higher score
		{Score: 300, Member: "balance:key3"}, // unique
	}

	err := repo.ScheduleBalanceSyncBatch(context.Background(), members)

	require.NoError(t, err)
	assert.Len(t, capturedMembers, 3, "Should have 3 unique members after deduplication")

	// Build map from captured members for easier assertion
	scoreByMember := make(map[string]float64)
	for _, m := range capturedMembers {
		scoreByMember[m.Member.(string)] = m.Score
	}

	assert.Equal(t, float64(100), scoreByMember["balance:key1"], "key1 should have minimum score 100")
	assert.Equal(t, float64(50), scoreByMember["balance:key2"], "key2 should have minimum score 50")
	assert.Equal(t, float64(300), scoreByMember["balance:key3"], "key3 should have score 300")
}
