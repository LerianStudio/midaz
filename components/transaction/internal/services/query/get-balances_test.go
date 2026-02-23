// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
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

	uc := &UseCase{
		BalanceRepo: mockBalanceRepo,
		RedisRepo:   mockRedisRepo,
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
		balanceRedisJSON, marshalErr := json.Marshal(balanceRedis)
		require.NoError(t, marshalErr)

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
			Return([]*mmodel.Balance{
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
			}, nil).
			Times(1)

		transactionID := uuid.New()
		balances, err := uc.GetBalances(ctx, organizationID, ledgerID, transactionID, nil, validate, constant.CREATED)
		assert.NoError(t, err)
		assert.Len(t, balances, 3)

		sort.Slice(balances, func(i, j int) bool {
			return balances[i].Alias < balances[j].Alias
		})

		assert.Equal(t, "alias1", balances[0].Alias)
		assert.Equal(t, balanceRedis.ID, balances[0].ID)

		assert.Equal(t, "alias2", balances[1].Alias)
		assert.Equal(t, databaseBalances[0].ID, balances[1].ID)

		assert.Equal(t, "alias3", balances[2].Alias)
		assert.Equal(t, databaseBalances[1].ID, balances[2].ID)
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
		balance1JSON, marshalErr := json.Marshal(balance1)
		require.NoError(t, marshalErr)

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
		balance2JSON, marshalErr := json.Marshal(balance2)
		require.NoError(t, marshalErr)

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
			Return([]*mmodel.Balance{
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
			}, nil).
			Times(1)

		transactionID := uuid.New()
		balances, err := uc.GetBalances(ctx, organizationID, ledgerID, transactionID, nil, validate, constant.CREATED)

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
		RedisRepo: mockRedisRepo,
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
			Return(balances, nil)

		transactionID := uuid.New()
		lockedBalances, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, transactionID, nil, validate, balances, constant.CREATED)

		assert.NoError(t, err)
		assert.Len(t, lockedBalances, 1)
	})
}

func TestValidateIfBalanceExistsOnRedis(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
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
		balance1JSON, marshalErr := json.Marshal(balance1)
		require.NoError(t, marshalErr)

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

		balances, remainingAliases, err := uc.ValidateIfBalanceExistsOnRedis(ctx, organizationID, ledgerID, aliases)
		require.NoError(t, err)

		assert.Len(t, balances, 1)
		assert.Equal(t, balance1.ID, balances[0].ID)
		assert.Equal(t, "alias1", balances[0].Alias)

		assert.Len(t, remainingAliases, 2)
		assert.Contains(t, remainingAliases, "alias2#default")
		assert.Contains(t, remainingAliases, "alias3#default")
	})
}

func TestValidateIfBalanceExistsOnRedis_ShardedUsesShardKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo:   mockRedisRepo,
		ShardRouter: shard.NewRouter(8),
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	aliases := []string{"@alice#default", "@bob#default"}

	balanceAlice := mmodel.BalanceRedis{
		ID:             uuid.New().String(),
		AccountID:      uuid.New().String(),
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.Zero,
		Version:        2,
		AccountType:    "deposit",
		AllowSending:   1,
		AllowReceiving: 1,
		AssetCode:      "USD",
	}
	balanceAliceJSON, marshalErr := json.Marshal(balanceAlice)
	require.NoError(t, marshalErr)

	aliceShard := uc.ShardRouter.Resolve(shard.ExtractAccountAlias(aliases[0]))
	bobShard := uc.ShardRouter.Resolve(shard.ExtractAccountAlias(aliases[1]))

	aliceKey := utils.BalanceShardKey(aliceShard, organizationID, ledgerID, aliases[0])
	bobKey := utils.BalanceShardKey(bobShard, organizationID, ledgerID, aliases[1])

	mockRedisRepo.EXPECT().Get(gomock.Any(), aliceKey).Return(string(balanceAliceJSON), nil).Times(1)
	mockRedisRepo.EXPECT().Get(gomock.Any(), bobKey).Return("", nil).Times(1)

	balances, remainingAliases, err := uc.ValidateIfBalanceExistsOnRedis(ctx, organizationID, ledgerID, aliases)
	require.NoError(t, err)

	require.Len(t, balances, 1)
	assert.Equal(t, "@alice", balances[0].Alias)
	assert.True(t, balances[0].Available.Equal(decimal.NewFromInt(100)))
	require.Len(t, remainingAliases, 1)
	assert.Equal(t, "@bob#default", remainingAliases[0])
}

