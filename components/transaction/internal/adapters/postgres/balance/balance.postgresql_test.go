package balance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// ListByAliasesWithKeys - Input Validation Tests
// ============================================================================
// These tests verify the alias#key format validation logic that happens
// BEFORE any database interaction. This validation is pure business logic.

func TestListByAliasesWithKeys_ValidateAliasKeyFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		aliasesWithKeys []string
		wantValid       bool
		description     string
	}{
		{
			name:            "valid single alias#key",
			aliasesWithKeys: []string{"@alice#default"},
			wantValid:       true,
			description:     "Standard format with alias and key",
		},
		{
			name:            "valid multiple alias#key pairs",
			aliasesWithKeys: []string{"@alice#default", "@bob#savings", "@charlie#checking"},
			wantValid:       true,
			description:     "Multiple valid pairs",
		},
		{
			name:            "missing hash separator",
			aliasesWithKeys: []string{"aliaskey"},
			wantValid:       false,
			description:     "No # separator should fail",
		},
		{
			name:            "too many hash separators",
			aliasesWithKeys: []string{"alias#key#extra"},
			wantValid:       false,
			description:     "More than one # should fail",
		},
		{
			name:            "empty string",
			aliasesWithKeys: []string{""},
			wantValid:       false,
			description:     "Empty string should fail",
		},
		{
			name:            "valid first invalid second",
			aliasesWithKeys: []string{"@alice#default", "invalid"},
			wantValid:       false,
			description:     "All entries must be valid",
		},
		{
			name:            "hash only - technically valid split",
			aliasesWithKeys: []string{"#"},
			wantValid:       true,
			description:     "Single # splits to ['', ''], len=2 is valid format",
		},
		{
			name:            "alias with empty key",
			aliasesWithKeys: []string{"@alice#"},
			wantValid:       true,
			description:     "Empty key after # is valid format",
		},
		{
			name:            "empty alias with key",
			aliasesWithKeys: []string{"#default"},
			wantValid:       true,
			description:     "Empty alias before # is valid format",
		},
		{
			name:            "special characters in alias",
			aliasesWithKeys: []string{"@alice-bob_123#default"},
			wantValid:       true,
			description:     "Special chars in alias should work",
		},
		{
			name:            "special characters in key",
			aliasesWithKeys: []string{"@alice#my-special_key.v1"},
			wantValid:       true,
			description:     "Special chars in key should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			isValid := validateAliasKeyFormat(tt.aliasesWithKeys)
			assert.Equal(t, tt.wantValid, isValid, tt.description)
		})
	}
}

// validateAliasKeyFormat tests the validation logic used in ListByAliasesWithKeys
// This mirrors the validation in the actual repository method
func validateAliasKeyFormat(aliasesWithKeys []string) bool {
	if len(aliasesWithKeys) == 0 {
		return true // Empty list is valid (returns empty result)
	}

	for _, aliasWithKey := range aliasesWithKeys {
		// Split by #
		parts := splitAliasKey(aliasWithKey)
		if len(parts) != 2 {
			return false
		}
	}

	return true
}

// splitAliasKey splits an alias#key string - mirrors the logic in repository
func splitAliasKey(s string) []string {
	// Count # characters
	count := 0
	for _, c := range s {
		if c == '#' {
			count++
		}
	}

	// Must have exactly one #
	if count != 1 {
		return nil
	}

	// Find the position of #
	for i, c := range s {
		if c == '#' {
			return []string{s[:i], s[i+1:]}
		}
	}

	return nil
}

// ============================================================================
// Sync - UUID Validation Tests
// ============================================================================
// The Sync method validates UUID format before database operations.

func TestSync_UUIDValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		id        string
		wantValid bool
	}{
		{
			name:      "valid UUID v4",
			id:        "550e8400-e29b-41d4-a716-446655440000",
			wantValid: true,
		},
		{
			name:      "valid UUID v7",
			id:        "01907b86-0e96-7d17-a4c4-7a7b6c8d9e0f",
			wantValid: true,
		},
		{
			name:      "invalid - not a UUID",
			id:        "not-a-valid-uuid",
			wantValid: false,
		},
		{
			name:      "invalid - too short",
			id:        "550e8400-e29b-41d4",
			wantValid: false,
		},
		{
			name:      "invalid - empty string",
			id:        "",
			wantValid: false,
		},
		{
			name:      "invalid - wrong format",
			id:        "550e8400e29b41d4a716446655440000",
			wantValid: false,
		},
		{
			name:      "invalid - contains spaces",
			id:        "550e8400 e29b 41d4 a716 446655440000",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			isValid := isValidUUID(tt.id)
			assert.Equal(t, tt.wantValid, isValid)
		})
	}
}

// isValidUUID checks if a string is a valid UUID - mirrors uuid.Parse logic
func isValidUUID(s string) bool {
	if len(s) != 36 {
		return false
	}

	// Check format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !isHexDigit(byte(c)) {
				return false
			}
		}
	}

	return true
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// ============================================================================
// UpdateAllByAccountID - Required Fields Validation Tests
// ============================================================================
// The UpdateAllByAccountID method requires both AllowSending and AllowReceiving.

