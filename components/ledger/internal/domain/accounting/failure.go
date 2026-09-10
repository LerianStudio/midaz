// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accounting

const (
	FailureInsufficientFunds         = "insufficient_funds"
	FailureOverdraftLimitExceeded    = "overdraft_limit_exceeded"
	FailureOverdraftNotEligible      = "overdraft_not_eligible"
	FailureOverdraftCompanionMissing = "overdraft_companion_missing"
	FailureBalanceDeleted            = "balance_deleted"
	FailureOnHoldUnderflow           = "onhold_underflow"
	FailureBalanceMissing            = "balance_missing"
	FailureAssetMismatch             = "asset_mismatch"
	FailureSendingNotAllowed         = "sending_not_allowed"
	FailureReceivingNotAllowed       = "receiving_not_allowed"
	FailureExternalHoldNotAllowed    = "external_hold_not_allowed"
)

// Failure is a recognized refusal produced before any accounting write.
// TransactionIndex and PostingIndex are zero-based, or -1 when not applicable.
// Code must be one of the Failure constants; unknown protocol codes and
// transport, runtime or indeterminate outcomes remain technical errors.
// Missing companions and on-hold underflow indicate integrity failures, not
// ordinary insufficient funds. Public error mapping belongs to the caller.
type Failure struct {
	Code             string `json:"code"`
	TransactionIndex int    `json:"transactionIndex"`
	PostingIndex     int    `json:"postingIndex"`
	BalanceRef       string `json:"balanceRef"`
}

// Error returns the stable accounting failure code.
func (f *Failure) Error() string {
	return f.Code
}

var _ error = (*Failure)(nil)
