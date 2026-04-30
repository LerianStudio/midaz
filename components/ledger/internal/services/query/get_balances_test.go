// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetBalances(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	t.Run("get balances from redis and database", func(t *testing.T) {
		aliases := []string{"alias1#default", "alias2#default", "alias3#default"}

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

		allBalances, err := uc.GetBalances(ctx, organizationID, ledgerID, aliases)
		assert.NoError(t, err)
		assert.Len(t, allBalances, 3)

		sort.Slice(allBalances, func(i, j int) bool {
			return allBalances[i].Alias < allBalances[j].Alias
		})

		assert.Equal(t, "alias1", allBalances[0].Alias)
		assert.Equal(t, balanceRedis.ID, allBalances[0].ID)

		assert.Equal(t, "alias2", allBalances[1].Alias)
		assert.Equal(t, databaseBalances[0].ID, allBalances[1].ID)

		assert.Equal(t, "alias3", allBalances[2].Alias)
		assert.Equal(t, databaseBalances[1].ID, allBalances[2].ID)
	})

	t.Run("all balances from redis", func(t *testing.T) {
		aliases := []string{"alias1#default", "alias2#default"}

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

		allBalances, err := uc.GetBalances(ctx, organizationID, ledgerID, aliases)
		assert.NoError(t, err)
		assert.Len(t, allBalances, 2)
	})
}

func TestGetBalancesFromCache(t *testing.T) {
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

		balances, remainingAliases := uc.getBalancesFromCache(ctx, organizationID, ledgerID, aliases)

		assert.Len(t, balances, 1)
		assert.Equal(t, balance1.ID, balances[0].ID)
		assert.Equal(t, "alias1", balances[0].Alias)

		assert.Len(t, remainingAliases, 2)
		assert.Contains(t, remainingAliases, "alias2#default")
		assert.Contains(t, remainingAliases, "alias3#default")
	})
}

// TestGetBalancesFromCache_PropagatesOverdraftFields guards against the
// regression where the cache path stripped Direction, OverdraftUsed, and
// Settings off of mmodel.Balance, causing the Lua atomic script to observe
// Direction="" + AllowOverdraft=0 and reject overdraft-enabled transactions
// with 0018 (insufficient funds) instead of splitting at zero.
func TestGetBalancesFromCache_PropagatesOverdraftFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	t.Run("overdraft enabled with limit", func(t *testing.T) {
		aliases := []string{"@alice#default"}

		cached := mmodel.BalanceRedis{
			ID:                    uuid.New().String(),
			AccountID:             uuid.New().String(),
			Available:             decimal.NewFromInt(50),
			OnHold:                decimal.NewFromInt(0),
			Version:               3,
			AccountType:           "deposit",
			AllowSending:          1,
			AllowReceiving:        1,
			AssetCode:             "BRL",
			Direction:             "credit",
			OverdraftUsed:         "10",
			AllowOverdraft:        1,
			OverdraftLimitEnabled: 1,
			OverdraftLimit:        "100",
			BalanceScope:          mmodel.BalanceScopeTransactional,
		}
		cachedJSON, err := json.Marshal(cached)
		assert.NoError(t, err)

		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, "@alice#default")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return(string(cachedJSON), nil).
			Times(1)

		balances, misses := uc.getBalancesFromCache(ctx, organizationID, ledgerID, aliases)
		assert.Empty(t, misses)
		assert.Len(t, balances, 1)

		got := balances[0]
		assert.Equal(t, "credit", got.Direction, "Direction must propagate so Lua can gate overdraft")
		assert.True(t, got.OverdraftUsed.Equal(decimal.NewFromInt(10)),
			"OverdraftUsed must round-trip through the cache path, got %s", got.OverdraftUsed)
		if assert.NotNil(t, got.Settings, "Settings must be materialized when overdraft is configured") {
			assert.True(t, got.Settings.AllowOverdraft, "AllowOverdraft must flow into Settings")
			assert.True(t, got.Settings.OverdraftLimitEnabled, "OverdraftLimitEnabled must flow into Settings")
			if assert.NotNil(t, got.Settings.OverdraftLimit, "OverdraftLimit must be exposed when enabled") {
				assert.Equal(t, "100", *got.Settings.OverdraftLimit)
			}
		}
	})

	t.Run("legacy balance without overdraft stays nil", func(t *testing.T) {
		aliases := []string{"@legacy#default"}

		cached := mmodel.BalanceRedis{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			Available:      decimal.NewFromInt(500),
			OnHold:         decimal.NewFromInt(0),
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   1,
			AllowReceiving: 1,
			AssetCode:      "USD",
			// All overdraft fields zero/empty, as a legacy cache entry.
		}
		cachedJSON, err := json.Marshal(cached)
		assert.NoError(t, err)

		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, "@legacy#default")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return(string(cachedJSON), nil).
			Times(1)

		balances, misses := uc.getBalancesFromCache(ctx, organizationID, ledgerID, aliases)
		assert.Empty(t, misses)
		assert.Len(t, balances, 1)

		got := balances[0]
		assert.Nil(t, got.Settings,
			"Settings must stay nil for legacy balances to avoid spurious history snapshots")
		assert.True(t, got.OverdraftUsed.IsZero(), "OverdraftUsed must default to zero")
	})

	t.Run("overdraft enabled without limit omits OverdraftLimit pointer", func(t *testing.T) {
		aliases := []string{"@unlimited#default"}

		cached := mmodel.BalanceRedis{
			ID:                    uuid.New().String(),
			AccountID:             uuid.New().String(),
			Available:             decimal.NewFromInt(0),
			OnHold:                decimal.NewFromInt(0),
			Version:               1,
			AccountType:           "deposit",
			AllowSending:          1,
			AllowReceiving:        1,
			AssetCode:             "BRL",
			Direction:             "credit",
			OverdraftUsed:         "0",
			AllowOverdraft:        1,
			OverdraftLimitEnabled: 0,
			OverdraftLimit:        "0",
			BalanceScope:          mmodel.BalanceScopeTransactional,
		}
		cachedJSON, err := json.Marshal(cached)
		assert.NoError(t, err)

		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, "@unlimited#default")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return(string(cachedJSON), nil).
			Times(1)

		balances, misses := uc.getBalancesFromCache(ctx, organizationID, ledgerID, aliases)
		assert.Empty(t, misses)
		assert.Len(t, balances, 1)

		got := balances[0]
		if assert.NotNil(t, got.Settings) {
			assert.True(t, got.Settings.AllowOverdraft)
			assert.False(t, got.Settings.OverdraftLimitEnabled)
			assert.Nil(t, got.Settings.OverdraftLimit,
				"OverdraftLimit must be nil when OverdraftLimitEnabled is false (Validate() contract)")
		}
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
