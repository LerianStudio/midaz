// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOperationPostgreSQLModel_ToEntity_BalanceOverdraftUsed verifies that the
// typed decimal.Decimal convenience fields on operation.Balance and
// operation.BalanceAfter are populated from the snapshot's string-encoded
// OverdraftUsedBefore / OverdraftUsedAfter. Under the always-populated wire-
// shape contract, these fields are ALWAYS set (decimal.Zero for non-overdraft
// ops, parsed value otherwise). ToEntity is the canonical entry point for
// PG-to-domain mapping, so this is where the surface needs to be correct for
// both the HTTP response DTO and the audit log.
//
// Resilience posture: malformed historical rows (non-parseable strings) MUST
// surface OverdraftUsed as decimal.Zero without surfacing an error or
// panicking. The snapshot is read-only audit context, not a correctness-
// critical field.
func TestOperationPostgreSQLModel_ToEntity_BalanceOverdraftUsed(t *testing.T) {
	t.Parallel()

	expectedBefore := decimal.RequireFromString("50.00")
	expectedAfter := decimal.RequireFromString("130.00")

	tests := []struct {
		name            string
		raw             json.RawMessage
		wantBeforeValue decimal.Decimal
		wantAfterValue  decimal.Decimal
	}{
		{
			name:            "nil snapshot → both OverdraftUsed default to zero",
			raw:             nil,
			wantBeforeValue: decimal.Zero,
			wantAfterValue:  decimal.Zero,
		},
		{
			name:            "empty-object snapshot → both OverdraftUsed default to zero",
			raw:             json.RawMessage(`{}`),
			wantBeforeValue: decimal.Zero,
			wantAfterValue:  decimal.Zero,
		},
		{
			name:            "explicit zero shape → both decimal.Zero",
			raw:             json.RawMessage(`{"overdraftUsedBefore":"0","overdraftUsedAfter":"0"}`),
			wantBeforeValue: decimal.Zero,
			wantAfterValue:  decimal.Zero,
		},
		{
			name:            "both fields populated",
			raw:             json.RawMessage(`{"overdraftUsedBefore":"50.00","overdraftUsedAfter":"130.00"}`),
			wantBeforeValue: expectedBefore,
			wantAfterValue:  expectedAfter,
		},
		{
			name:            "only OverdraftUsedAfter set → Before defaults to zero",
			raw:             json.RawMessage(`{"overdraftUsedAfter":"130.00"}`),
			wantBeforeValue: decimal.Zero,
			wantAfterValue:  expectedAfter,
		},
		{
			name: "malformed OverdraftUsedBefore → Before defaults to zero, After populated (per-field isolation)",
			raw: func() json.RawMessage {
				b, _ := json.Marshal(map[string]string{
					"overdraftUsedBefore": "not-a-number",
					"overdraftUsedAfter":  "130.00",
				})
				return b
			}(),
			wantBeforeValue: decimal.Zero,
			wantAfterValue:  expectedAfter,
		},
		{
			name: "malformed OverdraftUsedAfter → Before populated, After defaults to zero",
			raw: func() json.RawMessage {
				b, _ := json.Marshal(map[string]string{
					"overdraftUsedBefore": "50.00",
					"overdraftUsedAfter":  "also-garbage",
				})
				return b
			}(),
			wantBeforeValue: expectedBefore,
			wantAfterValue:  decimal.Zero,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := &OperationPostgreSQLModel{
				ID:              "op-1",
				TransactionID:   "tx-1",
				BalanceAffected: true,
				Snapshot:        tt.raw,
			}

			entity := model.ToEntity()
			require.NotNil(t, entity)

			assert.True(t,
				entity.Balance.OverdraftUsed.Equal(tt.wantBeforeValue),
				"Balance.OverdraftUsed: want %s, got %s",
				tt.wantBeforeValue.String(), entity.Balance.OverdraftUsed.String(),
			)
			assert.True(t,
				entity.BalanceAfter.OverdraftUsed.Equal(tt.wantAfterValue),
				"BalanceAfter.OverdraftUsed: want %s, got %s",
				tt.wantAfterValue.String(), entity.BalanceAfter.OverdraftUsed.String(),
			)
		})
	}
}

// TestOperationPointInTimeModel_ToEntity_BalanceOverdraftUsed verifies the same
// mapping for the point-in-time reconstruction path. PIT queries only build
// BalanceAfter (they reconstruct historical state from a single snapshot), so
// only OverdraftUsedAfter → BalanceAfter.OverdraftUsed is asserted.
func TestOperationPointInTimeModel_ToEntity_BalanceOverdraftUsed(t *testing.T) {
	t.Parallel()

	expectedAfter := decimal.RequireFromString("130.00")

	tests := []struct {
		name           string
		raw            json.RawMessage
		wantAfterValue decimal.Decimal
	}{
		{
			name:           "nil snapshot → BalanceAfter.OverdraftUsed defaults to zero",
			raw:            nil,
			wantAfterValue: decimal.Zero,
		},
		{
			name:           "empty-object snapshot → BalanceAfter.OverdraftUsed defaults to zero",
			raw:            json.RawMessage(`{}`),
			wantAfterValue: decimal.Zero,
		},
		{
			name:           "explicit zero shape → BalanceAfter.OverdraftUsed is zero",
			raw:            json.RawMessage(`{"overdraftUsedBefore":"0","overdraftUsedAfter":"0"}`),
			wantAfterValue: decimal.Zero,
		},
		{
			name:           "OverdraftUsedAfter populated",
			raw:            json.RawMessage(`{"overdraftUsedAfter":"130.00"}`),
			wantAfterValue: expectedAfter,
		},
		{
			name:           "malformed OverdraftUsedAfter → defaults to zero, no panic",
			raw:            json.RawMessage(`{"overdraftUsedAfter":"garbage"}`),
			wantAfterValue: decimal.Zero,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pit := &OperationPointInTimeModel{
				ID:        "op-1",
				BalanceID: "bal-1",
				AccountID: "acc-1",
				AssetCode: "BRL",
				Snapshot:  tt.raw,
			}

			entity := pit.ToEntity()
			require.NotNil(t, entity)

			assert.True(t,
				entity.BalanceAfter.OverdraftUsed.Equal(tt.wantAfterValue),
				"BalanceAfter.OverdraftUsed: want %s, got %s",
				tt.wantAfterValue.String(), entity.BalanceAfter.OverdraftUsed.String(),
			)
		})
	}
}

