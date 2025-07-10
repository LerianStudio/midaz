package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestReloadOperationRouteCache_Success tests successful cache reload with multiple transaction routes
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

	// Mock Redis Set calls for CreateAccountingRouteCache
	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(2)

	err := uc.ReloadOperationRouteCache(context.Background(), organizationID, ledgerID, operationRouteID)

	assert.NoError(t, err)
}

// TestReloadOperationRouteCache_NoTransactionRoutes tests successful handling when no transaction routes are found
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

	assert.NoError(t, err)
}

// TestReloadOperationRouteCache_FindTransactionRouteIDsError tests error handling when FindTransactionRouteIDs fails
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

	dbError := errors.New("database connection error")

	mockOperationRouteRepo.EXPECT().
		FindTransactionRouteIDs(gomock.Any(), operationRouteID).
		Return(nil, dbError).
		Times(1)

	err := uc.ReloadOperationRouteCache(context.Background(), organizationID, ledgerID, operationRouteID)

	assert.Error(t, err)
	assert.Equal(t, dbError, err)
}

// TestReloadOperationRouteCache_TransactionRouteNotFound tests handling when transaction route is not found
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

	dbError := errors.New("transaction route not found")

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, dbError).
		Times(1)

	err := uc.ReloadOperationRouteCache(context.Background(), organizationID, ledgerID, operationRouteID)

	assert.NoError(t, err)
}

// TestReloadOperationRouteCache_CreateCacheError tests handling when cache creation fails
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

	redisError := errors.New("redis connection error")

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(redisError).
		Times(1)

	err := uc.ReloadOperationRouteCache(context.Background(), organizationID, ledgerID, operationRouteID)

	assert.NoError(t, err)
}

// TestReloadOperationRouteCache_PartialFailure tests handling when some operations fail but others succeed
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
		Return(nil, errors.New("route not found")).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID2).
		Return(transactionRoute2, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	err := uc.ReloadOperationRouteCache(context.Background(), organizationID, ledgerID, operationRouteID)

	assert.NoError(t, err)
}
