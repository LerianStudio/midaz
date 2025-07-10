package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetOrCreateTransactionRouteCache_CacheHit tests successful cache retrieval from Redis
func TestGetOrCreateTransactionRouteCache_CacheHit(t *testing.T) {
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

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID.String())
	expectedCacheValue := `{"operation1":{"type":"debit","account":{"ruleType":"alias","validIf":"@cash"}}}`

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), expectedKey).
		Return(expectedCacheValue, nil).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.NoError(t, err)
	assert.Equal(t, expectedCacheValue, result)
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

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID.String())

	transactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:   operationRouteID,
				Type: "debit",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash_account",
				},
			},
		},
	}

	expectedCacheData := `{"` + operationRouteID.String() + `":{"account":{"ruleType":"alias","validIf":"@cash_account"},"type":"debit"}}`

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), expectedKey).
		Return("", goredis.Nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), expectedKey, expectedCacheData, gomock.Any()).
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

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID.String())

	transactionRoute := &mmodel.TransactionRoute{
		ID:              transactionRouteID,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	expectedCacheData := `{}`

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), expectedKey).
		Return("", nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), expectedKey, expectedCacheData, gomock.Any()).
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

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID.String())

	transactionRoute := &mmodel.TransactionRoute{
		ID:              transactionRouteID,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	expectedCacheData := `{}`

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), expectedKey).
		Return("", errors.New("redis connection error")).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), expectedKey, expectedCacheData, gomock.Any()).
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

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID.String())

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), expectedKey).
		Return("", goredis.Nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)
	assert.Equal(t, services.ErrDatabaseItemNotFound, err)
	assert.Equal(t, "", result)
}

// TestGetOrCreateTransactionRouteCache_DatabaseError tests handling when database query fails
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

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID.String())
	dbError := errors.New("database connection error")

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), expectedKey).
		Return("", goredis.Nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, dbError).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)
	assert.Equal(t, dbError, err)
	assert.Equal(t, "", result)
}

// TestGetOrCreateTransactionRouteCache_CacheCreationFails tests handling when cache creation fails
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

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID.String())

	transactionRoute := &mmodel.TransactionRoute{
		ID:              transactionRouteID,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	expectedCacheData := `{}`

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), expectedKey).
		Return("", goredis.Nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), expectedKey, expectedCacheData, gomock.Any()).
		Return(errors.New("redis connection error")).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.NoError(t, err) // Function should still succeed even if cache creation fails
	assert.Equal(t, expectedCacheData, result)
}

// TestGetOrCreateTransactionRouteCache_ToCacheDataError tests handling when ToCacheData fails
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

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID.String())

	transactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:   libCommons.GenerateUUIDv7(),
				Type: "debit",
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  make(chan int), // Invalid data type that will cause JSON marshal error
				},
			},
		},
	}

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), expectedKey).
		Return("", goredis.Nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)
	assert.Equal(t, "", result)
}
