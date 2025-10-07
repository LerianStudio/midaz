// Package constant provides system-wide constant values used across the Midaz ledger system.
// This file contains operation route related constants.
package constant

// Operation Route Account Rule Types
//
// These constants define the types of rules that can be applied to accounts
// within operation routes. Operation routes define how transactions flow through
// the system, and account rules specify which accounts can participate in these routes.
const (
	// AccountRuleTypeAlias indicates that the account rule matches accounts by their alias.
	// When this rule type is used, the system will route operations to accounts that
	// match the specified alias pattern or exact alias value.
	AccountRuleTypeAlias = "alias"

	// AccountRuleTypeAccountType indicates that the account rule matches accounts by their type.
	// When this rule type is used, the system will route operations to accounts that
	// have the specified account type (e.g., "deposit", "loan", "external").
	AccountRuleTypeAccountType = "account_type"
)
