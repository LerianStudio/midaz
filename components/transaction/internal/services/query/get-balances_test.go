package query

import (
	"context"
	"encoding/json"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
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
		// Test data
		aliases := []string{"alias1", "alias2", "alias3"}
		validate := &libTransaction.Responses{
			Aliases: aliases,
			From:    make(map[string]libTransaction.Amount),
			To:      make(map[string]libTransaction.Amount),
		}

		// Redis balance for alias1
		balanceRedis := mmodel.BalanceRedis{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			Available:      int64(100),
			OnHold:         int64(0),
			Scale:          int64(2),
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   1,
			AllowReceiving: 1,
			AssetCode:      "USD",
		}
		balanceRedisJSON, _ := json.Marshal(balanceRedis)

		// Database balances for alias2 and alias3
		databaseBalances := []*mmodel.Balance{
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias2",
				Available:      int64(200),
				OnHold:         int64(0),
				Scale:          int64(2),
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
				Available:      int64(300),
				OnHold:         int64(0),
				Scale:          int64(2),
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "GBP",
			},
		}

		mockRabbitMQRepo.EXPECT().
			CheckRabbitMQHealth().
			Return(true).
			Times(1)

		// Mock Redis.Get for alias1 (found in Redis)
		internalKey1 := libCommons.LockInternalKey(organizationID, ledgerID, "alias1")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey1).
			Return(string(balanceRedisJSON), nil).
			Times(1)

		// Mock Redis.Get for alias2 and alias3 (not found in Redis)
		internalKey2 := libCommons.LockInternalKey(organizationID, ledgerID, "alias2")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey2).
			Return("", nil).
			Times(1)

		internalKey3 := libCommons.LockInternalKey(organizationID, ledgerID, "alias3")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey3).
			Return("", nil).
			Times(1)

		// Mock BalanceRepo.ListByAliases for alias2 and alias3
		mockBalanceRepo.EXPECT().
			ListByAliases(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(databaseBalances, nil).
			Times(1)

		// Mock Redis.LockBalanceRedis for all balances
		for _, b := range append([]*mmodel.Balance{
			{
				ID:             balanceRedis.ID,
				AccountID:      balanceRedis.AccountID,
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias1",
				Available:      balanceRedis.Available,
				OnHold:         balanceRedis.OnHold,
				Scale:          balanceRedis.Scale,
				Version:        balanceRedis.Version,
				AccountType:    balanceRedis.AccountType,
				AllowSending:   balanceRedis.AllowSending == 1,
				AllowReceiving: balanceRedis.AllowReceiving == 1,
				AssetCode:      balanceRedis.AssetCode,
			},
		}, databaseBalances...) {
			internalKey := libCommons.LockInternalKey(organizationID, ledgerID, b.Alias)
			mockRedisRepo.EXPECT().
				LockBalanceRedis(gomock.Any(), internalKey, gomock.Any(), gomock.Any(), gomock.Any()).
				Return(b, nil).
				Times(1)
		}

		// Call the method
		balances, err := uc.GetBalances(ctx, organizationID, ledgerID, validate)

		// Assertions
		assert.NoError(t, err)
		assert.Len(t, balances, 3)
		assert.Equal(t, "alias1", balances[0].Alias)
		assert.Equal(t, "alias2", balances[1].Alias)
		assert.Equal(t, "alias3", balances[2].Alias)
	})

	t.Run("all balances from redis", func(t *testing.T) {
		// Test data
		aliases := []string{"alias1", "alias2"}
		validate := &libTransaction.Responses{
			Aliases: aliases,
			From:    make(map[string]libTransaction.Amount),
			To:      make(map[string]libTransaction.Amount),
		}

		// Redis balances
		balance1 := mmodel.BalanceRedis{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			Available:      int64(100),
			OnHold:         int64(0),
			Scale:          int64(2),
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   1,
			AllowReceiving: 1,
			AssetCode:      "USD",
		}
		balance1JSON, _ := json.Marshal(balance1)

		balance2 := mmodel.BalanceRedis{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			Available:      int64(200),
			OnHold:         int64(0),
			Scale:          int64(2),
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   1,
			AllowReceiving: 1,
			AssetCode:      "EUR",
		}
		balance2JSON, _ := json.Marshal(balance2)

		mockRabbitMQRepo.EXPECT().
			CheckRabbitMQHealth().
			Return(true).
			Times(1)

		// Mock Redis.Get for both aliases (found in Redis)
		internalKey1 := libCommons.LockInternalKey(organizationID, ledgerID, "alias1")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey1).
			Return(string(balance1JSON), nil).
			Times(1)

		internalKey2 := libCommons.LockInternalKey(organizationID, ledgerID, "alias2")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey2).
			Return(string(balance2JSON), nil).
			Times(1)

		// Mock Redis.LockBalanceRedis for both balances
		expectedBalances := []*mmodel.Balance{
			{
				ID:             balance1.ID,
				AccountID:      balance1.AccountID,
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias1",
				Available:      balance1.Available,
				OnHold:         balance1.OnHold,
				Scale:          balance1.Scale,
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
				Scale:          balance2.Scale,
				Version:        balance2.Version,
				AccountType:    balance2.AccountType,
				AllowSending:   balance2.AllowSending == 1,
				AllowReceiving: balance2.AllowReceiving == 1,
				AssetCode:      balance2.AssetCode,
			},
		}

		for _, b := range expectedBalances {
			internalKey := libCommons.LockInternalKey(organizationID, ledgerID, b.Alias)
			mockRedisRepo.EXPECT().
				LockBalanceRedis(gomock.Any(), internalKey, gomock.Any(), gomock.Any(), gomock.Any()).
				Return(b, nil).
				Times(1)
		}

		// Call the method
		balances, err := uc.GetBalances(ctx, organizationID, ledgerID, validate)

		// Assertions
		assert.NoError(t, err)
		assert.Len(t, balances, 2)
		assert.Equal(t, "alias1", balances[0].Alias)
		assert.Equal(t, "alias2", balances[1].Alias)
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
			Available:      int64(100),
			OnHold:         int64(0),
			Scale:          int64(2),
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   1,
			AllowReceiving: 1,
			AssetCode:      "USD",
		}
		balance1JSON, _ := json.Marshal(balance1)

		// Mock Redis.Get for all aliases
		internalKey1 := libCommons.LockInternalKey(organizationID, ledgerID, "alias1")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey1).
			Return(string(balance1JSON), nil).
			Times(1)

		internalKey2 := libCommons.LockInternalKey(organizationID, ledgerID, "alias2")
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey2).
			Return("", nil).
			Times(1)

		internalKey3 := libCommons.LockInternalKey(organizationID, ledgerID, "alias3")
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

