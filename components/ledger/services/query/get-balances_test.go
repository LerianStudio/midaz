// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/rabbitmq"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetBalances(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)

	uc := &UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
		RabbitMQRepo:         mockRabbitMQRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	t.Run("get balances from redis and database", func(t *testing.T) {
		aliases := []string{"alias1#default", "alias2#default", "alias3#default"}

		fromAmount := pkgTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromFloat(50),
			Operation: constant.DEBIT,
		}
		toAmount2 := pkgTransaction.Amount{
			Asset:     "EUR",
			Value:     decimal.NewFromFloat(40),
			Operation: constant.CREDIT,
		}
		toAmount3 := pkgTransaction.Amount{
			Asset:     "GBP",
			Value:     decimal.NewFromFloat(30),
			Operation: constant.CREDIT,
		}

		validate := &pkgTransaction.Responses{
			Aliases: aliases,
			From: map[string]pkgTransaction.Amount{
				"alias1": fromAmount,
			},
			To: map[string]pkgTransaction.Amount{
				"alias2": toAmount2,
				"alias3": toAmount3,
			},
		}

		balanceRedis := mmodel.BalanceRedis{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			Available:      decimal.NewFromFloat(100),
			OnHold:         decimal.NewFromFloat(0),
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   1,
			AllowReceiving: 1,
			AssetCode:      "USD",
		}
		balanceRedisJSON, _ := json.Marshal(balanceRedis)

		databaseBalances := []*mmodel.Balance{
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias2",
				Key:            "default",
				Available:      decimal.NewFromFloat(100),
				OnHold:         decimal.NewFromFloat(0),
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "EUR",
			},
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias3",
				Key:            "default",
				Available:      decimal.NewFromFloat(300),
				OnHold:         decimal.NewFromFloat(0),
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "GBP",
			},
		}

		key1 := utils.BalanceInternalKey(organizationID, ledgerID, "alias1#default")
		key2 := utils.BalanceInternalKey(organizationID, ledgerID, "alias2#default")
		key3 := utils.BalanceInternalKey(organizationID, ledgerID, "alias3#default")

		mockRedisRepo.
			EXPECT().
			Get(gomock.Any(), key1).
			Return(string(balanceRedisJSON), nil).
			Times(1)
		mockRedisRepo.
			EXPECT().
			Get(gomock.Any(), key2).
			Return("", nil).
			Times(1)
		mockRedisRepo.
			EXPECT().
			Get(gomock.Any(), key3).
			Return("", nil).
			Times(1)

		mockBalanceRepo.
			EXPECT().
			ListByAliasesWithKeys(gomock.Any(), organizationID, ledgerID, []string{"alias2#default", "alias3#default"}).
			Return(databaseBalances, nil).
			Times(1)

		mockRedisRepo.
			EXPECT().
			ProcessBalanceAtomicOperation(
				gomock.Any(),
				organizationID,
				ledgerID,
				gomock.Any(), // transactionID
				constant.CREATED,
				false,
				gomock.Any(), // balance operations
			).
			Return(&mmodel.BalanceAtomicResult{
				Before: []*mmodel.Balance{
					{
						ID:             balanceRedis.ID,
						AccountID:      balanceRedis.AccountID,
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Alias:          "alias1",
						Available:      balanceRedis.Available,
						OnHold:         balanceRedis.OnHold,
						Version:        balanceRedis.Version,
						AccountType:    balanceRedis.AccountType,
						AllowSending:   balanceRedis.AllowSending == 1,
						AllowReceiving: balanceRedis.AllowReceiving == 1,
						AssetCode:      balanceRedis.AssetCode,
					},
					databaseBalances[0],
					databaseBalances[1],
				},
				After: []*mmodel.Balance{},
			}, nil).
			Times(1)

		transactionID := uuid.New()
		balancesBefore, balancesAfter, _, err := uc.GetBalances(ctx, organizationID, ledgerID, transactionID, nil, validate, constant.CREATED, constant.ActionDirect)
		assert.NoError(t, err)
		assert.Len(t, balancesBefore, 3)
		assert.NotNil(t, balancesAfter, "after balances should not be nil")

		sort.Slice(balancesBefore, func(i, j int) bool {
			return balancesBefore[i].Alias < balancesBefore[j].Alias
		})

		assert.Equal(t, "alias1", balancesBefore[0].Alias)
		assert.Equal(t, balanceRedis.ID, balancesBefore[0].ID)

		assert.Equal(t, "alias2", balancesBefore[1].Alias)
		assert.Equal(t, databaseBalances[0].ID, balancesBefore[1].ID)

		assert.Equal(t, "alias3", balancesBefore[2].Alias)
		assert.Equal(t, databaseBalances[1].ID, balancesBefore[2].ID)
	})

	t.Run("all balances from redis", func(t *testing.T) {
		aliases := []string{"alias1#default", "alias2#default"}
		fromAmount := pkgTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromFloat(50),
			Operation: constant.DEBIT,
		}

		toAmount := pkgTransaction.Amount{
			Asset:     "EUR",
			Value:     decimal.NewFromFloat(40),
			Operation: constant.CREDIT,
		}

		validate := &pkgTransaction.Responses{
			Aliases: aliases,
			From: map[string]pkgTransaction.Amount{
				"alias1": fromAmount,
			},
			To: map[string]pkgTransaction.Amount{
				"alias2": toAmount,
			},
		}

		balance1 := mmodel.BalanceRedis{
			ID:        uuid.New().String(),
			AccountID: uuid.New().String(),
			Available: decimal.NewFromFloat(100),
			OnHold:    decimal.NewFromFloat(0),

			Version:        1,
			AccountType:    "deposit",
			AllowSending:   1,
			AllowReceiving: 1,
			AssetCode:      "USD",
		}
		balance1JSON, _ := json.Marshal(balance1)

		balance2 := mmodel.BalanceRedis{
			ID:        uuid.New().String(),
			AccountID: uuid.New().String(),
			Available: decimal.NewFromFloat(200),
			OnHold:    decimal.NewFromFloat(0),

			Version:        1,
			AccountType:    "deposit",
			AllowSending:   1,
			AllowReceiving: 1,
			AssetCode:      "EUR",
		}
		balance2JSON, _ := json.Marshal(balance2)

		internalKey1 := utils.BalanceInternalKey(organizationID, ledgerID, "alias1#default")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey1).
			Return(string(balance1JSON), nil).
			Times(1)

		internalKey2 := utils.BalanceInternalKey(organizationID, ledgerID, "alias2#default")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey2).
			Return(string(balance2JSON), nil).
			Times(1)

		mockRedisRepo.EXPECT().
			ProcessBalanceAtomicOperation(
				gomock.Any(),
				organizationID,
				ledgerID,
				gomock.Any(), // transactionID
				constant.CREATED,
				false,
				gomock.Any(), // balance operations
			).
			Return(&mmodel.BalanceAtomicResult{
				Before: []*mmodel.Balance{
					{
						ID:             balance1.ID,
						AccountID:      balance1.AccountID,
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Alias:          "alias1",
						Available:      balance1.Available,
						OnHold:         balance1.OnHold,
						Version:        balance1.Version,
						AccountType:    balance1.AccountType,
						AllowSending:   balance1.AllowSending == 1,
						AllowReceiving: balance1.AllowReceiving == 1,
						AssetCode:      balance1.AssetCode,
					},
					{
						ID:             balance2.ID,
						AccountID:      balance2.AccountID,
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Alias:          "alias2",
						Available:      balance2.Available,
						OnHold:         balance2.OnHold,
						Version:        balance2.Version,
						AccountType:    balance2.AccountType,
						AllowSending:   balance2.AllowSending == 1,
						AllowReceiving: balance2.AllowReceiving == 1,
						AssetCode:      balance2.AssetCode,
					},
				},
				After: []*mmodel.Balance{},
			}, nil).
			Times(1)

		transactionID := uuid.New()
		balances, _, _, err := uc.GetBalances(ctx, organizationID, ledgerID, transactionID, nil, validate, constant.CREATED, constant.ActionDirect)

		assert.NoError(t, err)
		assert.Len(t, balances, 2)
	})
}

