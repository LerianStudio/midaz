// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationRouteHandler_validateAccountingEntries(t *testing.T) {
	t.Parallel()

	handler := &OperationRouteHandler{}

	tests := []struct {
		name        string
		entries     *mmodel.AccountingEntries
		expectError bool
		errorField  string
	}{
		{
			name:        "nil entries returns no error",
			entries:     nil,
			expectError: false,
		},
		{
			name: "valid full entries with all five actions",
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Held Funds"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Pending"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Committed"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Settled"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancelled"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Reversed"},
				},
				Revert: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1005", Description: "Reverted"},
					Credit: &mmodel.AccountingRubric{Code: "2005", Description: "Restored"},
				},
			},
			expectError: false,
		},
		{
			name: "valid partial entries with only direct",
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
				},
			},
			expectError: false,
		},
		{
			name: "direct entry missing debit returns error",
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
				},
			},
			expectError: true,
			errorField:  "accountingEntries.direct.debit",
		},
		{
			name: "hold entry missing credit returns error",
			entries: &mmodel.AccountingEntries{
				Hold: &mmodel.AccountingEntry{
					Debit: &mmodel.AccountingRubric{Code: "1002", Description: "Held Funds"},
				},
			},
			expectError: true,
			errorField:  "accountingEntries.hold.credit",
		},
		{
			name: "debit with empty code returns error",
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "", Description: "Cash"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
				},
			},
			expectError: true,
			errorField:  "accountingEntries.direct.debit.code",
		},
		{
			name: "credit with empty description returns error",
			entries: &mmodel.AccountingEntries{
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Committed"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: ""},
				},
			},
			expectError: true,
			errorField:  "accountingEntries.commit.credit.description",
		},
		{
			name: "debit with whitespace-only code returns error",
			entries: &mmodel.AccountingEntries{
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "   ", Description: "Cancelled"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Reversed"},
				},
			},
			expectError: true,
			errorField:  "accountingEntries.cancel.debit.code",
		},
		{
			name: "credit with whitespace-only description returns error",
			entries: &mmodel.AccountingEntries{
				Revert: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1005", Description: "Reverted"},
					Credit: &mmodel.AccountingRubric{Code: "2005", Description: "   "},
				},
			},
			expectError: true,
			errorField:  "accountingEntries.revert.credit.description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			err := handler.validateAccountingEntries(ctx, tt.entries)

			if tt.expectError {
				require.Error(t, err, "expected validation error for field: %s", tt.errorField)
				assert.Contains(t, err.Error(), tt.errorField, "error message should reference the invalid field path")
			} else {
				require.NoError(t, err, "expected no validation error")
			}
		})
	}
}

func TestFindUnknownAccountingEntryKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        json.RawMessage
		expectNil  bool
		expectKeys []string
	}{
		{
			name:       "all valid keys",
			raw:        json.RawMessage(`{"direct":{"debit":{}},"hold":{"debit":{}},"commit":{"debit":{}},"cancel":{"debit":{}},"revert":{"debit":{}}}`),
			expectNil:  true,
			expectKeys: nil,
		},
		{
			name:       "single valid key",
			raw:        json.RawMessage(`{"direct":{"debit":{"code":"1001"}}}`),
			expectNil:  true,
			expectKeys: nil,
		},
		{
			name:       "valid key with null value",
			raw:        json.RawMessage(`{"hold":null}`),
			expectNil:  true,
			expectKeys: nil,
		},
		{
			name:       "unknown key with value",
			raw:        json.RawMessage(`{"foobar":{"debit":{"code":"1001"}}}`),
			expectNil:  false,
			expectKeys: []string{"foobar"},
		},
		{
			name:       "unknown key with null value",
			raw:        json.RawMessage(`{"foobar":null}`),
			expectNil:  false,
			expectKeys: []string{"foobar"},
		},
		{
			name:       "mix of valid and unknown keys",
			raw:        json.RawMessage(`{"direct":{"debit":{}},"foobar":{"debit":{}},"hold":null}`),
			expectNil:  false,
			expectKeys: []string{"foobar"},
		},
		{
			name:       "multiple unknown keys",
			raw:        json.RawMessage(`{"foo":{"debit":{}},"bar":null,"direct":{"debit":{}}}`),
			expectNil:  false,
			expectKeys: []string{"bar", "foo"},
		},
		{
			name:       "empty object",
			raw:        json.RawMessage(`{}`),
			expectNil:  true,
			expectKeys: nil,
		},
		{
			name:       "invalid JSON returns nil",
			raw:        json.RawMessage(`{invalid}`),
			expectNil:  true,
			expectKeys: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := findUnknownAccountingEntryKeys(tt.raw)

			if tt.expectNil {
				assert.Nil(t, result, "expected nil for valid keys")
			} else {
				require.NotNil(t, result, "expected unknown keys to be detected")

				var actualKeys []string
				for k := range result {
					actualKeys = append(actualKeys, k)
				}

				assert.ElementsMatch(t, tt.expectKeys, actualKeys, "unexpected keys mismatch")
			}
		})
	}
}

