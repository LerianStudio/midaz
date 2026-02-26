// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

// LedgerSettings represents the settings for a ledger.
// These settings control various behaviors during transaction processing.
//
// Example JSON structure in ledger.settings:
//
//	{
//	  "accounting": {
//	    "validateAccountType": true,
//	    "validateRoutes": true
//	  }
//	}
type LedgerSettings struct {
	// Accounting contains validation settings for accounting operations.
	Accounting AccountingValidation `json:"accounting"`
}

// AccountingValidation represents the accounting-related validation settings.
// These settings control validation behavior during transaction processing.
type AccountingValidation struct {
	// ValidateAccountType enables validation of account types during transaction processing.
	// When true, accounts must have types that match the operation route rules.
	// Default: false (permissive - no validation)
	ValidateAccountType bool `json:"validateAccountType"`

	// ValidateRoutes enables validation of transaction routes during processing.
	// When true, transactions must specify valid route IDs that exist in the ledger.
	// Default: false (permissive - no validation)
	ValidateRoutes bool `json:"validateRoutes"`
}

// DefaultLedgerSettings returns the default ledger settings.
// All validation flags are false by default for backwards compatibility.
func DefaultLedgerSettings() LedgerSettings {
	return LedgerSettings{
		Accounting: AccountingValidation{
			ValidateAccountType: false,
			ValidateRoutes:      false,
		},
	}
}

// ParseLedgerSettings extracts and parses ledger settings from a settings map.
// Returns default settings if the map is nil, empty, or missing the "accounting" key.
// This function never returns an error - it falls back to safe defaults on any parse issue.
func ParseLedgerSettings(settings map[string]any) LedgerSettings {
	if settings == nil {
		return DefaultLedgerSettings()
	}

	accounting, ok := settings["accounting"]
	if !ok {
		return DefaultLedgerSettings()
	}

	accountingMap, ok := accounting.(map[string]any)
	if !ok {
		return DefaultLedgerSettings()
	}

	result := DefaultLedgerSettings()

	if validateAccountType, ok := accountingMap["validateAccountType"].(bool); ok {
		result.Accounting.ValidateAccountType = validateAccountType
	}

	if validateRoutes, ok := accountingMap["validateRoutes"].(bool); ok {
		result.Accounting.ValidateRoutes = validateRoutes
	}

	return result
}