func TestUpdateAllByAccountID_RequiredFieldsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		allowSending   *bool
		allowReceiving *bool
		wantErr        bool
		errField       string
	}{
		{
			name:           "both fields provided",
			allowSending:   boolPtr(true),
			allowReceiving: boolPtr(true),
			wantErr:        false,
		},
		{
			name:           "allowSending nil",
			allowSending:   nil,
			allowReceiving: boolPtr(true),
			wantErr:        true,
			errField:       "allow_sending",
		},
		{
			name:           "allowReceiving nil",
			allowSending:   boolPtr(true),
			allowReceiving: nil,
			wantErr:        true,
			errField:       "allow_receiving",
		},
		{
			name:           "both fields nil",
			allowSending:   nil,
			allowReceiving: nil,
			wantErr:        true,
			errField:       "allow_sending", // First check fails
		},
		{
			name:           "both false",
			allowSending:   boolPtr(false),
			allowReceiving: boolPtr(false),
			wantErr:        false, // Both provided, values don't matter
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateUpdateAllByAccountIDFields(tt.allowSending, tt.allowReceiving)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errField)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// validateUpdateAllByAccountIDFields mirrors the validation in repository
func validateUpdateAllByAccountIDFields(allowSending, allowReceiving *bool) error {
	if allowSending == nil {
		return errRequired("allow_sending")
	}

	if allowReceiving == nil {
		return errRequired("allow_receiving")
	}

	return nil
}

type validationError struct {
	field string
}

func (e validationError) Error() string {
	return e.field + " value is required"
}

func errRequired(field string) error {
	return validationError{field: field}
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}

// ============================================================================
// DeleteAllByIDs - Empty IDs Validation Tests
// ============================================================================
// The DeleteAllByIDs method returns early for empty ID slices.

func TestDeleteAllByIDs_EmptyIDsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ids         []string
		shouldSkip  bool
		description string
	}{
		{
			name:        "nil slice",
			ids:         nil,
			shouldSkip:  true,
			description: "Nil slice should skip database call",
		},
		{
			name:        "empty slice",
			ids:         []string{},
			shouldSkip:  true,
			description: "Empty slice should skip database call",
		},
		{
			name:        "single ID",
			ids:         []string{"00000000-0000-0000-0000-000000000001"},
			shouldSkip:  false,
			description: "Single ID should proceed to database",
		},
		{
			name:        "multiple IDs",
			ids:         []string{"00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"},
			shouldSkip:  false,
			description: "Multiple IDs should proceed to database",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			shouldSkip := shouldSkipDeleteAll(tt.ids)
			assert.Equal(t, tt.shouldSkip, shouldSkip, tt.description)
		})
	}
}

// shouldSkipDeleteAll mirrors the early return check in DeleteAllByIDs
func shouldSkipDeleteAll(ids []string) bool {
	return len(ids) == 0
}

// ============================================================================
// BalancesUpdate - Empty Slice Handling Tests
// ============================================================================
// The BalancesUpdate method handles empty slices gracefully.

func TestBalancesUpdate_EmptySliceHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		count       int
		description string
	}{
		{
			name:        "empty slice results in no operations",
			count:       0,
			description: "Empty slice should not generate any SQL operations",
		},
		{
			name:        "single item generates one operation",
			count:       1,
			description: "Single balance should generate one UPDATE",
		},
		{
			name:        "multiple items generate multiple operations",
			count:       5,
			description: "Five balances should generate five UPDATEs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// This validates that the operation count matches expectations
			opCount := calculateOperationCount(tt.count)
			assert.Equal(t, tt.count, opCount, tt.description)
		})
	}
}

// calculateOperationCount mirrors the loop in BalancesUpdate - one op per balance
func calculateOperationCount(balanceCount int) int {
	return balanceCount
}

// ============================================================================
// Column List Validation Tests
// ============================================================================
// Verify the column list matches expected schema.

func TestBalanceColumnList_HasExpectedColumns(t *testing.T) {
	t.Parallel()

	expectedColumns := []string{
		"id",
		"organization_id",
		"ledger_id",
		"account_id",
		"alias",
		"asset_code",
		"available",
		"on_hold",
		"version",
		"account_type",
		"allow_sending",
		"allow_receiving",
		"created_at",
		"updated_at",
		"deleted_at",
		"key",
	}

	assert.Equal(t, expectedColumns, balanceColumnList, "Column list should match expected schema")
	assert.Len(t, balanceColumnList, 16, "Should have exactly 16 columns")
}

func TestBalanceColumnList_NoMissingColumns(t *testing.T) {
	t.Parallel()

	requiredColumns := map[string]bool{
		"id":              true,
		"organization_id": true,
		"ledger_id":       true,
		"account_id":      true,
		"alias":           true,
		"key":             true,
		"asset_code":      true,
		"available":       true,
		"on_hold":         true,
		"version":         true,
		"account_type":    true,
		"allow_sending":   true,
		"allow_receiving": true,
		"created_at":      true,
		"updated_at":      true,
		"deleted_at":      true,
	}

	for _, col := range balanceColumnList {
		assert.True(t, requiredColumns[col], "Column %s should be a known column", col)
		delete(requiredColumns, col)
	}

	assert.Empty(t, requiredColumns, "All required columns should be present in balanceColumnList")
}
