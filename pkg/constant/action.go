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

// Supplementary accounting entry actions.
//
// These are NOT valid transaction-route action values and therefore are NOT
// included in ValidActions or the migration 000023 CHECK constraint. They
// identify additional accounting entries on an operation route
// (AccountingEntries.Overdraft) that are recorded alongside the primary
// direct scenario to describe the accounting impact of overdraft usage and
// repayment. Keeping them separate from ValidActions avoids loosening the
// DB-level whitelist for transaction-route associations.
const (
	ActionOverdraft = "overdraft"
)

// ValidActions contains all valid action values for programmatic validation.
//
// This slice mirrors the migration 000023 CHECK constraint on the
// transaction-route association table and MUST NOT include ActionOverdraft:
// it is an accounting-entry action, not a transaction-route action, and the
// DB constraint rejects it by design.
var ValidActions = []string{
	ActionDirect,
	ActionHold,
	ActionCommit,
	ActionCancel,
	ActionRevert,
}
