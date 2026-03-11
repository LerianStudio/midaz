// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

// deriveTransactionAction returns the action constant based on whether
// the transaction is pending (hold) or immediate (direct).
func deriveTransactionAction(pending bool) string {
	if pending {
		return constant.ActionHold
	}

	return constant.ActionDirect
}

// deriveCommitCancelAction returns the action constant based on the
// transaction status for commit/cancel operations.
func deriveCommitCancelAction(status string) string {
	if status == constant.APPROVED {
		return constant.ActionCommit
	}

	return constant.ActionCancel
}

// deriveRevertAction returns the action constant for revert operations.
func deriveRevertAction() string {
	return constant.ActionRevert
}
