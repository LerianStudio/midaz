// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/ledger"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestValidateAccountingRules(t *testing.T) {
	tests := []struct {
		name         string
		accountCache *mmodel.AccountCache
		operation    mmodel.BalanceOperation
		expectError  bool
		errorCode    string
	}{
		{
			name: "Valid alias rule",
			accountCache: &mmodel.AccountCache{
				RuleType: "alias",
				ValidIf:  "test-alias",
			},
			operation: mmodel.BalanceOperation{
				Alias: "key#test-alias",
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
			errorCode:   "0118",
		},
		{
			name: "Invalid alias rule with hash delimiter",
			accountCache: &mmodel.AccountCache{
				RuleType: "alias",
				ValidIf:  "test-alias",
			},
			operation: mmodel.BalanceOperation{
				Alias: "key#wrong-alias",
				Balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
			expectError: true,
			errorCode:   "0118",
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
			errorCode:   "0119",
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
			errorCode:   "0113",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSingleOperationRule(tt.operation, tt.accountCache)

			if tt.expectError {
				assert.Error(t, err)

				if tt.errorCode != "" {
					assert.Contains(t, err.Error(), tt.errorCode)
				}
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

func TestValidateAccountingRules_WithSettings(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	ctx := context.Background()

	t.Run("Returns nil when validateRoutes is false (default)", func(t *testing.T) {
		mockLedgerRepo := ledger.NewMockRepository(ctrl)
		mockLedgerRepo.EXPECT().
			GetSettings(gomock.Any(), organizationID, ledgerID).
			Return(map[string]any{
				"accounting": map[string]any{
					"validateRoutes": false,
				},
			}, nil)

		uc := &UseCase{
			TransactionRedisRepo: mockRedisRepo,
			LedgerRepo:           mockLedgerRepo,
		}

		operations := []mmodel.BalanceOperation{
			{
				Alias: "test-alias",
				Balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
		}

		validate := &pkgTransaction.Responses{
			TransactionRoute: transactionRouteID.String(),
		}

		_, err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate, constant.ActionDirect)

		assert.NoError(t, err)
	})

	t.Run("Returns nil when LedgerRepo returns error (graceful degradation)", func(t *testing.T) {
		mockLedgerRepo := ledger.NewMockRepository(ctrl)
		mockLedgerRepo.EXPECT().
			GetSettings(gomock.Any(), organizationID, ledgerID).
			Return(nil, errors.New("connection error"))

		uc := &UseCase{
			TransactionRedisRepo: mockRedisRepo,
			LedgerRepo:           mockLedgerRepo,
		}

		operations := []mmodel.BalanceOperation{
			{
				Alias: "test-alias",
				Balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
		}

		validate := &pkgTransaction.Responses{
			TransactionRoute: transactionRouteID.String(),
		}

		_, err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate, constant.ActionDirect)

		assert.NoError(t, err)
	})

	t.Run("Returns error when validateRoutes is true and transaction route is empty", func(t *testing.T) {
		mockLedgerRepo := ledger.NewMockRepository(ctrl)
		mockLedgerRepo.EXPECT().
			GetSettings(gomock.Any(), organizationID, ledgerID).
			Return(map[string]any{
				"accounting": map[string]any{
					"validateRoutes": true,
				},
			}, nil)

		uc := &UseCase{
			TransactionRedisRepo: mockRedisRepo,
			LedgerRepo:           mockLedgerRepo,
		}

		operations := []mmodel.BalanceOperation{
			{
				Alias: "test-alias",
				Balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
		}

		validate := &pkgTransaction.Responses{
			TransactionRoute: "",
		}

		_, err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate, constant.ActionDirect)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "0114")
	})

	t.Run("Returns error when validateRoutes is true and transaction route ID is invalid", func(t *testing.T) {
		mockLedgerRepo := ledger.NewMockRepository(ctrl)
		mockLedgerRepo.EXPECT().
			GetSettings(gomock.Any(), organizationID, ledgerID).
			Return(map[string]any{
				"accounting": map[string]any{
					"validateRoutes": true,
				},
			}, nil)

		uc := &UseCase{
			TransactionRedisRepo: mockRedisRepo,
			LedgerRepo:           mockLedgerRepo,
		}

		operations := []mmodel.BalanceOperation{
			{
				Alias: "test-alias",
				Balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
		}

		validate := &pkgTransaction.Responses{
			TransactionRoute: "invalid-uuid-format",
		}

		_, err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate, constant.ActionDirect)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "0115")
	})

	t.Run("Returns nil when settings fetch fails (graceful degradation)", func(t *testing.T) {
		mockLedgerRepo := ledger.NewMockRepository(ctrl)
		mockLedgerRepo.EXPECT().
			GetSettings(gomock.Any(), organizationID, ledgerID).
			Return(nil, errors.New("connection error"))

		uc := &UseCase{
			TransactionRedisRepo: mockRedisRepo,
			LedgerRepo:           mockLedgerRepo,
		}

		operations := []mmodel.BalanceOperation{
			{
				Alias: "test-alias",
				Balance: &mmodel.Balance{
					AccountType: "asset",
				},
			},
		}

		validate := &pkgTransaction.Responses{
			TransactionRoute: transactionRouteID.String(),
		}

		_, err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate, constant.ActionDirect)

		assert.NoError(t, err, "must return nil when settings fetch fails (graceful degradation)")
	})
}

func TestValidateAccountRules(t *testing.T) {
	ctx := context.Background()

	routeID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	tests := []struct {
		name                  string
		transactionRouteCache mmodel.TransactionRouteCache
		validate              *pkgTransaction.Responses
		operations            []mmodel.BalanceOperation
		ledgerSettings        mmodel.LedgerSettings
		expectError           bool
		errorCode             string
	}{
		{
			name: "Account type validation disabled skips account rules but validates route existence",
			transactionRouteCache: mmodel.TransactionRouteCache{
				Actions: map[string]mmodel.ActionRouteCache{
					"direct": {
						Source: map[string]mmodel.OperationRouteCache{
							routeID: {
								OperationType: "source",
							},
						},
						Destination:   map[string]mmodel.OperationRouteCache{},
						Bidirectional: map[string]mmodel.OperationRouteCache{},
					},
				},
			},
			validate: &pkgTransaction.Responses{
				From:                map[string]pkgTransaction.Amount{"op-alias": {}},
				To:                  map[string]pkgTransaction.Amount{},
				OperationRoutesFrom: map[string]string{"op-alias": routeID},
				OperationRoutesTo:   map[string]string{},
			},
			operations: []mmodel.BalanceOperation{
				{
					Alias:   "op-alias",
					Amount:  pkgTransaction.Amount{Direction: "debit"},
					Balance: &mmodel.Balance{AccountType: "asset"},
				},
			},
			ledgerSettings: mmodel.LedgerSettings{
				Accounting: mmodel.AccountingValidation{
					ValidateAccountType: false,
				},
			},
			expectError: false,
		},
		{
			name: "Operation found in From map with source route validates successfully",
			transactionRouteCache: mmodel.TransactionRouteCache{
				Actions: map[string]mmodel.ActionRouteCache{
					"direct": {
						Source: map[string]mmodel.OperationRouteCache{
							routeID: {
								OperationType: "source",
								Account: &mmodel.AccountCache{
									RuleType: "account_type",
									ValidIf:  []string{"asset"},
								},
							},
						},
						Destination:   map[string]mmodel.OperationRouteCache{},
						Bidirectional: map[string]mmodel.OperationRouteCache{},
					},
				},
			},
			validate: &pkgTransaction.Responses{
				From:                map[string]pkgTransaction.Amount{"op-alias": {}},
				To:                  map[string]pkgTransaction.Amount{},
				OperationRoutesFrom: map[string]string{"op-alias": routeID},
				OperationRoutesTo:   map[string]string{},
			},
			operations: []mmodel.BalanceOperation{
				{
					Alias:   "op-alias",
					Amount:  pkgTransaction.Amount{Direction: "debit"},
					Balance: &mmodel.Balance{AccountType: "asset"},
				},
			},
			ledgerSettings: mmodel.LedgerSettings{
				Accounting: mmodel.AccountingValidation{
					ValidateAccountType: true,
				},
			},
			expectError: false,
		},
		{
			name: "Operation found in To map with destination route validates successfully",
			transactionRouteCache: mmodel.TransactionRouteCache{
				Actions: map[string]mmodel.ActionRouteCache{
					"direct": {
						Source: map[string]mmodel.OperationRouteCache{},
						Destination: map[string]mmodel.OperationRouteCache{
							routeID: {
								OperationType: "destination",
								Account: &mmodel.AccountCache{
									RuleType: "account_type",
									ValidIf:  []string{"liability"},
								},
							},
						},
						Bidirectional: map[string]mmodel.OperationRouteCache{},
					},
				},
			},
			validate: &pkgTransaction.Responses{
				From:                map[string]pkgTransaction.Amount{},
				To:                  map[string]pkgTransaction.Amount{"op-alias": {}},
				OperationRoutesFrom: map[string]string{},
				OperationRoutesTo:   map[string]string{"op-alias": routeID},
			},
			operations: []mmodel.BalanceOperation{
				{
					Alias:   "op-alias",
					Amount:  pkgTransaction.Amount{Direction: "credit"},
					Balance: &mmodel.Balance{AccountType: "liability"},
				},
			},
			ledgerSettings: mmodel.LedgerSettings{
				Accounting: mmodel.AccountingValidation{
					ValidateAccountType: true,
				},
			},
			expectError: false,
		},
		{
			name: "Operation not in either From or To map is skipped",
			transactionRouteCache: mmodel.TransactionRouteCache{
				Actions: map[string]mmodel.ActionRouteCache{
					"direct": {
						Source:        map[string]mmodel.OperationRouteCache{},
						Destination:   map[string]mmodel.OperationRouteCache{},
						Bidirectional: map[string]mmodel.OperationRouteCache{},
					},
				},
			},
			validate: &pkgTransaction.Responses{
				From:                map[string]pkgTransaction.Amount{},
				To:                  map[string]pkgTransaction.Amount{},
				OperationRoutesFrom: map[string]string{},
				OperationRoutesTo:   map[string]string{},
			},
			operations: []mmodel.BalanceOperation{
				{
					Alias:   "unknown-alias",
					Amount:  pkgTransaction.Amount{Direction: "debit"},
					Balance: &mmodel.Balance{AccountType: "asset"},
				},
			},
			ledgerSettings: mmodel.LedgerSettings{
				Accounting: mmodel.AccountingValidation{
					ValidateAccountType: true,
				},
			},
			expectError: false,
		},
		{
			name: "Route not found in any cache returns error",
			transactionRouteCache: mmodel.TransactionRouteCache{
				Actions: map[string]mmodel.ActionRouteCache{
					"direct": {
						Source:        map[string]mmodel.OperationRouteCache{},
						Destination:   map[string]mmodel.OperationRouteCache{},
						Bidirectional: map[string]mmodel.OperationRouteCache{},
					},
				},
			},
			validate: &pkgTransaction.Responses{
				From:                map[string]pkgTransaction.Amount{"op-alias": {}},
				To:                  map[string]pkgTransaction.Amount{},
				OperationRoutesFrom: map[string]string{"op-alias": routeID},
				OperationRoutesTo:   map[string]string{},
			},
			operations: []mmodel.BalanceOperation{
				{
					Alias:   "op-alias",
					Amount:  pkgTransaction.Amount{Direction: "debit"},
					Balance: &mmodel.Balance{AccountType: "asset"},
				},
			},
			ledgerSettings: mmodel.LedgerSettings{
				Accounting: mmodel.AccountingValidation{
					ValidateAccountType: true,
				},
			},
			expectError: true,
			errorCode:   "0117",
		},
		{
			name: "Bidirectional fallback path succeeds",
			transactionRouteCache: mmodel.TransactionRouteCache{
				Actions: map[string]mmodel.ActionRouteCache{
					"direct": {
						Source:      map[string]mmodel.OperationRouteCache{},
						Destination: map[string]mmodel.OperationRouteCache{},
						Bidirectional: map[string]mmodel.OperationRouteCache{
							routeID: {
								OperationType: "bidirectional",
								Account: &mmodel.AccountCache{
									RuleType: "account_type",
									ValidIf:  []string{"asset"},
								},
							},
						},
					},
				},
			},
			validate: &pkgTransaction.Responses{
				From:                map[string]pkgTransaction.Amount{"op-alias": {}},
				To:                  map[string]pkgTransaction.Amount{},
				OperationRoutesFrom: map[string]string{"op-alias": routeID},
				OperationRoutesTo:   map[string]string{},
			},
			operations: []mmodel.BalanceOperation{
				{
					Alias:   "op-alias",
					Amount:  pkgTransaction.Amount{Direction: "debit"},
					Balance: &mmodel.Balance{AccountType: "asset"},
				},
			},
			ledgerSettings: mmodel.LedgerSettings{
				Accounting: mmodel.AccountingValidation{
					ValidateAccountType: true,
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actionCache := tt.transactionRouteCache.Actions["direct"]
			err := validateAccountRules(ctx, actionCache.Source, actionCache.Destination, actionCache.Bidirectional, tt.validate, tt.operations, tt.ledgerSettings)

			if tt.expectError {
				assert.Error(t, err)

				if tt.errorCode != "" {
					assert.Contains(t, err.Error(), tt.errorCode)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateDirectionRouteMatch(t *testing.T) {
	tests := []struct {
		name          string
		direction     string
		operationType string
		expectError   bool
		errorCode     string
	}{
		{
			name:          "debit direction with source route passes",
			direction:     "debit",
			operationType: "source",
			expectError:   false,
		},
		{
			name:          "credit direction with source route fails",
			direction:     "credit",
			operationType: "source",
			expectError:   true,
			errorCode:     "0152",
		},
		{
			name:          "debit direction with destination route fails",
			direction:     "debit",
			operationType: "destination",
			expectError:   true,
			errorCode:     "0152",
		},
		{
			name:          "credit direction with destination route passes",
			direction:     "credit",
			operationType: "destination",
			expectError:   false,
		},
		{
			name:          "debit direction with bidirectional route passes",
			direction:     "debit",
			operationType: "bidirectional",
			expectError:   false,
		},
		{
			name:          "credit direction with bidirectional route passes",
			direction:     "credit",
			operationType: "bidirectional",
			expectError:   false,
		},
		{
			name:          "uppercase DEBIT direction with source route passes",
			direction:     "DEBIT",
			operationType: "source",
			expectError:   false,
		},
		{
			name:          "unknown operationType returns error",
			direction:     "debit",
			operationType: "unknown",
			expectError:   true,
			errorCode:     "0103",
		},
		{
			name:          "empty operationType returns error",
			direction:     "debit",
			operationType: "",
			expectError:   true,
			errorCode:     "0103",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			operation := mmodel.BalanceOperation{
				Alias: "test-alias",
				Amount: pkgTransaction.Amount{
					Direction: tt.direction,
				},
				Balance: &mmodel.Balance{
					AccountType: "asset",
				},
			}

			routeCache := mmodel.OperationRouteCache{
				OperationType: tt.operationType,
			}

			err := validateDirectionRouteMatch(operation, routeCache)

			if tt.expectError {
				assert.Error(t, err)

				if tt.errorCode != "" {
					assert.Contains(t, err.Error(), tt.errorCode)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}

	t.Run("ON_HOLD operation skips direction validation", func(t *testing.T) {
		operation := mmodel.BalanceOperation{
			Alias: "test-alias",
			Amount: pkgTransaction.Amount{
				Direction: "credit",
				Operation: "ON_HOLD",
			},
			Balance: &mmodel.Balance{
				AccountType: "asset",
			},
		}

		routeCache := mmodel.OperationRouteCache{
			OperationType: "source",
		}

		err := validateDirectionRouteMatch(operation, routeCache)
		assert.NoError(t, err)
	})

	t.Run("RELEASE operation skips direction validation", func(t *testing.T) {
		operation := mmodel.BalanceOperation{
			Alias: "test-alias",
			Amount: pkgTransaction.Amount{
				Direction: "debit",
				Operation: "RELEASE",
			},
			Balance: &mmodel.Balance{
				AccountType: "asset",
			},
		}

		routeCache := mmodel.OperationRouteCache{
			OperationType: "destination",
		}

		err := validateDirectionRouteMatch(operation, routeCache)
		assert.NoError(t, err)
	})
}

func TestValidateCounterparts(t *testing.T) {
	tests := []struct {
		name        string
		operations  []mmodel.BalanceOperation
		routeMap    map[string]string
		expectError bool
		errorCode   string
	}{
		{
			name: "route with both debit and credit operations passes",
			operations: []mmodel.BalanceOperation{
				{
					Alias: "sender",
					Amount: pkgTransaction.Amount{
						Direction: "debit",
					},
				},
				{
					Alias: "receiver",
					Amount: pkgTransaction.Amount{
						Direction: "credit",
					},
				},
			},
			routeMap: map[string]string{
				"sender":   "route-1",
				"receiver": "route-1",
			},
			expectError: false,
		},
		{
			name: "route with only debit operations fails",
			operations: []mmodel.BalanceOperation{
				{
					Alias: "sender-1",
					Amount: pkgTransaction.Amount{
						Direction: "debit",
					},
				},
				{
					Alias: "sender-2",
					Amount: pkgTransaction.Amount{
						Direction: "debit",
					},
				},
			},
			routeMap: map[string]string{
				"sender-1": "route-1",
				"sender-2": "route-1",
			},
			expectError: true,
			errorCode:   "0151",
		},
		{
			name: "route with only credit operations fails",
			operations: []mmodel.BalanceOperation{
				{
					Alias: "receiver-1",
					Amount: pkgTransaction.Amount{
						Direction: "credit",
					},
				},
				{
					Alias: "receiver-2",
					Amount: pkgTransaction.Amount{
						Direction: "credit",
					},
				},
			},
			routeMap: map[string]string{
				"receiver-1": "route-1",
				"receiver-2": "route-1",
			},
			expectError: true,
			errorCode:   "0151",
		},
		{
			name: "multiple routes all with counterparts passes",
			operations: []mmodel.BalanceOperation{
				{
					Alias: "sender-a",
					Amount: pkgTransaction.Amount{
						Direction: "debit",
					},
				},
				{
					Alias: "receiver-a",
					Amount: pkgTransaction.Amount{
						Direction: "credit",
					},
				},
				{
					Alias: "sender-b",
					Amount: pkgTransaction.Amount{
						Direction: "debit",
					},
				},
				{
					Alias: "receiver-b",
					Amount: pkgTransaction.Amount{
						Direction: "credit",
					},
				},
			},
			routeMap: map[string]string{
				"sender-a":   "route-1",
				"receiver-a": "route-1",
				"sender-b":   "route-2",
				"receiver-b": "route-2",
			},
			expectError: false,
		},
		{
			name: "multiple routes with one missing counterpart fails",
			operations: []mmodel.BalanceOperation{
				{
					Alias: "sender-a",
					Amount: pkgTransaction.Amount{
						Direction: "debit",
					},
				},
				{
					Alias: "receiver-a",
					Amount: pkgTransaction.Amount{
						Direction: "credit",
					},
				},
				{
					Alias: "sender-b",
					Amount: pkgTransaction.Amount{
						Direction: "debit",
					},
				},
			},
			routeMap: map[string]string{
				"sender-a":   "route-1",
				"receiver-a": "route-1",
				"sender-b":   "route-2",
			},
			expectError: true,
			errorCode:   "0151",
		},
		{
			name:        "nil routeMap passes with no routes to validate",
			operations:  []mmodel.BalanceOperation{{Alias: "sender", Amount: pkgTransaction.Amount{Direction: "debit"}}},
			routeMap:    nil,
			expectError: false,
		},
		{
			name:        "empty routeMap passes with no routes to validate",
			operations:  []mmodel.BalanceOperation{{Alias: "sender", Amount: pkgTransaction.Amount{Direction: "debit"}}},
			routeMap:    map[string]string{},
			expectError: false,
		},
		{
			name:       "empty operations slice passes",
			operations: []mmodel.BalanceOperation{},
			routeMap: map[string]string{
				"sender":   "route-1",
				"receiver": "route-1",
			},
			expectError: false,
		},
		{
			name: "uppercase directions DEBIT and CREDIT passes",
			operations: []mmodel.BalanceOperation{
				{
					Alias: "sender",
					Amount: pkgTransaction.Amount{
						Direction: "DEBIT",
					},
				},
				{
					Alias: "receiver",
					Amount: pkgTransaction.Amount{
						Direction: "CREDIT",
					},
				},
			},
			routeMap: map[string]string{
				"sender":   "route-1",
				"receiver": "route-1",
			},
			expectError: false,
		},
		{
			name: "empty-string direction fails missing counterpart",
			operations: []mmodel.BalanceOperation{
				{
					Alias: "sender",
					Amount: pkgTransaction.Amount{
						Direction: "",
					},
				},
				{
					Alias: "receiver",
					Amount: pkgTransaction.Amount{
						Direction: "credit",
					},
				},
			},
			routeMap: map[string]string{
				"sender":   "route-1",
				"receiver": "route-1",
			},
			expectError: true,
			errorCode:   "0151",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCounterparts(tt.operations, tt.routeMap)

			if tt.expectError {
				assert.Error(t, err)

				if tt.errorCode != "" {
					assert.Contains(t, err.Error(), tt.errorCode)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
