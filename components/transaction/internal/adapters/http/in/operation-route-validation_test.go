// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
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
