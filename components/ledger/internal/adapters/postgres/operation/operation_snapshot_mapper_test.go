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

// TestOperationPostgreSQLModel_FromEntity_Snapshot verifies that FromEntity
// always marshals the domain Operation.Snapshot into the PG model's
// json.RawMessage. Under the always-populated wire-shape contract the entity
// snapshot is a value type (never nil) — non-overdraft ops carry the zero
// shape `{"overdraftUsedBefore":"0","overdraftUsedAfter":"0"}` rather than `{}`.
func TestOperationPostgreSQLModel_FromEntity_Snapshot(t *testing.T) {
	tests := []struct {
		name     string
		snapshot mmodel.OperationSnapshot
		// expectedJSON is compared structurally (unmarshal both sides and
		// assert equality) to avoid key-ordering brittleness.
		expectedJSON string
	}{
		{
			name: "non-overdraft op emits zero shape",
			snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "0",
			},
			expectedJSON: `{"overdraftUsedBefore":"0","overdraftUsedAfter":"0"}`,
		},
		{
			name: "active overdraft emits non-zero values",
			snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "50.00",
				OverdraftUsedAfter:  "130.00",
			},
			expectedJSON: `{"overdraftUsedBefore":"50.00","overdraftUsedAfter":"130.00"}`,
		},
		{
			name: "debit split (before=0, after=50) emits both keys",
			snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "50.00",
			},
			expectedJSON: `{"overdraftUsedBefore":"0","overdraftUsedAfter":"50.00"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entity := &Operation{
				ID:            "op-1",
				TransactionID: "tx-1",
				Snapshot:      tt.snapshot,
			}

			model := &OperationPostgreSQLModel{}
			model.FromEntity(entity)

			require.NotNil(t, model.Snapshot, "Snapshot must never be nil on PG model; DB column is NOT NULL DEFAULT '{}'")

			// Structural comparison: unmarshal both sides to a map and compare.
			var got, want map[string]any
			require.NoError(t, json.Unmarshal(model.Snapshot, &got), "PG model snapshot must be valid JSON")
			require.NoError(t, json.Unmarshal([]byte(tt.expectedJSON), &want))
			assert.Equal(t, want, got)
		})
	}
}

// TestOperationPostgreSQLModel_ToEntity_Snapshot verifies that ToEntity
// always populates the domain Operation.Snapshot value (never nil). Legacy
// rows (snapshot='{}'), missing keys, and malformed JSON all decode to the
// always-populated zero shape — uniform wire response regardless of row age.
func TestOperationPostgreSQLModel_ToEntity_Snapshot(t *testing.T) {
	tests := []struct {
		name     string
		raw      json.RawMessage
		expected mmodel.OperationSnapshot
	}{
		{
			name:     "nil raw bytes default-fill to zero shape",
			raw:      nil,
			expected: mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
		},
		{
			name:     "empty object default-fills to zero shape",
			raw:      json.RawMessage(`{}`),
			expected: mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
		},
		{
			name:     "both fields populated",
			raw:      json.RawMessage(`{"overdraftUsedBefore":"50.00","overdraftUsedAfter":"130.00"}`),
			expected: mmodel.OperationSnapshot{OverdraftUsedBefore: "50.00", OverdraftUsedAfter: "130.00"},
		},
		{
			name:     "only OverdraftUsedAfter present — Before defaults to 0",
			raw:      json.RawMessage(`{"overdraftUsedAfter":"130.00"}`),
			expected: mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "130.00"},
		},
		{
			name:     "explicit zero shape from PG round-trips intact",
			raw:      json.RawMessage(`{"overdraftUsedBefore":"0","overdraftUsedAfter":"0"}`),
			expected: mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
		},
		{
			name:     "malformed JSON falls back to zero shape (resilience)",
			raw:      json.RawMessage(`{not valid json`),
			expected: mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
		},
	}

	for _, tt := range tests {
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

			assert.Equal(t, tt.expected.OverdraftUsedBefore, entity.Snapshot.OverdraftUsedBefore,
				"OverdraftUsedBefore mismatch")
			assert.Equal(t, tt.expected.OverdraftUsedAfter, entity.Snapshot.OverdraftUsedAfter,
				"OverdraftUsedAfter mismatch")

			// Typed Balance.OverdraftUsed must reflect the snapshot strings.
			expectedBefore := decimal.Zero
			if tt.expected.OverdraftUsedBefore != "" && tt.expected.OverdraftUsedBefore != "0" {
				expectedBefore = decimal.RequireFromString(tt.expected.OverdraftUsedBefore)
			}
			assert.True(t, entity.Balance.OverdraftUsed.Equal(expectedBefore),
				"Balance.OverdraftUsed: want %s, got %s",
				expectedBefore.String(), entity.Balance.OverdraftUsed.String())

			expectedAfter := decimal.Zero
			if tt.expected.OverdraftUsedAfter != "" && tt.expected.OverdraftUsedAfter != "0" {
				expectedAfter = decimal.RequireFromString(tt.expected.OverdraftUsedAfter)
			}
			assert.True(t, entity.BalanceAfter.OverdraftUsed.Equal(expectedAfter),
				"BalanceAfter.OverdraftUsed: want %s, got %s",
				expectedAfter.String(), entity.BalanceAfter.OverdraftUsed.String())
		})
	}
}

// TestOperationPostgreSQLModel_Snapshot_RoundTrip verifies that a domain
// Operation.Snapshot survives FromEntity → ToEntity with semantic equality.
// This is the core invariant that protects against scan-site drift and
// marshal/unmarshal asymmetry under the always-populated contract.
func TestOperationPostgreSQLModel_Snapshot_RoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		snapshot mmodel.OperationSnapshot
	}{
		{
			name: "non-overdraft (zero shape) round-trips identically",
			snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "0",
			},
		},
		{
			name: "active overdraft round-trips identically",
			snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "50.00",
				OverdraftUsedAfter:  "130.00",
			},
		},
		{
			name: "credit repayment (after=0) round-trips identically",
			snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "130.00",
				OverdraftUsedAfter:  "0",
			},
		},
		{
			name: "debit split (before=0) round-trips identically",
			snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "50.00",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entity := &Operation{
				ID:              "op-1",
				TransactionID:   "tx-1",
				BalanceAffected: true,
				Snapshot:        tt.snapshot,
			}

			model := &OperationPostgreSQLModel{}
			model.FromEntity(entity)

			roundTripped := model.ToEntity()
			require.NotNil(t, roundTripped)

			assert.Equal(t, tt.snapshot.OverdraftUsedBefore, roundTripped.Snapshot.OverdraftUsedBefore)
			assert.Equal(t, tt.snapshot.OverdraftUsedAfter, roundTripped.Snapshot.OverdraftUsedAfter)
		})
	}
}

// TestOperationPointInTimeModel_ToEntity_Snapshot verifies that the
// point-in-time reconstruction model also default-fills the snapshot to the
// zero shape and applies any decoded fields on top. PIT queries must surface
// the same uniform wire shape as live reads.
func TestOperationPointInTimeModel_ToEntity_Snapshot(t *testing.T) {
	tests := []struct {
		name     string
		raw      json.RawMessage
		expected mmodel.OperationSnapshot
	}{
		{
			name:     "nil raw default-fills to zero shape",
			raw:      nil,
			expected: mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
		},
		{
			name:     "empty object default-fills to zero shape",
			raw:      json.RawMessage(`{}`),
			expected: mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
		},
		{
			name:     "both fields populated",
			raw:      json.RawMessage(`{"overdraftUsedBefore":"50.00","overdraftUsedAfter":"130.00"}`),
			expected: mmodel.OperationSnapshot{OverdraftUsedBefore: "50.00", OverdraftUsedAfter: "130.00"},
		},
		{
			name:     "only OverdraftUsedAfter present — Before defaults to 0",
			raw:      json.RawMessage(`{"overdraftUsedAfter":"130.00"}`),
			expected: mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "130.00"},
		},
	}

	for _, tt := range tests {
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

			assert.Equal(t, tt.expected.OverdraftUsedBefore, entity.Snapshot.OverdraftUsedBefore)
			assert.Equal(t, tt.expected.OverdraftUsedAfter, entity.Snapshot.OverdraftUsedAfter)
		})
	}
}
