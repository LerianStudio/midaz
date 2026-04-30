// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"testing"

	pkgConstant "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	constant "github.com/LerianStudio/lib-commons/v5/commons/constants"
)

// TestCalculateRefundSplit verifies the pure-function credit-split helper
// used when a credit operation targets a direction=credit balance whose
// OverdraftUsed is greater than zero. The function MUST partition the
// credit into a portion that repays overdraft (capped at OverdraftUsed)
// and a remainder that flows to the default balance. Decimal precision
// MUST be preserved across all arithmetic.
//
// Invariants asserted on every case:
//   - creditOnOverdraft + creditOnDefault == creditAmount (sum preservation)
//   - creditOnOverdraft >= 0 && creditOnDefault >= 0      (non-negativity)
//   - creditOnOverdraft <= overdraftUsed                  (never repay more than owed)
func TestCalculateRefundSplit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		creditAmount          decimal.Decimal
		overdraftUsed         decimal.Decimal
		wantCreditOnOverdraft decimal.Decimal
		wantCreditOnDefault   decimal.Decimal
	}{
		{
			// PRD Scenario 2: credit $80 to account with overdraft_used=$50
			// → repay $50 on overdraft, $30 to default.
			name:                  "full repayment with remainder routes overflow to default",
			creditAmount:          decimal.NewFromInt(80),
			overdraftUsed:         decimal.NewFromInt(50),
			wantCreditOnOverdraft: decimal.NewFromInt(50),
			wantCreditOnDefault:   decimal.NewFromInt(30),
		},
		{
			// PRD Scenario 3: credit $60 to account with overdraft_used=$100
			// → repay $60 on overdraft, $0 to default.
			name:                  "partial repayment sends full credit to overdraft",
			creditAmount:          decimal.NewFromInt(60),
			overdraftUsed:         decimal.NewFromInt(100),
			wantCreditOnOverdraft: decimal.NewFromInt(60),
			wantCreditOnDefault:   decimal.NewFromInt(0),
		},
		{
			name:                  "exact repayment fully clears overdraft with nothing to default",
			creditAmount:          decimal.NewFromInt(250),
			overdraftUsed:         decimal.NewFromInt(250),
			wantCreditOnOverdraft: decimal.NewFromInt(250),
			wantCreditOnDefault:   decimal.NewFromInt(0),
		},
		{
			// Normal credit: overdraft_used=$0 → no split, full credit to default.
			name:                  "no overdraft routes full credit to default",
			creditAmount:          decimal.NewFromInt(500),
			overdraftUsed:         decimal.NewFromInt(0),
			wantCreditOnOverdraft: decimal.NewFromInt(0),
			wantCreditOnDefault:   decimal.NewFromInt(500),
		},
		{
			// PRD Scenario 7: credit $1 to account with overdraft_used=$500
			// → repay $1 on overdraft, $0 to default.
			name:                  "small credit against large overdraft fully absorbed",
			creditAmount:          decimal.NewFromInt(1),
			overdraftUsed:         decimal.NewFromInt(500),
			wantCreditOnOverdraft: decimal.NewFromInt(1),
			wantCreditOnDefault:   decimal.NewFromInt(0),
		},
		{
			name:                  "fractional cent credit preserves precision",
			creditAmount:          decimal.RequireFromString("0.01"),
			overdraftUsed:         decimal.RequireFromString("10000.00"),
			wantCreditOnOverdraft: decimal.RequireFromString("0.01"),
			wantCreditOnDefault:   decimal.RequireFromString("0.00"),
		},
		{
			name:                  "large amounts preserve decimal precision on split",
			creditAmount:          decimal.RequireFromString("987654321.987654321"),
			overdraftUsed:         decimal.RequireFromString("123456789.123456789"),
			wantCreditOnOverdraft: decimal.RequireFromString("123456789.123456789"),
			wantCreditOnDefault:   decimal.RequireFromString("864197532.864197532"),
		},
		{
			name:                  "fractional overdraft fully repaid with fractional remainder",
			creditAmount:          decimal.RequireFromString("10.010"),
			overdraftUsed:         decimal.RequireFromString("10.005"),
			wantCreditOnOverdraft: decimal.RequireFromString("10.005"),
			wantCreditOnDefault:   decimal.RequireFromString("0.005"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotOverdraft, gotDefault := CalculateRefundSplit(tt.creditAmount, tt.overdraftUsed)

			assert.True(t, tt.wantCreditOnOverdraft.Equal(gotOverdraft),
				"creditOnOverdraft mismatch: want %s, got %s", tt.wantCreditOnOverdraft, gotOverdraft)
			assert.True(t, tt.wantCreditOnDefault.Equal(gotDefault),
				"creditOnDefault mismatch: want %s, got %s", tt.wantCreditOnDefault, gotDefault)

			// Invariant 1: halves MUST sum back to the original credit.
			sum := gotOverdraft.Add(gotDefault)
			assert.True(t, tt.creditAmount.Equal(sum),
				"split halves must sum to creditAmount: want %s, got %s", tt.creditAmount, sum)

			// Invariant 2: neither half may be negative.
			assert.False(t, gotOverdraft.IsNegative(), "creditOnOverdraft must never be negative")
			assert.False(t, gotDefault.IsNegative(), "creditOnDefault must never be negative")

			// Invariant 3: repayment MUST never exceed the outstanding overdraft.
			assert.False(t, gotOverdraft.GreaterThan(tt.overdraftUsed),
				"creditOnOverdraft (%s) must not exceed overdraftUsed (%s)",
				gotOverdraft, tt.overdraftUsed)
		})
	}
}

