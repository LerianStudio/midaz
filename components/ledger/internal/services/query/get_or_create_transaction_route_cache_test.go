// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	pkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestGetOrCreateTransactionRouteCache_CacheHit tests successful cache hit
func TestGetOrCreateTransactionRouteCache_CacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	// Create cache data with Actions populated
	cacheToSerialize := mmodel.TransactionRouteCache{
		Actions: map[string]mmodel.ActionRouteCache{
			"direct": {
				Source:        map[string]mmodel.OperationRouteCache{"operation1": {Account: &mmodel.AccountCache{RuleType: "alias", ValidIf: "@cash"}, OperationType: "source"}},
				Destination:   map[string]mmodel.OperationRouteCache{},
				Bidirectional: map[string]mmodel.OperationRouteCache{},
			},
		},
	}
	expectedCacheBytes, err := cacheToSerialize.ToMsgpack()
	require.NoError(t, err)

	// Deserialize to get the actual expected shape after msgpack roundtrip
	// (msgpack may convert empty maps to nil for omitempty fields)
	var expectedCacheData mmodel.TransactionRouteCache
	err = expectedCacheData.FromMsgpack(expectedCacheBytes)
	require.NoError(t, err)

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

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())
	operationRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	transactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:                operationRouteID,
				OperationType:     "source",
				AccountingEntries: &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{}},
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash_account",
				},
			},
		},
	}

	expectedCacheData := transactionRoute.ToCache()
	expectedCacheBytes, err := expectedCacheData.ToMsgpack()
	require.NoError(t, err)

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

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	transactionRoute := &mmodel.TransactionRoute{
		ID:              transactionRouteID,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	expectedCacheData := transactionRoute.ToCache()
	expectedCacheBytes, err := expectedCacheData.ToMsgpack()
	require.NoError(t, err)

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

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	transactionRoute := &mmodel.TransactionRoute{
		ID:              transactionRouteID,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	expectedCacheData := transactionRoute.ToCache()
	expectedCacheBytes, err := expectedCacheData.ToMsgpack()
	require.NoError(t, err)

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

// TestGetOrCreateTransactionRouteCache_TransactionRouteNotFound tests handling when transaction route is not found.
// When DB returns ErrDatabaseItemNotFound, the function must store a NOT_FOUND sentinel in Redis with 60s TTL
// so that subsequent lookups for the same non-existent ID are served from cache.
func TestGetOrCreateTransactionRouteCache_TransactionRouteNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return([]byte{}, goredis.Nil).
		Times(1)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	// Sentinel must be stored in Redis with 60s TTL when DB returns not-found
	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, []byte("NOT_FOUND"), time.Duration(60)).
		Return(nil).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err, "should return error when route not found in DB")
	requireTransactionRouteNotFound(t, err, "error should carry the 404 identity of a missing transaction route")
	assert.Equal(t, mmodel.TransactionRouteCache{}, result, "result should be zero-value cache struct")
}

// TestGetOrCreateTransactionRouteCache_DBNotFound_ReturnsTypedNotFound pins the HTTP identity of the
// database not-found arm. The repository returns the typed business error (production shape) when a
// transaction names a route owned by another ledger; the use case must hand that same typed error
// back so the HTTP layer answers 404, not 500.
func TestGetOrCreateTransactionRouteCache_DBNotFound_ReturnsTypedNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return(nil, goredis.Nil).
		Times(1)

	// Production shape: the PostgreSQL repository returns the typed business error.
	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, constant.EntityTransactionRoute)).
		Times(1)

	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, []byte("NOT_FOUND"), sentinelTTL).
		Return(nil).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	var entityNotFound pkg.EntityNotFoundError

	require.True(t, errors.As(err, &entityNotFound), "error must be a pkg.EntityNotFoundError so the HTTP layer answers 404")
	assert.Equal(t, "0105", entityNotFound.Code, "error code must stay 0105 (transaction route not found)")
	assert.Equal(t, mmodel.TransactionRouteCache{}, result, "result should be zero-value cache struct")
}

