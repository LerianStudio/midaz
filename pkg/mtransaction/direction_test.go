// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"testing"

	constant "github.com/LerianStudio/lib-commons/v5/commons/constants"
	pkgConstant "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOperateBalances_DirectionAwareArithmetic verifies that the balance
// state machine applies the correct sign to Available based on the
// balance's accounting direction.
//
// For direction=credit (asset-like balances), a DEBIT operation decreases
// Available and a CREDIT increases it. For direction=debit (liability-like
// balances), the arithmetic is INVERTED: a DEBIT increases Available and a
// CREDIT decreases it. Legacy balances with an empty Direction MUST be
// treated as direction=credit for backward compatibility.
func TestOperateBalances_DirectionAwareArithmetic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		direction        string
		operation        string
		transactionType  string
		startAvailable   decimal.Decimal
		amount           decimal.Decimal
		wantNewAvailable decimal.Decimal
	}{
		{
			name:             "direction=credit DEBIT decreases available",
			direction:        pkgConstant.DirectionCredit,
			operation:        constant.DEBIT,
			transactionType:  constant.CREATED,
			startAvailable:   decimal.NewFromInt(1000),
			amount:           decimal.NewFromInt(250),
			wantNewAvailable: decimal.NewFromInt(750),
		},
		{
			name:             "direction=credit CREDIT increases available",
			direction:        pkgConstant.DirectionCredit,
			operation:        constant.CREDIT,
			transactionType:  constant.CREATED,
			startAvailable:   decimal.NewFromInt(1000),
			amount:           decimal.NewFromInt(300),
			wantNewAvailable: decimal.NewFromInt(1300),
		},
		{
			name:             "direction=debit DEBIT increases available",
			direction:        pkgConstant.DirectionDebit,
			operation:        constant.DEBIT,
			transactionType:  constant.CREATED,
			startAvailable:   decimal.NewFromInt(1000),
			amount:           decimal.NewFromInt(400),
			wantNewAvailable: decimal.NewFromInt(1400),
		},
		{
			name:             "direction=debit CREDIT decreases available",
			direction:        pkgConstant.DirectionDebit,
			operation:        constant.CREDIT,
			transactionType:  constant.CREATED,
			startAvailable:   decimal.NewFromInt(1000),
			amount:           decimal.NewFromInt(200),
			wantNewAvailable: decimal.NewFromInt(800),
		},
		{
			name:             "empty direction defaults to credit behavior DEBIT",
			direction:        "",
			operation:        constant.DEBIT,
			transactionType:  constant.CREATED,
			startAvailable:   decimal.NewFromInt(1000),
			amount:           decimal.NewFromInt(150),
			wantNewAvailable: decimal.NewFromInt(850),
		},
		{
			name:             "empty direction defaults to credit behavior CREDIT",
			direction:        "",
			operation:        constant.CREDIT,
			transactionType:  constant.CREATED,
			startAvailable:   decimal.NewFromInt(1000),
			amount:           decimal.NewFromInt(100),
			wantNewAvailable: decimal.NewFromInt(1100),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			balance := Balance{
				Available: tt.startAvailable,
				OnHold:    decimal.NewFromInt(0),
				Direction: tt.direction,
				Version:   1,
			}
			amount := Amount{
				Value:           tt.amount,
				Operation:       tt.operation,
				TransactionType: tt.transactionType,
			}

			result, err := OperateBalances(amount, balance)
			require.NoError(t, err)
			assert.True(t, tt.wantNewAvailable.Equal(result.Available),
				"direction=%s op=%s: want available=%s, got %s",
				tt.direction, tt.operation, tt.wantNewAvailable, result.Available)
		})
	}
}

