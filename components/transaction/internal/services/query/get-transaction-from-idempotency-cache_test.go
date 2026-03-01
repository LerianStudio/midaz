// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

func TestGetTransactionFromIdempotencyCache_CacheHit(t *testing.T) {
	t.Parallel()

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
	expectedJSON, marshalErr := json.Marshal(expectedTran)
	require.NoError(t, marshalErr)

	reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, transactionID.String())
	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, idempotencyKey)

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
	t.Parallel()

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
	t.Parallel()

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
		Return("", errors.New("redis connection error")). //nolint:err113
		Times(1)

	result, found := uc.GetTransactionFromIdempotencyCache(context.Background(), organizationID, ledgerID, transactionID)

	assert.False(t, found)
	assert.Nil(t, result)
}

func TestGetTransactionFromIdempotencyCache_IdempotencyResponseNotFound(t *testing.T) {
	t.Parallel()

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
	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, idempotencyKey)

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
	t.Parallel()

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
	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, idempotencyKey)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), reverseKey).
		Return(idempotencyKey, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), internalKey).
		Return("", errors.New("redis connection error")). //nolint:err113
		Times(1)

	result, found := uc.GetTransactionFromIdempotencyCache(context.Background(), organizationID, ledgerID, transactionID)

	assert.False(t, found)
	assert.Nil(t, result)
}

func TestGetTransactionFromIdempotencyCache_UnmarshalError(t *testing.T) {
	t.Parallel()

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
	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, idempotencyKey)

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