// TestDetectRefundSplit verifies the detection predicate that reports
// whether an incoming credit on a direction=credit balance should trigger
// an overdraft repayment split. The predicate MUST signal a split only
// when all three conditions hold: direction=credit, CREDIT operation, and
// OverdraftUsed > 0. For every other case no split is signalled and the
// returned repay amount is zero.
//
// The cleared flag MUST be true when the repayment fully extinguishes the
// outstanding overdraft (overdraftUsed - repay == 0) and false otherwise.
func TestDetectRefundSplit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		balance         Balance
		amount          Amount
		wantSplitNeeded bool
		wantRepay       decimal.Decimal
		wantCleared     bool
	}{
		{
			// PRD Scenario 2: full repayment triggers cleared=true.
			name: "credit direction with CREDIT op fully repays and clears overdraft",
			balance: Balance{
				Available:     decimal.NewFromInt(0),
				Direction:     pkgConstant.DirectionCredit,
				OverdraftUsed: decimal.NewFromInt(50),
			},
			amount: Amount{
				Value:     decimal.NewFromInt(80),
				Operation: constant.CREDIT,
			},
			wantSplitNeeded: true,
			wantRepay:       decimal.NewFromInt(50),
			wantCleared:     true,
		},
		{
			// PRD Scenario 3: partial repayment leaves cleared=false.
			name: "credit direction with CREDIT op partially repays without clearing",
			balance: Balance{
				Available:     decimal.NewFromInt(0),
				Direction:     pkgConstant.DirectionCredit,
				OverdraftUsed: decimal.NewFromInt(100),
			},
			amount: Amount{
				Value:     decimal.NewFromInt(60),
				Operation: constant.CREDIT,
			},
			wantSplitNeeded: true,
			wantRepay:       decimal.NewFromInt(60),
			wantCleared:     false,
		},
		{
			// PRD Scenario 7: small credit against large overdraft.
			name: "tiny credit against large overdraft yields repay=credit and not cleared",
			balance: Balance{
				Available:     decimal.NewFromInt(0),
				Direction:     pkgConstant.DirectionCredit,
				OverdraftUsed: decimal.NewFromInt(500),
			},
			amount: Amount{
				Value:     decimal.NewFromInt(1),
				Operation: constant.CREDIT,
			},
			wantSplitNeeded: true,
			wantRepay:       decimal.NewFromInt(1),
			wantCleared:     false,
		},
		{
			// Exact repayment edge case: cleared=true with repay == overdraftUsed.
			name: "credit exactly equal to overdraft clears the balance",
			balance: Balance{
				Available:     decimal.NewFromInt(0),
				Direction:     pkgConstant.DirectionCredit,
				OverdraftUsed: decimal.NewFromInt(250),
			},
			amount: Amount{
				Value:     decimal.NewFromInt(250),
				Operation: constant.CREDIT,
			},
			wantSplitNeeded: true,
			wantRepay:       decimal.NewFromInt(250),
			wantCleared:     true,
		},
		{
			// Normal credit: overdraft_used=0 → no split.
			name: "credit direction with CREDIT op and no overdraft used signals no split",
			balance: Balance{
				Available:     decimal.NewFromInt(500),
				Direction:     pkgConstant.DirectionCredit,
				OverdraftUsed: decimal.NewFromInt(0),
			},
			amount: Amount{
				Value:     decimal.NewFromInt(100),
				Operation: constant.CREDIT,
			},
			wantSplitNeeded: false,
			wantRepay:       decimal.Zero,
			wantCleared:     false,
		},
		{
			// DEBIT operation on a direction=credit balance must NOT trigger
			// a refund split — that path belongs to the debit-side overdraft
			// flow, not the credit-side refund flow.
			name: "credit direction with DEBIT op never triggers refund split",
			balance: Balance{
				Available:     decimal.NewFromInt(0),
				Direction:     pkgConstant.DirectionCredit,
				OverdraftUsed: decimal.NewFromInt(100),
			},
			amount: Amount{
				Value:     decimal.NewFromInt(40),
				Operation: constant.DEBIT,
			},
			wantSplitNeeded: false,
			wantRepay:       decimal.Zero,
			wantCleared:     false,
		},
		{
			// direction=debit balance represents the overdraft ledger itself;
			// credits flowing to it must not be re-routed.
			name: "debit direction balance never triggers refund split",
			balance: Balance{
				Available:     decimal.NewFromInt(0),
				Direction:     pkgConstant.DirectionDebit,
				OverdraftUsed: decimal.NewFromInt(200),
			},
			amount: Amount{
				Value:     decimal.NewFromInt(75),
				Operation: constant.CREDIT,
			},
			wantSplitNeeded: false,
			wantRepay:       decimal.Zero,
			wantCleared:     false,
		},
		{
			// Legacy balances with empty Direction are treated as credit by
			// OperateBalances but MUST NOT opt-in to the refund split flow
			// since Direction is an explicit signal. Only direction=="credit"
			// (the exact constant) enables the flow.
			name: "empty direction balance never triggers refund split",
			balance: Balance{
				Available:     decimal.NewFromInt(0),
				Direction:     "",
				OverdraftUsed: decimal.NewFromInt(100),
			},
			amount: Amount{
				Value:     decimal.NewFromInt(40),
				Operation: constant.CREDIT,
			},
			wantSplitNeeded: false,
			wantRepay:       decimal.Zero,
			wantCleared:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotSplitNeeded, gotRepay, gotCleared := DetectRefundSplit(tt.amount, tt.balance)

			assert.Equal(t, tt.wantSplitNeeded, gotSplitNeeded,
				"splitNeeded mismatch: want %v, got %v", tt.wantSplitNeeded, gotSplitNeeded)

			assert.True(t, tt.wantRepay.Equal(gotRepay),
				"repay mismatch: want %s, got %s", tt.wantRepay, gotRepay)

			assert.Equal(t, tt.wantCleared, gotCleared,
				"cleared mismatch: want %v, got %v", tt.wantCleared, gotCleared)

			// Invariant: when no split is needed the repay amount MUST be
			// exactly zero and the cleared flag MUST be false.
			if !gotSplitNeeded {
				assert.True(t, gotRepay.Equal(decimal.Zero),
					"repay must be zero when split is not signalled, got %s", gotRepay)
				assert.False(t, gotCleared,
					"cleared must be false when split is not signalled")
			}

			// Invariant: repay must never exceed the outstanding overdraft
			// nor the incoming credit amount.
			if gotSplitNeeded {
				assert.False(t, gotRepay.GreaterThan(tt.balance.OverdraftUsed),
					"repay (%s) must not exceed overdraftUsed (%s)",
					gotRepay, tt.balance.OverdraftUsed)
				assert.False(t, gotRepay.GreaterThan(tt.amount.Value),
					"repay (%s) must not exceed credit amount (%s)",
					gotRepay, tt.amount.Value)
			}
		})
	}
}
