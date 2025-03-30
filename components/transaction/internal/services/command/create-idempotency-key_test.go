package command

import (
	"context"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
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
		internalKey := organizationID.String() + ":" + ledgerID.String() + ":" + key

		// Mock Redis.SetNX - success case (key doesn't exist)
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(true, nil).
			Times(1)

		// Call the method
		err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)

		// Assertions
		assert.NoError(t, err)
	})

	t.Run("success with empty key", func(t *testing.T) {
		// When key is empty, it should use the hash value
		internalKey := organizationID.String() + ":" + ledgerID.String() + ":" + hash

		// Mock Redis.SetNX - success case (key doesn't exist)
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(true, nil).
			Times(1)

		// Call the method
		err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, "", hash, ttl)

		// Assertions
		assert.NoError(t, err)
	})

	t.Run("key already exists", func(t *testing.T) {
		key := "existing-key"
		internalKey := organizationID.String() + ":" + ledgerID.String() + ":" + key

		// Mock Redis.SetNX - failure case (key already exists)
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		// Call the method
		err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "0084")
	})

	t.Run("redis error", func(t *testing.T) {
		key := "test-key"
		internalKey := organizationID.String() + ":" + ledgerID.String() + ":" + key

		// Mock Redis.SetNX - redis error
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		// Call the method
		err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "0084")
	})
}
