//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	redistestutil "github.com/LerianStudio/midaz/v3/tests/utils/redis"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests verify the snapshot round-trip through a real Redis container.
// They complement the pure-Go round-trip tests in operation_redis_snapshot_test.go
// by exercising the actual JSON wire encoding the production code uses when
// caching Operations in Redis (e.g. via TransactionRedisQueue.Operations —
// see components/ledger/internal/services/command/create_balance_transaction_operations_async.go).
//
// The production path is:
//
//	op.ToRedis() → json.Marshal(OperationRedis) → redis.SET → json.Unmarshal → OperationFromRedis(...)
//
// Wire-shape contract: every Operation carries a populated Snapshot value
// (never nil) and Balance.OverdraftUsed / BalanceAfter.OverdraftUsed
// (decimal.Decimal, always set). Legacy cache envelopes (no `snapshot` key)
// decode to the zero-value struct, which OperationFromRedis normalises to
// the always-populated zero shape.

// ============================================================================
// Round-Trip: Active Overdraft (Both Fields Non-Zero)
// ============================================================================

// TestIntegration_OperationRedis_RoundTrip_NonZeroSnapshot verifies that an
// Operation with a non-zero snapshot survives the full Redis round-trip
// (ToRedis → JSON → redis.SET → redis.GET → JSON → OperationFromRedis) with
// typed decimal rehydration intact on both Balance.OverdraftUsed and
// BalanceAfter.OverdraftUsed.
func TestIntegration_OperationRedis_RoundTrip_NonZeroSnapshot(t *testing.T) {
	redisContainer := redistestutil.SetupContainer(t)

	ctx := context.Background()

	amt := decimal.NewFromInt(80)
	availBefore := decimal.NewFromInt(0)
	availAfter := decimal.NewFromInt(0)
	onHold := decimal.Zero
	versionBefore := int64(2)
	versionAfter := int64(3)

	op := &Operation{
		ID:            "op-non-zero",
		TransactionID: "tx-non-zero",
		Type:          "DEBIT",
		AssetCode:     "USD",
		Amount:        Amount{Value: &amt},
		Balance: Balance{
			Available: &availBefore,
			OnHold:    &onHold,
			Version:   &versionBefore,
		},
		BalanceAfter: Balance{
			Available: &availAfter,
			OnHold:    &onHold,
			Version:   &versionAfter,
		},
		Status: Status{Code: "APPROVED"},
		Snapshot: mmodel.OperationSnapshot{
			OverdraftUsedBefore: "50",
			OverdraftUsedAfter:  "130",
		},
	}

	// Serialize via the production path: op.ToRedis() → json.Marshal.
	redisRepr := op.ToRedis()
	payload, err := json.Marshal(redisRepr)
	require.NoError(t, err, "OperationRedis must marshal to JSON")

	// Store in real Redis.
	key := "test:operation:non-zero:" + op.ID
	err = redisContainer.Client.Set(ctx, key, payload, 0).Err()
	require.NoError(t, err, "Redis SET should succeed")

	// Read back and deserialize.
	stored, err := redisContainer.Client.Get(ctx, key).Bytes()
	require.NoError(t, err, "Redis GET should succeed")

	var restoredRedis mmodel.OperationRedis
	err = json.Unmarshal(stored, &restoredRedis)
	require.NoError(t, err, "stored payload must unmarshal back into OperationRedis")

	// Rehydrate via the production path: OperationFromRedis(...).
	restored := OperationFromRedis(restoredRedis)
	require.NotNil(t, restored)

	// Assert: snapshot survives the round-trip with both string fields intact.
	assert.Equal(t, "50", restored.Snapshot.OverdraftUsedBefore)
	assert.Equal(t, "130", restored.Snapshot.OverdraftUsedAfter)

	// Assert: typed decimal fields are rehydrated from the snapshot.
	assert.True(t, decimal.NewFromInt(50).Equal(restored.Balance.OverdraftUsed))
	assert.True(t, decimal.NewFromInt(130).Equal(restored.BalanceAfter.OverdraftUsed))
}

// ============================================================================
// Round-Trip: Zero Shape (Non-Overdraft Op)
// ============================================================================

