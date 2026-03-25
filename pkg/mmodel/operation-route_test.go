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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.entries.Actions()
			assert.Equal(t, tt.expected, result)
		})
	}
}
