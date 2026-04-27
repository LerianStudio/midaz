// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"testing"

	pkgConstant "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestValidateOverdraftLimit verifies the pure-function limit check that
// rejects a debit whose resulting overdraft usage would exceed the
// configured limit. When the limit is disabled the function MUST treat
// the balance as having unlimited overdraft.
func TestValidateOverdraftLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		currentOverdraftUsed decimal.Decimal
		deficit              decimal.Decimal
		overdraftLimit       decimal.Decimal
		limitEnabled         bool
		wantErr              bool
		wantErrCode          string
	}{
		{
			name:                 "limit disabled allows any deficit",
			currentOverdraftUsed: decimal.NewFromInt(500),
			deficit:              decimal.NewFromInt(1000000),
			overdraftLimit:       decimal.NewFromInt(100),
			limitEnabled:         false,
			wantErr:              false,
		},
		{
			name:                 "deficit within remaining limit allowed",
			currentOverdraftUsed: decimal.NewFromInt(200),
			deficit:              decimal.NewFromInt(300),
			overdraftLimit:       decimal.NewFromInt(1000),
			limitEnabled:         true,
			wantErr:              false,
		},
		{
			name:                 "deficit plus usage exactly at limit allowed",
			currentOverdraftUsed: decimal.NewFromInt(400),
			deficit:              decimal.NewFromInt(600),
			overdraftLimit:       decimal.NewFromInt(1000),
			limitEnabled:         true,
			wantErr:              false,
		},
		{
			name:                 "deficit exceeds limit is rejected with 0167",
			currentOverdraftUsed: decimal.NewFromInt(800),
			deficit:              decimal.NewFromInt(500),
			overdraftLimit:       decimal.NewFromInt(1000),
			limitEnabled:         true,
			wantErr:              true,
			wantErrCode:          "0167",
		},
		{
			name:                 "first use of overdraft within limit allowed",
			currentOverdraftUsed: decimal.NewFromInt(0),
			deficit:              decimal.NewFromInt(500),
			overdraftLimit:       decimal.NewFromInt(500),
			limitEnabled:         true,
			wantErr:              false,
		},
		{
			name:                 "first use of overdraft exceeds limit rejected",
			currentOverdraftUsed: decimal.NewFromInt(0),
			deficit:              decimal.NewFromInt(501),
			overdraftLimit:       decimal.NewFromInt(500),
			limitEnabled:         true,
			wantErr:              true,
			wantErrCode:          "0167",
		},
		{
			name:                 "cumulative usage precision preserved",
			currentOverdraftUsed: decimal.RequireFromString("99.995"),
			deficit:              decimal.RequireFromString("0.010"),
			overdraftLimit:       decimal.RequireFromString("100.000"),
			limitEnabled:         true,
			wantErr:              true,
			wantErrCode:          "0167",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateOverdraftLimit(tt.currentOverdraftUsed, tt.deficit, tt.overdraftLimit, tt.limitEnabled)

			if tt.wantErr {
				require.Error(t, err, "expected limit violation to return error")
				if tt.wantErrCode != "" {
					assert.Equal(t, tt.wantErrCode, codeFromError(err),
						"expected error code %s, got %s from %v", tt.wantErrCode, codeFromError(err), err)
				}
				return
			}
			require.NoError(t, err, "expected success but got error: %v", err)
		})
	}
}

// TestValidateOverdraftLimit_SentinelMatches ensures the returned error
// wraps the canonical overdraft-limit sentinel so downstream callers can
// branch on the error with errors.Is.
func TestValidateOverdraftLimit_SentinelMatches(t *testing.T) {
	t.Parallel()

	err := ValidateOverdraftLimit(
		decimal.NewFromInt(0),
		decimal.NewFromInt(10),
		decimal.NewFromInt(5),
		true,
	)

	require.Error(t, err)
	assert.Equal(t, pkgConstant.ErrOverdraftLimitExceeded.Error(), codeFromError(err),
		"returned error must carry the 0167 sentinel code")
}
