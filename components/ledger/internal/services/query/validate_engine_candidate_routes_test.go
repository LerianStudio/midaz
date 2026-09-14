// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	transactionredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func TestValidateAccountingRulesReturnsEngineCandidateRoutesFromFullCache(t *testing.T) {
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionRouteID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	operationRouteID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	fixedDate := time.Date(2026, time.September, 8, 15, 0, 0, 0, time.UTC)

	tests := []struct {
		name               string
		overdraft          *mmodel.AccountingEntry
		wantOverdraftCache bool
	}{
		{
			name: "configured overdraft debit and credit rubrics survive primary-only validation",
			overdraft: &mmodel.AccountingEntry{
				Debit:  &mmodel.AccountingRubric{Code: "OD-DEBIT", Description: "Overdraft usage"},
				Credit: &mmodel.AccountingRubric{Code: "OD-CREDIT", Description: "Overdraft repayment"},
			},
			wantOverdraftCache: true,
		},
		{
			name:               "primary-only route validates without an overdraft rubric",
			wantOverdraftCache: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ledgerRepo := ledger.NewMockRepository(ctrl)
			routeRepo := transactionroute.NewMockRepository(ctrl)
			redisRepo := transactionredis.NewMockRedisRepository(ctrl)
			transactionRoute := &mmodel.TransactionRoute{
				ID: transactionRouteID, OrganizationID: organizationID, LedgerID: ledgerID,
				Title: "Primary source", CreatedAt: fixedDate, UpdatedAt: fixedDate,
				OperationRoutes: []mmodel.OperationRoute{{
					ID: operationRouteID, OrganizationID: organizationID, LedgerID: ledgerID,
					Title: "Customer debit", OperationType: "source", CreatedAt: fixedDate, UpdatedAt: fixedDate,
					AccountingEntries: &mmodel.AccountingEntries{
						Direct: &mmodel.AccountingEntry{
							Debit: &mmodel.AccountingRubric{Code: "PRIMARY-DEBIT", Description: "Primary debit"},
						},
						Overdraft: test.overdraft,
					},
				}},
			}
			expectedCache := transactionRoute.ToCache()
			cacheKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

			ledgerRepo.EXPECT().
				GetSettings(gomock.Any(), organizationID, ledgerID).
				Return(map[string]any{"accounting": map[string]any{"validateRoutes": true}}, nil)
			redisRepo.EXPECT().
				GetBytes(gomock.Any(), cacheKey).
				Return(nil, redis.Nil)
			routeRepo.EXPECT().
				FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
				Return(transactionRoute, nil)
			redisRepo.EXPECT().
				SetBytes(gomock.Any(), cacheKey, gomock.Any(), time.Duration(0)).
				DoAndReturn(func(_ context.Context, _ string, encoded []byte, _ time.Duration) error {
					var stored mmodel.TransactionRouteCache
					require.NoError(t, stored.FromMsgpack(encoded))
					require.Equal(t, expectedCache, stored)

					return nil
				})

			uc := &UseCase{
				LedgerRepo: ledgerRepo, TransactionRouteRepo: routeRepo, TransactionRedisRepo: redisRepo,
			}
			operations := []mmodel.BalanceOperation{{
				Alias: "0#@source#default",
				Amount: mtransaction.Amount{
					Asset: "USD", Value: decimal.NewFromInt(30), Direction: constant.DirectionDebit,
				},
				Balance: &mmodel.Balance{
					ID: "55555555-5555-4555-8555-555555555555", OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
					AccountID: "66666666-6666-4666-8666-666666666666", Alias: "@source", Key: constant.DefaultBalanceKey,
					AssetCode: "USD", Available: decimal.NewFromInt(100), Version: 7, AccountType: "deposit",
					AllowSending: true, AllowReceiving: true, CreatedAt: fixedDate, UpdatedAt: fixedDate,
				},
			}}
			validate := &mtransaction.Responses{
				TransactionRoute: transactionRouteID.String(),
				From: map[string]mtransaction.Amount{
					"0#@source#default": {Asset: "USD", Value: decimal.NewFromInt(30), Direction: constant.DirectionDebit},
				},
				To:                  map[string]mtransaction.Amount{},
				OperationRoutesFrom: map[string]string{"0#@source#default": operationRouteID.String()},
				OperationRoutesTo:   map[string]string{},
			}

			cache, err := uc.ValidateAccountingRules(
				context.Background(), organizationID, ledgerID, operations, validate, constant.ActionDirect,
			)
			require.NoError(t, err)
			require.NotNil(t, cache)
			require.Equal(t, expectedCache, *cache)
			require.Nil(t, operations[0].Balance.Settings)
			require.Equal(t, "100", operations[0].Balance.Available.String())
			require.Len(t, operations, 1)
			require.Equal(t, constant.DefaultBalanceKey, operations[0].Balance.Key)

			direct, ok := cache.Actions[constant.ActionDirect]
			require.True(t, ok)
			require.Contains(t, direct.Source, operationRouteID.String())
			if !test.wantOverdraftCache {
				require.NotContains(t, cache.Actions, constant.ActionOverdraft)
				return
			}

			overdraft, ok := cache.Actions[constant.ActionOverdraft]
			require.True(t, ok)
			candidate, ok := overdraft.Source[operationRouteID.String()]
			require.True(t, ok)
			require.NotNil(t, candidate.AccountingEntries)
			require.NotNil(t, candidate.AccountingEntries.Overdraft)
			require.Equal(t, "OD-DEBIT", candidate.AccountingEntries.Overdraft.Debit.Code)
			require.Equal(t, "Overdraft usage", candidate.AccountingEntries.Overdraft.Debit.Description)
			require.Equal(t, "OD-CREDIT", candidate.AccountingEntries.Overdraft.Credit.Code)
			require.Equal(t, "Overdraft repayment", candidate.AccountingEntries.Overdraft.Credit.Description)
		})
	}
}