func TestGetAccountAndLock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	organizationID := uuid.MustParse("ad0032e5-ccf5-45f4-a3b2-12045e71b38a")
	ledgerID := uuid.MustParse("5d8ac48a-af68-4544-9bf8-80c3cc0715f4")
	uc := UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	t.Run("lock balances successfully", func(t *testing.T) {
		balanceID1 := uuid.MustParse("c7d0fa07-3e11-4105-a0fc-6fa46834ce66")
		accountID1 := uuid.MustParse("bad0ddef-d697-4a4e-840d-1f5380de4607")

		fromAmount := pkgTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromFloat(50),
			Operation: constant.DEBIT,
		}

		validate := &pkgTransaction.Responses{
			Aliases: []string{"alias1"},
			From: map[string]pkgTransaction.Amount{
				"alias1": fromAmount,
			},
		}

		balances := []*mmodel.Balance{
			{
				ID:             balanceID1.String(),
				AccountID:      accountID1.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias1",
				Available:      decimal.NewFromFloat(100),
				OnHold:         decimal.NewFromFloat(0),
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "USD",
			},
		}

		mockRedisRepo.EXPECT().
			ProcessBalanceAtomicOperation(
				gomock.Any(),
				organizationID,
				ledgerID,
				gomock.Any(), // transactionID
				constant.CREATED,
				false,
				gomock.Any(), // balance operations
			).
			Return(&mmodel.BalanceAtomicResult{Before: balances, After: balances}, nil)

		transactionID := uuid.New()
		result, _, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, transactionID, nil, validate, balances, constant.CREATED, constant.ActionDirect)

		assert.NoError(t, err)
		assert.Len(t, result.Before, 1)
	})
}

