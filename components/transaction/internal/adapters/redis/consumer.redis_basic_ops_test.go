// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// newTestRepo builds a RedisConsumerRepository wired to a miniredis instance.
// sharding is off, shardRouter nil, balanceSync off — smallest working config
// for exercising non-sharded, non-Lua basic ops.
func newTestRepo(t *testing.T, mini *miniredis.Miniredis) *RedisConsumerRepository {
	t.Helper()

	repo, err := NewConsumerRedis(&libRedis.RedisConnection{
		Address: []string{mini.Addr()},
		Logger:  testLogger(t),
	}, false, nil)
	require.NoError(t, err)

	return repo
}

// startMini starts a miniredis and registers cleanup.
func startMini(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	mini, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mini.Close)

	return mini
}

func TestSet(t *testing.T) {
	t.Run("stores value with TTL", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		err := repo.Set(context.Background(), "key-ttl", "hello", 5*time.Second)
		require.NoError(t, err)

		got, err := mini.Get("key-ttl")
		require.NoError(t, err)
		assert.Equal(t, "hello", got)
		assert.Greater(t, mini.TTL("key-ttl"), time.Duration(0))
	})

	t.Run("stores value without TTL when zero", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		err := repo.Set(context.Background(), "key-no-ttl", "world", 0)
		require.NoError(t, err)

		got, err := mini.Get("key-no-ttl")
		require.NoError(t, err)
		assert.Equal(t, "world", got)
		assert.Equal(t, time.Duration(0), mini.TTL("key-no-ttl"))
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.Close() // force connection failures

		err := repo.Set(context.Background(), "k", "v", time.Second)
		require.Error(t, err)
	})
}

func TestSetNX(t *testing.T) {
	t.Run("acquires on missing key", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		acquired, err := repo.SetNX(context.Background(), "lock", "owner-1", time.Minute)
		require.NoError(t, err)
		assert.True(t, acquired)

		got, err := mini.Get("lock")
		require.NoError(t, err)
		assert.Equal(t, "owner-1", got)
	})

	t.Run("returns false when key already exists", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		_, err := repo.SetNX(context.Background(), "lock", "owner-1", time.Minute)
		require.NoError(t, err)

		acquired, err := repo.SetNX(context.Background(), "lock", "owner-2", time.Minute)
		require.NoError(t, err)
		assert.False(t, acquired)

		got, err := mini.Get("lock")
		require.NoError(t, err)
		assert.Equal(t, "owner-1", got, "value should remain from first acquirer")
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.Close()

		acquired, err := repo.SetNX(context.Background(), "lock", "v", time.Second)
		require.Error(t, err)
		assert.False(t, acquired)
	})
}

func TestGet(t *testing.T) {
	t.Run("returns value when present", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		require.NoError(t, mini.Set("hit", "cached"))

		got, err := repo.Get(context.Background(), "hit")
		require.NoError(t, err)
		assert.Equal(t, "cached", got)
	})

	t.Run("returns empty string for missing key without error", func(t *testing.T) {
		// Get() explicitly swallows redis.Nil via isRedisNilError.
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		got, err := repo.Get(context.Background(), "absent")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.Close()

		_, err := repo.Get(context.Background(), "whatever")
		require.Error(t, err)
	})
}

func TestMGet(t *testing.T) {
	t.Run("empty keys returns empty map", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		got, err := repo.MGet(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns only keys that exist", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		require.NoError(t, mini.Set("k1", "v1"))
		require.NoError(t, mini.Set("k3", "v3"))
		// k2 intentionally absent

		got, err := repo.MGet(context.Background(), []string{"k1", "k2", "k3"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"k1": "v1", "k3": "v3"}, got)
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.Close()

		_, err := repo.MGet(context.Background(), []string{"k1"})
		require.Error(t, err)
	})
}

func TestDel(t *testing.T) {
	t.Run("removes existing key", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		require.NoError(t, mini.Set("tmp", "value"))

		err := repo.Del(context.Background(), "tmp")
		require.NoError(t, err)
		assert.False(t, mini.Exists("tmp"))
	})

	t.Run("missing key is not an error", func(t *testing.T) {
		// DEL on non-existent key returns 0 deleted, no error.
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		err := repo.Del(context.Background(), "never-existed")
		require.NoError(t, err)
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.Close()

		err := repo.Del(context.Background(), "k")
		require.Error(t, err)
	})
}

