package query

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
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
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)

	uc := &UseCase{
		BalanceRepo:  mockBalanceRepo,
		RedisRepo:    mockRedisRepo,
		RabbitMQRepo: mockRabbitMQRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	t.Run("get balances from redis and database", func(t *testing.T) {
		aliases := []string{"alias1", "alias2", "alias3"}

		fromAmount := libTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromFloat(50),
			Operation: constant.DEBIT,
		}
		toAmount2 := libTransaction.Amount{
			Asset:     "EUR",
			Value:     decimal.NewFromFloat(40),
			Operation: constant.CREDIT,
		}
		toAmount3 := libTransaction.Amount{
			Asset:     "GBP",
			Value:     decimal.NewFromFloat(30),
			Operation: constant.CREDIT,
		}

		validate := &libTransaction.Responses{
			Aliases: aliases,
			From: map[string]libTransaction.Amount{
				"alias1": fromAmount,
			},
			To: map[string]libTransaction.Amount{
				"alias2": toAmount2,
				"alias3": toAmount3,
			},
		}

		// --- mock data ---

		// 1) Balance vindo do Redis
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

		// 2) Balances vindo do banco para alias2 e alias3
		databaseBalances := []*mmodel.Balance{
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias2",
				Available:      decimal.NewFromFloat(100),
				OnHold:         decimal.NewFromFloat(0),
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "EUR", // bate com toAmount2.Asset
			},
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias3",
				Available:      decimal.NewFromFloat(300),
				OnHold:         decimal.NewFromFloat(0),
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "GBP",
			},
		}

		// --- expectativas ---

		// 2) Get de Redis para cada alias
		key1 := libCommons.TransactionInternalKey(organizationID, ledgerID, "alias1")
		key2 := libCommons.TransactionInternalKey(organizationID, ledgerID, "alias2")
		key3 := libCommons.TransactionInternalKey(organizationID, ledgerID, "alias3")

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

		// 3) Busca no BD para os que não estavam no Redis
		mockBalanceRepo.
			EXPECT().
			ListByAliases(gomock.Any(), organizationID, ledgerID, []string{"alias2", "alias3"}).
			Return(databaseBalances, nil).
			Times(1)

		// 4) AddSumBalanceRedis para cada um (ignoramos o struct completo com Any())
		//    a) alias1 (do Redis)
		balanceResult1 := &mmodel.Balance{
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
		}
		mockRedisRepo.
			EXPECT().
			AddSumBalanceRedis(
				gomock.Any(),
				key1,
				constant.CREATED,
				false,
				fromAmount,
				gomock.Any(),
			).
			Return(balanceResult1, nil).
			Times(1)

		//    b) alias2 (do BD)
		mockRedisRepo.
			EXPECT().
			AddSumBalanceRedis(
				gomock.Any(),
				key2,
				constant.CREATED,
				false,
				toAmount2,
				gomock.Any(),
			).
			Return(databaseBalances[0], nil).
			Times(1)

		//    c) alias3 (do BD)
		mockRedisRepo.
			EXPECT().
			AddSumBalanceRedis(
				gomock.Any(),
				key3,
				constant.CREATED,
				false,
				toAmount3,
				gomock.Any(),
			).
			Return(databaseBalances[1], nil).
			Times(1)

		// --- execução & asserts ---
		balances, err := uc.GetBalances(ctx, organizationID, ledgerID, validate, constant.CREATED)
		assert.NoError(t, err)
		assert.Len(t, balances, 3)

		// para garantir determinismo independentemente da ordem interna
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
		// Test data
		aliases := []string{"alias1", "alias2"}
		fromAmount := libTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromFloat(50),
			Operation: constant.DEBIT,
		}

		toAmount := libTransaction.Amount{
			Asset:     "EUR",
			Value:     decimal.NewFromFloat(40),
			Operation: constant.CREDIT,
		}

		validate := &libTransaction.Responses{
			Aliases: aliases,
			From: map[string]libTransaction.Amount{
				"alias1": fromAmount,
			},
			To: map[string]libTransaction.Amount{
				"alias2": toAmount,
			},
		}

		// Redis balances
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

		// Mock Redis.Get for both aliases (found in Redis)
		internalKey1 := libCommons.TransactionInternalKey(organizationID, ledgerID, "alias1")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey1).
			Return(string(balance1JSON), nil).
			Times(1)

		internalKey2 := libCommons.TransactionInternalKey(organizationID, ledgerID, "alias2")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey2).
			Return(string(balance2JSON), nil).
			Times(1)

		// Mock Redis.LockBalanceRedis for alias1 with DEBIT operation
		mockRedisRepo.EXPECT().
			AddSumBalanceRedis(
				gomock.Any(),
				internalKey1,
				constant.CREATED,
				false,
				fromAmount,
				gomock.Any(),
			).
			Return(&mmodel.Balance{
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
			}, nil).
			Times(1)

		// Mock Redis.LockBalanceRedis for alias2 with CREDIT operation
		mockRedisRepo.EXPECT().
			AddSumBalanceRedis(
				gomock.Any(),
				internalKey2,
				constant.CREATED,
				false,
				toAmount,
				gomock.Any(),
			).
			Return(&mmodel.Balance{
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
			}, nil).
			Times(1)

		// Call the method
		balances, err := uc.GetBalances(ctx, organizationID, ledgerID, validate, constant.CREATED)

		// Assertions
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
		// Test data
		balanceID1 := uuid.MustParse("c7d0fa07-3e11-4105-a0fc-6fa46834ce66")
		accountID1 := uuid.MustParse("bad0ddef-d697-4a4e-840d-1f5380de4607")

		fromAmount := libTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromFloat(50),
			Operation: constant.DEBIT,
		}

		validate := &libTransaction.Responses{
			Aliases: []string{"alias1"},
			From: map[string]libTransaction.Amount{
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

		internalKey1 := libCommons.TransactionInternalKey(organizationID, ledgerID, "alias1")

		mockRedisRepo.EXPECT().
			AddSumBalanceRedis(
				gomock.Any(),
				internalKey1,
				constant.CREATED,
				false,
				fromAmount,
				*balances[0],
			).
			Return(balances[0], nil)

		lockedBalances, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, validate, balances, constant.CREATED)

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
		// Test data
		aliases := []string{"alias1", "alias2", "alias3"}

		// Redis balance for alias1
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

		// Mock Redis.Get for all aliases
		internalKey1 := libCommons.TransactionInternalKey(organizationID, ledgerID, "alias1")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey1).
			Return(string(balance1JSON), nil).
			Times(1)

		internalKey2 := libCommons.TransactionInternalKey(organizationID, ledgerID, "alias2")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey2).
			Return("", nil).
			Times(1)

		internalKey3 := libCommons.TransactionInternalKey(organizationID, ledgerID, "alias3")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey3).
			Return("", nil).
			Times(1)

		// Call the method
		balances, remainingAliases := uc.ValidateIfBalanceExistsOnRedis(ctx, organizationID, ledgerID, aliases)

		// Assertions
		assert.Len(t, balances, 1)
		assert.Equal(t, balance1.ID, balances[0].ID)
		assert.Equal(t, "alias1", balances[0].Alias)

		assert.Len(t, remainingAliases, 2)
		assert.Contains(t, remainingAliases, "alias2")
		assert.Contains(t, remainingAliases, "alias3")
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