// TestIntegration_OperationRedis_RoundTrip_ZeroShape verifies that an
// Operation with the zero-shape snapshot (the common non-overdraft case)
// survives the full Redis round-trip with the always-populated zero shape
// preserved. Both `snapshot` and the typed Balance.OverdraftUsed fields
// surface uniformly regardless of overdraft participation.
func TestIntegration_OperationRedis_RoundTrip_ZeroShape(t *testing.T) {
	redisContainer := redistestutil.SetupContainer(t)

	ctx := context.Background()

	amt := decimal.NewFromInt(100)
	avail := decimal.NewFromInt(500)
	onHold := decimal.Zero
	version := int64(1)

	op := &Operation{
		ID:            "op-zero",
		TransactionID: "tx-zero",
		Type:          "CREDIT",
		AssetCode:     "USD",
		Amount:        Amount{Value: &amt},
		Balance: Balance{
			Available:     &avail,
			OnHold:        &onHold,
			Version:       &version,
			OverdraftUsed: decimal.Zero,
		},
		BalanceAfter: Balance{
			Available:     &avail,
			OnHold:        &onHold,
			Version:       &version,
			OverdraftUsed: decimal.Zero,
		},
		Status: Status{Code: "APPROVED"},
		Snapshot: mmodel.OperationSnapshot{
			OverdraftUsedBefore: "0",
			OverdraftUsedAfter:  "0",
		},
	}

	redisRepr := op.ToRedis()
	assert.Equal(t, "0", redisRepr.Snapshot.OverdraftUsedBefore, "zero shape preserved on the Redis side")
	assert.Equal(t, "0", redisRepr.Snapshot.OverdraftUsedAfter)

	payload, err := json.Marshal(redisRepr)
	require.NoError(t, err)

	// Wire-level assertion: the JSON envelope MUST include the snapshot key
	// with both fields present (always-populated contract).
	var probe map[string]any
	require.NoError(t, json.Unmarshal(payload, &probe))
	require.Contains(t, probe, "snapshot", "snapshot key MUST be present on every cached envelope")

	snap, ok := probe["snapshot"].(map[string]any)
	require.True(t, ok, "snapshot must be an object")
	assert.Equal(t, "0", snap["overdraftUsedBefore"], "zero shape carries '0' verbatim on the wire")
	assert.Equal(t, "0", snap["overdraftUsedAfter"])

	key := "test:operation:zero:" + op.ID
	require.NoError(t, redisContainer.Client.Set(ctx, key, payload, 0).Err())

	stored, err := redisContainer.Client.Get(ctx, key).Bytes()
	require.NoError(t, err)

	var restoredRedis mmodel.OperationRedis
	require.NoError(t, json.Unmarshal(stored, &restoredRedis))

	restored := OperationFromRedis(restoredRedis)
	require.NotNil(t, restored)

	assert.Equal(t, "0", restored.Snapshot.OverdraftUsedBefore)
	assert.Equal(t, "0", restored.Snapshot.OverdraftUsedAfter)
	assert.True(t, restored.Balance.OverdraftUsed.Equal(decimal.Zero))
	assert.True(t, restored.BalanceAfter.OverdraftUsed.Equal(decimal.Zero))
}

// ============================================================================
// Backward Compatibility — Legacy Cache Envelope
// ============================================================================

// TestIntegration_OperationRedis_LegacyEnvelope verifies that an
// OperationRedis JSON payload written by older code (no snapshot key in the
// envelope) unmarshals cleanly via OperationFromRedis, with the snapshot
// normalised to the always-populated zero shape. This guards the Kiwi
// consumer replay path against in-flight cache entries from before the
// snapshot column landed.
//
// We write the payload directly as raw JSON (not via op.ToRedis) to simulate
// the bytes that actually sit in Redis from an older build.
func TestIntegration_OperationRedis_LegacyEnvelope(t *testing.T) {
	redisContainer := redistestutil.SetupContainer(t)

	ctx := context.Background()

	// Hand-crafted payload simulating a cache entry from before the snapshot
	// column was added. Mirrors the OperationRedis shape but WITHOUT the
	// snapshot key.
	legacyPayload := []byte(`{
		"id":"op-legacy",
		"transactionId":"tx-legacy",
		"description":"legacy cache envelope",
		"type":"DEBIT",
		"assetCode":"USD",
		"chartOfAccounts":"1000",
		"amountValue":"100",
		"balanceAvailable":"500",
		"balanceOnHold":"0",
		"balanceVersion":1,
		"balanceAfterAvailable":"400",
		"balanceAfterOnHold":"0",
		"balanceAfterVersion":2,
		"statusCode":"APPROVED",
		"balanceId":"bal-legacy",
		"accountId":"acc-legacy",
		"accountAlias":"@legacy",
		"balanceKey":"default",
		"organizationId":"org-legacy",
		"ledgerId":"ledger-legacy",
		"createdAt":"2026-04-20T00:00:00Z",
		"updatedAt":"2026-04-20T00:00:00Z",
		"route":"",
		"balanceAffected":true
	}`)

	key := "test:operation:legacy:op-legacy"
	require.NoError(t, redisContainer.Client.Set(ctx, key, legacyPayload, 0).Err())

	stored, err := redisContainer.Client.Get(ctx, key).Bytes()
	require.NoError(t, err)

	// Unmarshal must succeed — missing `snapshot` key must be tolerated.
	var restoredRedis mmodel.OperationRedis
	err = json.Unmarshal(stored, &restoredRedis)
	require.NoError(t, err, "legacy envelope without snapshot key must unmarshal without error")

	// On the Redis side the missing key decodes to the zero-value struct
	// (empty strings on both fields).
	assert.Equal(t, "", restoredRedis.Snapshot.OverdraftUsedBefore,
		"legacy envelope decodes to empty strings on the Redis representation")
	assert.Equal(t, "", restoredRedis.Snapshot.OverdraftUsedAfter)

	// Rehydrate via OperationFromRedis — this is the production-critical
	// entry point, hit by the cron consumer on replay. It normalises empty
	// strings to "0" so the rehydrated entity matches the always-populated
	// wire-shape contract.
	restored := OperationFromRedis(restoredRedis)
	require.NotNil(t, restored)

	assert.Equal(t, "0", restored.Snapshot.OverdraftUsedBefore,
		"legacy envelope normalises to '0' on the rehydrated entity")
	assert.Equal(t, "0", restored.Snapshot.OverdraftUsedAfter)
	assert.True(t, restored.Balance.OverdraftUsed.Equal(decimal.Zero))
	assert.True(t, restored.BalanceAfter.OverdraftUsed.Equal(decimal.Zero))

	// Sanity: the non-snapshot fields survived the round-trip normally.
	assert.Equal(t, "op-legacy", restored.ID)
	assert.Equal(t, "DEBIT", restored.Type)
	assert.Equal(t, "APPROVED", restored.Status.Code)
}
