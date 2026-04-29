// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRedisRoundTrip_ZeroShape verifies that an operation with the zero-
// shape snapshot (the common non-overdraft case) round-trips through ToRedis
// → OperationFromRedis with both Snapshot fields and the typed
// Balance.OverdraftUsed fields preserved as "0" / decimal.Zero. Under the
// always-populated wire-shape contract, snapshot is a value type — never nil —
// and Balance.OverdraftUsed is always set.
func TestRedisRoundTrip_ZeroShape(t *testing.T) {
	t.Parallel()

	amt := decimal.NewFromInt(100)
	avail := decimal.NewFromInt(500)
	onHold := decimal.Zero
	version := int64(1)

	op := &Operation{
		ID:            "op-zero",
		TransactionID: "tx-1",
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
		Snapshot: mmodel.OperationSnapshot{
			OverdraftUsedBefore: "0",
			OverdraftUsedAfter:  "0",
		},
	}

	r := op.ToRedis()
	assert.Equal(t, "0", r.Snapshot.OverdraftUsedBefore, "zero shape preserves '0' on the Redis side")
	assert.Equal(t, "0", r.Snapshot.OverdraftUsedAfter)

	restored := OperationFromRedis(r)
	assert.Equal(t, "0", restored.Snapshot.OverdraftUsedBefore, "zero shape round-trips")
	assert.Equal(t, "0", restored.Snapshot.OverdraftUsedAfter)
	assert.True(t, restored.Balance.OverdraftUsed.Equal(decimal.Zero), "rehydrated typed field is zero")
	assert.True(t, restored.BalanceAfter.OverdraftUsed.Equal(decimal.Zero))
}

// TestRedisRoundTrip_BothFieldsPopulated verifies that a snapshot with both
// OverdraftUsedBefore and OverdraftUsedAfter round-trips correctly, including
// rehydration of the typed Balance.OverdraftUsed fields.
func TestRedisRoundTrip_BothFieldsPopulated(t *testing.T) {
	t.Parallel()

	amt := decimal.NewFromInt(80)
	avail := decimal.NewFromInt(0)
	onHold := decimal.Zero
	version := int64(3)
	afterAvail := decimal.NewFromInt(0)

	op := &Operation{
		ID:            "op-both",
		TransactionID: "tx-2",
		Amount:        Amount{Value: &amt},
		Balance: Balance{
			Available: &avail,
			OnHold:    &onHold,
			Version:   &version,
		},
		BalanceAfter: Balance{
			Available: &afterAvail,
			OnHold:    &onHold,
			Version:   &version,
		},
		Snapshot: mmodel.OperationSnapshot{
			OverdraftUsedBefore: "50",
			OverdraftUsedAfter:  "130",
		},
	}

	r := op.ToRedis()
	assert.Equal(t, "50", r.Snapshot.OverdraftUsedBefore)
	assert.Equal(t, "130", r.Snapshot.OverdraftUsedAfter)

	restored := OperationFromRedis(r)
	assert.Equal(t, "50", restored.Snapshot.OverdraftUsedBefore)
	assert.Equal(t, "130", restored.Snapshot.OverdraftUsedAfter)

	// Typed fields rehydrated from snapshot.
	assert.True(t, decimal.NewFromInt(50).Equal(restored.Balance.OverdraftUsed),
		"Balance.OverdraftUsed must be rehydrated from snapshot.Before; got %s",
		restored.Balance.OverdraftUsed.String())
	assert.True(t, decimal.NewFromInt(130).Equal(restored.BalanceAfter.OverdraftUsed),
		"BalanceAfter.OverdraftUsed must be rehydrated from snapshot.After; got %s",
		restored.BalanceAfter.OverdraftUsed.String())
}

// TestRedisRoundTrip_LegacyEmptyEnvelope verifies that an OperationRedis loaded
// from a legacy cache envelope (no `snapshot` key in the JSON, deserialized
// to the zero-value struct with both fields empty strings) is normalised to
// the always-populated zero shape on rehydration. This is the
// backwards-compatibility guarantee for in-flight cache entries written by
// older code, replayed under the new contract.
func TestRedisRoundTrip_LegacyEmptyEnvelope(t *testing.T) {
	t.Parallel()

	r := mmodel.OperationRedis{
		ID:            "op-legacy",
		TransactionID: "tx-legacy",
		Type:          "DEBIT",
		// Snapshot left at zero value — empty strings for both fields,
		// simulating an envelope from before snapshot was added.
	}

	require.Equal(t, "", r.Snapshot.OverdraftUsedBefore)
	require.Equal(t, "", r.Snapshot.OverdraftUsedAfter)

	restored := OperationFromRedis(r)
	require.NotNil(t, restored)

	assert.Equal(t, "0", restored.Snapshot.OverdraftUsedBefore,
		"legacy empty-string envelope normalises to '0' on rehydration")
	assert.Equal(t, "0", restored.Snapshot.OverdraftUsedAfter)
	assert.True(t, restored.Balance.OverdraftUsed.Equal(decimal.Zero))
	assert.True(t, restored.BalanceAfter.OverdraftUsed.Equal(decimal.Zero))
}
