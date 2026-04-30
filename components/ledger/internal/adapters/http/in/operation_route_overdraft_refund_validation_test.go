// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// T-008 + T-014: Accounting Entries Extension — Overdraft validation.
//
// After T-014, the "refund" accounting entry was collapsed into "overdraft".
// These tests capture the expected HTTP-handler validation behavior for the
// "overdraft" accounting entry:
//
//  1. The field, when present, MUST include BOTH debit AND credit rubrics.
//  2. Missing debit or credit yields ErrAccountingEntryFieldRequired (0166).
//  3. Absence of the field continues to pass validation (backward compat).
//  4. Structural validation (empty code/description) still applies.
//  5. The allowed-keys gate accepts "overdraft" without triggering
//     "Unexpected Fields" (error 0053).
//  6. After T-014, the "refund" key is no longer recognized and produces
//     an "Unexpected Fields" error (0053).
package in

import (
	"context"
	"encoding/json"
	"testing"

	pkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateEntryFieldRequirements_Overdraft_RequiresDebitAndCredit
// verifies that overdraft entries require BOTH debit and credit rubrics.
func TestValidateEntryFieldRequirements_Overdraft_RequiresDebitAndCredit(t *testing.T) {
	t.Parallel()

	handler := &OperationRouteHandler{}

	tests := []struct {
		name          string
		operationType string
		entries       *mmodel.AccountingEntries
		expectError   bool
		errorCode     string
		errorField    string
	}{
		{
			name:          "overdraft with both debit and credit — valid",
			operationType: constant.OperationRouteTypeBidirectional,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Overdraft: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
				},
			},
			expectError: false,
		},
		{
			name:          "overdraft missing debit — invalid",
			operationType: constant.OperationRouteTypeBidirectional,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Overdraft: &mmodel.AccountingEntry{
					Credit: &mmodel.AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
				},
			},
			expectError: true,
			errorCode:   constant.ErrAccountingEntryFieldRequired.Error(),
			errorField:  "debit",
		},
		{
			name:          "overdraft missing credit — invalid",
			operationType: constant.OperationRouteTypeBidirectional,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Overdraft: &mmodel.AccountingEntry{
					Debit: &mmodel.AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
				},
			},
			expectError: true,
			errorCode:   constant.ErrAccountingEntryFieldRequired.Error(),
			errorField:  "credit",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			err := handler.validateEntryFieldRequirements(ctx, tt.operationType, tt.entries, "OperationRoute")

			if tt.expectError {
				require.Error(t, err, "expected validation error")

				var upErr pkg.UnprocessableOperationError
				require.ErrorAs(t, err, &upErr, "expected UnprocessableOperationError")
				assert.Equal(t, tt.errorCode, upErr.Code, "error code mismatch")

				if tt.errorField != "" {
					assert.Contains(t, upErr.Message, tt.errorField,
						"error message should reference the missing field")
				}
			} else {
				require.NoError(t, err, "expected no validation error")
			}
		})
	}
}

// TestValidateAccountingEntries_Overdraft_StructuralValidation
// verifies that the existing structure validator (debit/credit present,
// non-empty code/description) also runs on the overdraft field.
func TestValidateAccountingEntries_Overdraft_StructuralValidation(t *testing.T) {
	t.Parallel()

	handler := &OperationRouteHandler{}
	ctx := context.Background()

	tests := []struct {
		name        string
		entries     *mmodel.AccountingEntries
		expectError bool
		errorField  string
	}{
		{
			name: "overdraft with neither debit nor credit — invalid structure",
			entries: &mmodel.AccountingEntries{
				Overdraft: &mmodel.AccountingEntry{},
			},
			expectError: true,
			errorField:  "accountingEntries.overdraft",
		},
		{
			name: "overdraft with empty debit code — invalid structure",
			entries: &mmodel.AccountingEntries{
				Overdraft: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "", Description: "Overdraft Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
				},
			},
			expectError: true,
			errorField:  "accountingEntries.overdraft.debit.code",
		},
		{
			name: "overdraft fully populated — valid structure",
			entries: &mmodel.AccountingEntries{
				Overdraft: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := handler.validateAccountingEntries(ctx, tt.entries)

			if tt.expectError {
				require.Error(t, err, "expected structural validation error")

				var upErr pkg.UnprocessableOperationError
				if assert.ErrorAs(t, err, &upErr) && tt.errorField != "" {
					assert.Contains(t, upErr.Message, tt.errorField,
						"error message should reference the offending field path")
				}
			} else {
				require.NoError(t, err, "expected no validation error")
			}
		})
	}
}

// TestValidateAccountingEntries_BackwardCompatible_NoOverdraft
// ensures operation routes without the overdraft field continue to
// validate as before — no hidden required fields, no new errors.
func TestValidateAccountingEntries_BackwardCompatible_NoOverdraft(t *testing.T) {
	t.Parallel()

	handler := &OperationRouteHandler{}
	ctx := context.Background()

	entries := &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
			Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
		},
	}

	require.NoError(t, handler.validateAccountingEntries(ctx, entries),
		"legacy entries must still pass structural validation")

	for _, opType := range []string{
		constant.OperationRouteTypeSource,
		constant.OperationRouteTypeDestination,
		constant.OperationRouteTypeBidirectional,
	} {
		opType := opType
		t.Run("matrix validation — "+opType, func(t *testing.T) {
			t.Parallel()

			err := handler.validateAccountingRulesMatrix(ctx, opType, entries)
			require.NoError(t, err,
				"legacy entries without overdraft must still pass matrix validation for %s", opType)
		})
	}
}

// TestFindUnknownAccountingEntryKeys_AllowsOverdraft verifies that the
// allowed-keys gate recognizes "overdraft" as a valid top-level key.
func TestFindUnknownAccountingEntryKeys_AllowsOverdraft(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"direct":{"debit":{"code":"1001","description":"Cash"}},
		"overdraft":{
			"debit":{"code":"1006","description":"Overdraft Debit"},
			"credit":{"code":"2006","description":"Overdraft Credit"}
		}
	}`)

	unknowns := findUnknownAccountingEntryKeys(raw)

	assert.Nil(t, unknowns,
		`"overdraft" must be a valid top-level key; got unknowns = %v`, unknowns)
}

// TestFindUnknownAccountingEntryKeys_RejectsRefund verifies that after T-014,
// the "refund" key is no longer recognized and is flagged as unknown.
func TestFindUnknownAccountingEntryKeys_RejectsRefund(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"direct":{"debit":{"code":"1001","description":"Cash"}},
		"refund":{
			"debit":{"code":"1007","description":"Refund Debit"},
			"credit":{"code":"2007","description":"Refund Credit"}
		}
	}`)

	unknowns := findUnknownAccountingEntryKeys(raw)

	require.NotNil(t, unknowns, `"refund" must be flagged as unknown after T-014`)
	_, hasRefund := unknowns["refund"]
	assert.True(t, hasRefund, `unknowns map must contain "refund"`)
}
