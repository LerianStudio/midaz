// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
)

// TestDeriveActionForBuildOperations verifies that the action derivation logic
// in BuildOperations correctly maps pending flag to the appropriate action constant.
//
// Action derivation table:
//   - pending == false => "direct"
//   - pending == true  => "hold"
func TestDeriveActionForBuildOperations(t *testing.T) {
	tests := []struct {
		name           string
		pending        bool
		expectedAction string
	}{
		{
			name:           "non-pending transaction derives action=direct",
			pending:        false,
			expectedAction: cn.ActionDirect,
		},
		{
			name:           "pending transaction derives action=hold",
			pending:        true,
			expectedAction: cn.ActionHold,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// deriveTransactionAction is the function that will be extracted
			// to derive action from the pending flag in BuildOperations context
			action := deriveTransactionAction(tt.pending)
			assert.Equal(t, tt.expectedAction, action)
		})
	}
}

// TestDeriveActionForCommitOrCancel verifies that the action derivation logic
// in commitOrCancelTransaction correctly maps transaction status to action.
//
// Action derivation table:
//   - transactionStatus == APPROVED => "commit"
//   - transactionStatus == CANCELED => "cancel"
func TestDeriveActionForCommitOrCancel(t *testing.T) {
	tests := []struct {
		name              string
		transactionStatus string
		expectedAction    string
	}{
		{
			name:              "APPROVED status derives action=commit",
			transactionStatus: cn.APPROVED,
			expectedAction:    cn.ActionCommit,
		},
		{
			name:              "CANCELED status derives action=cancel",
			transactionStatus: cn.CANCELED,
			expectedAction:    cn.ActionCancel,
		},
		{
			name:              "unexpected status defaults to action=cancel",
			transactionStatus: "UNKNOWN",
			expectedAction:    cn.ActionCancel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// deriveCommitCancelAction is the function that will be extracted
			// to derive action from transactionStatus in commitOrCancelTransaction
			action := deriveCommitCancelAction(tt.transactionStatus)
			assert.Equal(t, tt.expectedAction, action)
		})
	}
}

// TestDeriveActionForRevert verifies that RevertTransaction always uses action="revert".
func TestDeriveActionForRevert(t *testing.T) {
	// deriveRevertAction always returns "revert"
	action := deriveRevertAction()
	assert.Equal(t, cn.ActionRevert, action)
}

// TestResolveAccountingEntry verifies that resolveAccountingEntry correctly maps
// an action string to the corresponding AccountingEntry field.
func TestResolveAccountingEntry(t *testing.T) {
	directEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
		Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
	}
	holdEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Held Cash"},
		Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Held Revenue"},
	}
	commitEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Committed"},
		Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Settled"},
	}
	cancelEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancelled"},
		Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Reversed"},
	}
	revertEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1005", Description: "Reverted"},
		Credit: &mmodel.AccountingRubric{Code: "2005", Description: "Undone"},
	}

	fullEntries := &mmodel.AccountingEntries{
		Direct: directEntry,
		Hold:   holdEntry,
		Commit: commitEntry,
		Cancel: cancelEntry,
		Revert: revertEntry,
	}

	tests := []struct {
		name     string
		action   string
		entries  *mmodel.AccountingEntries
		expected *mmodel.AccountingEntry
	}{
		{
			name:     "nil AccountingEntries returns nil",
			action:   cn.ActionDirect,
			entries:  nil,
			expected: nil,
		},
		{
			name:     "direct action returns Direct entry",
			action:   cn.ActionDirect,
			entries:  fullEntries,
			expected: directEntry,
		},
		{
			name:     "hold action returns Hold entry",
			action:   cn.ActionHold,
			entries:  fullEntries,
			expected: holdEntry,
		},
		{
			name:     "commit action returns Commit entry",
			action:   cn.ActionCommit,
			entries:  fullEntries,
			expected: commitEntry,
		},
		{
			name:     "cancel action returns Cancel entry",
			action:   cn.ActionCancel,
			entries:  fullEntries,
			expected: cancelEntry,
		},
		{
			name:     "revert action returns Revert entry",
			action:   cn.ActionRevert,
			entries:  fullEntries,
			expected: revertEntry,
		},
		{
			name:   "partial config with Direct nil returns nil",
			action: cn.ActionDirect,
			entries: &mmodel.AccountingEntries{
				Hold: holdEntry,
			},
			expected: nil,
		},
		{
			name:     "unknown action returns nil",
			action:   "unknown",
			entries:  fullEntries,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveAccountingEntry(tt.action, tt.entries)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestResolveRouteCode verifies that resolveRouteCode correctly looks up
// an operation route's Code from the cache map.
func TestResolveRouteCode(t *testing.T) {
	tests := []struct {
		name       string
		routeID    string
		routeCache map[string]mmodel.OperationRouteCache
		expected   string
	}{
		{
			name:    "route found with Code returns code",
			routeID: "route-1",
			routeCache: map[string]mmodel.OperationRouteCache{
				"route-1": {Code: "EXT-001", OperationType: "source"},
			},
			expected: "EXT-001",
		},
		{
			name:    "route found with empty Code returns empty string",
			routeID: "route-1",
			routeCache: map[string]mmodel.OperationRouteCache{
				"route-1": {OperationType: "source"},
			},
			expected: "",
		},
		{
			name:    "route not found returns empty string",
			routeID: "route-missing",
			routeCache: map[string]mmodel.OperationRouteCache{
				"route-1": {Code: "EXT-001"},
			},
			expected: "",
		},
		{
			name:       "empty cache map returns empty string",
			routeID:    "route-1",
			routeCache: map[string]mmodel.OperationRouteCache{},
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveRouteCode(tt.routeID, tt.routeCache)
			assert.Equal(t, tt.expected, result)
		})
	}
}
