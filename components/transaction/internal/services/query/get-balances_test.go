package query

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
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
		aliases := []string{"alias1#default", "alias2#default", "alias3#default"}

		fromAmount := pkgTransaction.NewTestDebitAmount("USD", decimal.NewFromFloat(50))
		toAmount2 := pkgTransaction.NewTestCreditAmount("EUR", decimal.NewFromFloat(40))
		toAmount3 := pkgTransaction.NewTestCreditAmount("GBP", decimal.NewFromFloat(30))

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
			AddSumBalancesRedis(
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
		fromAmount := pkgTransaction.NewTestDebitAmount("USD", decimal.NewFromFloat(50))
		toAmount := pkgTransaction.NewTestCreditAmount("EUR", decimal.NewFromFloat(40))

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
			AddSumBalancesRedis(
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

		fromAmount := pkgTransaction.NewTestDebitAmount("USD", decimal.NewFromFloat(50))

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
			AddSumBalancesRedis(
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

		balances, remainingAliases := uc.ValidateIfBalanceExistsOnRedis(ctx, &MockLogger{}, organizationID, ledgerID, aliases)

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

func TestListBalancesByAliasesWithKeysWithRetry_IncompleteAfterMaxRetries_ReturnsPartialAndError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := &UseCase{
		BalanceRepo: mockBalanceRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	aliases := []string{"alias1#default", "alias2#default", "alias3#default"}

	partial := []*mmodel.Balance{
		{ID: uuid.New().String(), Alias: "alias1", Key: "default"},
		{ID: uuid.New().String(), Alias: "alias2", Key: "default"},
	}

	mockBalanceRepo.
		EXPECT().
		ListByAliasesWithKeys(gomock.Any(), organizationID, ledgerID, aliases).
		Return(partial, nil).
		Times(maxBalanceLookupAttempts)

	logger := &MockLogger{}
	sleep := func(time.Duration) {}

	got, err := uc.listBalancesByAliasesWithKeysWithRetry(ctx, organizationID, ledgerID, aliases, len(aliases), logger, sleep)
	assert.Error(t, err)
	assert.Len(t, got, len(partial))
	var unprocessable pkg.UnprocessableOperationError
	if assert.ErrorAs(t, err, &unprocessable) {
		assert.Equal(t, constant.ErrAccountIneligibility.Error(), unprocessable.Code)
	}
}

type MockLogger struct{}

func (m *MockLogger) Infof(format string, args ...interface{})        {}
func (m *MockLogger) Warnf(format string, args ...interface{})        {}
func (m *MockLogger) Errorf(format string, args ...interface{})       {}
func (m *MockLogger) Error(args ...interface{})                       {}
func (m *MockLogger) Fatalf(format string, args ...interface{})       {}
func (m *MockLogger) Fatal(args ...interface{})                       {}
func (m *MockLogger) Debugf(format string, args ...interface{})       {}
func (m *MockLogger) Debug(args ...interface{})                       {}
func (m *MockLogger) Info(args ...interface{})                        {}
func (m *MockLogger) Warn(args ...interface{})                        {}
func (m *MockLogger) Debugln(args ...interface{})                     {}
func (m *MockLogger) Infoln(args ...interface{})                      {}
func (m *MockLogger) Warnln(args ...interface{})                      {}
func (m *MockLogger) Errorln(args ...interface{})                     {}
func (m *MockLogger) Fatalln(args ...interface{})                     {}
func (m *MockLogger) Sync() error                                     { return nil }
func (m *MockLogger) WithDefaultMessageTemplate(string) libLog.Logger { return m }
func (m *MockLogger) WithFields(...any) libLog.Logger                 { return m }

// Precondition tests for GetBalances

func TestValidateIfBalanceExistsOnRedis_MalformedAlias_DoesNotPanic_AndFallsBackToDB(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	// Mock Redis to return a valid balance JSON for a malformed alias (no # separator)
	malformedAlias := "alias_without_separator"
	orgID := uuid.New()
	ledgerID := uuid.New()

	// The key format includes the alias, so we need to match any key
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(`{"id":"test-id","accountId":"acc-123","available":"100","onHold":"0","version":1}`, nil)

	ctx := context.Background()
	logger := &MockLogger{}
	balances, remaining := uc.ValidateIfBalanceExistsOnRedis(ctx, logger, orgID, ledgerID, []string{malformedAlias})

	assert.Len(t, balances, 0)
	assert.Len(t, remaining, 1)
	assert.Equal(t, malformedAlias, remaining[0])
}

func TestGetBalances_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetBalances(ctx, uuid.Nil, uuid.New(), uuid.New(), nil, nil, "")
}

func TestGetBalances_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetBalances(ctx, uuid.New(), uuid.Nil, uuid.New(), nil, nil, "")
}

func TestGetBalances_NilTransactionID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil transactionID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "transactionID must not be nil UUID"),
			"panic message should mention transactionID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetBalances(ctx, uuid.New(), uuid.New(), uuid.Nil, nil, nil, "")
}
