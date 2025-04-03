package query

import (
	"context"
	"encoding/json"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// \1 performs an operation
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

// \1 performs an operation
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

// \1 performs an operation
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
