// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

// Action constants represent the valid actions for operation-transaction route associations.
// These values must match the CHECK constraint in migration 000023:
// LOWER(action) IN ('direct', 'hold', 'commit', 'cancel', 'revert')
const (
	ActionDirect = "direct"
	ActionHold   = "hold"
	ActionCommit = "commit"
	ActionCancel = "cancel"
	ActionRevert = "revert"
)

// ValidActions contains all valid action values for programmatic validation.
var ValidActions = []string{
	ActionDirect,
	ActionHold,
	ActionCommit,
	ActionCancel,
	ActionRevert,
}
