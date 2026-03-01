// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// Sentinel errors for test assertions.
var (
	errTestDBConnectionRORC         = errors.New("database connection error")
	errTestTransactionRouteNotFound = errors.New("transaction route not found")
	errTestRedisConnectionRORC      = errors.New("redis connection error")
	errTestRouteNotFound            = errors.New("route not found")
)

// TestReloadOperationRouteCache_Success tests successful cache reload with multiple transaction routes.
func TestReloadOperationRouteCache_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	operationRouteID := libCommons.GenerateUUIDv7()
	transactionRouteID1 := libCommons.GenerateUUIDv7()
	transactionRouteID2 := libCommons.GenerateUUIDv7()

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:   mockOperationRouteRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
		RedisRepo:            mockRedisRepo,
	}

	transactionRouteIDs := []uuid.UUID{transactionRouteID1, transactionRouteID2}

	transactionRoute1 := &mmodel.TransactionRoute{
		ID:              transactionRouteID1,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route 1",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	transactionRoute2 := &mmodel.TransactionRoute{
		ID:              transactionRouteID2,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route 2",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	// Mock expectations
	mockOperationRouteRepo.EXPECT().
		FindTransactionRouteIDs(gomock.Any(), operationRouteID).
		Return(transactionRouteIDs, nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID1).
		Return(transactionRoute1, nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID2).
		Return(transactionRoute2, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(2)

	err := uc.ReloadOperationRouteCache(context.Background(), organizationID, ledgerID, operationRouteID)

	require.NoError(t, err)
}

// TestReloadOperationRouteCache_NoTransactionRoutes tests successful handling when no transaction routes are found.
func TestReloadOperationRouteCache_NoTransactionRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	operationRouteID := libCommons.GenerateUUIDv7()

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:   mockOperationRouteRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
		RedisRepo:            mockRedisRepo,
	}

	mockOperationRouteRepo.EXPECT().
		FindTransactionRouteIDs(gomock.Any(), operationRouteID).
		Return([]uuid.UUID{}, nil).
		Times(1)

	err := uc.ReloadOperationRouteCache(context.Background(), organizationID, ledgerID, operationRouteID)

	require.NoError(t, err)
}

// TestReloadOperationRouteCache_FindTransactionRouteIDsError tests error handling when FindTransactionRouteIDs fails.
func TestReloadOperationRouteCache_FindTransactionRouteIDsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	operationRouteID := libCommons.GenerateUUIDv7()

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:   mockOperationRouteRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
		RedisRepo:            mockRedisRepo,
	}

	dbError := errTestDBConnectionRORC

	mockOperationRouteRepo.EXPECT().
		FindTransactionRouteIDs(gomock.Any(), operationRouteID).
		Return(nil, dbError).
		Times(1)

	err := uc.ReloadOperationRouteCache(context.Background(), organizationID, ledgerID, operationRouteID)

	require.Error(t, err)
	assert.Equal(t, dbError, err)
}

// TestReloadOperationRouteCache_TransactionRouteNotFound tests handling when transaction route is not found.
func TestReloadOperationRouteCache_TransactionRouteNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	operationRouteID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:   mockOperationRouteRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
		RedisRepo:            mockRedisRepo,
	}

	transactionRouteIDs := []uuid.UUID{transactionRouteID}

	mockOperationRouteRepo.EXPECT().
		FindTransactionRouteIDs(gomock.Any(), operationRouteID).
		Return(transactionRouteIDs, nil).
		Times(1)

	dbError := errTestTransactionRouteNotFound

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, dbError).
		Times(1)

	err := uc.ReloadOperationRouteCache(context.Background(), organizationID, ledgerID, operationRouteID)

	require.NoError(t, err)
}

// TestReloadOperationRouteCache_CreateCacheError tests handling when cache creation fails.
func TestReloadOperationRouteCache_CreateCacheError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	operationRouteID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:   mockOperationRouteRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
		RedisRepo:            mockRedisRepo,
	}

	transactionRouteIDs := []uuid.UUID{transactionRouteID}

	transactionRoute := &mmodel.TransactionRoute{
		ID:              transactionRouteID,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	mockOperationRouteRepo.EXPECT().
		FindTransactionRouteIDs(gomock.Any(), operationRouteID).
		Return(transactionRouteIDs, nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	redisError := errTestRedisConnectionRORC

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(redisError).
		Times(1)

	err := uc.ReloadOperationRouteCache(context.Background(), organizationID, ledgerID, operationRouteID)

	require.NoError(t, err)
}

// TestReloadOperationRouteCache_PartialFailure tests handling when some operations fail but others succeed.
func TestReloadOperationRouteCache_PartialFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	operationRouteID := libCommons.GenerateUUIDv7()
	transactionRouteID1 := libCommons.GenerateUUIDv7()
	transactionRouteID2 := libCommons.GenerateUUIDv7()

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:   mockOperationRouteRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
		RedisRepo:            mockRedisRepo,
	}

	transactionRouteIDs := []uuid.UUID{transactionRouteID1, transactionRouteID2}

	transactionRoute2 := &mmodel.TransactionRoute{
		ID:              transactionRouteID2,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route 2",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	mockOperationRouteRepo.EXPECT().
		FindTransactionRouteIDs(gomock.Any(), operationRouteID).
		Return(transactionRouteIDs, nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID1).
		Return(nil, errTestRouteNotFound).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID2).
		Return(transactionRoute2, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	err := uc.ReloadOperationRouteCache(context.Background(), organizationID, ledgerID, operationRouteID)

	require.NoError(t, err)
}
