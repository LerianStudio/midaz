// Package constant provides system-wide constant values used across the Midaz ledger system.
// This file contains account-related constants that define account behavior and validation rules.
package constant

const (
	// DefaultExternalAccountAliasPrefix is the standard prefix used to identify external accounts.
	// External accounts represent entities outside the ledger system (e.g., external banks, payment processors).
	// The "@external/" prefix helps distinguish these accounts from internal ledger accounts.
	DefaultExternalAccountAliasPrefix = "@external/"

	// ExternalAccountType defines the type identifier for external accounts.
	// This constant is used to categorize accounts that interact with external systems.
	ExternalAccountType = "external"

	// AccountAliasAcceptedChars is a regular expression pattern that defines valid characters
	// for account aliases. Account aliases must contain only:
	// - Alphanumeric characters (a-z, A-Z, 0-9)
	// - Special characters: @ (at sign), : (colon), _ (underscore), - (hyphen)
	// This ensures consistent and safe alias formatting across the system.
	AccountAliasAcceptedChars = `^[a-zA-Z0-9@:_-]+$`
)