func TestGetAccountAndLock_DoubleEntrySplitting(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("ad0032e5-ccf5-45f4-a3b2-12045e71b38a")
	ledgerID := uuid.MustParse("5d8ac48a-af68-4544-9bf8-80c3cc0715f4")

	tests := []struct {
		name             string
		fromAmount       pkgTransaction.Amount
		transactionType  string
		expectedOpsCount int
		expectedOp1      string
		expectedOp2      string
	}{
		{
			name: "CANCELED with RouteValidationEnabled produces 2 ops (RELEASE + CREDIT)",
			fromAmount: pkgTransaction.Amount{
				Asset:                  "USD",
				Value:                  decimal.NewFromFloat(50),
				Operation:              constant.RELEASE,
				TransactionType:        constant.CANCELED,
				RouteValidationEnabled: true,
			},
			transactionType:  constant.CANCELED,
			expectedOpsCount: 2,
			expectedOp1:      constant.RELEASE,
			expectedOp2:      constant.CREDIT,
		},
		{
			name: "CANCELED without route flag produces 1 op (RELEASE)",
			fromAmount: pkgTransaction.Amount{
				Asset:           "USD",
				Value:           decimal.NewFromFloat(50),
				Operation:       constant.RELEASE,
				TransactionType: constant.CANCELED,
			},
			transactionType:  constant.CANCELED,
			expectedOpsCount: 1,
			expectedOp1:      constant.RELEASE,
		},
		{
			name: "PENDING with RouteValidationEnabled produces 2 ops (DEBIT + ONHOLD)",
			fromAmount: pkgTransaction.Amount{
				Asset:                  "USD",
				Value:                  decimal.NewFromFloat(50),
				Operation:              constant.ONHOLD,
				TransactionType:        constant.PENDING,
				RouteValidationEnabled: true,
			},
			transactionType:  constant.PENDING,
			expectedOpsCount: 2,
			expectedOp1:      constant.DEBIT,
			expectedOp2:      constant.ONHOLD,
		},
		{
			name: "PENDING without route flag produces 1 op (ONHOLD)",
			fromAmount: pkgTransaction.Amount{
				Asset:           "USD",
				Value:           decimal.NewFromFloat(50),
				Operation:       constant.ONHOLD,
				TransactionType: constant.PENDING,
			},
			transactionType:  constant.PENDING,
			expectedOpsCount: 1,
			expectedOp1:      constant.ONHOLD,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)

			uc := UseCase{
				TransactionRedisRepo: mockRedisRepo,
			}

			balanceID := uuid.New().String()
			accountID := uuid.New().String()

			validate := &pkgTransaction.Responses{
				Aliases: []string{"alias1#default"},
				From: map[string]pkgTransaction.Amount{
					"0#alias1#default": tt.fromAmount,
				},
			}

			balances := []*mmodel.Balance{
				{
					ID:             balanceID,
					AccountID:      accountID,
					OrganizationID: organizationID.String(),
					LedgerID:       ledgerID.String(),
					Alias:          "alias1",
					Key:            "default",
					Available:      decimal.NewFromFloat(100),
					OnHold:         decimal.NewFromFloat(50),
					Version:        1,
					AccountType:    "deposit",
					AllowSending:   true,
					AllowReceiving: true,
					AssetCode:      "USD",
				},
			}

			var capturedOps []mmodel.BalanceOperation

			mockRedisRepo.EXPECT().
				ProcessBalanceAtomicOperation(
					gomock.Any(),
					organizationID,
					ledgerID,
					gomock.Any(),
					tt.transactionType,
					false,
					gomock.Any(),
				).
				DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ uuid.UUID, _ string, _ bool, ops []mmodel.BalanceOperation) (*mmodel.BalanceAtomicResult, error) {
					capturedOps = ops
					return &mmodel.BalanceAtomicResult{Before: balances, After: balances}, nil
				})

			transactionID := uuid.New()
			result, _, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, transactionID, nil, validate, balances, tt.transactionType, constant.ActionDirect)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, capturedOps, tt.expectedOpsCount,
				"expected %d balance operations, got %d", tt.expectedOpsCount, len(capturedOps))

			assert.Equal(t, tt.expectedOp1, capturedOps[0].Amount.Operation,
				"first operation should be %s", tt.expectedOp1)

			if tt.expectedOpsCount == 2 {
				assert.Equal(t, tt.expectedOp2, capturedOps[1].Amount.Operation,
					"second operation should be %s", tt.expectedOp2)

				// Both operations reference the same balance alias
				assert.Equal(t, capturedOps[0].Alias, capturedOps[1].Alias,
					"both operations should reference the same alias")
			}
		})
	}
}