// TestOperateBalances_DirectionAwareArithmetic_Approved mirrors the
// CREATED-state check against APPROVED commits. The direction inversion
// MUST apply uniformly across transaction states so that commits on a
// debit-direction balance move Available in the opposite direction from
// a credit-direction balance.
func TestOperateBalances_DirectionAwareArithmetic_Approved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		direction        string
		operation        string
		startAvailable   decimal.Decimal
		startOnHold      decimal.Decimal
		amount           decimal.Decimal
		wantNewAvailable decimal.Decimal
	}{
		{
			name:             "direction=credit APPROVED CREDIT increases available",
			direction:        pkgConstant.DirectionCredit,
			operation:        constant.CREDIT,
			startAvailable:   decimal.NewFromInt(500),
			startOnHold:      decimal.NewFromInt(0),
			amount:           decimal.NewFromInt(100),
			wantNewAvailable: decimal.NewFromInt(600),
		},
		{
			name:             "direction=debit APPROVED CREDIT decreases available",
			direction:        pkgConstant.DirectionDebit,
			operation:        constant.CREDIT,
			startAvailable:   decimal.NewFromInt(500),
			startOnHold:      decimal.NewFromInt(0),
			amount:           decimal.NewFromInt(100),
			wantNewAvailable: decimal.NewFromInt(400),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			balance := Balance{
				Available: tt.startAvailable,
				OnHold:    tt.startOnHold,
				Direction: tt.direction,
				Version:   1,
			}
			amount := Amount{
				Value:           tt.amount,
				Operation:       tt.operation,
				TransactionType: constant.APPROVED,
			}

			result, err := OperateBalances(amount, balance)
			require.NoError(t, err)
			assert.True(t, tt.wantNewAvailable.Equal(result.Available),
				"direction=%s op=%s: want available=%s, got %s",
				tt.direction, tt.operation, tt.wantNewAvailable, result.Available)
		})
	}
}

// TestOperateBalances_DirectionDebit_PendingAndCanceled closes the coverage
// gap on applyDebitDirectionBalance for transaction states that involve
// ON_HOLD and RELEASE operations. Per the implementation, these delegate
// to the credit-direction machinery (applyBalanceChange) because the
// hold semantics are identical regardless of accounting direction.
//
// Note on DEBIT and CREDIT: applyDebitDirectionBalance catches these
// operations at the top of the switch and applies the inverted arithmetic
// for direction=debit balances (DEBIT increases Available, CREDIT
// decreases it). This inversion holds uniformly across PENDING and
// CANCELED transaction states because direction=debit balances do not
// participate in the Available↔OnHold flow used by credit-direction
// balances.
func TestOperateBalances_DirectionDebit_PendingAndCanceled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		operation              string
		transactionType        string
		routeValidationEnabled bool
		wantNewAvailable       decimal.Decimal
		wantNewOnHold          decimal.Decimal
	}{
		{
			// ON_HOLD falls through to applyBalanceChange → applyPendingBalance.
			// Route-validated ON_HOLD only increments OnHold; Available is preserved.
			name:                   "PENDING ON_HOLD with route validation increments OnHold only",
			operation:              constant.ONHOLD,
			transactionType:        constant.PENDING,
			routeValidationEnabled: true,
			wantNewAvailable:       decimal.NewFromInt(1000),
			wantNewOnHold:          decimal.NewFromInt(300),
		},
		{
			// Legacy ON_HOLD (no route validation) moves funds from Available to OnHold.
			name:                   "PENDING ON_HOLD legacy moves Available to OnHold",
			operation:              constant.ONHOLD,
			transactionType:        constant.PENDING,
			routeValidationEnabled: false,
			wantNewAvailable:       decimal.NewFromInt(900),
			wantNewOnHold:          decimal.NewFromInt(300),
		},
		{
			// DEBIT is caught by applyDebitDirectionBalance directly: for
			// direction=debit balances, DEBIT INCREASES Available regardless
			// of transaction state. OnHold is preserved.
			name:                   "PENDING DEBIT with route validation increases Available (direction=debit inversion)",
			operation:              constant.DEBIT,
			transactionType:        constant.PENDING,
			routeValidationEnabled: true,
			wantNewAvailable:       decimal.NewFromInt(1100),
			wantNewOnHold:          decimal.NewFromInt(200),
		},
		{
			// RELEASE falls through to applyBalanceChange → applyCanceledBalance.
			// Route-validated RELEASE only decrements OnHold; Available is preserved.
			name:                   "CANCELED RELEASE with route validation decrements OnHold only",
			operation:              constant.RELEASE,
			transactionType:        constant.CANCELED,
			routeValidationEnabled: true,
			wantNewAvailable:       decimal.NewFromInt(1000),
			wantNewOnHold:          decimal.NewFromInt(100),
		},
		{
			// Legacy RELEASE (no route validation) moves funds from OnHold to Available.
			name:                   "CANCELED RELEASE legacy moves OnHold to Available",
			operation:              constant.RELEASE,
			transactionType:        constant.CANCELED,
			routeValidationEnabled: false,
			wantNewAvailable:       decimal.NewFromInt(1100),
			wantNewOnHold:          decimal.NewFromInt(100),
		},
		{
			// CREDIT is caught by applyDebitDirectionBalance directly: for
			// direction=debit balances, CREDIT DECREASES Available regardless
			// of transaction state. OnHold is preserved.
			name:                   "CANCELED CREDIT with route validation decreases Available (direction=debit inversion)",
			operation:              constant.CREDIT,
			transactionType:        constant.CANCELED,
			routeValidationEnabled: true,
			wantNewAvailable:       decimal.NewFromInt(900),
			wantNewOnHold:          decimal.NewFromInt(200),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			balance := Balance{
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.NewFromInt(200),
				Direction: pkgConstant.DirectionDebit,
				Version:   1,
			}
			amount := Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              tt.operation,
				TransactionType:        tt.transactionType,
				RouteValidationEnabled: tt.routeValidationEnabled,
			}

			result, err := OperateBalances(amount, balance)
			require.NoError(t, err)
			assert.True(t, tt.wantNewAvailable.Equal(result.Available),
				"direction=debit op=%s txType=%s routeValidation=%v: want available=%s, got %s",
				tt.operation, tt.transactionType, tt.routeValidationEnabled,
				tt.wantNewAvailable, result.Available)
			assert.True(t, tt.wantNewOnHold.Equal(result.OnHold),
				"direction=debit op=%s txType=%s routeValidation=%v: want onHold=%s, got %s",
				tt.operation, tt.transactionType, tt.routeValidationEnabled,
				tt.wantNewOnHold, result.OnHold)
		})
	}
}

