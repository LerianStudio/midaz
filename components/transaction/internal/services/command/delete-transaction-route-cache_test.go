package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteTransactionRouteCache_Success tests successful cache deletion
func TestDeleteTransactionRouteCache_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID.String())

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), expectedKey).
		Return(nil).
		Times(1)

	err := uc.DeleteTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.NoError(t, err)
}

// TestDeleteTransactionRouteCache_RedisError tests error handling when Redis Del fails
func TestDeleteTransactionRouteCache_RedisError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	redisError := errors.New("redis connection error")
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID.String())

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), expectedKey).
		Return(redisError).
		Times(1)

	err := uc.DeleteTransactionRouteCache(context.Background(), organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)
	assert.Equal(t, redisError, err)
}

// TestDeleteTransactionRouteCache_ContextCancelled tests error handling when context is cancelled
func TestDeleteTransactionRouteCache_ContextCancelled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionRouteID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	expectedKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID.String())

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), expectedKey).
		Return(context.Canceled).
		Times(1)

	err := uc.DeleteTransactionRouteCache(ctx, organizationID, ledgerID, transactionRouteID)

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}