func TestIncr(t *testing.T) {
	t.Run("increments from zero", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		first := repo.Incr(context.Background(), "counter")
		second := repo.Incr(context.Background(), "counter")

		assert.Equal(t, int64(1), first)
		assert.Equal(t, int64(2), second)

		stored, err := mini.Get("counter")
		require.NoError(t, err)
		assert.Equal(t, "2", stored)
	})

	t.Run("returns 0 when server unreachable (by design)", func(t *testing.T) {
		// Incr does not return error; on connection failure it logs and returns 0.
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.Close()

		got := repo.Incr(context.Background(), "counter")
		assert.Equal(t, int64(0), got)
	})
}

func TestSetBytes(t *testing.T) {
	t.Run("persists binary payload", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		payload := []byte{0x00, 0xff, 0x01, 0x02, 0x03}
		err := repo.SetBytes(context.Background(), "bin", payload, 10*time.Second)
		require.NoError(t, err)

		got, err := mini.Get("bin")
		require.NoError(t, err)
		assert.Equal(t, string(payload), got)
		assert.Greater(t, mini.TTL("bin"), time.Duration(0))
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.Close()

		err := repo.SetBytes(context.Background(), "bin", []byte{1}, time.Second)
		require.Error(t, err)
	})
}

func TestGetBytes(t *testing.T) {
	t.Run("retrieves binary payload", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		payload := []byte{0x10, 0x20, 0x30}
		require.NoError(t, mini.Set("bin", string(payload)))

		got, err := repo.GetBytes(context.Background(), "bin")
		require.NoError(t, err)
		assert.Equal(t, payload, got)
	})

	t.Run("returns error for missing key", func(t *testing.T) {
		// GetBytes does NOT swallow redis.Nil — missing key is an error.
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		_, err := repo.GetBytes(context.Background(), "absent")
		require.Error(t, err)
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.Close()

		_, err := repo.GetBytes(context.Background(), "k")
		require.Error(t, err)
	})
}

func TestAddMessageToQueue(t *testing.T) {
	t.Run("adds message to legacy backup queue", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		txKey := "tx-key-1"
		msg := []byte(`{"id":"abc"}`)

		err := repo.AddMessageToQueue(context.Background(), txKey, msg)
		require.NoError(t, err)

		got := mini.HGet(TransactionBackupQueue, txKey)
		assert.Equal(t, string(msg), got)
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.Close()

		err := repo.AddMessageToQueue(context.Background(), "k", []byte("m"))
		require.Error(t, err)
	})
}

func TestReadMessageFromQueue(t *testing.T) {
	t.Run("reads previously added message", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		txKey := "tx-key-2"
		msg := []byte("payload-bytes")
		mini.HSet(TransactionBackupQueue, txKey, string(msg))

		got, err := repo.ReadMessageFromQueue(context.Background(), txKey)
		require.NoError(t, err)
		assert.Equal(t, msg, got)
	})

	t.Run("returns error for missing field", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		_, err := repo.ReadMessageFromQueue(context.Background(), "nonexistent")
		require.Error(t, err)
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.Close()

		_, err := repo.ReadMessageFromQueue(context.Background(), "k")
		require.Error(t, err)
	})
}

func TestReadAllMessagesFromQueue(t *testing.T) {
	t.Run("returns empty map when queue is empty", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		got, err := repo.ReadAllMessagesFromQueue(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns all messages from legacy queue", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.HSet(TransactionBackupQueue, "k1", "v1", "k2", "v2")

		got, err := repo.ReadAllMessagesFromQueue(context.Background())
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"k1": "v1", "k2": "v2"}, got)
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.Close()

		_, err := repo.ReadAllMessagesFromQueue(context.Background())
		require.Error(t, err)
	})
}

