// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestUpdateBalance tests the Update method with no Redis cached value
func TestUpdateBalance(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	allowSending := false

	balanceUpdate := mmodel.UpdateBalance{
		AllowSending:   &allowSending,
		AllowReceiving: nil,
	}

	expectedBalance := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@test",
		Key:            "default",
		AllowSending:   false,
		AllowReceiving: true,
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(expectedBalance, nil).
		Times(1)

	// Redis returns empty (no cached value)
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return("", nil).
		Times(1)

	uc := UseCase{
		BalanceRepo: mockBalanceRepo,
		RedisRepo:   mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedBalance.ID, result.ID)
	assert.False(t, result.AllowSending)
}

// TestUpdateBalance_RepoError tests the Update method when repository returns error
func TestUpdateBalance_RepoError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

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

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(nil, errors.New(errMSG))
	// Redis is NOT called when Update fails

	uc := UseCase{
		BalanceRepo: mockBalanceRepo,
	}

	result, err := uc.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	assert.Nil(t, result)
	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}

// TestUpdateBalance_RedisOverlay verifies that when Redis has cached balance values,
// they are overlayed onto the balance returned from the repository.
func TestUpdateBalance_RedisOverlay(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	balanceID := libCommons.GenerateUUIDv7()

	allowSending := false

	balanceUpdate := mmodel.UpdateBalance{
		AllowSending: &allowSending,
	}

	// Repository returns balance with initial values
	repoBalance := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@user1",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.NewFromInt(10),
		Version:        1,
		AllowSending:   false,
		AllowReceiving: true,
	}

	// Redis has fresher values that should be overlayed
	cachedBalance := mmodel.BalanceRedis{
		Available: decimal.NewFromInt(500),
		OnHold:    decimal.NewFromInt(50),
		Version:   5,
	}
	cachedJSON, err := json.Marshal(cachedBalance)
	require.NoError(t, err)

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, balanceID, balanceUpdate).
		Return(repoBalance, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(string(cachedJSON), nil).
		Times(1)

	uc := UseCase{
		BalanceRepo: mockBalanceRepo,
		RedisRepo:   mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), organizationID, ledgerID, balanceID, balanceUpdate)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Redis values should be overlayed
	assert.True(t, result.Available.Equal(decimal.NewFromInt(500)), "Available should be overlayed from Redis")
	assert.True(t, result.OnHold.Equal(decimal.NewFromInt(50)), "OnHold should be overlayed from Redis")
	assert.Equal(t, int64(5), result.Version, "Version should be overlayed from Redis")

	// Other fields should remain unchanged from repository
	assert.Equal(t, balanceID.String(), result.ID)
	assert.Equal(t, "@user1", result.Alias)
	assert.Equal(t, "USD", result.AssetCode)
	assert.False(t, result.AllowSending)
	assert.True(t, result.AllowReceiving)
}
