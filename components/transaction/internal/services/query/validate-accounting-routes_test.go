package query

import (
	"context"
	"os"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestValidateAccountingRules(t *testing.T) {
	tests := []struct {
		name         string
		accountCache *mmodel.AccountCache
		operation    mmodel.BalanceOperation
		expectError  bool
	}{
		{
			name: "Valid alias rule",
			accountCache: &mmodel.AccountCache{
				RuleType: "alias",
				ValidIf:  "prefix:test-alias",
			},
			operation: mmodel.BalanceOperation{
				Alias: "prefix:test-alias",
				Balance: &mmodel.Balance{
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
			operation: mmodel.BalanceOperation{
				Alias: "prefix:wrong-alias",
				Balance: &mmodel.Balance{
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
			operation: mmodel.BalanceOperation{
				Alias: "prefix:test-alias",
				Balance: &mmodel.Balance{
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
			operation: mmodel.BalanceOperation{
				Alias: "prefix:test-alias",
				Balance: &mmodel.Balance{
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
			operation: mmodel.BalanceOperation{
				Alias: "prefix:test-alias",
				Balance: &mmodel.Balance{
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
			operation: mmodel.BalanceOperation{
				Alias: "prefix:test-alias",
				Balance: &mmodel.Balance{
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

func TestValidateAccountingRules_WithEnvironmentVariable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()

	t.Run("Returns nil when organization:ledger not in TRANSACTION_ROUTE_VALIDATION env var", func(t *testing.T) {
		originalEnv := os.Getenv("TRANSACTION_ROUTE_VALIDATION")
		defer func() {
			if originalEnv != "" {
				os.Setenv("TRANSACTION_ROUTE_VALIDATION", originalEnv)
			} else {
				os.Unsetenv("TRANSACTION_ROUTE_VALIDATION")
			}
		}()

		differentOrg := libCommons.GenerateUUIDv7()
		differentLedger := libCommons.GenerateUUIDv7()
		os.Setenv("TRANSACTION_ROUTE_VALIDATION", differentOrg.String()+":"+differentLedger.String())

		operations := []mmodel.BalanceOperation{
			{
				Alias: "test-alias",
				Balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
		}

		validate := &libTransaction.Responses{
			TransactionRoute: transactionRouteID.String(),
		}

		err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate)

		assert.NoError(t, err)
	})

	t.Run("Returns error when transaction route is empty", func(t *testing.T) {
		originalEnv := os.Getenv("TRANSACTION_ROUTE_VALIDATION")
		defer func() {
			if originalEnv != "" {
				os.Setenv("TRANSACTION_ROUTE_VALIDATION", originalEnv)
			} else {
				os.Unsetenv("TRANSACTION_ROUTE_VALIDATION")
			}
		}()

		os.Setenv("TRANSACTION_ROUTE_VALIDATION", organizationID.String()+":"+ledgerID.String())

		operations := []mmodel.BalanceOperation{
			{
				Alias: "test-alias",
				Balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
		}

		validate := &libTransaction.Responses{
			TransactionRoute: "",
		}

		err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate)

		assert.Error(t, err)
	})

	t.Run("Returns error when transaction route ID is invalid", func(t *testing.T) {
		originalEnv := os.Getenv("TRANSACTION_ROUTE_VALIDATION")
		defer func() {
			if originalEnv != "" {
				os.Setenv("TRANSACTION_ROUTE_VALIDATION", originalEnv)
			} else {
				os.Unsetenv("TRANSACTION_ROUTE_VALIDATION")
			}
		}()

		os.Setenv("TRANSACTION_ROUTE_VALIDATION", organizationID.String()+":"+ledgerID.String())

		operations := []mmodel.BalanceOperation{
			{
				Alias: "test-alias",
				Balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
		}

		validate := &libTransaction.Responses{
			TransactionRoute: "invalid-uuid-format",
		}

		err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate)

		assert.Error(t, err)
	})

	t.Run("Empty TRANSACTION_ROUTE_VALIDATION environment variable", func(t *testing.T) {
		originalEnv := os.Getenv("TRANSACTION_ROUTE_VALIDATION")
		defer func() {
			if originalEnv != "" {
				os.Setenv("TRANSACTION_ROUTE_VALIDATION", originalEnv)
			} else {
				os.Unsetenv("TRANSACTION_ROUTE_VALIDATION")
			}
		}()

		os.Unsetenv("TRANSACTION_ROUTE_VALIDATION")

		operations := []mmodel.BalanceOperation{
			{
				Alias: "test-alias",
				Balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
		}

		validate := &libTransaction.Responses{
			TransactionRoute: transactionRouteID.String(),
		}

		err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate)

		assert.NoError(t, err)
	})
}

func TestUniqueValues(t *testing.T) {
	t.Run("Empty map returns 0", func(t *testing.T) {
		result := uniqueValues(map[string]string{})
		assert.Equal(t, 0, result)
	})

	t.Run("Single item map returns 1", func(t *testing.T) {
		result := uniqueValues(map[string]string{"key1": "value1"})
		assert.Equal(t, 1, result)
	})

	t.Run("Map with duplicate values returns correct count", func(t *testing.T) {
		result := uniqueValues(map[string]string{
			"key1": "value1",
			"key2": "value1",
			"key3": "value2",
		})
		assert.Equal(t, 2, result)
	})

	t.Run("Map with all unique values returns correct count", func(t *testing.T) {
		result := uniqueValues(map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		})
		assert.Equal(t, 3, result)
	})
}
