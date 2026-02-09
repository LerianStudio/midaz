package command

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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
		internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, key)

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
		assert.Equal(t, createdInternalKey, &internalKey)
	})

	t.Run("success with empty key", func(t *testing.T) {
		// When key is empty, it should use the hash value
		internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, hash)

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
		assert.Equal(t, createdInternalKey, &internalKey)
	})

	t.Run("key already exists", func(t *testing.T) {
		key := "existing-key"
		internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, key)
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
		assert.Equal(t, createdInternalKey, &internalKey)
	})

	t.Run("redis error", func(t *testing.T) {
		key := "test-key"
		internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, key)

		// Mock Redis.SetNX - redis error
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		// Mock Redis.Get - return empty value
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return("", nil).
			Times(1)

		// Call the method
		value, createdInternalKey, err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already in use")
		assert.Nil(t, value)
		assert.Equal(t, createdInternalKey, &internalKey)
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
		internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, key)

		txn := transaction.Transaction{
			ID:                       uuid.New().String(),
			ParentTransactionID:      nil,
			OrganizationID:           organizationID.String(),
			LedgerID:                 ledgerID.String(),
			Description:              "Test transaction",
			ChartOfAccountsGroupName: "test-group",
			Status:                   transaction.Status{Code: "COMMITTED"},
		}

		expectedValue, _ := json.Marshal(txn)

		// Mock Redis.Set - success case
		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, string(expectedValue), ttl).
			Return(nil).
			Times(1)

		// Call the method
		uc.SetValueOnExistingIdempotencyKey(ctx, organizationID, ledgerID, key, hash, txn, ttl)
	})

	t.Run("success with empty key", func(t *testing.T) {
		internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, hash)

		txn := transaction.Transaction{
			ID:                       uuid.New().String(),
			ParentTransactionID:      nil,
			OrganizationID:           organizationID.String(),
			LedgerID:                 ledgerID.String(),
			Description:              "Test transaction with empty key",
			ChartOfAccountsGroupName: "test-group",
			Status:                   transaction.Status{Code: "COMMITTED"},
		}

		expectedValue, _ := json.Marshal(txn)

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
		internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, key)

		txn := transaction.Transaction{
			ID:                       uuid.New().String(),
			ParentTransactionID:      nil,
			OrganizationID:           organizationID.String(),
			LedgerID:                 ledgerID.String(),
			Description:              "Test transaction with redis error",
			ChartOfAccountsGroupName: "test-group",
			Status:                   transaction.Status{Code: "COMMITTED"},
		}

		expectedValue, _ := json.Marshal(txn)

		// Mock Redis.Set - error case
		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, string(expectedValue), ttl).
			Return(assert.AnError).
			Times(1)

		// Call the method - should not panic or return error
		uc.SetValueOnExistingIdempotencyKey(ctx, organizationID, ledgerID, key, hash, txn, ttl)
	})
}

func TestSetTransactionIdempotencyMapping(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()
	idempotencyKey := "test-idempotency-key"
	ttl := time.Duration(5) // Value in seconds (Redis Set multiplies by time.Second internally)

	t.Run("success", func(t *testing.T) {
		expectedKey := utils.IdempotencyReverseKey(organizationID, ledgerID, transactionID)

		// Mock Redis.Set - success case
		mockRedisRepo.EXPECT().
			Set(gomock.Any(), expectedKey, idempotencyKey, ttl).
			Return(nil).
			Times(1)

		// Call the method
		uc.SetTransactionIdempotencyMapping(ctx, organizationID, ledgerID, transactionID, idempotencyKey, ttl)
	})

	t.Run("redis set error logs but does not panic", func(t *testing.T) {
		expectedKey := utils.IdempotencyReverseKey(organizationID, ledgerID, transactionID)

		// Mock Redis.Set - error case
		mockRedisRepo.EXPECT().
			Set(gomock.Any(), expectedKey, idempotencyKey, ttl).
			Return(assert.AnError).
			Times(1)

		// Call the method - should not panic
		uc.SetTransactionIdempotencyMapping(ctx, organizationID, ledgerID, transactionID, idempotencyKey, ttl)
	})
}
