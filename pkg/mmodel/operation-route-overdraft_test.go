// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// T-008 + T-014: Accounting Entries Extension — Overdraft
//
// These tests capture the expected behavior for the `Overdraft` field on
// the AccountingEntries struct. After T-014, the separate `Refund` field
// was collapsed into Overdraft — the single entry covers the full overdraft
// lifecycle (Debit rubric = deficit grows, Credit rubric = repayment).
//
// The field follows the same pattern as the existing scenario fields
// (Direct, Hold, Commit, Cancel, Revert) and requires BOTH `Debit` +
// `Credit` rubrics to be present.
package mmodel

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAccountingEntries_HasOverdraftField verifies that the Overdraft
// field exists on the AccountingEntries struct.
func TestAccountingEntries_HasOverdraftField(t *testing.T) {
	t.Parallel()

	entries := &AccountingEntries{
		Overdraft: &AccountingEntry{
			Debit:  &AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
			Credit: &AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
		},
	}

	require.NotNil(t, entries.Overdraft, "Overdraft field must exist on AccountingEntries")
	require.NotNil(t, entries.Overdraft.Debit, "Overdraft.Debit must be addressable")
	require.NotNil(t, entries.Overdraft.Credit, "Overdraft.Credit must be addressable")

	assert.Equal(t, "1006", entries.Overdraft.Debit.Code)
	assert.Equal(t, "2006", entries.Overdraft.Credit.Code)
}

// TestAccountingEntries_Actions_IncludesOverdraft ensures that the
// Actions() helper reports "overdraft" when the field is non-nil.
// After T-014, "refund" is no longer a valid action.
func TestAccountingEntries_Actions_IncludesOverdraft(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		entries  *AccountingEntries
		expected []string
	}{
		{
			name: "overdraft only",
			entries: &AccountingEntries{
				Overdraft: &AccountingEntry{},
			},
			expected: []string{"overdraft"},
		},
		{
			name: "direct + overdraft",
			entries: &AccountingEntries{
				Direct:    &AccountingEntry{},
				Overdraft: &AccountingEntry{},
			},
			expected: []string{"direct", "overdraft"},
		},
		{
			name: "all six scenarios",
			entries: &AccountingEntries{
				Direct:    &AccountingEntry{},
				Hold:      &AccountingEntry{},
				Commit:    &AccountingEntry{},
				Cancel:    &AccountingEntry{},
				Revert:    &AccountingEntry{},
				Overdraft: &AccountingEntry{},
			},
			expected: []string{"direct", "hold", "commit", "cancel", "revert", "overdraft"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.entries.Actions())
		})
	}
}

// TestAccountingEntries_Overdraft_JSONMarshal checks that the Overdraft
// field is serialized with the expected JSON key "overdraft", and that
// a nil Overdraft is omitted from JSON output.
func TestAccountingEntries_Overdraft_JSONMarshal(t *testing.T) {
	t.Parallel()

	route := OperationRoute{
		ID:            uuid.New(),
		OperationType: "source",
		AccountingEntries: &AccountingEntries{
			Direct: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1001", Description: "Cash"},
				Credit: &AccountingRubric{Code: "2001", Description: "Revenue"},
			},
			Overdraft: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
				Credit: &AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
			},
		},
	}

	data, err := json.Marshal(route)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	ae, ok := raw["accountingEntries"].(map[string]any)
	require.True(t, ok, "accountingEntries must be a JSON object")

	_, hasOverdraft := ae["overdraft"]
	assert.True(t, hasOverdraft, `JSON must contain the "overdraft" key`)

	// Refund key must NOT appear (field removed in T-014)
	_, hasRefund := ae["refund"]
	assert.False(t, hasRefund, `JSON must NOT contain the "refund" key (removed in T-014)`)

	// Nil Overdraft must be omitted (omitempty) — backward compatible payload.
	routeWithoutNew := OperationRoute{
		ID:            uuid.New(),
		OperationType: "source",
		AccountingEntries: &AccountingEntries{
			Direct: &AccountingEntry{
				Debit: &AccountingRubric{Code: "1001", Description: "Cash"},
			},
		},
	}

	data2, err := json.Marshal(routeWithoutNew)
	require.NoError(t, err)

	var raw2 map[string]any
	require.NoError(t, json.Unmarshal(data2, &raw2))

	ae2, ok := raw2["accountingEntries"].(map[string]any)
	require.True(t, ok)

	_, hasOverdraft2 := ae2["overdraft"]
	assert.False(t, hasOverdraft2, "nil Overdraft must be omitted from JSON")
}

// TestAccountingEntries_Overdraft_JSONUnmarshal verifies that the
// Overdraft field is decoded correctly from JSON input. Also verifies
// that a legacy "refund" key in JSON is silently dropped (unknown keys
// are ignored by Go's json.Unmarshal).
func TestAccountingEntries_Overdraft_JSONUnmarshal(t *testing.T) {
	t.Parallel()

	jsonInput := `{
		"operationType":"source",
		"accountingEntries":{
			"direct":{"debit":{"code":"1001","description":"Cash"}},
			"overdraft":{
				"debit":{"code":"1006","description":"Overdraft Debit"},
				"credit":{"code":"2006","description":"Overdraft Credit"}
			}
		}
	}`

	var route OperationRoute
	require.NoError(t, json.Unmarshal([]byte(jsonInput), &route))

	require.NotNil(t, route.AccountingEntries)
	require.NotNil(t, route.AccountingEntries.Overdraft, "Overdraft must be decoded from JSON")

	assert.Equal(t, "1006", route.AccountingEntries.Overdraft.Debit.Code)
	assert.Equal(t, "Overdraft Debit", route.AccountingEntries.Overdraft.Debit.Description)
	assert.Equal(t, "2006", route.AccountingEntries.Overdraft.Credit.Code)
	assert.Equal(t, "Overdraft Credit", route.AccountingEntries.Overdraft.Credit.Description)
}