func TestGetAccountAndLock_DoubleEntry_SeenDeduplication(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	organizationID := uuid.MustParse("ad0032e5-ccf5-45f4-a3b2-12045e71b38a")
	ledgerID := uuid.MustParse("5d8ac48a-af68-4544-9bf8-80c3cc0715f4")

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	balanceID := uuid.New().String()
	accountID := uuid.New().String()

	// Use a transactionInput to trigger the "seen" deduplication path.
	// Asset must match the balance's AssetCode to pass validateFromBalances.
	transactionInput := &pkgTransaction.Transaction{
		Pending: true,
		Send:    pkgTransaction.Send{Asset: "USD"},
	}

	validate := &pkgTransaction.Responses{
		Aliases: []string{"alias1#default"},
		Asset:   "USD",
		Pending: true,
		From: map[string]pkgTransaction.Amount{
			"0#alias1#default": {
				Asset:                  "USD",
				Value:                  decimal.NewFromFloat(50),
				Operation:              constant.ONHOLD,
				TransactionType:        constant.PENDING,
				RouteValidationEnabled: true,
			},
		},
	}

	balances := []*mmodel.Balance{
		{
			ID:             balanceID,
			AccountID:      accountID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          "alias1",
			Key:            "default",
			Available:      decimal.NewFromFloat(100),
			OnHold:         decimal.NewFromFloat(0),
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			AssetCode:      "USD",
		},
	}

	var capturedOps []mmodel.BalanceOperation

	mockRedisRepo.EXPECT().
		ProcessBalanceAtomicOperation(
			gomock.Any(),
			organizationID,
			ledgerID,
			gomock.Any(),
			constant.PENDING,
			true, // validate.Pending
			gomock.Any(),
		).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ uuid.UUID, _ string, _ bool, ops []mmodel.BalanceOperation) (*mmodel.BalanceAtomicResult, error) {
			capturedOps = ops
			return &mmodel.BalanceAtomicResult{Before: balances, After: balances}, nil
		})

	transactionID := uuid.New()
	result, _, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, transactionID, transactionInput, validate, balances, constant.PENDING, constant.ActionHold)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Double-entry splitting produces 2 operations (DEBIT + ONHOLD) for the same alias
	require.Len(t, capturedOps, 2, "expected 2 balance operations for double-entry PENDING")

	// Both share the same alias -- the "seen" deduplication ensures only one
	// txBalance entry is created despite two operations for the same alias.
	assert.Equal(t, capturedOps[0].Alias, capturedOps[1].Alias,
		"both operations should reference the same alias")
}

