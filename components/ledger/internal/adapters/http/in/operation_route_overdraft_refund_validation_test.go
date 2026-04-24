// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// T-008: Accounting Entries Extension — Overdraft & Refund validation.
//
// These tests capture the expected HTTP-handler validation behavior for
// the new "overdraft" and "refund" accounting entries:
//
//  1. Both fields, when present, MUST include BOTH debit AND credit
//     rubrics (same requirement as the bidirectional field pattern —
//     source/hold, source/cancel, or any bidirectional scenario).
//  2. Missing debit or credit yields ErrAccountingEntryFieldRequired (0166).
//  3. Absence of the fields continues to pass validation (backward compat).
//  4. Structural validation (empty code/description) still applies.
//  5. The allowed-keys gate accepts "overdraft" and "refund" without
//     triggering "Unexpected Fields" (error 0053).
//
// These tests MUST FAIL until T-008 GREEN introduces:
//   - AccountingEntries.Overdraft and AccountingEntries.Refund fields
//   - Handler validation for the new scenarios (debit+credit required)
//   - "overdraft" and "refund" entries in validAccountingEntryKeys
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

// TestValidateEntryFieldRequirements_Refund_RequiresDebitAndCredit verifies
// that refund entries require BOTH debit and credit rubrics.
func TestValidateEntryFieldRequirements_Refund_RequiresDebitAndCredit(t *testing.T) {
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
			name:          "refund with both debit and credit — valid",
			operationType: constant.OperationRouteTypeBidirectional,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Refund: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1007", Description: "Refund Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2007", Description: "Refund Credit"},
				},
			},
			expectError: false,
		},
		{
			name:          "refund missing debit — invalid",
			operationType: constant.OperationRouteTypeBidirectional,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Refund: &mmodel.AccountingEntry{
					Credit: &mmodel.AccountingRubric{Code: "2007", Description: "Refund Credit"},
				},
			},
			expectError: true,
			errorCode:   constant.ErrAccountingEntryFieldRequired.Error(),
			errorField:  "debit",
		},
		{
			name:          "refund missing credit — invalid",
			operationType: constant.OperationRouteTypeBidirectional,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Credit"},
				},
				Refund: &mmodel.AccountingEntry{
					Debit: &mmodel.AccountingRubric{Code: "1007", Description: "Refund Debit"},
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

// TestValidateAccountingEntries_OverdraftRefund_StructuralValidation
// verifies that the existing structure validator (debit/credit present,
// non-empty code/description) also runs on the new fields.
func TestValidateAccountingEntries_OverdraftRefund_StructuralValidation(t *testing.T) {
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
			name: "refund with neither debit nor credit — invalid structure",
			entries: &mmodel.AccountingEntries{
				Refund: &mmodel.AccountingEntry{},
			},
			expectError: true,
			errorField:  "accountingEntries.refund",
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
			name: "refund with empty credit description — invalid structure",
			entries: &mmodel.AccountingEntries{
				Refund: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1007", Description: "Refund Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2007", Description: ""},
				},
			},
			expectError: true,
			errorField:  "accountingEntries.refund.credit.description",
		},
		{
			name: "overdraft and refund fully populated — valid structure",
			entries: &mmodel.AccountingEntries{
				Overdraft: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1006", Description: "Overdraft Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2006", Description: "Overdraft Credit"},
				},
				Refund: &mmodel.AccountingEntry{
					Debit:  &mmodel.AccountingRubric{Code: "1007", Description: "Refund Debit"},
					Credit: &mmodel.AccountingRubric{Code: "2007", Description: "Refund Credit"},
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

// TestValidateAccountingEntries_BackwardCompatible_NoOverdraftRefund
// ensures operation routes without the new fields continue to validate
// as before — no hidden required fields, no new errors.
func TestValidateAccountingEntries_BackwardCompatible_NoOverdraftRefund(t *testing.T) {
	t.Parallel()

	handler := &OperationRouteHandler{}
	ctx := context.Background()

	// A minimal legacy payload (T-007 and earlier): only Direct is set.
	entries := &mmodel.AccountingEntries{
		Direct: &mmodel.AccountingEntry{
			Debit:  &mmodel.AccountingRubric{Code: "1001", Description: "Cash"},
			Credit: &mmodel.AccountingRubric{Code: "2001", Description: "Revenue"},
		},
	}

	// Structure validation passes.
	require.NoError(t, handler.validateAccountingEntries(ctx, entries),
		"legacy entries must still pass structural validation")

	// Full matrix validation passes on every operation type.
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
				"legacy entries without overdraft/refund must still pass matrix validation for %s", opType)
		})
	}
}

// TestFindUnknownAccountingEntryKeys_AllowsOverdraftAndRefund verifies
// that the allowed-keys gate recognizes "overdraft" and "refund" as
// valid top-level keys — without this, the handler would reject any
// payload containing these new fields as "Unexpected Fields".
func TestFindUnknownAccountingEntryKeys_AllowsOverdraftAndRefund(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"direct":{"debit":{"code":"1001","description":"Cash"}},
		"overdraft":{
			"debit":{"code":"1006","description":"Overdraft Debit"},
			"credit":{"code":"2006","description":"Overdraft Credit"}
		},
		"refund":{
			"debit":{"code":"1007","description":"Refund Debit"},
			"credit":{"code":"2007","description":"Refund Credit"}
		}
	}`)

	unknowns := findUnknownAccountingEntryKeys(raw)

	assert.Nil(t, unknowns,
		`"overdraft" and "refund" must be valid top-level keys; got unknowns = %v`, unknowns)
}
