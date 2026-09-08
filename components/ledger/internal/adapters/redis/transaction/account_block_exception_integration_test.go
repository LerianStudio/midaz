//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// These tests exercise CreateAccountBlockExceptions against the real engine
// (Valkey via testcontainers) to lock what only a live server can show: the key
// shape as stored, the native TTL actually applied (custom and default), that
// the whole batch lands in one transactional pass, and that expiry is Redis's
// job — an elapsed exception simply is not there any more, with no sweeper
// involved.

// blockExceptionTTLTolerance absorbs the round trip between the SET and the TTL
// read. Assertions are bounded on both sides so a TTL that was never applied
// (-1, no expiry) or a wildly wrong one still fails.
const blockExceptionTTLTolerance = 5 * time.Second

func TestIntegration_CreateAccountBlockExceptions_AppliesCustomAndDefaultTTL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID, ledgerID := uuid.New(), uuid.New()

	custom := AccountBlockException{
		ID: uuid.New(), Alias: "@fraud_account", Amount: "150", TTL: 900 * time.Second,
	}
	defaulted := AccountBlockException{
		ID: uuid.New(), Alias: "@other_account", Amount: "0.01", TTL: 300 * time.Second,
	}

	require.NoError(t, infra.repo.CreateAccountBlockExceptions(ctx, orgID, ledgerID,
		[]AccountBlockException{custom, defaulted}))

	for _, tc := range []struct {
		name      string
		exception AccountBlockException
	}{
		{name: "custom ttl", exception: custom},
		{name: "default ttl", exception: defaulted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := utils.AccountBlockExceptionInternalKey(orgID, ledgerID, tc.exception.ID)

			raw, err := infra.redisContainer.Client.Get(ctx, key).Result()
			require.NoError(t, err, "the exception must be stored under its internal key")

			var decoded map[string]string
			require.NoError(t, json.Unmarshal([]byte(raw), &decoded))
			assert.Equal(t, map[string]string{"Alias": tc.exception.Alias, "Amount": tc.exception.Amount}, decoded,
				"the stored blob must carry the CamelCase Alias/Amount pair")

			ttl, err := infra.redisContainer.Client.TTL(ctx, key).Result()
			require.NoError(t, err)

			assert.Positive(t, ttl, "a stored exception must carry a native expiry, never persist forever")
			assert.InDelta(t, tc.exception.TTL.Seconds(), ttl.Seconds(), blockExceptionTTLTolerance.Seconds(),
				"the applied TTL must be the one the command asked for")
		})
	}
}

func TestIntegration_CreateAccountBlockExceptions_ExpiresWithoutSweeper(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID, ledgerID := uuid.New(), uuid.New()
	exception := AccountBlockException{
		ID: uuid.New(), Alias: "@short_lived", Amount: "1", TTL: time.Second,
	}

	require.NoError(t, infra.repo.CreateAccountBlockExceptions(ctx, orgID, ledgerID,
		[]AccountBlockException{exception}))

	key := utils.AccountBlockExceptionInternalKey(orgID, ledgerID, exception.ID)

	exists, err := infra.redisContainer.Client.Exists(ctx, key).Result()
	require.NoError(t, err)
	require.EqualValues(t, 1, exists, "the exception must exist before its TTL elapses")

	require.Eventually(t, func() bool {
		exists, err := infra.redisContainer.Client.Exists(ctx, key).Result()

		return err == nil && exists == 0
	}, 10*time.Second, 200*time.Millisecond,
		"the exception must disappear on its own TTL — expiry is Redis's job, there is no sweeper")
}

// TestIntegration_CreateAccountBlockExceptions_WritesWholeBatchAtomically drives a
// multi-key batch through the real MULTI/EXEC path and asserts EVERY key is
// present afterwards. The batch shares one hash slot, so the transaction is
// legal in cluster mode; a partial result here would mean the write stopped
// being all-or-nothing.
func TestIntegration_CreateAccountBlockExceptions_WritesWholeBatchAtomically(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID, ledgerID := uuid.New(), uuid.New()

	const batchSize = 25

	exceptions := make([]AccountBlockException, 0, batchSize)
	keys := make([]string, 0, batchSize)

	for range batchSize {
		exception := AccountBlockException{
			ID: uuid.New(), Alias: "@batched", Amount: "1", TTL: 300 * time.Second,
		}
		exceptions = append(exceptions, exception)
		keys = append(keys, utils.AccountBlockExceptionInternalKey(orgID, ledgerID, exception.ID))
	}

	require.NoError(t, infra.repo.CreateAccountBlockExceptions(ctx, orgID, ledgerID, exceptions))

	values, err := infra.redisContainer.Client.MGet(ctx, keys...).Result()
	require.NoError(t, err)
	require.Len(t, values, batchSize)

	for i, value := range values {
		assert.NotNilf(t, value, "exception %d must be present: the batch applies all-or-nothing", i)
	}
}