// TestGetOrCreateTransactionRouteCache_DatabaseError tests database error handling
func TestGetOrCreateTransactionRouteCache_DatabaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)
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

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	transactionRoute := &mmodel.TransactionRoute{
		ID:              transactionRouteID,
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Title:           "Test Route",
		OperationRoutes: []mmodel.OperationRoute{},
	}

	expectedCacheData := transactionRoute.ToCache()
	expectedCacheBytes, err := expectedCacheData.ToMsgpack()
	require.NoError(t, err)
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

// TestGetOrCreateTransactionRouteCache_CacheHit_NotFoundSentinel tests that when Redis returns the NOT_FOUND
// sentinel value, the function returns ErrDatabaseItemNotFound immediately without making a DB call.
func TestGetOrCreateTransactionRouteCache_CacheHit_NotFoundSentinel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// No TransactionRouteRepo mock set — if DB is called, gomock will fail with unexpected call
	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	// Redis returns the sentinel value
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return([]byte("NOT_FOUND"), nil).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err, "should return error when sentinel found in cache")
	requireTransactionRouteNotFound(t, err, "sentinel hit should carry the 404 identity of a missing transaction route")
	assert.Equal(t, mmodel.TransactionRouteCache{}, result, "result should be zero-value cache struct")
}

// TestGetOrCreateTransactionRouteCache_CacheHit_NotFoundSentinel_ReturnsTypedNotFound pins the HTTP
// identity of the sentinel arm. A retry inside the sentinel TTL must answer 404 exactly like the first
// attempt: this is the arm whose 500s opened client circuit breakers.
func TestGetOrCreateTransactionRouteCache_CacheHit_NotFoundSentinel_ReturnsTypedNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// No TransactionRouteRepo set — any DB touch fails the test.
	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return([]byte("NOT_FOUND"), nil).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	var entityNotFound pkg.EntityNotFoundError

	require.True(t, errors.As(err, &entityNotFound), "sentinel hit must be a pkg.EntityNotFoundError so the retry also answers 404")
	assert.Equal(t, "0105", entityNotFound.Code, "error code must stay 0105 (transaction route not found)")
	assert.Equal(t, mmodel.TransactionRouteCache{}, result, "result should be zero-value cache struct")
}

// TestGetOrCreateTransactionRouteCache_CacheMiss_NotFound_StoresSentinel tests that when Redis returns a miss
// and DB returns ErrDatabaseItemNotFound, the sentinel is stored in Redis with a 60-second TTL.
func TestGetOrCreateTransactionRouteCache_CacheMiss_NotFound_StoresSentinel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	// Cache miss
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return(nil, goredis.Nil).
		Times(1)

	// DB returns not found
	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	// Sentinel must be stored with exactly 60s TTL
	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, []byte("NOT_FOUND"), time.Duration(60)).
		Return(nil).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err, "should return error when route not found")
	requireTransactionRouteNotFound(t, err, "error should carry the 404 identity of a missing transaction route")
	assert.Equal(t, mmodel.TransactionRouteCache{}, result, "result should be zero-value cache struct")
}

// TestGetOrCreateTransactionRouteCache_CacheHit_CorruptedData_FallsBackToDB tests that when Redis returns
// data that is neither a sentinel nor valid msgpack, the function falls back to DB instead of returning
// a decode error.
func TestGetOrCreateTransactionRouteCache_CacheHit_CorruptedData_FallsBackToDB(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())
	operationRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	// Redis returns corrupted data (not sentinel, not valid msgpack)
	corruptedBytes := []byte{0xFF, 0xFE, 0xAB, 0xCD, 0x00, 0x01}

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return(corruptedBytes, nil).
		Times(1)

	// DB should be called as fallback
	transactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:                operationRouteID,
				OperationType:     "source",
				AccountingEntries: &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{}},
				Account: &mmodel.AccountRule{
					RuleType: "alias",
					ValidIf:  "@cash_account",
				},
			},
		},
	}

	expectedCacheData := transactionRoute.ToCache()
	expectedCacheBytes, err := expectedCacheData.ToMsgpack()
	require.NoError(t, err)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(transactionRoute, nil).
		Times(1)

	// After successful DB fetch, cache should be updated
	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, expectedCacheBytes, gomock.Any()).
		Return(nil).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.NoError(t, err, "should not return error when falling back to DB after corrupted cache")
	assert.Equal(t, expectedCacheData, result, "should return valid data from DB fallback")
}