func TestValidateIfBalanceExistsOnRedis(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	t.Run("some balances in redis", func(t *testing.T) {
		aliases := []string{"alias1#default", "alias2#default", "alias3#default"}

		balance1 := mmodel.BalanceRedis{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			Available:      decimal.NewFromFloat(100),
			OnHold:         decimal.NewFromFloat(0),
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   1,
			AllowReceiving: 1,
			AssetCode:      "USD",
		}
		balance1JSON, _ := json.Marshal(balance1)

		internalKey1 := utils.BalanceInternalKey(organizationID, ledgerID, "alias1#default")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey1).
			Return(string(balance1JSON), nil).
			Times(1)

		internalKey2 := utils.BalanceInternalKey(organizationID, ledgerID, "alias2#default")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey2).
			Return("", nil).
			Times(1)

		internalKey3 := utils.BalanceInternalKey(organizationID, ledgerID, "alias3#default")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey3).
			Return("", nil).
			Times(1)

		balances, remainingAliases := uc.ValidateIfBalanceExistsOnRedis(ctx, organizationID, ledgerID, aliases)

		assert.Len(t, balances, 1)
		assert.Equal(t, balance1.ID, balances[0].ID)
		assert.Equal(t, "alias1", balances[0].Alias)

		assert.Len(t, remainingAliases, 2)
		assert.Contains(t, remainingAliases, "alias2#default")
		assert.Contains(t, remainingAliases, "alias3#default")
	})
}

