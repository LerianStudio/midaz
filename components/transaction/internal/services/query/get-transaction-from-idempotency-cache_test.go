package query

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetTransactionFromIdempotencyCache_CacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	idempotencyKey := "test-idempotency-key"

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	expectedTran := transaction.Transaction{
		ID:             transactionID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Description:    "Test transaction",
		Status:         transaction.Status{Code: "APPROVED"},
	}
	expectedJSON, _ := json.Marshal(expectedTran)

	reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, transactionID.String())
	internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, idempotencyKey)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), reverseKey).
		Return(idempotencyKey, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), internalKey).
		Return(string(expectedJSON), nil).
		Times(1)

	result, found := uc.GetTransactionFromIdempotencyCache(context.Background(), organizationID, ledgerID, transactionID)

	assert.True(t, found)
	assert.NotNil(t, result)
	assert.Equal(t, expectedTran.ID, result.ID)
	assert.Equal(t, expectedTran.Description, result.Description)
}

func TestGetTransactionFromIdempotencyCache_ReverseKeyNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, transactionID.String())

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), reverseKey).
		Return("", nil).
		Times(1)

	result, found := uc.GetTransactionFromIdempotencyCache(context.Background(), organizationID, ledgerID, transactionID)

	assert.False(t, found)
	assert.Nil(t, result)
}

func TestGetTransactionFromIdempotencyCache_ReverseKeyError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, transactionID.String())

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), reverseKey).
		Return("", errors.New("redis connection error")).
		Times(1)

	result, found := uc.GetTransactionFromIdempotencyCache(context.Background(), organizationID, ledgerID, transactionID)

	assert.False(t, found)
	assert.Nil(t, result)
}

func TestGetTransactionFromIdempotencyCache_IdempotencyResponseNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	idempotencyKey := "test-idempotency-key"

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, transactionID.String())
	internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, idempotencyKey)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), reverseKey).
		Return(idempotencyKey, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), internalKey).
		Return("", nil).
		Times(1)

	result, found := uc.GetTransactionFromIdempotencyCache(context.Background(), organizationID, ledgerID, transactionID)

	assert.False(t, found)
	assert.Nil(t, result)
}

func TestGetTransactionFromIdempotencyCache_IdempotencyResponseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	idempotencyKey := "test-idempotency-key"

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, transactionID.String())
	internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, idempotencyKey)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), reverseKey).
		Return(idempotencyKey, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), internalKey).
		Return("", errors.New("redis connection error")).
		Times(1)

	result, found := uc.GetTransactionFromIdempotencyCache(context.Background(), organizationID, ledgerID, transactionID)

	assert.False(t, found)
	assert.Nil(t, result)
}

func TestGetTransactionFromIdempotencyCache_UnmarshalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	idempotencyKey := "test-idempotency-key"

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, transactionID.String())
	internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, idempotencyKey)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), reverseKey).
		Return(idempotencyKey, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), internalKey).
		Return("invalid json", nil).
		Times(1)

	result, found := uc.GetTransactionFromIdempotencyCache(context.Background(), organizationID, ledgerID, transactionID)

	assert.False(t, found)
	assert.Nil(t, result)
}
