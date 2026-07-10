// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountingEntries_ExistingBehaviorUnchanged(t *testing.T) {
	t.Parallel()

	// Verify that OperationRoute without AccountingEntries serializes
	// the same as before (no new required fields breaking things)
	route := OperationRoute{
		ID:            uuid.New(),
		OperationType: "source",
		Title:         "Test Route",
		Code:          "EXT-001",
	}

	data, err := json.Marshal(route)
	require.NoError(t, err)

	var raw map[string]any
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	// accountingEntries should NOT be present when nil
	_, hasAccountingEntries := raw["accountingEntries"]
	assert.False(t, hasAccountingEntries, "nil AccountingEntries must not appear in JSON output")

	// Existing fields must still be present
	assert.Equal(t, "source", raw["operationType"])
	assert.Equal(t, "Test Route", raw["title"])
}

func TestAccountingEntries_Actions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		entries  *AccountingEntries
		expected []string
	}{
		{
			name:     "nil receiver returns nil",
			entries:  nil,
			expected: nil,
		},
		{
			name:     "empty struct returns nil",
			entries:  &AccountingEntries{},
			expected: nil,
		},
		{
			name: "direct only",
			entries: &AccountingEntries{
				Direct: &AccountingEntry{},
			},
			expected: []string{"direct"},
		},
		{
			name: "direct and hold",
			entries: &AccountingEntries{
				Direct: &AccountingEntry{},
				Hold:   &AccountingEntry{},
			},
			expected: []string{"direct", "hold"},
		},
		{
			name: "all five actions",
			entries: &AccountingEntries{
				Direct: &AccountingEntry{},
				Hold:   &AccountingEntry{},
				Commit: &AccountingEntry{},
				Cancel: &AccountingEntry{},
				Revert: &AccountingEntry{},
			},
			expected: []string{"direct", "hold", "commit", "cancel", "revert"},
		},
		{
			name: "block only",
			entries: &AccountingEntries{
				Block: &AccountingEntry{},
			},
			expected: []string{"block"},
		},
		{
			name: "unblock only",
			entries: &AccountingEntries{
				Unblock: &AccountingEntry{},
			},
			expected: []string{"unblock"},
		},
		{
			name: "block and unblock",
			entries: &AccountingEntries{
				Block:   &AccountingEntry{},
				Unblock: &AccountingEntry{},
			},
			expected: []string{"block", "unblock"},
		},
		{
			name: "all actions including overdraft, block and unblock",
			entries: &AccountingEntries{
				Direct:    &AccountingEntry{},
				Hold:      &AccountingEntry{},
				Commit:    &AccountingEntry{},
				Cancel:    &AccountingEntry{},
				Revert:    &AccountingEntry{},
				Overdraft: &AccountingEntry{},
				Block:     &AccountingEntry{},
				Unblock:   &AccountingEntry{},
			},
			expected: []string{"direct", "hold", "commit", "cancel", "revert", "overdraft", "block", "unblock"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.entries.Actions()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAccountingEntries_BlockUnblockJSONRoundTrip(t *testing.T) {
	t.Parallel()

	entries := AccountingEntries{
		Block: &AccountingEntry{
			Debit:  &AccountingRubric{Code: "2001", Description: "Block debit"},
			Credit: &AccountingRubric{Code: "2002", Description: "Block credit"},
		},
		Unblock: &AccountingEntry{
			Debit:  &AccountingRubric{Code: "3001", Description: "Unblock debit"},
			Credit: &AccountingRubric{Code: "3002", Description: "Unblock credit"},
		},
	}

	data, err := json.Marshal(entries)
	require.NoError(t, err)

	var raw map[string]any
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	_, hasBlock := raw["block"]
	assert.True(t, hasBlock, "populated Block must appear under the \"block\" JSON key")

	_, hasUnblock := raw["unblock"]
	assert.True(t, hasUnblock, "populated Unblock must appear under the \"unblock\" JSON key")

	var decoded AccountingEntries
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.NotNil(t, decoded.Block)
	require.NotNil(t, decoded.Block.Debit)
	assert.Equal(t, "2001", decoded.Block.Debit.Code)
	require.NotNil(t, decoded.Block.Credit)
	assert.Equal(t, "2002", decoded.Block.Credit.Code)

	require.NotNil(t, decoded.Unblock)
	require.NotNil(t, decoded.Unblock.Debit)
	assert.Equal(t, "3001", decoded.Unblock.Debit.Code)
	require.NotNil(t, decoded.Unblock.Credit)
	assert.Equal(t, "3002", decoded.Unblock.Credit.Code)
}

func TestAccountingEntries_BlockUnblockOmitEmpty(t *testing.T) {
	t.Parallel()

	entries := AccountingEntries{
		Direct: &AccountingEntry{},
	}

	data, err := json.Marshal(entries)
	require.NoError(t, err)

	var raw map[string]any
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	_, hasBlock := raw["block"]
	assert.False(t, hasBlock, "nil Block must be omitted from JSON output")

	_, hasUnblock := raw["unblock"]
	assert.False(t, hasUnblock, "nil Unblock must be omitted from JSON output")
}
