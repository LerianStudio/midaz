package command

import (
	"context"
	"testing"
	"time"

	redis "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCreateOrCheckIdempotencyKey_PanicsOnNilOrganizationID(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	assert.Panics(t, func() {
		_, _ = uc.CreateOrCheckIdempotencyKey(ctx, uuid.Nil, uuid.New(), "key", "hash", time.Minute)
	}, "Expected panic on nil OrganizationID")
}

func TestCreateOrCheckIdempotencyKey_PanicsOnNilLedgerID(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	assert.Panics(t, func() {
		_, _ = uc.CreateOrCheckIdempotencyKey(ctx, uuid.New(), uuid.Nil, "key", "hash", time.Minute)
	}, "Expected panic on nil LedgerID")
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
