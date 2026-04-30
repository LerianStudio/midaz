// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeAccountingEntries_OverdraftPreservation verifies that the RFC 7396
// merge function preserves existing Overdraft entries when a PATCH body
// updates an unrelated field (e.g., Direct). Without explicit handling of
// the Overdraft field, existing entries are silently dropped, causing data loss.
func TestMergeAccountingEntries_OverdraftPreservation(t *testing.T) {
	t.Parallel()

	directEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Direct Debit"},
		Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Direct Credit"},
	}
	newDirectEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "NEW1", Description: "New Direct Debit"},
		Credit: &mmodel.AccountingRubric{Code: "NEW2", Description: "New Direct Credit"},
	}
	overdraftEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
		Credit: &mmodel.AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
	}
	newOverdraftEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "NEW6D", Description: "New Overdraft Debit"},
		Credit: &mmodel.AccountingRubric{Code: "NEW6C", Description: "New Overdraft Credit"},
	}

	tests := []struct {
		name       string
		existing   *mmodel.AccountingEntries
		incoming   *mmodel.AccountingEntries
		rawUpdates string
		expected   *mmodel.AccountingEntries
	}{
		{
			name: "rfc7396: updating only direct preserves existing overdraft",
			existing: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: overdraftEntry,
			},
			incoming: &mmodel.AccountingEntries{
				Direct: newDirectEntry,
			},
			rawUpdates: `{"direct": {"debit": {"code": "NEW1", "description": "New Direct Debit"}, "credit": {"code": "NEW2", "description": "New Direct Credit"}}}`,
			expected: &mmodel.AccountingEntries{
				Direct:    newDirectEntry,
				Overdraft: overdraftEntry,
			},
		},
		{
			name: "rfc7396: incoming overdraft is applied to merged result",
			existing: &mmodel.AccountingEntries{
				Direct: directEntry,
			},
			incoming: &mmodel.AccountingEntries{
				Overdraft: newOverdraftEntry,
			},
			rawUpdates: `{"overdraft": {"debit": {"code": "NEW6D", "description": "New Overdraft Debit"}, "credit": {"code": "NEW6C", "description": "New Overdraft Credit"}}}`,
			expected: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: newOverdraftEntry,
			},
		},
		{
			name: "rfc7396: explicit null removes overdraft entry",
			existing: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: overdraftEntry,
			},
			incoming:   &mmodel.AccountingEntries{},
			rawUpdates: `{"overdraft": null}`,
			expected: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: nil,
			},
		},
		{
			name: "rfc7396: nil-check returns nil only when all 6 fields are nil",
			existing: &mmodel.AccountingEntries{
				Overdraft: overdraftEntry,
			},
			incoming:   &mmodel.AccountingEntries{},
			rawUpdates: `{"overdraft": null}`,
			expected:   nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := mergeAccountingEntries(tt.existing, tt.incoming, []byte(tt.rawUpdates))

			if tt.expected == nil {
				assert.Nil(t, result, "expected nil result when all fields are nil")
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.expected.Direct, result.Direct, "Direct mismatch")
			assert.Equal(t, tt.expected.Hold, result.Hold, "Hold mismatch")
			assert.Equal(t, tt.expected.Commit, result.Commit, "Commit mismatch")
			assert.Equal(t, tt.expected.Cancel, result.Cancel, "Cancel mismatch")
			assert.Equal(t, tt.expected.Revert, result.Revert, "Revert mismatch")
			assert.Equal(t, tt.expected.Overdraft, result.Overdraft, "Overdraft mismatch")
		})
	}
}

// TestMergeAccountingEntriesSimple_OverdraftPreservation verifies that the
// simple merge fallback (used when raw JSON is unavailable) preserves existing
// Overdraft entries and applies incoming ones.
func TestMergeAccountingEntriesSimple_OverdraftPreservation(t *testing.T) {
	t.Parallel()

	directEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Direct Debit"},
		Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Direct Credit"},
	}
	newDirectEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "NEW1", Description: "New Direct Debit"},
		Credit: &mmodel.AccountingRubric{Code: "NEW2", Description: "New Direct Credit"},
	}
	overdraftEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
		Credit: &mmodel.AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
	}
	newOverdraftEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "NEW6D", Description: "New Overdraft Debit"},
		Credit: &mmodel.AccountingRubric{Code: "NEW6C", Description: "New Overdraft Credit"},
	}

	tests := []struct {
		name     string
		existing *mmodel.AccountingEntries
		incoming *mmodel.AccountingEntries
		expected *mmodel.AccountingEntries
	}{
		{
			name: "simple: updating only direct preserves existing overdraft",
			existing: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: overdraftEntry,
			},
			incoming: &mmodel.AccountingEntries{
				Direct: newDirectEntry,
			},
			expected: &mmodel.AccountingEntries{
				Direct:    newDirectEntry,
				Overdraft: overdraftEntry,
			},
		},
		{
			name: "simple: incoming overdraft overrides existing",
			existing: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: overdraftEntry,
			},
			incoming: &mmodel.AccountingEntries{
				Overdraft: newOverdraftEntry,
			},
			expected: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: newOverdraftEntry,
			},
		},
		{
			name:     "simple: incoming adds overdraft to existing",
			existing: &mmodel.AccountingEntries{Direct: directEntry},
			incoming: &mmodel.AccountingEntries{
				Overdraft: newOverdraftEntry,
			},
			expected: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: newOverdraftEntry,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := mergeAccountingEntriesSimple(tt.existing, tt.incoming)

			require.NotNil(t, result)
			assert.Equal(t, tt.expected.Direct, result.Direct, "Direct mismatch")
			assert.Equal(t, tt.expected.Hold, result.Hold, "Hold mismatch")
			assert.Equal(t, tt.expected.Commit, result.Commit, "Commit mismatch")
			assert.Equal(t, tt.expected.Cancel, result.Cancel, "Cancel mismatch")
			assert.Equal(t, tt.expected.Revert, result.Revert, "Revert mismatch")
			assert.Equal(t, tt.expected.Overdraft, result.Overdraft, "Overdraft mismatch")
		})
	}
}
