// Package constant provides system-wide constant values used across the Midaz ledger system.
// This file contains operation type constants used in double-entry accounting transactions.
package constant

// Operation Types
//
// These constants define the types of operations that can be performed on accounts
// in the double-entry ledger system. Each operation type affects account balances
// in specific ways according to accounting principles.
const (
	// DEBIT represents a debit operation that decreases liability/equity/revenue accounts
	// or increases asset/expense accounts. In the Midaz system, debits are one side of
	// the double-entry equation and must always balance with corresponding credits.
	DEBIT = "DEBIT"

	// CREDIT represents a credit operation that increases liability/equity/revenue accounts
	// or decreases asset/expense accounts. Credits form the other side of the double-entry
	// equation and must always balance with corresponding debits.
	CREDIT = "CREDIT"

	// ONHOLD represents an operation that places funds on hold (reserves them) without
	// transferring them. This is used for scenarios like pending transactions, authorizations,
	// or escrow situations where funds need to be reserved but not yet moved.
	// Held funds reduce the available balance but remain in the account.
	ONHOLD = "ON_HOLD"

	// RELEASE represents an operation that releases previously held funds, making them
	// available again. This is the counterpart to ONHOLD and is used when a hold is
	// no longer needed (e.g., transaction cancelled, authorization expired).
	RELEASE = "RELEASE"
)