// TestOperateBalances_OverdraftSplitDetection verifies that when a debit
// on a direction=credit balance exceeds available funds AND overdraft is
// enabled, the operation is flagged for splitting. The caller expects a
// signal that a secondary operation targeting the overdraft balance MUST
// be appended to the transaction. Without overdraft enabled, no split is
// signalled and the caller falls back to existing insufficient-funds
// handling at the script layer.
func TestOperateBalances_OverdraftSplitDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		balance         Balance
		amount          Amount
		wantSplitNeeded bool
		wantDeficit     decimal.Decimal
	}{
		{
			name: "credit balance debit within available does not split",
			balance: Balance{
				Available:      decimal.NewFromInt(1000),
				Direction:      pkgConstant.DirectionCredit,
				AllowOverdraft: true,
			},
			amount: Amount{
				Value:           decimal.NewFromInt(500),
				Operation:       constant.DEBIT,
				TransactionType: constant.CREATED,
			},
			wantSplitNeeded: false,
			wantDeficit:     decimal.NewFromInt(0),
		},
		{
			name: "credit balance debit exceeds available with overdraft enabled splits",
			balance: Balance{
				Available:      decimal.NewFromInt(100),
				Direction:      pkgConstant.DirectionCredit,
				AllowOverdraft: true,
			},
			amount: Amount{
				Value:           decimal.NewFromInt(250),
				Operation:       constant.DEBIT,
				TransactionType: constant.CREATED,
			},
			wantSplitNeeded: true,
			wantDeficit:     decimal.NewFromInt(150),
		},
		{
			name: "credit balance debit exceeds available without overdraft does not split",
			balance: Balance{
				Available:      decimal.NewFromInt(100),
				Direction:      pkgConstant.DirectionCredit,
				AllowOverdraft: false,
			},
			amount: Amount{
				Value:           decimal.NewFromInt(250),
				Operation:       constant.DEBIT,
				TransactionType: constant.CREATED,
			},
			wantSplitNeeded: false,
			wantDeficit:     decimal.NewFromInt(0),
		},
		{
			name: "debit balance never splits regardless of overdraft flag",
			balance: Balance{
				Available:      decimal.NewFromInt(0),
				Direction:      pkgConstant.DirectionDebit,
				AllowOverdraft: true,
			},
			amount: Amount{
				Value:           decimal.NewFromInt(500),
				Operation:       constant.DEBIT,
				TransactionType: constant.CREATED,
			},
			wantSplitNeeded: false,
			wantDeficit:     decimal.NewFromInt(0),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			splitNeeded, deficit := DetectOverdraftSplit(tt.amount, tt.balance)

			assert.Equal(t, tt.wantSplitNeeded, splitNeeded,
				"split detection mismatch for %s", tt.name)
			assert.True(t, tt.wantDeficit.Equal(deficit),
				"deficit mismatch: want %s, got %s", tt.wantDeficit, deficit)
		})
	}
}