func TestValidateIfBalanceExistsOnRedis_ShardedCorruptJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo:   mockRedisRepo,
		ShardRouter: shard.NewRouter(8),
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	aliases := []string{"@carol#default", "@dave#default"}

	// @carol gets corrupt JSON, @dave gets an empty response (cache miss)
	carolShard := uc.ShardRouter.Resolve(shard.ExtractAccountAlias(aliases[0]))
	daveShard := uc.ShardRouter.Resolve(shard.ExtractAccountAlias(aliases[1]))

	carolKey := utils.BalanceShardKey(carolShard, organizationID, ledgerID, aliases[0])
	daveKey := utils.BalanceShardKey(daveShard, organizationID, ledgerID, aliases[1])

	mockRedisRepo.EXPECT().Get(gomock.Any(), carolKey).Return("not-valid-json", nil).Times(1)
	mockRedisRepo.EXPECT().Get(gomock.Any(), daveKey).Return("", nil).Times(1)

	balances, remainingAliases, err := uc.ValidateIfBalanceExistsOnRedis(ctx, organizationID, ledgerID, aliases)
	require.NoError(t, err)

	// When the sharded Redis entry contains corrupt JSON, the unmarshal error
	// is logged as a warning and the loop continues. The alias is silently
	// skipped: it does NOT appear in the returned balances (because
	// deserialization failed) and it does NOT appear in remainingAliases
	// (because the value was non-empty, so the else-branch adding it to
	// newAliases is never reached). Only @dave, which had a cache miss,
	// ends up in remainingAliases.
	assert.Empty(t, balances, "corrupt JSON entry must not produce a balance")
	require.Len(t, remainingAliases, 1)
	assert.Equal(t, "@dave#default", remainingAliases[0])
}

func TestGetAccountAndLock_ShardedOperationsContainShardData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	organizationID := uuid.MustParse("ad0032e5-ccf5-45f4-a3b2-12045e71b38a")
	ledgerID := uuid.MustParse("5d8ac48a-af68-4544-9bf8-80c3cc0715f4")
	uc := UseCase{
		RedisRepo:   mockRedisRepo,
		ShardRouter: shard.NewRouter(8),
	}

	validate := &pkgTransaction.Responses{
		Aliases: []string{"@alice", "@bob"},
		From: map[string]pkgTransaction.Amount{
			"0#@alice#default": {
				Asset:     "USD",
				Value:     decimal.NewFromInt(40),
				Operation: constant.DEBIT,
			},
		},
		To: map[string]pkgTransaction.Amount{
			"1#@bob#default": {
				Asset:     "USD",
				Value:     decimal.NewFromInt(40),
				Operation: constant.CREDIT,
			},
		},
	}

	balances := []*mmodel.Balance{
		{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          "@alice",
			Key:            "default",
			Available:      decimal.NewFromInt(100),
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			AssetCode:      "USD",
		},
		{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          "@bob",
			Key:            "default",
			Available:      decimal.NewFromInt(100),
			OnHold:         decimal.Zero,
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
			constant.CREATED,
			false,
			gomock.Any(),
		).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, _ string, _ bool, ops []mmodel.BalanceOperation) ([]*mmodel.Balance, error) {
			capturedOps = ops
			return balances, nil
		})

	transactionID := uuid.New()
	lockedBalances, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, transactionID, nil, validate, balances, constant.CREATED)
	require.NoError(t, err)
	require.Len(t, lockedBalances, 2)
	require.Len(t, capturedOps, 2)

	for _, op := range capturedOps {
		alias := op.Balance.Alias
		aliasKey := alias + "#" + op.Balance.Key
		expectedShard := uc.ShardRouter.Resolve(alias)
		expectedInternalKey := utils.BalanceShardKey(expectedShard, organizationID, ledgerID, aliasKey)

		assert.Equal(t, expectedShard, op.ShardID)
		assert.Equal(t, expectedInternalKey, op.InternalKey)
	}
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
				Available:      decimal.NewFromFloat(100), // 10000 / 10^2 = 100 (auto-unscaled)
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
				Available:      decimal.NewFromFloat(100), // 1e16 / 10^14 = 100 (auto-unscaled)
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
				Available:      decimal.NewFromFloat(50), // 5000 / 10^2 = 50 (auto-unscaled)
				OnHold:         decimal.NewFromFloat(10), // 1000 / 10^2 = 10 (auto-unscaled)
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
				Available:      decimal.NewFromFloat(-100), // -10000 / 10^2 = -100 (auto-unscaled)
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
				Available:      decimal.NewFromFloat(15), // 1500 / 10^2 = 15 (auto-unscaled)
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

