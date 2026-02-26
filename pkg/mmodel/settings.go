// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import "maps"

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

// DefaultLedgerSettingsMap returns the default ledger settings as a map[string]any.
// This is useful for API responses where the typed struct needs to be serialized.
func DefaultLedgerSettingsMap() map[string]any {
	return map[string]any{
		"accounting": map[string]any{
			"validateAccountType": false,
			"validateRoutes":      false,
		},
	}
}

// MergeSettingsWithDefaults merges persisted settings with default values.
// Returns a complete settings map where persisted values override defaults.
// If settings is nil or empty, returns the full default settings.
// Uses deep merge for nested objects (e.g., "accounting" section).
func MergeSettingsWithDefaults(settings map[string]any) map[string]any {
	defaults := DefaultLedgerSettingsMap()

	if len(settings) == 0 {
		return defaults
	}

	// Deep merge: iterate over defaults and overlay persisted values
	result := make(map[string]any)

	for key, defaultValue := range defaults {
		persistedValue, exists := settings[key]
		if !exists {
			result[key] = defaultValue
			continue
		}

		// If both are maps, merge them recursively
		defaultMap, defaultIsMap := defaultValue.(map[string]any)
		persistedMap, persistedIsMap := persistedValue.(map[string]any)

		if defaultIsMap && persistedIsMap {
			merged := make(map[string]any)
			// Start with defaults
			maps.Copy(merged, defaultMap)
			// Overlay persisted values
			maps.Copy(merged, persistedMap)

			result[key] = merged
		} else {
			// Not both maps, persisted value wins
			result[key] = persistedValue
		}
	}

	// Include any extra keys from persisted settings not in defaults
	for key, value := range settings {
		if _, exists := result[key]; !exists {
			result[key] = value
		}
	}

	return result
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
