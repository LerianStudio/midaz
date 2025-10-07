// Package constant provides system-wide constant values used across the Midaz ledger system.
// This file contains balance-related constants.
package constant

const (
	// DefaultBalanceKey is the key used to identify the default balance entry for an account.
	// In Midaz, accounts can have multiple balance entries (e.g., available, on-hold, pending).
	// The "default" key represents the primary balance that is used when no specific balance
	// key is specified in operations.
	DefaultBalanceKey = "default"
)
