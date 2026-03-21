// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"testing"

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
