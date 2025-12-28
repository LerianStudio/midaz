package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestUpdateBalanceSuccess is responsible to test UpdateBalanceSuccess with success
func TestUpdateBalanceSuccess(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	allowSending := false

	balanceUpdate := mmodel.UpdateBalance{
		AllowSending:   &allowSending,
		AllowReceiving: nil,
	}

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(nil).
		Times(1)
	err := uc.BalanceRepo.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	assert.Nil(t, err)
}

// TestUpdateBalanceError is responsible to test UpdateBalanceError with error
func TestUpdateBalanceError(t *testing.T) {
	errMSG := "errDatabaseItemNotFound"
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	allowSending := true
	allowReceiving := false

	balanceUpdate := mmodel.UpdateBalance{
		AllowSending:   &allowSending,
		AllowReceiving: &allowReceiving,
	}

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(errors.New(errMSG))
	err := uc.BalanceRepo.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}

func TestFilterStaleBalances(t *testing.T) {
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	tests := []struct {
		name           string
		balances       []*mmodel.Balance
		setupMocks     func(mockRedis *redis.MockRedisRepository)
		expectedCount  int
		expectedIDs    []string
	}{
		{
			name: "cache_newer_version_filters_balance",
			balances: []*mmodel.Balance{
				{ID: "balance-1", Alias: "0#@account1#default", Version: 5},
			},
			setupMocks: func(mockRedis *redis.MockRedisRepository) {
				mockRedis.EXPECT().
					ListBalanceByKey(gomock.Any(), orgID, ledgerID, "@account1#default").
					Return(&mmodel.Balance{Version: 10}, nil)
			},
			expectedCount: 0,
			expectedIDs:   []string{},
		},
		{
			name: "cache_older_version_includes_balance",
			balances: []*mmodel.Balance{
				{ID: "balance-1", Alias: "0#@account1#default", Version: 5},
			},
			setupMocks: func(mockRedis *redis.MockRedisRepository) {
				mockRedis.EXPECT().
					ListBalanceByKey(gomock.Any(), orgID, ledgerID, "@account1#default").
					Return(&mmodel.Balance{Version: 3}, nil)
			},
			expectedCount: 1,
			expectedIDs:   []string{"balance-1"},
		},
		{
			name: "cache_equal_version_includes_balance",
			balances: []*mmodel.Balance{
				{ID: "balance-1", Alias: "0#@account1#default", Version: 5},
			},
			setupMocks: func(mockRedis *redis.MockRedisRepository) {
				mockRedis.EXPECT().
					ListBalanceByKey(gomock.Any(), orgID, ledgerID, "@account1#default").
					Return(&mmodel.Balance{Version: 5}, nil)
			},
			expectedCount: 1,
			expectedIDs:   []string{"balance-1"},
		},
		{
			name: "cache_error_fail_open_includes_balance",
			balances: []*mmodel.Balance{
				{ID: "balance-1", Alias: "0#@account1#default", Version: 5},
			},
			setupMocks: func(mockRedis *redis.MockRedisRepository) {
				mockRedis.EXPECT().
					ListBalanceByKey(gomock.Any(), orgID, ledgerID, "@account1#default").
					Return(nil, errors.New("redis connection error"))
			},
			expectedCount: 1,
			expectedIDs:   []string{"balance-1"},
		},
		{
			name: "cache_returns_nil_includes_balance",
			balances: []*mmodel.Balance{
				{ID: "balance-1", Alias: "0#@account1#default", Version: 5},
			},
			setupMocks: func(mockRedis *redis.MockRedisRepository) {
				mockRedis.EXPECT().
					ListBalanceByKey(gomock.Any(), orgID, ledgerID, "@account1#default").
					Return(nil, nil)
			},
			expectedCount: 1,
			expectedIDs:   []string{"balance-1"},
		},
		{
			name: "multiple_balances_filters_only_stale",
			balances: []*mmodel.Balance{
				{ID: "balance-1", Alias: "0#@account1#default", Version: 5},
				{ID: "balance-2", Alias: "1#@account2#default", Version: 8},
				{ID: "balance-3", Alias: "2#@account3#default", Version: 3},
			},
			setupMocks: func(mockRedis *redis.MockRedisRepository) {
				// balance-1: cache version 10 > update version 5 → filtered
				mockRedis.EXPECT().
					ListBalanceByKey(gomock.Any(), orgID, ledgerID, "@account1#default").
					Return(&mmodel.Balance{Version: 10}, nil)
				// balance-2: cache version 5 < update version 8 → included
				mockRedis.EXPECT().
					ListBalanceByKey(gomock.Any(), orgID, ledgerID, "@account2#default").
					Return(&mmodel.Balance{Version: 5}, nil)
				// balance-3: cache error → included (fail-open)
				mockRedis.EXPECT().
					ListBalanceByKey(gomock.Any(), orgID, ledgerID, "@account3#default").
					Return(nil, errors.New("timeout"))
			},
			expectedCount: 2,
			expectedIDs:   []string{"balance-2", "balance-3"},
		},
		{
			name:     "empty_balances_returns_empty",
			balances: []*mmodel.Balance{},
			setupMocks: func(mockRedis *redis.MockRedisRepository) {
				// No calls expected
			},
			expectedCount: 0,
			expectedIDs:   []string{},
		},
		{
			name: "alias_without_index_prefix",
			balances: []*mmodel.Balance{
				// Alias without index prefix: "@account1#default" → SplitAliasWithKey returns "default"
				{ID: "balance-1", Alias: "@account1#default", Version: 5},
			},
			setupMocks: func(mockRedis *redis.MockRedisRepository) {
				// SplitAliasWithKey("@account1#default") returns "default" (after first #)
				mockRedis.EXPECT().
					ListBalanceByKey(gomock.Any(), orgID, ledgerID, "default").
					Return(&mmodel.Balance{Version: 3}, nil)
			},
			expectedCount: 1,
			expectedIDs:   []string{"balance-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockRedis := redis.NewMockRedisRepository(ctrl)
			tt.setupMocks(mockRedis)

			uc := &UseCase{
				RedisRepo: mockRedis,
			}

			logger := &libLog.GoLogger{Level: libLog.InfoLevel}
			ctx := context.Background()

			result := uc.filterStaleBalances(ctx, orgID, ledgerID, tt.balances, logger)

			require.Len(t, result, tt.expectedCount)

			resultIDs := make([]string, len(result))
			for i, b := range result {
				resultIDs[i] = b.ID
			}
			assert.ElementsMatch(t, tt.expectedIDs, resultIDs)
		})
	}
}