func TestOperationRouteHandler_validateDirectionScenarioMatrix(t *testing.T) {
	t.Parallel()

	handler := &OperationRouteHandler{}

	tests := []struct {
		name          string
		operationType string
		entries       *mmodel.AccountingEntries
		expectError   bool
		errorCode     string
	}{
		// Source direction tests
		{
			name:          "source with direct only - valid",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
			},
			expectError: false,
		},
		{
			name:          "source with hold, commit, cancel - valid",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancel Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Cancel Credit"},
				},
			},
			expectError: false,
		},
		{
			name:          "source with revert - invalid",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Revert: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1005", Description: "Revert Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2005", Description: "Revert Credit"},
				},
			},
			expectError: true,
			errorCode:   "0165",
		},

		// Destination direction tests
		{
			name:          "destination with direct only - valid",
			operationType: constant.OperationRouteTypeDestination,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
			},
			expectError: false,
		},
		{
			name:          "destination with direct and commit - valid",
			operationType: constant.OperationRouteTypeDestination,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
			},
			expectError: false,
		},
		{
			name:          "destination with hold - invalid",
			operationType: constant.OperationRouteTypeDestination,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
			},
			expectError: true,
			errorCode:   "0162",
		},
		{
			name:          "destination with cancel - invalid",
			operationType: constant.OperationRouteTypeDestination,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancel Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Cancel Credit"},
				},
			},
			expectError: true,
			errorCode:   "0162",
		},
		{
			name:          "destination with revert - invalid",
			operationType: constant.OperationRouteTypeDestination,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Revert: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1005", Description: "Revert Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2005", Description: "Revert Credit"},
				},
			},
			expectError: true,
			errorCode:   "0162",
		},

		// Bidirectional direction tests
		{
			name:          "bidirectional with all scenarios - valid",
			operationType: constant.OperationRouteTypeBidirectional,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancel Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Cancel Credit"},
				},
				Revert: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1005", Description: "Revert Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2005", Description: "Revert Credit"},
				},
			},
			expectError: false,
		},
		{
			name:          "bidirectional with revert only - valid for direction check",
			operationType: constant.OperationRouteTypeBidirectional,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Revert: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1005", Description: "Revert Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2005", Description: "Revert Credit"},
				},
			},
			expectError: false,
		},

		// Edge case: invalid operation type
		{
			name:          "invalid operation type - no validation error (falls through default)",
			operationType: "invalid_type",
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			err := handler.validateDirectionScenarioMatrix(ctx, tt.operationType, tt.entries, "operation_route")

			if tt.expectError {
				require.Error(t, err, "expected validation error")
				assert.Contains(t, err.Error(), tt.errorCode, "error should contain expected code")
			} else {
				require.NoError(t, err, "expected no validation error")
			}
		})
	}
}

