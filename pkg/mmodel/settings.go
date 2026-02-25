// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

// AccountingSettings represents the accounting-related settings for a ledger.
// These settings control validation behavior during transaction processing.
//
// Example JSON structure in ledger.settings:
//
//	{
//	  "accounting": {
//	    "validateAccountType": true,
//	    "validateRoutes": true
//	  }
//	}
type AccountingSettings struct {
	// ValidateAccountType enables validation of account types during transaction processing.
	// When true, accounts must have types that match the operation route rules.
	// Default: false (permissive - no validation)
	ValidateAccountType bool `json:"validateAccountType"`

	// ValidateRoutes enables validation of transaction routes during processing.
	// When true, transactions must specify valid route IDs that exist in the ledger.
	// Default: false (permissive - no validation)
	ValidateRoutes bool `json:"validateRoutes"`
}

// DefaultAccountingSettings returns the default accounting settings.
// All validation flags are false by default for backwards compatibility.
func DefaultAccountingSettings() AccountingSettings {
	return AccountingSettings{
		ValidateAccountType: false,
		ValidateRoutes:      false,
	}
}

// ParseAccountingSettings extracts and parses accounting settings from a settings map.
// Returns default settings if the map is nil, empty, or missing the "accounting" key.
// This function never returns an error - it falls back to safe defaults on any parse issue.
func ParseAccountingSettings(settings map[string]any) AccountingSettings {
	if settings == nil {
		return DefaultAccountingSettings()
	}

	accounting, ok := settings["accounting"]
	if !ok {
		return DefaultAccountingSettings()
	}

	accountingMap, ok := accounting.(map[string]any)
	if !ok {
		return DefaultAccountingSettings()
	}

	result := DefaultAccountingSettings()

	if validateAccountType, ok := accountingMap["validateAccountType"].(bool); ok {
		result.ValidateAccountType = validateAccountType
	}

	if validateRoutes, ok := accountingMap["validateRoutes"].(bool); ok {
		result.ValidateRoutes = validateRoutes
	}

	return result
}
