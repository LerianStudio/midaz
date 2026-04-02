// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBalanceAtomicResponse_UnmarshalJSON_SingleObjectAfter(t *testing.T) {
	t.Parallel()

	payload := []byte(`{
		"before": [{
			"id": "before-id",
			"alias": "@before",
			"key": "default",
			"accountId": "acc-before",
			"assetCode": "USD",
			"available": "100.00",
			"onHold": "0",
			"version": 1,
			"accountType": "deposit",
			"allowSending": 1,
			"allowReceiving": 1
		}],
		"after": {
			"id": "after-id",
			"alias": "@after",
			"key": "default",
			"accountId": "acc-after",
			"assetCode": "USD",
			"available": "90.00",
			"onHold": "10.00",
			"version": 2,
			"accountType": "deposit",
			"allowSending": 1,
			"allowReceiving": 1
		}
	}`)

	var response balanceAtomicResponse
	err := json.Unmarshal(payload, &response)

	require.NoError(t, err)
	require.Len(t, response.Before, 1)
	require.Len(t, response.After, 1)
	assert.Equal(t, "@before", response.Before[0].Alias)
	assert.Equal(t, "@after", response.After[0].Alias)
	assert.EqualValues(t, 2, response.After[0].Version)
	assert.True(t, response.After[0].Available.Equal(redisDecimalFromString(t, "90.00")))
	assert.True(t, response.After[0].OnHold.Equal(redisDecimalFromString(t, "10.00")))
}

func redisDecimalFromString(t *testing.T, value string) decimal.Decimal {
	t.Helper()

	parsed, err := decimal.NewFromString(value)
	require.NoError(t, err)

	return parsed
}

func TestKeyNamespacing_MalformedTenantID_FailsClosedBalanceSyncScripts(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant:invalid")

	t.Run("get balance sync keys", func(t *testing.T) {
		t.Parallel()

		conn, scripter := newScriptCapturingConnection(t)
		repo := &RedisConsumerRepository{conn: conn}

		_, err := repo.GetBalanceSyncKeys(ctx, 10)
		require.Error(t, err)
		assert.Empty(t, scripter.evalShaCalls, "GetBalanceSyncKeys must fail closed before EVALSHA")
		assert.Empty(t, scripter.evalCalls, "GetBalanceSyncKeys must fail closed before EVAL")
	})

	t.Run("remove balance sync key", func(t *testing.T) {
		t.Parallel()

		conn, scripter := newScriptCapturingConnection(t)
		repo := &RedisConsumerRepository{conn: conn}

		err := repo.RemoveBalanceSyncKey(ctx, "balance:key")
		require.Error(t, err)
		assert.Empty(t, scripter.evalShaCalls, "RemoveBalanceSyncKey must fail closed before EVALSHA")
		assert.Empty(t, scripter.evalCalls, "RemoveBalanceSyncKey must fail closed before EVAL")
	})
}

func TestKeyNamespacing_MalformedTenantID_FailsClosedGetBalancesByKeys(t *testing.T) {
	t.Parallel()

	mockClient := &mockMGetClient{
		mGetFunc: func(_ context.Context, _ ...string) *redis.SliceCmd {
			t.Fatal("GetBalancesByKeys must fail closed before calling MGet")

			return nil
		},
	}

	repo := &RedisConsumerRepository{
		conn: newMockMGetConnection(mockClient),
	}

	_, err := repo.GetBalancesByKeys(tmcore.ContextWithTenantID(context.Background(), "tenant:invalid"), []string{"key1", "key2"})
	require.Error(t, err)
}

func TestKeyNamespacing_MalformedTenantID_FailsClosedBatchScheduleAndRemove(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant:invalid")

	t.Run("schedule balance sync batch", func(t *testing.T) {
		t.Parallel()

		mockClient := &mockZAddNXClient{
			zAddNXFunc: func(_ context.Context, _ string, _ ...redis.Z) *redis.IntCmd {
				t.Fatal("ScheduleBalanceSyncBatch must fail closed before calling ZAddNX")

				return nil
			},
		}

		repo := &RedisConsumerRepository{
			conn: newMockZAddNXConnection(mockClient),
		}

		err := repo.ScheduleBalanceSyncBatch(ctx, []redis.Z{{Score: float64(time.Now().Unix()), Member: "balance:key"}})
		require.Error(t, err)
	})

	t.Run("remove balance sync keys batch", func(t *testing.T) {
		t.Parallel()

		mockClient := &mockEvalClient{
			evalFunc: func(_ context.Context, _ string, _ []string, _ ...any) *redis.Cmd {
				t.Fatal("RemoveBalanceSyncKeysBatch must fail closed before calling Eval")

				return nil
			},
		}

		repo := &RedisConsumerRepository{
			conn: newMockEvalConnection(mockClient),
		}

		_, err := repo.RemoveBalanceSyncKeysBatch(ctx, []SyncKey{{Key: "balance:key", Score: 0}})
		require.Error(t, err)
	})
}