// TestValidateIfBalanceExistsOnRedis_ShardResolutionFailure verifies that
// ValidateIfBalanceExistsOnRedis fails open when shard resolution encounters
// an error. Instead of propagating the error to the caller, it should fall
// back to the non-sharded (legacy) Redis key and continue the lookup.
func TestValidateIfBalanceExistsOnRedis_ShardResolutionFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// A zero-value Router has ShardCount()==0, which triggers an error in
	// shardrouting.ResolveBalanceShard ("invalid shard count"). This
	// simulates a misconfigured or degraded shard router.
	uc := &UseCase{
		RedisRepo:   mockRedisRepo,
		ShardRouter: &shard.Router{},
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	aliases := []string{"@alice#default", "@bob#default"}

	// Prepare a valid balance payload that should be found via the legacy key.
	balanceAlice := mmodel.BalanceRedis{
		ID:             uuid.New().String(),
		AccountID:      uuid.New().String(),
		Available:      decimal.NewFromInt(500),
		OnHold:         decimal.Zero,
		Version:        3,
		AccountType:    "deposit",
		AllowSending:   1,
		AllowReceiving: 1,
		AssetCode:      "USD",
	}
	balanceAliceJSON, marshalErr := json.Marshal(balanceAlice)
	require.NoError(t, marshalErr)

	// The fail-open path should construct non-sharded (legacy) keys because
	// the shard router's ShardCount is invalid (0). The function should NOT
	// return an error.
	aliceLegacyKey := utils.BalanceInternalKey(organizationID, ledgerID, "@alice#default")
	bobLegacyKey := utils.BalanceInternalKey(organizationID, ledgerID, "@bob#default")

	// @alice is found via the legacy key; @bob is a cache miss.
	mockRedisRepo.EXPECT().Get(gomock.Any(), aliceLegacyKey).Return(string(balanceAliceJSON), nil).Times(1)
	mockRedisRepo.EXPECT().Get(gomock.Any(), bobLegacyKey).Return("", nil).Times(1)

	balances, remainingAliases, err := uc.ValidateIfBalanceExistsOnRedis(ctx, organizationID, ledgerID, aliases)

	// The function must NOT return an error -- fail-open behavior.
	require.NoError(t, err, "shard resolution failure must not propagate as an error")

	// @alice should have been found via the legacy (fallback) key.
	require.Len(t, balances, 1, "exactly one balance should be returned")
	assert.Equal(t, "@alice", balances[0].Alias)
	assert.Equal(t, balanceAlice.ID, balances[0].ID)
	assert.True(t, balances[0].Available.Equal(decimal.NewFromInt(500)))

	// @bob was a cache miss, so it should appear in the remaining aliases
	// for downstream PostgreSQL lookup.
	require.Len(t, remainingAliases, 1)
	assert.Equal(t, "@bob#default", remainingAliases[0])
}
