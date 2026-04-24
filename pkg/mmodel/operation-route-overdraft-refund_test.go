// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// T-008: Accounting Entries Extension — Overdraft & Refund
//
// These tests capture the expected behavior for the new `Overdraft` and
// `Refund` fields on the AccountingEntries struct. Both fields follow the
// same pattern as the existing scenario fields (Direct, Hold, Commit,
// Cancel, Revert) and, per the task spec, BOTH require `Debit` + `Credit`
// rubrics to be present (the "bidirectional-like" requirement).
//
// This file is written as TDD-RED: it MUST FAIL until the model, handler
// validation, and PostgreSQL JSONB adapter are extended (T-008 GREEN).
package mmodel

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAccountingEntries_HasOverdraftAndRefundFields verifies that the new
// Overdraft and Refund fields exist on the AccountingEntries struct.
//
// This test uses struct-literal field assignment; if the fields do not
// exist the file fails to compile (which is the expected RED behavior).
func TestAccountingEntries_HasOverdraftAndRefundFields(t *testing.T) {
	t.Parallel()

	entries := &AccountingEntries{
		Overdraft: &AccountingEntry{
			Debit:  &AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
			Credit: &AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
		},
		Refund: &AccountingEntry{
			Debit:  &AccountingRubric{Code: "1007", Description: "Refund Debit"},
			Credit: &AccountingRubric{Code: "2007", Description: "Refund Credit"},
		},
	}

	require.NotNil(t, entries.Overdraft, "Overdraft field must exist on AccountingEntries")
	require.NotNil(t, entries.Refund, "Refund field must exist on AccountingEntries")
	require.NotNil(t, entries.Overdraft.Debit, "Overdraft.Debit must be addressable")
	require.NotNil(t, entries.Overdraft.Credit, "Overdraft.Credit must be addressable")
	require.NotNil(t, entries.Refund.Debit, "Refund.Debit must be addressable")
	require.NotNil(t, entries.Refund.Credit, "Refund.Credit must be addressable")

	assert.Equal(t, "1006", entries.Overdraft.Debit.Code)
	assert.Equal(t, "2006", entries.Overdraft.Credit.Code)
	assert.Equal(t, "1007", entries.Refund.Debit.Code)
	assert.Equal(t, "2007", entries.Refund.Credit.Code)
}

