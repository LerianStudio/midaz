// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"fmt"
	"maps"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

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

// defaultAccountingValidation is the canonical source of default validation settings.
// All validation flags are false by default for backwards compatibility.
var defaultAccountingValidation = AccountingValidation{
	ValidateAccountType: false,
	ValidateRoutes:      false,
}

// DefaultLedgerSettings returns the default ledger settings as a typed struct.
// All validation flags are false by default for backwards compatibility.
func DefaultLedgerSettings() LedgerSettings {
	return LedgerSettings{
		Accounting: defaultAccountingValidation,
	}
}

// DefaultLedgerSettingsMap returns the default ledger settings as a map[string]any.
// This is useful for API responses where the typed struct needs to be serialized.
// Uses the same canonical defaults as DefaultLedgerSettings.
func DefaultLedgerSettingsMap() map[string]any {
	return map[string]any{
		"accounting": map[string]any{
			"validateAccountType": defaultAccountingValidation.ValidateAccountType,
			"validateRoutes":      defaultAccountingValidation.ValidateRoutes,
		},
	}
}

// MergeSettingsWithDefaults merges persisted settings with default values.
// Returns a complete settings map where persisted values override defaults.
// If settings is nil or empty, returns the full default settings.
// Performs a one-level nested merge: top-level map keys are merged, and if both
// the default and persisted values for a key are maps, those maps are also merged
// (persisted keys override default keys). Deeper nesting is not recursively merged.
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

// settingsSchema defines the allowed structure for ledger settings.
// This schema is used for strict validation - only these paths are allowed.
//
// To add new settings:
//  1. Add the field to LedgerSettings struct
//  2. Add the corresponding entry here with the appropriate type ("bool", "string", "number")
//  3. Update DefaultLedgerSettings() and DefaultLedgerSettingsMap()
//  4. Add tests in settings_test.go
var settingsSchema = map[string]map[string]string{
	"accounting": {
		"validateAccountType": "bool",
		"validateRoutes":      "bool",
	},
}

// ValidateSettings validates that the input settings contain only known fields
// with correct types. Returns an error if unknown fields are found or types are invalid.
// This enforces strict schema compliance for settings updates.
//
// Parameters:
//   - settings: The settings map to validate
//
// Returns:
//   - error: A validation error if the settings are invalid, nil otherwise
func ValidateSettings(settings map[string]any) error {
	if settings == nil {
		return nil
	}

	// Check each top-level key
	for key, value := range settings {
		nestedSchema, knownKey := settingsSchema[key]
		if !knownKey {
			return pkg.ValidateBusinessError(constant.ErrUnknownSettingsField, "LedgerSettings", key)
		}

		// If value is nil, it's valid (null semantics: store null)
		if value == nil {
			continue
		}

		// Value must be a map for nested settings
		nestedMap, isMap := value.(map[string]any)
		if !isMap {
			return pkg.ValidateBusinessError(constant.ErrInvalidSettingsFieldType, "LedgerSettings", key, "object")
		}

		// Validate nested keys
		for nestedKey, nestedValue := range nestedMap {
			expectedType, knownNestedKey := nestedSchema[nestedKey]
			if !knownNestedKey {
				return pkg.ValidateBusinessError(constant.ErrUnknownSettingsField, "LedgerSettings", fmt.Sprintf("%s.%s", key, nestedKey))
			}

			// If nested value is nil, it's valid (null semantics)
			if nestedValue == nil {
				continue
			}

			// Validate type
			if err := validateSettingsFieldType(nestedValue, expectedType, fmt.Sprintf("%s.%s", key, nestedKey)); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateSettingsFieldType checks if a value matches the expected type.
// Supported types: "bool" (Go bool), "string" (Go string), "number" (Go float64/int/int64).
// These correspond to JSON Schema primitive types.
func validateSettingsFieldType(value any, expectedType, fieldPath string) error {
	switch expectedType {
	case "bool":
		if _, ok := value.(bool); !ok {
			return pkg.ValidateBusinessError(constant.ErrInvalidSettingsFieldType, "LedgerSettings", fieldPath, "boolean")
		}
	case "string":
		if _, ok := value.(string); !ok {
			return pkg.ValidateBusinessError(constant.ErrInvalidSettingsFieldType, "LedgerSettings", fieldPath, "string")
		}
	case "number":
		switch value.(type) {
		case float64, int, int64:
			// valid number types from JSON unmarshaling
		default:
			return pkg.ValidateBusinessError(constant.ErrInvalidSettingsFieldType, "LedgerSettings", fieldPath, "number")
		}
	}

	return nil
}

// DeepMergeSettings performs a deep merge of new settings into existing settings.
// For nested objects (like "accounting"), individual keys are merged rather than
// replaced entirely. This preserves existing nested values not specified in the update.
//
// Example:
//
//	existing: {"accounting": {"validateAccountType": true, "validateRoutes": false}}
//	new:      {"accounting": {"validateRoutes": true}}
//	result:   {"accounting": {"validateAccountType": true, "validateRoutes": true}}
//
// PRECONDITION: newSettings MUST be validated via ValidateSettings() before calling.
// This function assumes all keys in newSettings are valid schema paths.
//
// Parameters:
//   - existing: The current settings from the database (may be nil)
//   - newSettings: The new settings to merge (must be pre-validated via ValidateSettings)
//
// Returns:
//   - The merged settings map (a new map; inputs are not mutated)
func DeepMergeSettings(existing, newSettings map[string]any) map[string]any {
	if existing == nil {
		existing = make(map[string]any)
	}

	result := make(map[string]any)

	// Copy existing settings first (always copy to maintain immutability contract)
	for key, value := range existing {
		if existingMap, isMap := value.(map[string]any); isMap {
			// Deep copy nested maps to avoid mutation
			copiedMap := make(map[string]any)
			maps.Copy(copiedMap, existingMap)
			result[key] = copiedMap
		} else {
			result[key] = value
		}
	}

	// If no new settings, return the copy of existing
	if newSettings == nil {
		return result
	}

	// Merge new settings
	for key, newValue := range newSettings {
		existingValue, exists := result[key]

		// If new value is nil, store it (null semantics)
		if newValue == nil {
			result[key] = nil
			continue
		}

		newMap, newIsMap := newValue.(map[string]any)
		existingMap, existingIsMap := existingValue.(map[string]any)

		// Both are maps: deep merge into a fresh map to avoid reference issues
		if newIsMap && exists && existingIsMap {
			merged := make(map[string]any)
			maps.Copy(merged, existingMap)
			maps.Copy(merged, newMap)

			result[key] = merged
		} else {
			// Otherwise, new value replaces existing
			result[key] = newValue
		}
	}

	return result
}
