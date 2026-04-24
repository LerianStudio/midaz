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

// TestMergeAccountingEntries_OverdraftRefundPreservation verifies that the RFC 7396
// merge function preserves existing Overdraft and Refund entries when a PATCH body
// updates an unrelated field (e.g., Direct). Without explicit handling of these
// fields, existing Overdraft/Refund entries are silently dropped, causing data loss.
//
// Defect: mergeAccountingEntries historically handled only 5 fields (Direct, Hold,
// Commit, Cancel, Revert). The AccountingEntries struct has 7 fields (adding
// Overdraft and Refund). This test protects against regression.
func TestMergeAccountingEntries_OverdraftRefundPreservation(t *testing.T) {
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
	refundEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1007", Description: "Refund Debit"},
		Credit: &mmodel.AccountingRubric{Code: "2007", Description: "Refund Credit"},
	}
	newOverdraftEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "NEW6D", Description: "New Overdraft Debit"},
		Credit: &mmodel.AccountingRubric{Code: "NEW6C", Description: "New Overdraft Credit"},
	}
	newRefundEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "NEW7D", Description: "New Refund Debit"},
		Credit: &mmodel.AccountingRubric{Code: "NEW7C", Description: "New Refund Credit"},
	}

	tests := []struct {
		name       string
		existing   *mmodel.AccountingEntries
		incoming   *mmodel.AccountingEntries
		rawUpdates string
		expected   *mmodel.AccountingEntries
	}{
		{
			name: "rfc7396: updating only direct preserves existing overdraft and refund",
			existing: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: overdraftEntry,
				Refund:    refundEntry,
			},
			incoming: &mmodel.AccountingEntries{
				Direct: newDirectEntry,
			},
			rawUpdates: `{"direct": {"debit": {"code": "NEW1", "description": "New Direct Debit"}, "credit": {"code": "NEW2", "description": "New Direct Credit"}}}`,
			expected: &mmodel.AccountingEntries{
				Direct:    newDirectEntry,
				Overdraft: overdraftEntry,
				Refund:    refundEntry,
			},
		},
		{
			name: "rfc7396: incoming overdraft and refund are applied to merged result",
			existing: &mmodel.AccountingEntries{
				Direct: directEntry,
			},
			incoming: &mmodel.AccountingEntries{
				Overdraft: newOverdraftEntry,
				Refund:    newRefundEntry,
			},
			rawUpdates: `{"overdraft": {"debit": {"code": "NEW6D", "description": "New Overdraft Debit"}, "credit": {"code": "NEW6C", "description": "New Overdraft Credit"}}, "refund": {"debit": {"code": "NEW7D", "description": "New Refund Debit"}, "credit": {"code": "NEW7C", "description": "New Refund Credit"}}}`,
			expected: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: newOverdraftEntry,
				Refund:    newRefundEntry,
			},
		},
		{
			name: "rfc7396: explicit null removes overdraft and refund entries",
			existing: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: overdraftEntry,
				Refund:    refundEntry,
			},
			incoming:   &mmodel.AccountingEntries{},
			rawUpdates: `{"overdraft": null, "refund": null}`,
			expected: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: nil,
				Refund:    nil,
			},
		},
		{
			name: "rfc7396: nil-check returns nil only when all 7 fields are nil",
			existing: &mmodel.AccountingEntries{
				Overdraft: overdraftEntry,
			},
			incoming:   &mmodel.AccountingEntries{},
			rawUpdates: `{"overdraft": null}`,
			expected:   nil,
		},
		{
			name: "rfc7396: refund-only existing removed returns nil",
			existing: &mmodel.AccountingEntries{
				Refund: refundEntry,
			},
			incoming:   &mmodel.AccountingEntries{},
			rawUpdates: `{"refund": null}`,
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
			assert.Equal(t, tt.expected.Refund, result.Refund, "Refund mismatch")
		})
	}
}

// TestMergeAccountingEntriesSimple_OverdraftRefundPreservation verifies that the
// simple merge fallback (used when raw JSON is unavailable) preserves existing
// Overdraft and Refund entries and applies incoming ones.
//
// Defect: mergeAccountingEntriesSimple historically handled only 5 fields,
// silently dropping existing Overdraft/Refund entries during partial updates.
func TestMergeAccountingEntriesSimple_OverdraftRefundPreservation(t *testing.T) {
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
	refundEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1007", Description: "Refund Debit"},
		Credit: &mmodel.AccountingRubric{Code: "2007", Description: "Refund Credit"},
	}
	newOverdraftEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "NEW6D", Description: "New Overdraft Debit"},
		Credit: &mmodel.AccountingRubric{Code: "NEW6C", Description: "New Overdraft Credit"},
	}
	newRefundEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "NEW7D", Description: "New Refund Debit"},
		Credit: &mmodel.AccountingRubric{Code: "NEW7C", Description: "New Refund Credit"},
	}

	tests := []struct {
		name     string
		existing *mmodel.AccountingEntries
		incoming *mmodel.AccountingEntries
		expected *mmodel.AccountingEntries
	}{
		{
			name: "simple: updating only direct preserves existing overdraft and refund",
			existing: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: overdraftEntry,
				Refund:    refundEntry,
			},
			incoming: &mmodel.AccountingEntries{
				Direct: newDirectEntry,
			},
			expected: &mmodel.AccountingEntries{
				Direct:    newDirectEntry,
				Overdraft: overdraftEntry,
				Refund:    refundEntry,
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
			name: "simple: incoming refund overrides existing",
			existing: &mmodel.AccountingEntries{
				Direct: directEntry,
				Refund: refundEntry,
			},
			incoming: &mmodel.AccountingEntries{
				Refund: newRefundEntry,
			},
			expected: &mmodel.AccountingEntries{
				Direct: directEntry,
				Refund: newRefundEntry,
			},
		},
		{
			name:     "simple: incoming adds overdraft and refund to existing",
			existing: &mmodel.AccountingEntries{Direct: directEntry},
			incoming: &mmodel.AccountingEntries{
				Overdraft: newOverdraftEntry,
				Refund:    newRefundEntry,
			},
			expected: &mmodel.AccountingEntries{
				Direct:    directEntry,
				Overdraft: newOverdraftEntry,
				Refund:    newRefundEntry,
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
			assert.Equal(t, tt.expected.Refund, result.Refund, "Refund mismatch")
		})
	}
}