// TestAccountingEntries_Actions_IncludesOverdraftAndRefund ensures that
// the Actions() helper reports "overdraft" and "refund" when the
// respective fields are non-nil (same pattern as direct/hold/commit/...).
func TestAccountingEntries_Actions_IncludesOverdraftAndRefund(t *testing.T) {
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
			name: "refund only",
			entries: &AccountingEntries{
				Refund: &AccountingEntry{},
			},
			expected: []string{"refund"},
		},
		{
			name: "overdraft and refund",
			entries: &AccountingEntries{
				Overdraft: &AccountingEntry{},
				Refund:    &AccountingEntry{},
			},
			expected: []string{"overdraft", "refund"},
		},
		{
			name: "direct + overdraft + refund",
			entries: &AccountingEntries{
				Direct:    &AccountingEntry{},
				Overdraft: &AccountingEntry{},
				Refund:    &AccountingEntry{},
			},
			expected: []string{"direct", "overdraft", "refund"},
		},
		{
			name: "all seven scenarios",
			entries: &AccountingEntries{
				Direct:    &AccountingEntry{},
				Hold:      &AccountingEntry{},
				Commit:    &AccountingEntry{},
				Cancel:    &AccountingEntry{},
				Revert:    &AccountingEntry{},
				Overdraft: &AccountingEntry{},
				Refund:    &AccountingEntry{},
			},
			expected: []string{"direct", "hold", "commit", "cancel", "revert", "overdraft", "refund"},
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

// TestAccountingEntries_OverdraftRefund_JSONMarshal checks that the new
// fields are correctly serialized with the expected camelCase JSON keys
// "overdraft" and "refund", and that nil fields remain omitted.
func TestAccountingEntries_OverdraftRefund_JSONMarshal(t *testing.T) {
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
			Refund: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1007", Description: "Refund Debit"},
				Credit: &AccountingRubric{Code: "2007", Description: "Refund Credit"},
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
	_, hasRefund := ae["refund"]
	assert.True(t, hasOverdraft, `JSON must contain the "overdraft" key`)
	assert.True(t, hasRefund, `JSON must contain the "refund" key`)

	// Nil new fields must be omitted (omitempty) — backward compatible payload.
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
	_, hasRefund2 := ae2["refund"]
	assert.False(t, hasOverdraft2, "nil Overdraft must be omitted from JSON")
	assert.False(t, hasRefund2, "nil Refund must be omitted from JSON")
}

// TestAccountingEntries_OverdraftRefund_JSONUnmarshal verifies that both
// fields are decoded correctly from JSON input.
func TestAccountingEntries_OverdraftRefund_JSONUnmarshal(t *testing.T) {
	t.Parallel()

	jsonInput := `{
		"operationType":"source",
		"accountingEntries":{
			"direct":{"debit":{"code":"1001","description":"Cash"}},
			"overdraft":{
				"debit":{"code":"1006","description":"Overdraft Debit"},
				"credit":{"code":"2006","description":"Overdraft Credit"}
			},
			"refund":{
				"debit":{"code":"1007","description":"Refund Debit"},
				"credit":{"code":"2007","description":"Refund Credit"}
			}
		}
	}`

	var route OperationRoute
	require.NoError(t, json.Unmarshal([]byte(jsonInput), &route))

	require.NotNil(t, route.AccountingEntries)
	require.NotNil(t, route.AccountingEntries.Overdraft, "Overdraft must be decoded from JSON")
	require.NotNil(t, route.AccountingEntries.Refund, "Refund must be decoded from JSON")

	assert.Equal(t, "1006", route.AccountingEntries.Overdraft.Debit.Code)
	assert.Equal(t, "Overdraft Debit", route.AccountingEntries.Overdraft.Debit.Description)
	assert.Equal(t, "2006", route.AccountingEntries.Overdraft.Credit.Code)
	assert.Equal(t, "Overdraft Credit", route.AccountingEntries.Overdraft.Credit.Description)

	assert.Equal(t, "1007", route.AccountingEntries.Refund.Debit.Code)
	assert.Equal(t, "Refund Debit", route.AccountingEntries.Refund.Debit.Description)
	assert.Equal(t, "2007", route.AccountingEntries.Refund.Credit.Code)
	assert.Equal(t, "Refund Credit", route.AccountingEntries.Refund.Credit.Description)
}

// TestAccountingEntries_OverdraftRefund_JSONUnmarshal_BackwardCompatible
// guarantees that JSON payloads *without* the new fields still unmarshal
// cleanly — old stored rows continue to work.
func TestAccountingEntries_OverdraftRefund_JSONUnmarshal_BackwardCompatible(t *testing.T) {
	t.Parallel()

	// JSON representing a row stored BEFORE T-008: no overdraft / refund keys.
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

	// New fields must be nil — absent JSON keys stay at zero value.
	assert.Nil(t, route.AccountingEntries.Overdraft,
		"Overdraft must be nil when absent from legacy JSON (JSONB backward compat)")
	assert.Nil(t, route.AccountingEntries.Refund,
		"Refund must be nil when absent from legacy JSON (JSONB backward compat)")
}

// TestAccountingEntries_OverdraftRefund_RoundTrip verifies lossless JSON
// round-trip: marshal → unmarshal preserves all new-field values.
// This simulates the PostgreSQL JSONB persistence cycle.
func TestAccountingEntries_OverdraftRefund_RoundTrip(t *testing.T) {
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
			Refund: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1007", Description: "Refund Debit"},
				Credit: &AccountingRubric{Code: "2007", Description: "Refund Credit"},
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
	require.NotNil(t, ae.Refund)

	assert.Equal(t, "1006", ae.Overdraft.Debit.Code)
	assert.Equal(t, "Overdraft Debit", ae.Overdraft.Debit.Description)
	assert.Equal(t, "2006", ae.Overdraft.Credit.Code)
	assert.Equal(t, "Overdraft Credit", ae.Overdraft.Credit.Description)

	assert.Equal(t, "1007", ae.Refund.Debit.Code)
	assert.Equal(t, "Refund Debit", ae.Refund.Debit.Description)
	assert.Equal(t, "2007", ae.Refund.Credit.Code)
	assert.Equal(t, "Refund Credit", ae.Refund.Credit.Description)

	// Unused scenarios remain nil.
	assert.Nil(t, ae.Hold)
	assert.Nil(t, ae.Commit)
	assert.Nil(t, ae.Cancel)
	assert.Nil(t, ae.Revert)
}

// TestCreateOperationRouteInput_AcceptsOverdraftAndRefund verifies that
// the CreateOperationRouteInput payload accepts the new scenarios so
// the HTTP create handler surface exposes them to API clients.
func TestCreateOperationRouteInput_AcceptsOverdraftAndRefund(t *testing.T) {
	t.Parallel()

	input := CreateOperationRouteInput{
		Title:         "Overdraft Route",
		OperationType: "bidirectional",
		AccountingEntries: &AccountingEntries{
			Overdraft: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
				Credit: &AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
			},
			Refund: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1007", Description: "Refund Debit"},
				Credit: &AccountingRubric{Code: "2007", Description: "Refund Credit"},
			},
		},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded CreateOperationRouteInput
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.NotNil(t, decoded.AccountingEntries)
	require.NotNil(t, decoded.AccountingEntries.Overdraft)
	require.NotNil(t, decoded.AccountingEntries.Refund)
	assert.Equal(t, "1006", decoded.AccountingEntries.Overdraft.Debit.Code)
	assert.Equal(t, "2007", decoded.AccountingEntries.Refund.Credit.Code)
}

// TestUpdateOperationRouteInput_AcceptsOverdraftAndRefund verifies the
// update (PATCH) payload exposes the same new fields.
func TestUpdateOperationRouteInput_AcceptsOverdraftAndRefund(t *testing.T) {
	t.Parallel()

	input := UpdateOperationRouteInput{
		Title: "Updated Overdraft Route",
		AccountingEntries: &AccountingEntries{
			Overdraft: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
				Credit: &AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
			},
			Refund: &AccountingEntry{
				Debit:  &AccountingRubric{Code: "1007", Description: "Refund Debit"},
				Credit: &AccountingRubric{Code: "2007", Description: "Refund Credit"},
			},
		},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded UpdateOperationRouteInput
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.NotNil(t, decoded.AccountingEntries)
	require.NotNil(t, decoded.AccountingEntries.Overdraft)
	require.NotNil(t, decoded.AccountingEntries.Refund)
	assert.Equal(t, "1006", decoded.AccountingEntries.Overdraft.Debit.Code)
	assert.Equal(t, "2007", decoded.AccountingEntries.Refund.Credit.Code)
}