// TestGetOrCreateTransactionRouteCache_SentinelSetBytesFails tests that when Redis fails to store the
// not-found sentinel, the function still returns ErrDatabaseItemNotFound (the write error is logged but swallowed).
func TestGetOrCreateTransactionRouteCache_SentinelSetBytesFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	// Cache miss
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return(nil, goredis.Nil).
		Times(1)

	// DB returns not found
	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, transactionRouteID).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	// Sentinel storage fails
	mockRedisRepo.EXPECT().
		SetBytes(gomock.Any(), expectedKey, []byte("NOT_FOUND"), time.Duration(60)).
		Return(errors.New("redis write error")).
		Times(1)

	result, err := uc.GetOrCreateTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err, "should still return error when sentinel storage fails")
	requireTransactionRouteNotFound(t, err, "error should carry the 404 identity regardless of sentinel write failure")
	assert.Equal(t, mmodel.TransactionRouteCache{}, result, "result should be zero-value cache struct")
}

// TestGetOrCreateTransactionRouteCache_ToCacheDataError tests msgpack encoding error handling
func TestGetOrCreateTransactionRouteCache_ToCacheDataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
		TransactionRouteRepo: mockTransactionRouteRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	// Create a transaction route with data that might cause msgpack encoding issues
	transactionRoute := &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Test Route",
		OperationRoutes: []mmodel.OperationRoute{
			{
				ID:                uuid.UUID{},
				OperationType:     "source",
				AccountingEntries: &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{}},
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

// requireTransactionRouteNotFound asserts that err carries the 404 identity of a missing transaction
// route: the typed pkg.EntityNotFoundError with code 0105. The type alone is not enough — the code is
// what the HTTP layer maps to 404.
func requireTransactionRouteNotFound(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()

	var entityNotFound pkg.EntityNotFoundError

	require.True(t, errors.As(err, &entityNotFound), msgAndArgs...)
	require.Equal(t, "0105", entityNotFound.Code, msgAndArgs...)
}

// TestGetOrCreateTransactionRouteCache_SentinelArmStaysCheap pins the one property that makes
// negative caching worth having: a retry inside the sentinel TTL must cost almost nothing. The
// arm touches no Postgres and no Redis write, so its rate is bounded only by the caller — which
// is exactly the midaz#2506 shape, a plugin pointed at the wrong ledger retrying in a loop.
//
// pkg.ValidateBusinessError rebuilds midaz's entire 426-entry business-error catalogue, with all
// of its fmt.Sprintf calls, on every invocation (~500 allocations, ~63 KB) before doing a single
// map lookup. Calling it inside this arm would make the I/O-free shortcut the most expensive
// branch in the function, so the error is built once at package level instead.
func TestGetOrCreateTransactionRouteCache_SentinelArmStaysCheap(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionRouteID := uuid.Must(libCommons.GenerateUUIDv7())

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// No TransactionRouteRepo set — any DB touch fails the test.
	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	expectedKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), expectedKey).
		Return([]byte("NOT_FOUND"), nil).
		AnyTimes()

	ctx := context.Background()

	var lastErr error

	allocs := testing.AllocsPerRun(20, func() {
		_, lastErr = uc.GetOrCreateTransactionRouteCache(ctx, organizationID, ledgerID, transactionRouteID)
	})

	requireTransactionRouteNotFound(t, lastErr, "the cheap arm must still answer with the 404 identity")

	// The ceiling is the error catalogue, not a micro-benchmark: rebuilding it costs ~500
	// allocations, while the arm itself plus the gomock stub costs well under 100.
	assert.Lessf(t, allocs, 100.0, "the sentinel arm allocated %.0f objects per call; it must not rebuild the business-error catalogue", allocs)
}
