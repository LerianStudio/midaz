// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateOrCheckTransactionIdempotency(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	hash := "test-hash-value"
	ttl := 24 * time.Hour

	t.Run("success with key", func(t *testing.T) {
		key := "test-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(true, nil).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, ttl)

		assert.NoError(t, err)
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("success with empty key", func(t *testing.T) {
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, hash)

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(true, nil).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, "", hash, ttl)

		assert.NoError(t, err)
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("key already exists with cached value", func(t *testing.T) {
		key := "existing-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		cachedTxn := transaction.Transaction{
			ID:             uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Description:    "cached transaction",
		}
		cachedJSON, err := json.Marshal(cachedTxn)
		require.NoError(t, err)

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return(string(cachedJSON), nil).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, ttl)

		assert.NoError(t, err)
		require.NotNil(t, result.Replay)
		assert.Equal(t, cachedTxn.ID, result.Replay.ID)
		assert.Equal(t, cachedTxn.Description, result.Replay.Description)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("key already exists with invalid JSON returns error", func(t *testing.T) {
		key := "bad-json-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return("not-valid-json", nil).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, ttl)

		assert.Error(t, err)
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("key exists with empty value returns idempotency error", func(t *testing.T) {
		key := "test-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return("", nil).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, ttl)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already in use")
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("key disappeared between SetNX and Get returns idempotency error", func(t *testing.T) {
		key := "disappearing-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return("", goredis.Nil).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, ttl)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already in use")
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("SetNX returns error", func(t *testing.T) {
		key := "error-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		expectedErr := assert.AnError

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, expectedErr).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, ttl)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})

	t.Run("Get returns non-nil error other than redis.Nil", func(t *testing.T) {
		key := "get-error-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		expectedErr := assert.AnError

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return("", expectedErr).
			Times(1)

		result, err := uc.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, key, hash, ttl)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, result.Replay)
		assert.Equal(t, &internalKey, result.InternalKey)
	})
}

func TestSetTransactionIdempotencyValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRedisRepo: mockRedisRepo,
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

		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, string(expectedValue), ttl).
			Return(nil).
			Times(1)

		uc.SetTransactionIdempotencyValue(ctx, organizationID, ledgerID, key, hash, txn, ttl)
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

		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, string(expectedValue), ttl).
			Return(nil).
			Times(1)

		uc.SetTransactionIdempotencyValue(ctx, organizationID, ledgerID, "", hash, txn, ttl)
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

		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, string(expectedValue), ttl).
			Return(assert.AnError).
			Times(1)

		// Should not panic or return error
		uc.SetTransactionIdempotencyValue(ctx, organizationID, ledgerID, key, hash, txn, ttl)
	})
}