func TestRemoveMessageFromQueue(t *testing.T) {
	t.Run("removes existing message", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		txKey := "tx-key-3"
		mini.HSet(TransactionBackupQueue, txKey, "some-value")

		err := repo.RemoveMessageFromQueue(context.Background(), txKey)
		require.NoError(t, err)

		assert.Empty(t, mini.HGet(TransactionBackupQueue, txKey), "field should be gone")
	})

	t.Run("missing message is not an error", func(t *testing.T) {
		// HDEL on non-existent field returns 0 deleted, no error.
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		err := repo.RemoveMessageFromQueue(context.Background(), "never-existed")
		require.NoError(t, err)
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.Close()

		err := repo.RemoveMessageFromQueue(context.Background(), "k")
		require.Error(t, err)
	})
}

func TestListBalanceByKey_Legacy(t *testing.T) {
	// Non-sharded path: uses BalanceInternalKey.
	t.Run("returns balance when present", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		organizationID := uuid.New()
		ledgerID := uuid.New()
		key := "@alice#default"
		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, key)

		payload := mmodel.BalanceRedis{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			Alias:          "@alice",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(100),
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   1,
			AllowReceiving: 1,
			Key:            "default",
		}
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		require.NoError(t, mini.Set(internalKey, string(raw)))

		got, err := repo.ListBalanceByKey(context.Background(), organizationID, ledgerID, key)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "@alice", got.Alias)
		assert.Equal(t, "USD", got.AssetCode)
		assert.True(t, got.Available.Equal(decimal.NewFromInt(100)))
		assert.True(t, got.AllowSending)
		assert.True(t, got.AllowReceiving)
		assert.Equal(t, organizationID.String(), got.OrganizationID)
		assert.Equal(t, ledgerID.String(), got.LedgerID)
	})

	t.Run("returns error when key missing", func(t *testing.T) {
		// ListBalanceByKey does NOT swallow redis.Nil — missing key is an error.
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		_, err := repo.ListBalanceByKey(context.Background(), uuid.New(), uuid.New(), "@absent#default")
		require.Error(t, err)
	})

	t.Run("returns error when payload is invalid JSON", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		organizationID := uuid.New()
		ledgerID := uuid.New()
		key := "@bad#default"
		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, key)

		require.NoError(t, mini.Set(internalKey, "{not valid json"))

		_, err := repo.ListBalanceByKey(context.Background(), organizationID, ledgerID, key)
		require.Error(t, err)
	})

	t.Run("returns error when server unreachable", func(t *testing.T) {
		mini := startMini(t)
		repo := newTestRepo(t, mini)

		mini.Close()

		_, err := repo.ListBalanceByKey(context.Background(), uuid.New(), uuid.New(), "@x#default")
		require.Error(t, err)
	})
}

func TestExtractShardID_BasicOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		key       string
		wantShard int
		wantOK    bool
	}{
		{name: "shard-aware key", key: "balances:{shard_5}:org:ledger:alias", wantShard: 5, wantOK: true},
		{name: "shard zero", key: "{shard_0}:something", wantShard: 0, wantOK: true},
		{name: "large shard number", key: "prefix:{shard_123}:suffix", wantShard: 123, wantOK: true},
		{name: "no shard prefix", key: "balances:{transactions}:org", wantShard: 0, wantOK: false},
		{name: "missing closing brace", key: "balances:{shard_5", wantShard: 0, wantOK: false},
		{name: "non-numeric shard id", key: "balances:{shard_abc}:x", wantShard: 0, wantOK: false},
		{name: "empty key", key: "", wantShard: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotShard, gotOK := extractShardID(tt.key)
			assert.Equal(t, tt.wantShard, gotShard)
			assert.Equal(t, tt.wantOK, gotOK)
		})
	}
}

func TestSetPipeline_EmptyKeysNoop(t *testing.T) {
	// Empty keys slice should early-return without touching Redis.
	mini := startMini(t)
	repo := newTestRepo(t, mini)

	err := repo.SetPipeline(context.Background(), nil, nil, nil)
	require.NoError(t, err)
}

func TestSetPipeline_ServerUnreachable(t *testing.T) {
	mini := startMini(t)
	repo := newTestRepo(t, mini)

	mini.Close()

	err := repo.SetPipeline(
		context.Background(),
		[]string{"k"},
		[]string{"v"},
		[]time.Duration{time.Second},
	)
	require.Error(t, err)
}

