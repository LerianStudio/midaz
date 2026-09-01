// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// TestCalculateOverdraftSplit verifies the pure-function debit-split helper
// used when a debit operation targets a direction=credit balance whose
// available funds are insufficient. The function MUST partition the debit
// into a portion that consumes available funds (capped at available) and a
// deficit that must flow to the overdraft balance. Decimal precision MUST
// be preserved across all arithmetic.
func TestCalculateOverdraftSplit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		available            decimal.Decimal
		debitAmount          decimal.Decimal
		wantDebitOnDefault   decimal.Decimal
		wantDebitOnOverdraft decimal.Decimal
	}{
		{
			name:                 "debit within available produces no deficit",
			available:            decimal.NewFromInt(500),
			debitAmount:          decimal.NewFromInt(200),
			wantDebitOnDefault:   decimal.NewFromInt(200),
			wantDebitOnOverdraft: decimal.NewFromInt(0),
		},
		{
			name:                 "debit exceeds available splits into deficit",
			available:            decimal.NewFromInt(100),
			debitAmount:          decimal.NewFromInt(250),
			wantDebitOnDefault:   decimal.NewFromInt(100),
			wantDebitOnOverdraft: decimal.NewFromInt(150),
		},
		{
			name:                 "zero available routes full amount to overdraft",
			available:            decimal.NewFromInt(0),
			debitAmount:          decimal.NewFromInt(300),
			wantDebitOnDefault:   decimal.NewFromInt(0),
			wantDebitOnOverdraft: decimal.NewFromInt(300),
		},
		{
			name:                 "debit exactly equals available produces no deficit",
			available:            decimal.NewFromInt(750),
			debitAmount:          decimal.NewFromInt(750),
			wantDebitOnDefault:   decimal.NewFromInt(750),
			wantDebitOnOverdraft: decimal.NewFromInt(0),
		},
		{
			name:                 "large amounts preserve decimal precision",
			available:            decimal.RequireFromString("123456789.123456789"),
			debitAmount:          decimal.RequireFromString("987654321.987654321"),
			wantDebitOnDefault:   decimal.RequireFromString("123456789.123456789"),
			wantDebitOnOverdraft: decimal.RequireFromString("864197532.864197532"),
		},
		{
			name:                 "fractional cent deficit preserves precision",
			available:            decimal.RequireFromString("10.005"),
			debitAmount:          decimal.RequireFromString("10.010"),
			wantDebitOnDefault:   decimal.RequireFromString("10.005"),
			wantDebitOnOverdraft: decimal.RequireFromString("0.005"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotDefault, gotOverdraft := CalculateOverdraftSplit(tt.available, tt.debitAmount)

			assert.True(t, tt.wantDebitOnDefault.Equal(gotDefault),
				"debitOnDefault mismatch: want %s, got %s", tt.wantDebitOnDefault, gotDefault)
			assert.True(t, tt.wantDebitOnOverdraft.Equal(gotOverdraft),
				"debitOnOverdraft mismatch: want %s, got %s", tt.wantDebitOnOverdraft, gotOverdraft)

			// Invariant: the two halves MUST sum back to the original debit.
			sum := gotDefault.Add(gotOverdraft)
			assert.True(t, tt.debitAmount.Equal(sum),
				"split halves must sum to debitAmount: want %s, got %s", tt.debitAmount, sum)

			// Invariant: neither half may be negative.
			assert.False(t, gotDefault.IsNegative(), "debitOnDefault must never be negative")
			assert.False(t, gotOverdraft.IsNegative(), "debitOnOverdraft must never be negative")
		})
	}
}
