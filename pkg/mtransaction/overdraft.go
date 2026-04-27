// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"github.com/LerianStudio/midaz/v3/pkg"
	pkgConstant "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"

	constant "github.com/LerianStudio/lib-commons/v4/commons/constants"
)

// CalculateOverdraftSplit partitions a debit amount targeted at a
// direction=credit balance whose Available funds may be insufficient to cover
// the full debit. The returned pair satisfies:
//
//	debitOnDefault   = min(available, debitAmount)
//	debitOnOverdraft = debitAmount - debitOnDefault
//
// Invariants (enforced by tests):
//   - debitOnDefault + debitOnOverdraft == debitAmount
//   - neither half is negative
//
// Decimal precision is preserved because all arithmetic is performed on
// shopspring/decimal values with no intermediate float conversion.
func CalculateOverdraftSplit(available, debitAmount decimal.Decimal) (debitOnDefault, debitOnOverdraft decimal.Decimal) {
	debitOnDefault = decimal.Min(available, debitAmount)
	debitOnOverdraft = debitAmount.Sub(debitOnDefault)

	return debitOnDefault, debitOnOverdraft
}

// ValidateOverdraftLimit checks whether adding a deficit to the currently
// consumed overdraft would breach the configured overdraft limit.
//
//   - When limitEnabled is false the balance is treated as having unlimited
//     overdraft and the function always returns nil.
//   - When the resulting cumulative usage (currentOverdraftUsed + deficit)
//     is less than or equal to the limit the function returns nil.
//   - Otherwise the function returns an error wrapping the canonical
//     constant.ErrOverdraftLimitExceeded sentinel (code 0167) so callers
//     can branch with errors.Is.
func ValidateOverdraftLimit(currentOverdraftUsed, deficit, overdraftLimit decimal.Decimal, limitEnabled bool) error {
	if !limitEnabled {
		return nil
	}

	projected := currentOverdraftUsed.Add(deficit)
	if projected.GreaterThan(overdraftLimit) {
		return pkg.ValidateBusinessError(pkgConstant.ErrOverdraftLimitExceeded, pkgConstant.EntityTransaction)
	}

	return nil
}

// NOTE: Empty-direction balances (Direction="") return (false, decimal.Zero)
// because the first check requires DirectionCredit. This is intentional:
// legacy balances that predate the overdraft feature never have
// AllowOverdraft=true (the Settings block is nil or default), so the
// !AllowOverdraft early return below would catch them even if the
// direction check were relaxed. The direction check is a defense-in-depth
// guard against data corruption where AllowOverdraft was manually set on
// a legacy balance without also setting Direction.

// DetectOverdraftSplit reports whether a given (amount, balance) pair needs
// to be split into two operations: one that consumes available funds on the
// default balance and one that routes the remaining deficit to the overdraft
// balance.
//
// A split is signalled only when all of the following hold:
//   - the balance direction is "credit" (asset-like balance);
//   - the operation is a DEBIT;
//   - overdraft is enabled on the balance (AllowOverdraft == true);
//   - the debit amount strictly exceeds the balance's Available funds.
//
// For every other case (including direction=debit balances) no split is
// signalled and the deficit is zero. The caller is then expected to fall
// back to existing insufficient-funds handling at the script layer.
func DetectOverdraftSplit(amount Amount, balance Balance) (splitNeeded bool, deficit decimal.Decimal) {
	if balance.Direction != pkgConstant.DirectionCredit {
		return false, decimal.Zero
	}

	if !balance.AllowOverdraft {
		return false, decimal.Zero
	}

	if amount.Operation != constant.DEBIT {
		return false, decimal.Zero
	}

	if !amount.Value.GreaterThan(balance.Available) {
		return false, decimal.Zero
	}

	return true, amount.Value.Sub(balance.Available)
}

// CalculateRefundSplit partitions a credit amount targeted at a
// direction=credit balance whose OverdraftUsed is greater than zero. The
// returned pair satisfies:
//
//	creditOnOverdraft = min(creditAmount, overdraftUsed)
//	creditOnDefault   = creditAmount - creditOnOverdraft
//
// Invariants (enforced by tests):
//   - creditOnOverdraft + creditOnDefault == creditAmount
//   - neither half is negative
//   - creditOnOverdraft never exceeds overdraftUsed
//
// Decimal precision is preserved because all arithmetic is performed on
// shopspring/decimal values with no intermediate float conversion.
func CalculateRefundSplit(creditAmount, overdraftUsed decimal.Decimal) (creditOnOverdraft, creditOnDefault decimal.Decimal) {
	creditOnOverdraft = decimal.Min(creditAmount, overdraftUsed)
	creditOnDefault = creditAmount.Sub(creditOnOverdraft)

	return creditOnOverdraft, creditOnDefault
}

// DetectRefundSplit reports whether an incoming credit on a direction=credit
// balance should trigger an overdraft repayment split. A split is signalled
// only when all of the following hold:
//   - the balance direction is exactly "credit" (legacy empty direction is
//     intentionally excluded: Direction is an explicit opt-in signal);
//   - the operation is a CREDIT;
//   - the balance has outstanding overdraft usage (OverdraftUsed > 0).
//
// When a split is signalled, repayAmount is capped at the outstanding
// overdraft (min(creditAmount, overdraftUsed)) and cleared is true when the
// repayment fully extinguishes the overdraft (overdraftUsed - repay == 0).
// For every non-signalling case the repay amount is zero and cleared is
// false.
func DetectRefundSplit(amount Amount, balance Balance) (splitNeeded bool, repayAmount decimal.Decimal, cleared bool) {
	if balance.Direction != pkgConstant.DirectionCredit {
		return false, decimal.Zero, false
	}

	if amount.Operation != constant.CREDIT {
		return false, decimal.Zero, false
	}

	if !balance.OverdraftUsed.GreaterThan(decimal.Zero) {
		return false, decimal.Zero, false
	}

	repayAmount = decimal.Min(amount.Value, balance.OverdraftUsed)
	cleared = balance.OverdraftUsed.Sub(repayAmount).IsZero()

	return true, repayAmount, cleared
}