func TestPreWarmExternalBalances_EmptyInput(t *testing.T) {
	mini := startMini(t)
	repo := newTestRepo(t, mini)

	got, err := repo.PreWarmExternalBalances(context.Background(), uuid.New(), uuid.New(), nil, time.Hour)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestPreWarmExternalBalances_SkipsNonExternalAndNil(t *testing.T) {
	// Non-external aliases and nil entries are skipped.
	mini := startMini(t)
	repo := newTestRepo(t, mini)

	organizationID := uuid.New()
	ledgerID := uuid.New()

	balances := []*mmodel.Balance{
		nil,
		{
			Alias:          "@alice", // not external
			Key:            "default",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1),
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		},
	}

	coverage, err := repo.PreWarmExternalBalances(context.Background(), organizationID, ledgerID, balances, time.Minute)
	require.NoError(t, err)
	assert.Empty(t, coverage)
	// Nothing should have been written
	assert.Empty(t, mini.Keys())
}

func TestCheckOrAcquireIdempotencyKey_ReturnsExistingValue(t *testing.T) {
	// When a prior owner SET the key with a response payload,
	// a later caller must receive that payload and acquired=false.
	mini := startMini(t)
	repo := newTestRepo(t, mini)

	ctx := context.Background()
	key := "idempotency:with-payload"

	// Simulate a previously stored cached response. The Lua script GETs the key,
	// so any existing string value should come back to the caller as existingValue.
	require.NoError(t, mini.Set(key, "cached-response-body"))

	existingValue, acquired, err := repo.CheckOrAcquireIdempotencyKey(ctx, key, 5*time.Second)
	require.NoError(t, err)
	assert.False(t, acquired)
	assert.Equal(t, "cached-response-body", existingValue)
}

func TestGetBalanceSyncKeys_ZeroLimit(t *testing.T) {
	// limit=0 should short-circuit before hitting Redis.
	mini := startMini(t)
	repo := newTestRepo(t, mini)

	got, err := repo.GetBalanceSyncKeys(context.Background(), 0)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestGetBalanceSyncKeys_EmptyScheduleReturnsEmpty(t *testing.T) {
	// No scheduled keys → Lua returns empty array → function returns [].
	mini := startMini(t)
	repo := newTestRepo(t, mini)

	got, err := repo.GetBalanceSyncKeys(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestGetBalanceSyncKeys_ServerUnreachable(t *testing.T) {
	mini := startMini(t)
	repo := newTestRepo(t, mini)

	mini.Close()

	_, err := repo.GetBalanceSyncKeys(context.Background(), 10)
	require.Error(t, err)
}

func TestRemoveBalanceSyncKey_Legacy(t *testing.T) {
	// Non-sharded path: resolveScheduleKeys returns the legacy schedule key.
	// Even if the ZSET is empty, the Lua script runs successfully (ZREM of non-existent
	// member is a no-op).
	mini := startMini(t)
	repo := newTestRepo(t, mini)

	err := repo.RemoveBalanceSyncKey(context.Background(), "some-balance-key")
	require.NoError(t, err)
}

func TestRemoveBalanceSyncKey_ServerUnreachable(t *testing.T) {
	mini := startMini(t)
	repo := newTestRepo(t, mini)

	mini.Close()

	err := repo.RemoveBalanceSyncKey(context.Background(), "some-key")
	require.Error(t, err)
}

func TestCheckOrAcquireIdempotencyKey_NormalizesZeroTTL(t *testing.T) {
	// Zero TTL should be normalized to 1 second by normalizeRedisTTLSeconds.
	mini := startMini(t)
	repo := newTestRepo(t, mini)

	ctx := context.Background()
	key := "idempotency:zero-ttl"

	_, acquired, err := repo.CheckOrAcquireIdempotencyKey(ctx, key, 0)
	require.NoError(t, err)
	assert.True(t, acquired)

	// Script should have set the key with a positive TTL.
	assert.Greater(t, mini.TTL(key), time.Duration(0))
}
