package command

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateOrCheckIdempotencyKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	hash := "test-hash-value"
	ttl := 24 * time.Hour

	t.Run("success with key", func(t *testing.T) {
		key := "test-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		// Mock Redis.SetNX - success case (key doesn't exist)
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(true, nil).
			Times(1)

		// Call the method
		value, createdInternalKey, err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)

		// Assertions
		assert.NoError(t, err)
		assert.Nil(t, value)
		assert.Equal(t, &internalKey, createdInternalKey)
	})

	t.Run("success with empty key", func(t *testing.T) {
		// When key is empty, it should use the hash value
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, hash)

		// Mock Redis.SetNX - success case (key doesn't exist)
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(true, nil).
			Times(1)

		// Call the method
		value, createdInternalKey, err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, "", hash, ttl)

		// Assertions
		assert.NoError(t, err)
		assert.Nil(t, value)
		assert.Equal(t, &internalKey, createdInternalKey)
	})

	t.Run("key already exists with cached value", func(t *testing.T) {
		key := "existing-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		existingValue := "existing-transaction-json"

		// Mock Redis.SetNX - failure case (key already exists)
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		// Mock Redis.Get - return existing value
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return(existingValue, nil).
			Times(1)

		// Call the method
		value, createdInternalKey, err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)

		// Assertions
		assert.NoError(t, err) // Based on the actual implementation, this should not error when value is found
		assert.NotNil(t, value)
		assert.Equal(t, existingValue, *value)
		assert.Equal(t, &internalKey, createdInternalKey)
	})

	t.Run("key exists with empty value returns idempotency error", func(t *testing.T) {
		key := "test-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		// Mock Redis.SetNX - key exists (in-flight transaction)
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		// Mock Redis.Get - return empty value (transaction still processing)
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return("", nil).
			Times(1)

		// Call the method
		value, createdInternalKey, err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)

		// Assertions - should return idempotency error because key is in use
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already in use")
		assert.Nil(t, value)
		assert.Equal(t, &internalKey, createdInternalKey)
	})

	t.Run("key disappeared between SetNX and Get returns idempotency error", func(t *testing.T) {
		key := "disappearing-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		// Mock Redis.SetNX - key exists (returns false)
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		// Mock Redis.Get - key no longer exists (redis.Nil)
		// This simulates the edge case where key expired/was deleted between SetNX and Get
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return("", goredis.Nil).
			Times(1)

		// Call the method
		value, createdInternalKey, err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)

		// Current behavior: returns idempotency error (key existed but now has no value)
		// Note: This could be improved to retry SetNX instead
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already in use")
		assert.Nil(t, value)
		assert.Equal(t, &internalKey, createdInternalKey)
	})

	t.Run("SetNX returns error", func(t *testing.T) {
		key := "error-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		expectedErr := assert.AnError

		// Mock Redis.SetNX - returns error
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, expectedErr).
			Times(1)

		// Call the method
		value, createdInternalKey, err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)

		// Assertions
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, value)
		assert.Equal(t, &internalKey, createdInternalKey)
	})

	t.Run("Get returns non-nil error other than redis.Nil", func(t *testing.T) {
		key := "get-error-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		expectedErr := assert.AnError

		// Mock Redis.SetNX - key exists
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		// Mock Redis.Get - returns error (not redis.Nil)
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return("", expectedErr).
			Times(1)

		// Call the method
		value, createdInternalKey, err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)

		// Assertions
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, value)
		assert.Equal(t, &internalKey, createdInternalKey)
	})
}

func TestSetValueOnExistingIdempotencyKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	hash := "test-hash-value"
	ttl := 24 * time.Hour

	t.Run("success with key", func(t *testing.T) {
		key := "test-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		txn := transaction.Transaction{
			ID:                       uuid.New().String(),
			ParentTransactionID:      nil,
			OrganizationID:           organizationID.String(),
			LedgerID:                 ledgerID.String(),
			Description:              "Test transaction",
			ChartOfAccountsGroupName: "test-group",
			Status:                   transaction.Status{Code: "COMMITTED"},
		}

		expectedValue, err := json.Marshal(txn)
		require.NoError(t, err)

		// Mock Redis.Set - success case
		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, string(expectedValue), ttl).
			Return(nil).
			Times(1)

		// Call the method
		uc.SetValueOnExistingIdempotencyKey(ctx, organizationID, ledgerID, key, hash, txn, ttl)
	})

	t.Run("success with empty key", func(t *testing.T) {
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, hash)

		txn := transaction.Transaction{
			ID:                       uuid.New().String(),
			ParentTransactionID:      nil,
			OrganizationID:           organizationID.String(),
			LedgerID:                 ledgerID.String(),
			Description:              "Test transaction with empty key",
			ChartOfAccountsGroupName: "test-group",
			Status:                   transaction.Status{Code: "COMMITTED"},
		}

		expectedValue, err := json.Marshal(txn)
		require.NoError(t, err)

		// Mock Redis.Set - success case
		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, string(expectedValue), ttl).
			Return(nil).
			Times(1)

		// Call the method
		uc.SetValueOnExistingIdempotencyKey(ctx, organizationID, ledgerID, "", hash, txn, ttl)
	})

	t.Run("redis set error", func(t *testing.T) {
		key := "test-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		txn := transaction.Transaction{
			ID:                       uuid.New().String(),
			ParentTransactionID:      nil,
			OrganizationID:           organizationID.String(),
			LedgerID:                 ledgerID.String(),
			Description:              "Test transaction with redis error",
			ChartOfAccountsGroupName: "test-group",
			Status:                   transaction.Status{Code: "COMMITTED"},
		}

		expectedValue, err := json.Marshal(txn)
		require.NoError(t, err)

		// Mock Redis.Set - error case
		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, string(expectedValue), ttl).
			Return(assert.AnError).
			Times(1)

		// Call the method - should not panic or return error
		uc.SetValueOnExistingIdempotencyKey(ctx, organizationID, ledgerID, key, hash, txn, ttl)
	})
}
