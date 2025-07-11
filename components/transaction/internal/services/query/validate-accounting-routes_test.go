package query

import (
	"testing"

	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
)

func TestValidateAccountingRules(t *testing.T) {
	tests := []struct {
		name         string
		accountCache *mmodel.AccountCache
		operation    lockOperation
		expectError  bool
	}{
		{
			name: "Valid alias rule",
			accountCache: &mmodel.AccountCache{
				RuleType: "alias",
				ValidIf:  "prefix:test-alias",
			},
			operation: lockOperation{
				alias: "prefix:test-alias",
				balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
			expectError: false,
		},
		{
			name: "Invalid alias rule",
			accountCache: &mmodel.AccountCache{
				RuleType: "alias",
				ValidIf:  "expected-alias",
			},
			operation: lockOperation{
				alias: "prefix:wrong-alias",
				balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
			expectError: true,
		},
		{
			name: "Valid account type rule - string slice",
			accountCache: &mmodel.AccountCache{
				RuleType: "account_type",
				ValidIf:  []string{"asset", "liability"},
			},
			operation: lockOperation{
				alias: "prefix:test-alias",
				balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
			expectError: false,
		},
		{
			name: "Invalid account type rule - string slice",
			accountCache: &mmodel.AccountCache{
				RuleType: "account_type",
				ValidIf:  []string{"asset", "liability"},
			},
			operation: lockOperation{
				alias: "prefix:test-alias",
				balance: &mmodel.Balance{
					AccountType: "equity",
				},
			},
			expectError: true,
		},
		{
			name: "Valid account type rule - any slice",
			accountCache: &mmodel.AccountCache{
				RuleType: "account_type",
				ValidIf:  []any{"asset", "liability"},
			},
			operation: lockOperation{
				alias: "prefix:test-alias",
				balance: &mmodel.Balance{
					AccountType: "liability",
				},
			},
			expectError: false,
		},
		{
			name: "Invalid rule type",
			accountCache: &mmodel.AccountCache{
				RuleType: "invalid",
				ValidIf:  "test",
			},
			operation: lockOperation{
				alias: "prefix:test-alias",
				balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSingleOperationRule(tt.operation, tt.accountCache)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExtractStringSlice(t *testing.T) {
	t.Run("Handles string slice", func(t *testing.T) {
		input := []string{"asset", "liability"}
		result := extractStringSlice(input)
		assert.Equal(t, []string{"asset", "liability"}, result)
	})

	t.Run("Handles any slice with strings", func(t *testing.T) {
		input := []any{"asset", "liability"}
		result := extractStringSlice(input)
		assert.Equal(t, []string{"asset", "liability"}, result)
	})

	t.Run("Handles any slice with mixed types", func(t *testing.T) {
		input := []any{"asset", 123}
		result := extractStringSlice(input)
		assert.Nil(t, result)
	})

	t.Run("Handles invalid input", func(t *testing.T) {
		input := "not a slice"
		result := extractStringSlice(input)
		assert.Nil(t, result)
	})
}
