package query

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetOrCreateTransactionRouteCache_CacheHit tests successful cache hit
func TestGetOrCreateTransactionRouteCache_CacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	// Create expected cache data in msgpack format
	expectedCacheData := mmodel.TransactionRouteCache{
		Source:      map[string]mmodel.OperationRouteCache{"operation1": {Account: &mmodel.AccountCache{RuleType: "alias", ValidIf: "@cash"}}},
		Destination: map[string]mmodel.OperationRouteCache{},
	}
	expectedCacheBytes, _ := expectedCacheData.ToMsgpack()

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return(expectedCacheBytes, nil).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.NoError(t, err)
	assert.Equal(t, expectedCacheData, result)
}

// TestGetOrCreateTransactionRouteCache_CacheMiss_Success tests successful cache miss with database retrieval
func TestGetOrCreateTransactionRouteCache_CacheMiss_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()
	operationRouteID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:            mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	transactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:            operationRouteID,
				OperationType: "source",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash_account",
				},
			},
		},
	}

	expectedCacheData := transactionRoute.ToCache()
	expectedCacheBytes, _ := expectedCacheData.ToMsgpack()

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return([]byte{}, goredis.Nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, expectedCacheBytes, gomock.Any()).
		Return(nil).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.NoError(t, err)
	assert.Equal(t, expectedCacheData, result)
}

// TestGetOrCreateTransactionRouteCache_CacheMiss_EmptyCache tests cache miss with empty cache value
func TestGetOrCreateTransactionRouteCache_CacheMiss_EmptyCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:            mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	transactionRoute := &mmodel.TransactionRoute{
		ID:              transactionRouteID,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	expectedCacheData := transactionRoute.ToCache()
	expectedCacheBytes, _ := expectedCacheData.ToMsgpack()

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return([]byte{}, nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, expectedCacheBytes, gomock.Any()).
		Return(nil).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.NoError(t, err)
	assert.Equal(t, expectedCacheData, result)
}

// TestGetOrCreateTransactionRouteCache_RedisGetError tests Redis get error handling
func TestGetOrCreateTransactionRouteCache_RedisGetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:            mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	transactionRoute := &mmodel.TransactionRoute{
		ID:              transactionRouteID,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	expectedCacheData := transactionRoute.ToCache()
	expectedCacheBytes, _ := expectedCacheData.ToMsgpack()

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return([]byte{}, errors.New("redis connection error")).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, expectedCacheBytes, gomock.Any()).
		Return(nil).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.NoError(t, err)
	assert.Equal(t, expectedCacheData, result)
}

// TestGetOrCreateTransactionRouteCache_TransactionRouteNotFound tests handling when transaction route is not found
func TestGetOrCreateTransactionRouteCache_TransactionRouteNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:            mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return([]byte{}, goredis.Nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)
	assert.Equal(t, services.ErrDatabaseItemNotFound, err)
	assert.Equal(t, mmodel.TransactionRouteCache{}, result)
}

// TestGetOrCreateTransactionRouteCache_DatabaseError tests database error handling
func TestGetOrCreateTransactionRouteCache_DatabaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:            mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)
	dbError := errors.New("database connection error")

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return([]byte{}, goredis.Nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, dbError).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)
	assert.Equal(t, dbError, err)
	assert.Equal(t, mmodel.TransactionRouteCache{}, result)
}

// TestGetOrCreateTransactionRouteCache_CacheCreationFails tests Redis set error handling
func TestGetOrCreateTransactionRouteCache_CacheCreationFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:            mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	transactionRoute := &mmodel.TransactionRoute{
		ID:              transactionRouteID,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	expectedCacheData := transactionRoute.ToCache()
	expectedCacheBytes, _ := expectedCacheData.ToMsgpack()
	redisError := errors.New("redis connection error")

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return([]byte{}, goredis.Nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, expectedCacheBytes, gomock.Any()).
		Return(redisError).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)
	assert.Equal(t, redisError, err)
	assert.Equal(t, mmodel.TransactionRouteCache{}, result)
}

// TestGetOrCreateTransactionRouteCache_ToCacheDataError tests msgpack encoding error handling
func TestGetOrCreateTransactionRouteCache_ToCacheDataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		RedisRepo:            mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	// Create a transaction route with data that might cause msgpack encoding issues
	transactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:            uuid.UUID{},
				OperationType: "source",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  make(chan int), // This will cause msgpack encoding error
				},
			},
		},
	}

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return([]byte{}, goredis.Nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)
	assert.Equal(t, mmodel.TransactionRouteCache{}, result)
}