// TestOperation_ToLog_SnapshotAndOverdraftUsed verifies that:
//   - OperationLog carries the Snapshot value verbatim (value copy — Snapshot is a value type).
//   - OperationLog.Balance shares the Balance type so OverdraftUsed propagates via the nested struct.
//   - JSON shape of OperationLog includes `snapshot` and `overdraftUsed` on nested Balance/BalanceAfter
//     (always present under the always-populated wire-shape contract).
func TestOperation_ToLog_SnapshotAndOverdraftUsed(t *testing.T) {
	t.Parallel()

	snap := mmodel.OperationSnapshot{
		OverdraftUsedBefore: "50.00",
		OverdraftUsedAfter:  "130.00",
	}

	usedBefore := decimal.RequireFromString("50.00")
	usedAfter := decimal.RequireFromString("130.00")

	op := &Operation{
		ID:            "op-1",
		TransactionID: "tx-1",
		Balance: Balance{
			OverdraftUsed: usedBefore,
		},
		BalanceAfter: Balance{
			OverdraftUsed: usedAfter,
		},
		Snapshot: snap,
	}

	logEntry := op.ToLog()
	require.NotNil(t, logEntry)

	// Snapshot value-copied onto the log entry. Independent value-typed
	// copies, asserting field equality covers both identity-preserved-by-
	// value-copy and the no-mutation invariant.
	assert.Equal(t, snap.OverdraftUsedBefore, logEntry.Snapshot.OverdraftUsedBefore)
	assert.Equal(t, snap.OverdraftUsedAfter, logEntry.Snapshot.OverdraftUsedAfter)

	// Balance.OverdraftUsed propagated via the shared Balance type.
	assert.True(t, logEntry.Balance.OverdraftUsed.Equal(usedBefore))
	assert.True(t, logEntry.BalanceAfter.OverdraftUsed.Equal(usedAfter))

	// JSON surface check: the log entry marshals with camelCase keys.
	// Snapshot is internal-only (json:"-") — it must NOT appear on the
	// wire. The typed balance.overdraftUsed / balanceAfter.overdraftUsed
	// fields carry the same information.
	raw, err := json.Marshal(logEntry)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(raw, &generic))

	assert.NotContains(t, generic, "snapshot",
		"snapshot must NOT appear on JSON wire — values surface via balance.overdraftUsed/balanceAfter.overdraftUsed")

	bal, ok := generic["balance"].(map[string]any)
	require.True(t, ok, "balance field must be an object")
	assert.Equal(t, "50", bal["overdraftUsed"], "Balance.overdraftUsed serializes as decimal string")

	balAfter, ok := generic["balanceAfter"].(map[string]any)
	require.True(t, ok, "balanceAfter field must be an object")
	assert.Equal(t, "130", balAfter["overdraftUsed"], "BalanceAfter.overdraftUsed serializes as decimal string")
}

// TestOperation_ToLog_NonOverdraft_SnapshotInternalOnly verifies the wire-
// internal contract for non-overdraft operations: the snapshot block MUST NOT
// appear on the JSON wire even when zero. The typed balance.overdraftUsed and
// balanceAfter.overdraftUsed fields carry the "0" value instead.
func TestOperation_ToLog_NonOverdraft_SnapshotInternalOnly(t *testing.T) {
	t.Parallel()

	op := &Operation{
		ID:            "op-1",
		TransactionID: "tx-1",
		Balance: Balance{
			OverdraftUsed: decimal.Zero,
		},
		BalanceAfter: Balance{
			OverdraftUsed: decimal.Zero,
		},
		Snapshot: mmodel.OperationSnapshot{
			OverdraftUsedBefore: "0",
			OverdraftUsedAfter:  "0",
		},
	}

	logEntry := op.ToLog()
	require.NotNil(t, logEntry)

	// In-memory: snapshot value carries zero strings.
	assert.Equal(t, "0", logEntry.Snapshot.OverdraftUsedBefore)
	assert.Equal(t, "0", logEntry.Snapshot.OverdraftUsedAfter)

	raw, err := json.Marshal(logEntry)
	require.NoError(t, err)

	var generic map[string]any
	require.NoError(t, json.Unmarshal(raw, &generic))

	// snapshot key MUST NOT appear on the wire.
	assert.NotContains(t, generic, "snapshot",
		"snapshot must NOT appear on JSON wire — values surface via balance.overdraftUsed/balanceAfter.overdraftUsed")

	// Typed balance.overdraftUsed MUST still be present with "0".
	bal, ok := generic["balance"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "0", bal["overdraftUsed"])

	balAfter, ok := generic["balanceAfter"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "0", balAfter["overdraftUsed"])
}