// TestAccountingEntries_LegacyRefundKey_SilentlyDropped verifies that
// JSON payloads containing the removed "refund" key unmarshal cleanly —
// the unknown key is silently ignored by Go's json.Unmarshal.
func TestAccountingEntries_LegacyRefundKey_SilentlyDropped(t *testing.T) {
	t.Parallel()

	jsonWithRefund := `{
		"operationType":"source",
		"accountingEntries":{
			"direct":{"debit":{"code":"1001","description":"Cash"}},
			"overdraft":{
				"debit":{"code":"1006","description":"Overdraft Debit"},
				"credit":{"code":"2006","description":"Overdraft Credit"}
			},
			"refund":{
				"debit":{"code":"1007","description":"Legacy Refund Debit"},
				"credit":{"code":"2007","description":"Legacy Refund Credit"}
			}
		}
	}`

	var route OperationRoute
	require.NoError(t, json.Unmarshal([]byte(jsonWithRefund), &route))

	require.NotNil(t, route.AccountingEntries)
	require.NotNil(t, route.AccountingEntries.Overdraft, "Overdraft must still be decoded")
	assert.Equal(t, "1006", route.AccountingEntries.Overdraft.Debit.Code)
}

// TestAccountingEntries_JSONUnmarshal_BackwardCompatible guarantees that
// JSON payloads without overdraft unmarshal cleanly — old stored rows
// continue to work.
func TestAccountingEntries_JSONUnmarshal_BackwardCompatible(t *testing.T) {
	t.Parallel()

	legacyJSON := `{
		"operationType":"source",
		"accountingEntries":{
			"direct":{
				"debit":{"code":"1001","description":"Cash"},
				"credit":{"code":"2001","description":"Revenue"}
			},
			"hold":{
				"debit":{"code":"1002","description":"Held"},
				"credit":{"code":"2002","description":"Held Revenue"}
			}
		}
	}`

	var route OperationRoute
	require.NoError(t, json.Unmarshal([]byte(legacyJSON), &route))

	require.NotNil(t, route.AccountingEntries)
	require.NotNil(t, route.AccountingEntries.Direct)
	require.NotNil(t, route.AccountingEntries.Hold)

	assert.Nil(t, route.AccountingEntries.Overdraft,
		"Overdraft must be nil when absent from legacy JSON (JSONB backward compat)")
}

// TestAccountingEntries_Overdraft_RoundTrip verifies lossless JSON
// round-trip: marshal → unmarshal preserves all overdraft field values.
// This simulates the PostgreSQL JSONB persistence cycle.
func TestAccountingEntries_Overdraft_RoundTrip(t *testing.T) {
	t.Parallel()

	original := OperationRoute{
		ID:            uuid.New(),
		OperationType: "bidirectional",
		AccountingEntries: &AccountingEntries{
			Direct: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1001", Description: "Cash"},
				Credit: &AccountingRubric{Code: "2001", Description: "Revenue"},
			},
			Overdraft: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
				Credit: &AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
			},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var roundTripped OperationRoute
	require.NoError(t, json.Unmarshal(data, &roundTripped))

	ae := roundTripped.AccountingEntries
	require.NotNil(t, ae)
	require.NotNil(t, ae.Overdraft)

	assert.Equal(t, "1006", ae.Overdraft.Debit.Code)
	assert.Equal(t, "Overdraft Debit", ae.Overdraft.Debit.Description)
	assert.Equal(t, "2006", ae.Overdraft.Credit.Code)
	assert.Equal(t, "Overdraft Credit", ae.Overdraft.Credit.Description)

	// Unused scenarios remain nil.
	assert.Nil(t, ae.Hold)
	assert.Nil(t, ae.Commit)
	assert.Nil(t, ae.Cancel)
	assert.Nil(t, ae.Revert)
}

// TestCreateOperationRouteInput_AcceptsOverdraft verifies that
// the CreateOperationRouteInput payload accepts the overdraft scenario
// so the HTTP create handler surface exposes it to API clients.
func TestCreateOperationRouteInput_AcceptsOverdraft(t *testing.T) {
	t.Parallel()

	input := CreateOperationRouteInput{
		Title:         "Overdraft Route",
		OperationType: "bidirectional",
		AccountingEntries: &AccountingEntries{
			Overdraft: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
				Credit: &AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
			},
		},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded CreateOperationRouteInput
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.NotNil(t, decoded.AccountingEntries)
	require.NotNil(t, decoded.AccountingEntries.Overdraft)
	assert.Equal(t, "1006", decoded.AccountingEntries.Overdraft.Debit.Code)
	assert.Equal(t, "2006", decoded.AccountingEntries.Overdraft.Credit.Code)
}

// TestUpdateOperationRouteInput_AcceptsOverdraft verifies the
// update (PATCH) payload exposes the overdraft field.
func TestUpdateOperationRouteInput_AcceptsOverdraft(t *testing.T) {
	t.Parallel()

	input := UpdateOperationRouteInput{
		Title: "Updated Overdraft Route",
		AccountingEntries: &AccountingEntries{
			Overdraft: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
				Credit: &AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
			},
		},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded UpdateOperationRouteInput
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.NotNil(t, decoded.AccountingEntries)
	require.NotNil(t, decoded.AccountingEntries.Overdraft)
	assert.Equal(t, "1006", decoded.AccountingEntries.Overdraft.Debit.Code)
	assert.Equal(t, "2006", decoded.AccountingEntries.Overdraft.Credit.Code)
}