func TestOperationRouteHandler_validateReserveGroupAtomicity(t *testing.T) {
	t.Parallel()

	handler := &OperationRouteHandler{}

	tests := []struct {
		name          string
		operationType string
		entries       *mmodel.AccountingEntries
		expectError   bool
		errorCode     string
	}{
		// Complete reserve group
		{
			name:          "source with complete reserve group - valid",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancel Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Cancel Credit"},
				},
			},
			expectError: false,
		},
		{
			name:          "source with no reserve group - valid",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
			},
			expectError: false,
		},

		// Incomplete reserve group - missing commit
		{
			name:          "source with hold and cancel but missing commit - invalid",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancel Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Cancel Credit"},
				},
			},
			expectError: true,
			errorCode:   "0163",
		},

		// Incomplete reserve group - missing cancel
		{
			name:          "source with hold and commit but missing cancel - invalid",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
			},
			expectError: true,
			errorCode:   "0163",
		},

		// Incomplete reserve group - hold only
		{
			name:          "source with hold only - invalid",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
			},
			expectError: true,
			errorCode:   "0163",
		},

		// Commit/cancel without hold
		{
			name:          "source with commit only (no hold) - invalid",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
			},
			expectError: true,
			errorCode:   "0163",
		},

		// Destination skips reserve group validation
		{
			name:          "destination with commit only - valid (reserve group not applicable)",
			operationType: constant.OperationRouteTypeDestination,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
			},
			expectError: false,
		},

		// Bidirectional with reserve group
		{
			name:          "bidirectional with complete reserve group - valid",
			operationType: constant.OperationRouteTypeBidirectional,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancel Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Cancel Credit"},
				},
			},
			expectError: false,
		},
		{
			name:          "bidirectional with hold only - invalid",
			operationType: constant.OperationRouteTypeBidirectional,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
			},
			expectError: true,
			errorCode:   "0163",
		},

		// Edge case: cancel only without hold
		{
			name:          "source with cancel only (no hold) - invalid",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancel Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Cancel Credit"},
				},
			},
			expectError: true,
			errorCode:   "0163",
		},

		// Edge case: bidirectional with partial reserve group (hold + commit, missing cancel)
		{
			name:          "bidirectional with hold and commit but missing cancel - invalid",
			operationType: constant.OperationRouteTypeBidirectional,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
			},
			expectError: true,
			errorCode:   "0163",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			err := handler.validateReserveGroupAtomicity(ctx, tt.operationType, tt.entries, "operation_route")

			if tt.expectError {
				require.Error(t, err, "expected validation error")
				assert.Contains(t, err.Error(), tt.errorCode, "error should contain expected code")
			} else {
				require.NoError(t, err, "expected no validation error")
			}
		})
	}
}

func TestOperationRouteHandler_validateDirectMandatory(t *testing.T) {
	t.Parallel()

	handler := &OperationRouteHandler{}

	tests := []struct {
		name        string
		entries     *mmodel.AccountingEntries
		expectError bool
		errorCode   string
	}{
		{
			name: "direct only - valid",
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
			},
			expectError: false,
		},
		{
			name: "direct with other scenarios - valid",
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancel Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Cancel Credit"},
				},
			},
			expectError: false,
		},
		{
			name: "hold without direct - invalid",
			entries: &mmodel.AccountingEntries{
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancel Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Cancel Credit"},
				},
			},
			expectError: true,
			errorCode:   "0164",
		},
		{
			name: "revert without direct - invalid",
			entries: &mmodel.AccountingEntries{
				Revert: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1005", Description: "Revert Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2005", Description: "Revert Credit"},
				},
			},
			expectError: true,
			errorCode:   "0164",
		},
		{
			name: "commit without direct - invalid",
			entries: &mmodel.AccountingEntries{
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
			},
			expectError: true,
			errorCode:   "0164",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			err := handler.validateDirectMandatory(ctx, tt.entries, "operation_route")

			if tt.expectError {
				require.Error(t, err, "expected validation error")
				assert.Contains(t, err.Error(), tt.errorCode, "error should contain expected code")
			} else {
				require.NoError(t, err, "expected no validation error")
			}
		})
	}
}

