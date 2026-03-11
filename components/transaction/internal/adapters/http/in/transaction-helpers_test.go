// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/stretchr/testify/assert"
)

// TestDeriveActionForBuildOperations verifies that the action derivation logic
// in BuildOperations correctly maps pending flag to the appropriate action constant.
//
// Action derivation table:
//   - pending == false => "direct"
//   - pending == true  => "hold"
func TestDeriveActionForBuildOperations(t *testing.T) {
	tests := []struct {
		name           string
		pending        bool
		expectedAction string
	}{
		{
			name:           "non-pending transaction derives action=direct",
			pending:        false,
			expectedAction: cn.ActionDirect,
		},
		{
			name:           "pending transaction derives action=hold",
			pending:        true,
			expectedAction: cn.ActionHold,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// deriveTransactionAction is the function that will be extracted
			// to derive action from the pending flag in BuildOperations context
			action := deriveTransactionAction(tt.pending)
			assert.Equal(t, tt.expectedAction, action)
		})
	}
}

// TestDeriveActionForCommitOrCancel verifies that the action derivation logic
// in commitOrCancelTransaction correctly maps transaction status to action.
//
// Action derivation table:
//   - transactionStatus == APPROVED => "commit"
//   - transactionStatus == CANCELED => "cancel"
func TestDeriveActionForCommitOrCancel(t *testing.T) {
	tests := []struct {
		name              string
		transactionStatus string
		expectedAction    string
	}{
		{
			name:              "APPROVED status derives action=commit",
			transactionStatus: cn.APPROVED,
			expectedAction:    cn.ActionCommit,
		},
		{
			name:              "CANCELED status derives action=cancel",
			transactionStatus: cn.CANCELED,
			expectedAction:    cn.ActionCancel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// deriveCommitCancelAction is the function that will be extracted
			// to derive action from transactionStatus in commitOrCancelTransaction
			action := deriveCommitCancelAction(tt.transactionStatus)
			assert.Equal(t, tt.expectedAction, action)
		})
	}
}

// TestDeriveActionForRevert verifies that RevertTransaction always uses action="revert".
func TestDeriveActionForRevert(t *testing.T) {
	// deriveRevertAction always returns "revert"
	action := deriveRevertAction()
	assert.Equal(t, cn.ActionRevert, action)
}