func TestGetAccountAndLock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	t.Run("lock balances successfully", func(t *testing.T) {
		// Test data
		validate := &libTransaction.Responses{
			Aliases: []string{"alias1", "alias2"},
			From: map[string]libTransaction.Amount{
				"alias1": {
					Asset: "USD",
					Value: int64(50),
					Scale: int64(2),
				},
			},
			To: map[string]libTransaction.Amount{
				"alias2": {
					Asset: "EUR",
					Value: int64(40),
					Scale: int64(2),
				},
			},
		}

		balances := []*mmodel.Balance{
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias1",
				Available:      int64(100),
				OnHold:         int64(0),
				Scale:          int64(2),
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
				Alias:          "alias2",
				Available:      int64(200),
				OnHold:         int64(0),
				Scale:          int64(2),
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "EUR",
			},
		}

		// Mock Redis.LockBalanceRedis for both balances
		for _, b := range balances {
			internalKey := libCommons.LockInternalKey(organizationID, ledgerID, b.Alias)
			mockRedisRepo.EXPECT().
				LockBalanceRedis(gomock.Any(), internalKey, gomock.Any(), gomock.Any(), gomock.Any()).
				Return(b, nil).
				Times(1)
		}

		// Call the method
		lockedBalances, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, validate, balances)

		// Assertions
		assert.NoError(t, err)
		assert.Len(t, lockedBalances, 2)
		assert.Equal(t, "alias1", lockedBalances[0].Alias)
		assert.Equal(t, "alias2", lockedBalances[1].Alias)
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
				Available:      10000,
				OnHold:         0,
				Scale:          2,
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
				Available:      10000000000000000,
				OnHold:         0,
				Scale:          14,
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
				Available:      5000,
				OnHold:         1000,
				Scale:          2,
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
				Available:      -10000,
				OnHold:         0,
				Scale:          2,
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
				Available:      1500,
				OnHold:         0,
				Scale:          2,
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

			// Verifica se deve ocorrer erro
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Se não esperamos erro, verifica os valores
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
				if b.Available != tt.want.Available {
					t.Errorf("Available: got = %v, want %v", b.Available, tt.want.Available)
				}
				if b.OnHold != tt.want.OnHold {
					t.Errorf("OnHold: got = %v, want %v", b.OnHold, tt.want.OnHold)
				}
				if b.Scale != tt.want.Scale {
					t.Errorf("Scale: got = %v, want %v", b.Scale, tt.want.Scale)
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