func TestMergeAccountingEntries(t *testing.T) {
	t.Parallel()

	directEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Direct Debit"},
		Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Direct Credit"},
	}
	holdEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
		Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
	}
	commitEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
		Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
	}
	cancelEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancel Debit"},
		Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Cancel Credit"},
	}
	revertEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "1005", Description: "Revert Debit"},
		Credit: &mmodel.AccountingRubric{Code: "2005", Description: "Revert Credit"},
	}

	newDirectEntry := &mmodel.AccountingEntry{
		Debit:  &mmodel.AccountingRubric{Code: "NEW1", Description: "New Direct Debit"},
		Credit: &mmodel.AccountingRubric{Code: "NEW2", Description: "New Direct Credit"},
	}

	tests := []struct {
		name     string
		existing *mmodel.AccountingEntries
		incoming *mmodel.AccountingEntries
		expected *mmodel.AccountingEntries
	}{
		{
			name:     "both nil returns nil",
			existing: nil,
			incoming: nil,
			expected: nil,
		},
		{
			name:     "existing nil returns incoming",
			existing: nil,
			incoming: &mmodel.AccountingEntries{Direct: directEntry},
			expected: &mmodel.AccountingEntries{Direct: directEntry},
		},
		{
			name:     "incoming nil returns existing",
			existing: &mmodel.AccountingEntries{Direct: directEntry},
			incoming: nil,
			expected: &mmodel.AccountingEntries{Direct: directEntry},
		},
		{
			name:     "incoming overwrites existing direct",
			existing: &mmodel.AccountingEntries{Direct: directEntry},
			incoming: &mmodel.AccountingEntries{Direct: newDirectEntry},
			expected: &mmodel.AccountingEntries{Direct: newDirectEntry},
		},
		{
			name: "incoming adds to existing",
			existing: &mmodel.AccountingEntries{
				Direct: directEntry,
			},
			incoming: &mmodel.AccountingEntries{
				Hold:   holdEntry,
				Commit: commitEntry,
				Cancel: cancelEntry,
			},
			expected: &mmodel.AccountingEntries{
				Direct: directEntry,
				Hold:   holdEntry,
				Commit: commitEntry,
				Cancel: cancelEntry,
			},
		},
		{
			name: "existing preserved when incoming is partial",
			existing: &mmodel.AccountingEntries{
				Direct: directEntry,
				Hold:   holdEntry,
				Commit: commitEntry,
				Cancel: cancelEntry,
				Revert: revertEntry,
			},
			incoming: &mmodel.AccountingEntries{
				Direct: newDirectEntry,
			},
			expected: &mmodel.AccountingEntries{
				Direct: newDirectEntry,
				Hold:   holdEntry,
				Commit: commitEntry,
				Cancel: cancelEntry,
				Revert: revertEntry,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := mergeAccountingEntries(tt.existing, tt.incoming)

			if tt.expected == nil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.expected.Direct, result.Direct)
			assert.Equal(t, tt.expected.Hold, result.Hold)
			assert.Equal(t, tt.expected.Commit, result.Commit)
			assert.Equal(t, tt.expected.Cancel, result.Cancel)
			assert.Equal(t, tt.expected.Revert, result.Revert)
		})
	}
}

func TestOperationRouteHandler_validateAccountingRulesMatrix(t *testing.T) {
	t.Parallel()

	handler := &OperationRouteHandler{}

	tests := []struct {
		name          string
		operationType string
		entries       *mmodel.AccountingEntries
		expectError   bool
		errorCode     string
	}{
		{
			name:          "nil entries returns no error",
			operationType: constant.OperationRouteTypeSource,
			entries:       nil,
			expectError:   false,
		},
		{
			name:          "valid source with direct only",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
			},
			expectError: false,
		},
		{
			name:          "valid source with complete reserve group",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancel Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Cancel Credit"},
				},
			},
			expectError: false,
		},
		{
			name:          "source with revert fails direction check",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Revert: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1005", Description: "Revert Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2005", Description: "Revert Credit"},
				},
			},
			expectError: true,
			errorCode:   "0165",
		},
		{
			name:          "source with incomplete reserve group fails",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
			},
			expectError: true,
			errorCode:   "0163",
		},
		{
			name:          "missing direct with other scenarios fails",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancel Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Cancel Credit"},
				},
			},
			expectError: true,
			errorCode:   "0164",
		},
		{
			name:          "valid bidirectional with all scenarios",
			operationType: constant.OperationRouteTypeBidirectional,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
				Commit: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1003", Description: "Commit Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2003", Description: "Commit Credit"},
				},
				Cancel: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1004", Description: "Cancel Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2004", Description: "Cancel Credit"},
				},
				Revert: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1005", Description: "Revert Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2005", Description: "Revert Credit"},
				},
			},
			expectError: false,
		},
		{
			name:          "destination with hold fails direction check",
			operationType: constant.OperationRouteTypeDestination,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Hold: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1002", Description: "Hold Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2002", Description: "Hold Credit"},
				},
			},
			expectError: true,
			errorCode:   "0162",
		},

		// Edge case: empty AccountingEntries struct (all fields nil)
		{
			name:          "empty entries struct with all nil fields - valid",
			operationType: constant.OperationRouteTypeSource,
			entries:       &mmodel.AccountingEntries{},
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			err := handler.validateAccountingRulesMatrix(ctx, tt.operationType, tt.entries)

			if tt.expectError {
				require.Error(t, err, "expected validation error")
				assert.Contains(t, err.Error(), tt.errorCode, "error should contain expected code")
			} else {
				require.NoError(t, err, "expected no validation error")
			}
		})
	}
}