func TestBalanceRedis_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    mmodel.BalanceRedis
		wantErr bool
	}{
		{
			name: "normal integer values",
			json: `{
				"id": "01968142-fba6-7c96-bcdd-877b46020b84",
				"accountId": "01968142-fba1-7399-88e9-0d69f1ecf1d3",
				"assetCode": "BRL",
				"available": 10000,
				"onHold": 0,
				"scale": 2,
				"version": 1,
				"accountType": "external",
				"allowSending": 1,
				"allowReceiving": 1
			}`,
			want: mmodel.BalanceRedis{
				ID:             "01968142-fba6-7c96-bcdd-877b46020b84",
				AccountID:      "01968142-fba1-7399-88e9-0d69f1ecf1d3",
				AssetCode:      "BRL",
				Available:      decimal.NewFromFloat(10000),
				OnHold:         decimal.NewFromFloat(0),
				Version:        1,
				AccountType:    "external",
				AllowSending:   1,
				AllowReceiving: 1,
			},
			wantErr: false,
		},
		{
			name: "scientific notation large value",
			json: `{
				"id": "01968143-6677-7d4a-ad4b-0b0c8ae366fb",
				"accountId": "01968143-666c-7e4d-b127-bc5ac9af3058",
				"assetCode": "BRL",
				"available": 1e+16,
				"onHold": 0,
				"scale": 14,
				"version": 1,
				"accountType": "creditCard",
				"allowSending": 1,
				"allowReceiving": 1
			}`,
			want: mmodel.BalanceRedis{
				ID:             "01968143-6677-7d4a-ad4b-0b0c8ae366fb",
				AccountID:      "01968143-666c-7e4d-b127-bc5ac9af3058",
				AssetCode:      "BRL",
				Available:      decimal.NewFromFloat(10000000000000000),
				OnHold:         decimal.NewFromFloat(0),
				Version:        1,
				AccountType:    "creditCard",
				AllowSending:   1,
				AllowReceiving: 1,
			},
			wantErr: false,
		},
		{
			name: "string number values",
			json: `{
				"id": "01968142-fba6-7c96-bcdd-877b46020b84",
				"accountId": "01968142-fba1-7399-88e9-0d69f1ecf1d3",
				"assetCode": "BRL",
				"available": "5000",
				"onHold": "1000",
				"scale": 2,
				"version": 1,
				"accountType": "external",
				"allowSending": 1,
				"allowReceiving": 1
			}`,
			want: mmodel.BalanceRedis{
				ID:             "01968142-fba6-7c96-bcdd-877b46020b84",
				AccountID:      "01968142-fba1-7399-88e9-0d69f1ecf1d3",
				AssetCode:      "BRL",
				Available:      decimal.NewFromFloat(5000),
				OnHold:         decimal.NewFromFloat(1000),
				Version:        1,
				AccountType:    "external",
				AllowSending:   1,
				AllowReceiving: 1,
			},
			wantErr: false,
		},
		{
			name: "negative values",
			json: `{
				"id": "01968142-fba6-7c96-bcdd-877b46020b84",
				"accountId": "01968142-fba1-7399-88e9-0d69f1ecf1d3",
				"assetCode": "BRL",
				"available": -10000,
				"onHold": 0,
				"scale": 2,
				"version": 1,
				"accountType": "external",
				"allowSending": 1,
				"allowReceiving": 1
			}`,
			want: mmodel.BalanceRedis{
				ID:             "01968142-fba6-7c96-bcdd-877b46020b84",
				AccountID:      "01968142-fba1-7399-88e9-0d69f1ecf1d3",
				AssetCode:      "BRL",
				Available:      decimal.NewFromFloat(-10000),
				OnHold:         decimal.NewFromFloat(0),
				Version:        1,
				AccountType:    "external",
				AllowSending:   1,
				AllowReceiving: 1,
			},
			wantErr: false,
		},
		{
			name: "scientific notation small value",
			json: `{
				"id": "01968142-fba6-7c96-bcdd-877b46020b84",
				"accountId": "01968142-fba1-7399-88e9-0d69f1ecf1d3",
				"assetCode": "BRL",
				"available": 1.5e+3,
				"onHold": 0,
				"scale": 2,
				"version": 1,
				"accountType": "external",
				"allowSending": 1,
				"allowReceiving": 1
			}`,
			want: mmodel.BalanceRedis{
				ID:             "01968142-fba6-7c96-bcdd-877b46020b84",
				AccountID:      "01968142-fba1-7399-88e9-0d69f1ecf1d3",
				AssetCode:      "BRL",
				Available:      decimal.NewFromFloat(1500),
				OnHold:         decimal.NewFromFloat(0),
				Version:        1,
				AccountType:    "external",
				AllowSending:   1,
				AllowReceiving: 1,
			},
			wantErr: false,
		},
		{
			name: "invalid available value",
			json: `{
				"id": "01968142-fba6-7c96-bcdd-877b46020b84",
				"accountId": "01968142-fba1-7399-88e9-0d69f1ecf1d3",
				"assetCode": "BRL",
				"available": "invalid",
				"onHold": 0,
				"scale": 2,
				"version": 1,
				"accountType": "external",
				"allowSending": 1,
				"allowReceiving": 1
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := mmodel.BalanceRedis{}
			err := json.Unmarshal([]byte(tt.json), &b)

			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if b.ID != tt.want.ID {
					t.Errorf("ID: got = %v, want %v", b.ID, tt.want.ID)
				}
				if b.AccountID != tt.want.AccountID {
					t.Errorf("AccountID: got = %v, want %v", b.AccountID, tt.want.AccountID)
				}
				if b.AssetCode != tt.want.AssetCode {
					t.Errorf("AssetCode: got = %v, want %v", b.AssetCode, tt.want.AssetCode)
				}
				if b.Available.String() != tt.want.Available.String() {
					t.Errorf("Available: got = %v, want %v", b.Available, tt.want.Available)
				}
				if b.OnHold.String() != tt.want.OnHold.String() {
					t.Errorf("OnHold: got = %v, want %v", b.OnHold, tt.want.OnHold)
				}
				if b.Version != tt.want.Version {
					t.Errorf("Version: got = %v, want %v", b.Version, tt.want.Version)
				}
				if b.AccountType != tt.want.AccountType {
					t.Errorf("AccountType: got = %v, want %v", b.AccountType, tt.want.AccountType)
				}
				if b.AllowSending != tt.want.AllowSending {
					t.Errorf("AllowSending: got = %v, want %v", b.AllowSending, tt.want.AllowSending)
				}
				if b.AllowReceiving != tt.want.AllowReceiving {
					t.Errorf("AllowReceiving: got = %v, want %v", b.AllowReceiving, tt.want.AllowReceiving)
				}
			}
		})
	}
}
