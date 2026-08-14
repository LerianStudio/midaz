// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFindUnknownAccountingEntryKeys_BlockUnblock asserts that block and unblock
// are recognized as valid accountingEntries keys (Epic 3.1 write-path fix) and
// that genuinely unknown keys remain rejected so the guard is not loosened.
func TestFindUnknownAccountingEntryKeys_BlockUnblock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        json.RawMessage
		expectNil  bool
		expectKeys []string
	}{
		{
			name:      "block key is accepted",
			raw:       json.RawMessage(`{"block":{"debit":{"code":"1001","description":"Block Debit"}}}`),
			expectNil: true,
		},
		{
			name:      "unblock key is accepted",
			raw:       json.RawMessage(`{"unblock":{"credit":{"code":"2001","description":"Unblock Credit"}}}`),
			expectNil: true,
		},
		{
			name:      "block and unblock alongside direct are accepted",
			raw:       json.RawMessage(`{"direct":{"debit":{"code":"1000","description":"Direct"}},"block":{"debit":{"code":"1001","description":"Block"}},"unblock":{"credit":{"code":"2001","description":"Unblock"}}}`),
			expectNil: true,
		},
		{
			name:       "unknown key is still rejected alongside block",
			raw:        json.RawMessage(`{"block":{"debit":{"code":"1001"}},"foobar":{"debit":{}}}`),
			expectNil:  false,
			expectKeys: []string{"foobar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := findUnknownAccountingEntryKeys(tt.raw)

			if tt.expectNil {
				assert.Nil(t, result, "expected block/unblock to be valid keys")
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

// TestGetFieldRequirements_BlockUnblock asserts that block/unblock mirror the
// direct scenario field requirements per operationType:
//   - source:        debit-only required
//   - destination:   credit-only required
//   - bidirectional: both required
func TestGetFieldRequirements_BlockUnblock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		operationType string
		scenario      string
		expectDebit   bool
		expectCredit  bool
	}{
		{
			name:          "source/block requires only debit",
			operationType: constant.OperationRouteTypeSource,
			scenario:      constant.ActionBlock,
			expectDebit:   true,
			expectCredit:  false,
		},
		{
			name:          "source/unblock requires only debit",
			operationType: constant.OperationRouteTypeSource,
			scenario:      constant.ActionUnblock,
			expectDebit:   true,
			expectCredit:  false,
		},
		{
			name:          "destination/block requires only credit",
			operationType: constant.OperationRouteTypeDestination,
			scenario:      constant.ActionBlock,
			expectDebit:   false,
			expectCredit:  true,
		},
		{
			name:          "destination/unblock requires only credit",
			operationType: constant.OperationRouteTypeDestination,
			scenario:      constant.ActionUnblock,
			expectDebit:   false,
			expectCredit:  true,
		},
		{
			name:          "bidirectional/block requires both",
			operationType: constant.OperationRouteTypeBidirectional,
			scenario:      constant.ActionBlock,
			expectDebit:   true,
			expectCredit:  true,
		},
		{
			name:          "bidirectional/unblock requires both",
			operationType: constant.OperationRouteTypeBidirectional,
			scenario:      constant.ActionUnblock,
			expectDebit:   true,
			expectCredit:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := getFieldRequirements(tt.operationType, tt.scenario)

			assert.Equal(t, tt.expectDebit, req.debitRequired, "debit requirement mismatch")
			assert.Equal(t, tt.expectCredit, req.creditRequired, "credit requirement mismatch")
		})
	}
}

// TestOperationRouteHandler_validateAccountingRulesMatrix_BlockUnblock asserts
// the full matrix accepts block/unblock entries for source, destination, and
// bidirectional operation types, mirroring the direct scenario.
func TestOperationRouteHandler_validateAccountingRulesMatrix_BlockUnblock(t *testing.T) {
	t.Parallel()

	handler := &OperationRouteHandler{}

	debit := func(code string) *mmodel.AccountingRubric {
		return &mmodel.AccountingRubric{Code: code, Description: "Debit " + code}
	}
	credit := func(code string) *mmodel.AccountingRubric {
		return &mmodel.AccountingRubric{Code: code, Description: "Credit " + code}
	}

	tests := []struct {
		name          string
		operationType string
		entries       *mmodel.AccountingEntries
		expectError   bool
	}{
		{
			name:          "source accepts block (debit only)",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{Debit: debit("1000")},
				Block:  &mmodel.AccountingEntry{Debit: debit("1001")},
			},
			expectError: false,
		},
		{
			name:          "source accepts unblock (debit only)",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct:  &mmodel.AccountingEntry{Debit: debit("1000")},
				Unblock: &mmodel.AccountingEntry{Debit: debit("1002")},
			},
			expectError: false,
		},
		{
			name:          "destination accepts block and unblock (credit only)",
			operationType: constant.OperationRouteTypeDestination,
			entries: &mmodel.AccountingEntries{
				Direct:  &mmodel.AccountingEntry{Credit: credit("2000")},
				Block:   &mmodel.AccountingEntry{Credit: credit("2001")},
				Unblock: &mmodel.AccountingEntry{Credit: credit("2002")},
			},
			expectError: false,
		},
		{
			name:          "bidirectional accepts block and unblock (both)",
			operationType: constant.OperationRouteTypeBidirectional,
			entries: &mmodel.AccountingEntries{
				Direct:  &mmodel.AccountingEntry{Debit: debit("1000"), Credit: credit("2000")},
				Block:   &mmodel.AccountingEntry{Debit: debit("1001"), Credit: credit("2001")},
				Unblock: &mmodel.AccountingEntry{Debit: debit("1002"), Credit: credit("2002")},
			},
			expectError: false,
		},
		{
			name:          "block-only payload (no direct) is accepted like a standalone direct movement",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Block: &mmodel.AccountingEntry{Debit: debit("1001")},
			},
			expectError: false,
		},
		{
			name:          "source block missing required debit is rejected",
			operationType: constant.OperationRouteTypeSource,
			entries: &mmodel.AccountingEntries{
				Direct: &mmodel.AccountingEntry{Debit: debit("1000")},
				Block:  &mmodel.AccountingEntry{Credit: credit("2001")},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			err := handler.validateAccountingRulesMatrix(ctx, tt.operationType, tt.entries)

			if tt.expectError {
				require.Error(t, err, "expected validation error")
			} else {
				require.NoError(t, err, "expected block/unblock to be accepted")
			}
		})
	}
}
