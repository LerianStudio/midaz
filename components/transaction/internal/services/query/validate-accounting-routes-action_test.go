// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestValidateAccountingRules_ActionParam verifies that ValidateAccountingRules
// accepts an action parameter and uses it to look up action-specific routes
// from TransactionRouteCache.Actions[action].
func TestValidateAccountingRules_ActionParam(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	sourceRouteID := libCommons.GenerateUUIDv7().String()

	ctx := context.Background()

	tests := []struct {
		name        string
		action      string
		cache       mmodel.TransactionRouteCache
		operations  []mmodel.BalanceOperation
		validate    *pkgTransaction.Responses
		expectError bool
		errorCode   string
	}{
		{
			name:   "action=direct uses direct-specific routes from Actions map",
			action: constant.ActionDirect,
			cache: mmodel.TransactionRouteCache{
				Actions: map[string]mmodel.ActionRouteCache{
					constant.ActionDirect: {
						Source: map[string]mmodel.OperationRouteCache{
							sourceRouteID: {OperationType: "source"},
						},
						Destination:   map[string]mmodel.OperationRouteCache{},
						Bidirectional: map[string]mmodel.OperationRouteCache{},
					},
				},
			},
			operations: []mmodel.BalanceOperation{
				{
					Alias:   "sender",
					Amount:  pkgTransaction.Amount{Direction: "debit"},
					Balance: &mmodel.Balance{AccountType: "asset"},
				},
			},
			validate: &pkgTransaction.Responses{
				TransactionRoute:    transactionRouteID.String(),
				From:                map[string]pkgTransaction.Amount{"sender": {}},
				To:                  map[string]pkgTransaction.Amount{},
				OperationRoutesFrom: map[string]string{"sender": sourceRouteID},
				OperationRoutesTo:   map[string]string{},
			},
			expectError: false,
		},
		{
			name:   "action=hold uses hold-specific routes from Actions map",
			action: constant.ActionHold,
			cache: mmodel.TransactionRouteCache{
				Actions: map[string]mmodel.ActionRouteCache{
					constant.ActionHold: {
						Source: map[string]mmodel.OperationRouteCache{
							sourceRouteID: {OperationType: "source"},
						},
						Destination:   map[string]mmodel.OperationRouteCache{},
						Bidirectional: map[string]mmodel.OperationRouteCache{},
					},
				},
			},
			operations: []mmodel.BalanceOperation{
				{
					Alias:   "sender",
					Amount:  pkgTransaction.Amount{Direction: "debit"},
					Balance: &mmodel.Balance{AccountType: "asset"},
				},
			},
			validate: &pkgTransaction.Responses{
				TransactionRoute:    transactionRouteID.String(),
				From:                map[string]pkgTransaction.Amount{"sender": {}},
				To:                  map[string]pkgTransaction.Amount{},
				OperationRoutesFrom: map[string]string{"sender": sourceRouteID},
				OperationRoutesTo:   map[string]string{},
			},
			expectError: false,
		},
		{
			name:   "missing action in cache returns ErrNoRoutesForAction",
			action: constant.ActionCommit,
			cache: mmodel.TransactionRouteCache{
				Actions: map[string]mmodel.ActionRouteCache{
					constant.ActionDirect: {
						Source:        map[string]mmodel.OperationRouteCache{},
						Destination:   map[string]mmodel.OperationRouteCache{},
						Bidirectional: map[string]mmodel.OperationRouteCache{},
					},
				},
			},
			operations: []mmodel.BalanceOperation{
				{
					Alias:   "sender",
					Amount:  pkgTransaction.Amount{Direction: "debit"},
					Balance: &mmodel.Balance{AccountType: "asset"},
				},
			},
			validate: &pkgTransaction.Responses{
				TransactionRoute:    transactionRouteID.String(),
				From:                map[string]pkgTransaction.Amount{"sender": {}},
				To:                  map[string]pkgTransaction.Amount{},
				OperationRoutesFrom: map[string]string{"sender": sourceRouteID},
				OperationRoutesTo:   map[string]string{},
			},
			expectError: true,
			errorCode:   "0157", // ErrNoRoutesForAction
		},
		{
			name:   "empty Actions map returns ErrNoRoutesForAction",
			action: constant.ActionDirect,
			cache: mmodel.TransactionRouteCache{
				Actions: map[string]mmodel.ActionRouteCache{},
			},
			operations: []mmodel.BalanceOperation{
				{
					Alias:   "sender",
					Amount:  pkgTransaction.Amount{Direction: "debit"},
					Balance: &mmodel.Balance{AccountType: "asset"},
				},
			},
			validate: &pkgTransaction.Responses{
				TransactionRoute:    transactionRouteID.String(),
				From:                map[string]pkgTransaction.Amount{"sender": {}},
				To:                  map[string]pkgTransaction.Amount{},
				OperationRoutesFrom: map[string]string{"sender": sourceRouteID},
				OperationRoutesTo:   map[string]string{},
			},
			expectError: true,
			errorCode:   "0157", // ErrNoRoutesForAction
		},
		{
			name:   "action=revert uses revert-specific routes",
			action: constant.ActionRevert,
			cache: mmodel.TransactionRouteCache{
				Actions: map[string]mmodel.ActionRouteCache{
					constant.ActionRevert: {
						Source:      map[string]mmodel.OperationRouteCache{},
						Destination: map[string]mmodel.OperationRouteCache{},
						Bidirectional: map[string]mmodel.OperationRouteCache{
							sourceRouteID: {OperationType: "bidirectional"},
						},
					},
				},
			},
			operations: []mmodel.BalanceOperation{
				{
					Alias:   "sender",
					Amount:  pkgTransaction.Amount{Direction: "debit"},
					Balance: &mmodel.Balance{AccountType: "asset"},
				},
			},
			validate: &pkgTransaction.Responses{
				TransactionRoute:    transactionRouteID.String(),
				From:                map[string]pkgTransaction.Amount{"sender": {}},
				To:                  map[string]pkgTransaction.Amount{},
				OperationRoutesFrom: map[string]string{"sender": sourceRouteID},
				OperationRoutesTo:   map[string]string{},
			},
			expectError: false,
		},
		{
			name:   "action=cancel uses cancel-specific routes",
			action: constant.ActionCancel,
			cache: mmodel.TransactionRouteCache{
				Actions: map[string]mmodel.ActionRouteCache{
					constant.ActionCancel: {
						Source: map[string]mmodel.OperationRouteCache{
							sourceRouteID: {OperationType: "source"},
						},
						Destination:   map[string]mmodel.OperationRouteCache{},
						Bidirectional: map[string]mmodel.OperationRouteCache{},
					},
				},
			},
			operations: []mmodel.BalanceOperation{
				{
					Alias:   "sender",
					Amount:  pkgTransaction.Amount{Direction: "debit"},
					Balance: &mmodel.Balance{AccountType: "asset"},
				},
			},
			validate: &pkgTransaction.Responses{
				TransactionRoute:    transactionRouteID.String(),
				From:                map[string]pkgTransaction.Amount{"sender": {}},
				To:                  map[string]pkgTransaction.Amount{},
				OperationRoutesFrom: map[string]string{"sender": sourceRouteID},
				OperationRoutesTo:   map[string]string{},
			},
			expectError: false,
		},
		{
			name:   "empty action string returns ErrNoRoutesForAction",
			action: "",
			cache: mmodel.TransactionRouteCache{
				Actions: map[string]mmodel.ActionRouteCache{
					constant.ActionDirect: {
						Source: map[string]mmodel.OperationRouteCache{
							sourceRouteID: {OperationType: "source"},
						},
						Destination:   map[string]mmodel.OperationRouteCache{},
						Bidirectional: map[string]mmodel.OperationRouteCache{},
					},
				},
			},
			operations: []mmodel.BalanceOperation{
				{
					Alias:   "sender",
					Amount:  pkgTransaction.Amount{Direction: "debit"},
					Balance: &mmodel.Balance{AccountType: "asset"},
				},
			},
			validate: &pkgTransaction.Responses{
				TransactionRoute:    transactionRouteID.String(),
				From:                map[string]pkgTransaction.Amount{"sender": {}},
				To:                  map[string]pkgTransaction.Amount{},
				OperationRoutesFrom: map[string]string{"sender": sourceRouteID},
				OperationRoutesTo:   map[string]string{},
			},
			expectError: true,
			errorCode:   "0157",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSettingsPort := mbootstrap.NewMockSettingsPort(ctrl)
			mockSettingsPort.EXPECT().
				GetLedgerSettings(gomock.Any(), organizationID, ledgerID).
				Return(map[string]any{
					"accounting": map[string]any{
						"validateRoutes": true,
					},
				}, nil)

			// Expect the cache to be fetched from Redis
			cacheData, err := tt.cache.ToMsgpack()
			require.NoError(t, err)
			mockRedisRepo.EXPECT().
				GetBytes(gomock.Any(), gomock.Any()).
				Return(cacheData, nil)

			uc := &UseCase{
				RedisRepo:    mockRedisRepo,
				SettingsPort: mockSettingsPort,
			}

			// This call must include the action parameter.
			// Current signature: ValidateAccountingRules(ctx, orgID, ledgerID, operations, validate)
			// Expected signature: ValidateAccountingRules(ctx, orgID, ledgerID, operations, validate, action)
			err = uc.ValidateAccountingRules(ctx, organizationID, ledgerID, tt.operations, tt.validate, tt.action)

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

// TestValidateAccountingRules_ActionFilteredRouteCount verifies that route count
// validation operates on the action-filtered subset of routes, not the full cache.
func TestValidateAccountingRules_ActionFilteredRouteCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	directSourceRouteID := libCommons.GenerateUUIDv7().String()
	directDestRouteID := libCommons.GenerateUUIDv7().String()
	holdSourceRouteID := libCommons.GenerateUUIDv7().String()

	ctx := context.Background()

	// Cache has routes for both "direct" and "hold" actions,
	// but we request action="direct" which has 2 routes (1 source + 1 destination).
	// The count validation should use only the direct action's routes.
	cache := mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			constant.ActionDirect: {
				Source: map[string]mmodel.OperationRouteCache{
					directSourceRouteID: {OperationType: "source"},
				},
				Destination: map[string]mmodel.OperationRouteCache{
					directDestRouteID: {OperationType: "destination"},
				},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
			constant.ActionHold: {
				Source: map[string]mmodel.OperationRouteCache{
					holdSourceRouteID: {OperationType: "source"},
				},
				Destination:   map[string]mmodel.OperationRouteCache{},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}

	mockSettingsPort := mbootstrap.NewMockSettingsPort(ctrl)
	mockSettingsPort.EXPECT().
		GetLedgerSettings(gomock.Any(), organizationID, ledgerID).
		Return(map[string]any{
			"accounting": map[string]any{
				"validateRoutes": true,
			},
		}, nil)

	cacheData, err := cache.ToMsgpack()
	require.NoError(t, err)
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(cacheData, nil)

	uc := &UseCase{
		RedisRepo:    mockRedisRepo,
		SettingsPort: mockSettingsPort,
	}

	validate := &pkgTransaction.Responses{
		TransactionRoute:    transactionRouteID.String(),
		From:                map[string]pkgTransaction.Amount{"sender": {}},
		To:                  map[string]pkgTransaction.Amount{"receiver": {}},
		OperationRoutesFrom: map[string]string{"sender": directSourceRouteID},
		OperationRoutesTo:   map[string]string{"receiver": directDestRouteID},
	}

	operations := []mmodel.BalanceOperation{
		{
			Alias:   "sender",
			Amount:  pkgTransaction.Amount{Direction: "debit"},
			Balance: &mmodel.Balance{AccountType: "asset"},
		},
		{
			Alias:   "receiver",
			Amount:  pkgTransaction.Amount{Direction: "credit"},
			Balance: &mmodel.Balance{AccountType: "liability"},
		},
	}

	// Should pass: 2 operations match 2 direct-action routes (1 source + 1 destination)
	err = uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate, constant.ActionDirect)
	assert.NoError(t, err)
}

// TestValidateAccountingRules_ActionFilteredAccountRules verifies that account rule
// validation operates on the action-filtered subset of routes.
func TestValidateAccountingRules_ActionFilteredAccountRules(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	sourceRouteID := libCommons.GenerateUUIDv7().String()

	ctx := context.Background()

	// direct-action route requires account_type=asset;
	// hold-action route requires account_type=liability.
	// We request action=direct with an "asset" account, which should pass.
	cache := mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			constant.ActionDirect: {
				Source: map[string]mmodel.OperationRouteCache{
					sourceRouteID: {
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
			constant.ActionHold: {
				Source: map[string]mmodel.OperationRouteCache{
					sourceRouteID: {
						OperationType: "source",
						Account: &mmodel.AccountCache{
							RuleType: "account_type",
							ValidIf:  []string{"liability"},
						},
					},
				},
				Destination:   map[string]mmodel.OperationRouteCache{},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}

	mockSettingsPort := mbootstrap.NewMockSettingsPort(ctrl)
	mockSettingsPort.EXPECT().
		GetLedgerSettings(gomock.Any(), organizationID, ledgerID).
		Return(map[string]any{
			"accounting": map[string]any{
				"validateRoutes":      true,
				"validateAccountType": true,
			},
		}, nil)

	cacheData, err := cache.ToMsgpack()
	require.NoError(t, err)
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(cacheData, nil)

	uc := &UseCase{
		RedisRepo:    mockRedisRepo,
		SettingsPort: mockSettingsPort,
	}

	validate := &pkgTransaction.Responses{
		TransactionRoute:    transactionRouteID.String(),
		From:                map[string]pkgTransaction.Amount{"sender": {}},
		To:                  map[string]pkgTransaction.Amount{},
		OperationRoutesFrom: map[string]string{"sender": sourceRouteID},
		OperationRoutesTo:   map[string]string{},
	}

	operations := []mmodel.BalanceOperation{
		{
			Alias:   "sender",
			Amount:  pkgTransaction.Amount{Direction: "debit"},
			Balance: &mmodel.Balance{AccountType: "asset"},
		},
	}

	// action=direct: asset account should pass against direct route (requires asset)
	err = uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate, constant.ActionDirect)
	assert.NoError(t, err)
}

// TestValidateAccountingRules_ActionAccountTypeMismatch verifies that when
// validateAccountType is enabled, an account type mismatch under action-filtered
// routes returns the appropriate error.
func TestValidateAccountingRules_ActionAccountTypeMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	sourceRouteID := libCommons.GenerateUUIDv7().String()

	ctx := context.Background()

	// direct-action route requires account_type=liability,
	// but the operation uses account_type=asset → should fail
	cache := mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			constant.ActionDirect: {
				Source: map[string]mmodel.OperationRouteCache{
					sourceRouteID: {
						OperationType: "source",
						Account: &mmodel.AccountCache{
							RuleType: "account_type",
							ValidIf:  []string{"liability"},
						},
					},
				},
				Destination:   map[string]mmodel.OperationRouteCache{},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}

	mockSettingsPort := mbootstrap.NewMockSettingsPort(ctrl)
	mockSettingsPort.EXPECT().
		GetLedgerSettings(gomock.Any(), organizationID, ledgerID).
		Return(map[string]any{
			"accounting": map[string]any{
				"validateRoutes":      true,
				"validateAccountType": true,
			},
		}, nil)

	cacheData, err := cache.ToMsgpack()
	require.NoError(t, err)
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(cacheData, nil)

	uc := &UseCase{
		RedisRepo:    mockRedisRepo,
		SettingsPort: mockSettingsPort,
	}

	validate := &pkgTransaction.Responses{
		TransactionRoute:    transactionRouteID.String(),
		From:                map[string]pkgTransaction.Amount{"sender": {}},
		To:                  map[string]pkgTransaction.Amount{},
		OperationRoutesFrom: map[string]string{"sender": sourceRouteID},
		OperationRoutesTo:   map[string]string{},
	}

	operations := []mmodel.BalanceOperation{
		{
			Alias:   "sender",
			Amount:  pkgTransaction.Amount{Direction: "debit"},
			Balance: &mmodel.Balance{AccountType: "asset"},
		},
	}

	// action=direct with asset account against route requiring liability → should fail
	err = uc.ValidateAccountingRules(ctx, organizationID, ledgerID, operations, validate, constant.ActionDirect)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "0119")
}
