// Package constant provides system-wide constant values used across the Midaz ledger system.
// This file contains transaction status constants and database error codes.
package constant

// Transaction Status Constants
//
// These constants define the possible states of a transaction throughout its lifecycle
// in the Midaz ledger system. Transactions move through these states based on business
// rules and validation results.
const (
	// CREATED indicates a transaction has been created but not yet processed or validated.
	// This is typically the initial state when a transaction is first submitted to the system.
	CREATED = "CREATED"

	// APPROVED indicates a transaction has been approved and successfully processed.
	// The transaction has passed all validations, balances have been updated, and the
	// transaction is considered final and immutable.
	APPROVED = "APPROVED"

	// PENDING indicates a transaction is awaiting approval or scheduled for future execution.
	// Pending transactions may be for future-dated transactions or transactions requiring
	// additional approval steps before being committed.
	PENDING = "PENDING"

	// CANCELED indicates a transaction has been canceled and will not be processed.
	// Canceled transactions do not affect account balances and cannot be reverted or modified.
	CANCELED = "CANCELED"

	// NOTED indicates a transaction has been recorded for informational purposes only.
	// Noted transactions may be used for audit trails, reconciliation, or tracking purposes
	// without affecting account balances.
	NOTED = "NOTED"

	// Database Error Codes

	// UniqueViolationCode is the PostgreSQL error code for unique constraint violations (23505).
	// This error occurs when attempting to insert or update a record that would violate
	// a unique constraint (e.g., duplicate IDs, unique keys, or unique indexes).
	// The system uses this code to detect and handle duplicate entry attempts gracefully.
	UniqueViolationCode = "23505"
)
