// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redisv9 "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type redisRepoWithPipeline struct {
	*redis.MockRedisRepository
	setPipelineFn func(ctx context.Context, keys, values []string, ttls []time.Duration) error
}

type raceAwareIdempotencyRepo struct {
	*redis.MockRedisRepository
	mu     sync.Mutex
	values map[string]string
}

func (r *redisRepoWithPipeline) SetPipeline(ctx context.Context, keys, values []string, ttls []time.Duration) error {
	if r.setPipelineFn == nil {
		return nil
	}

	return r.setPipelineFn(ctx, keys, values, ttls)
}

func (r *raceAwareIdempotencyRepo) CheckOrAcquireIdempotencyKey(_ context.Context, key string, _ time.Duration) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.values[key]; ok {
		return existing, false, nil
	}

	r.values[key] = ""

	return "", true, nil
}

func (r *raceAwareIdempotencyRepo) Get(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.values[key], nil
}

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
		value, err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)

		// Assertions
		assert.NoError(t, err)
		assert.Nil(t, value)
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
		value, err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, "", hash, ttl)

		// Assertions
		assert.NoError(t, err)
		assert.Nil(t, value)
	})

	t.Run("key already exists", func(t *testing.T) {
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
		value, err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)

		// Assertions
		assert.NoError(t, err) // Based on the actual implementation, this should not error when value is found
		assert.NotNil(t, value)
		assert.Equal(t, existingValue, *value)
	})

	t.Run("in-flight key resolves shortly after lock", func(t *testing.T) {
		key := "in-flight-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		resolvedValue := "resolved-transaction-json"

		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		gomock.InOrder(
			mockRedisRepo.EXPECT().
				Get(gomock.Any(), internalKey).
				Return("", nil).
				Times(1),
			mockRedisRepo.EXPECT().
				Get(gomock.Any(), internalKey).
				Return(resolvedValue, nil).
				Times(1),
		)

		value, err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)
		assert.NoError(t, err)
		assert.NotNil(t, value)
		assert.Equal(t, resolvedValue, *value)
	})

	t.Run("in-flight key times out and returns conflict error", func(t *testing.T) {
		key := "test-key"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

		// Mock Redis.SetNX - redis error
		mockRedisRepo.EXPECT().
			SetNX(gomock.Any(), internalKey, "", ttl).
			Return(false, nil).
			Times(1)

		// Mock Redis.Get - return empty value
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), internalKey).
			Return("", nil).
			AnyTimes()

		// Call the method
		value, err := uc.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already in use")
		assert.Nil(t, value)
	})
}

func TestSetIdempotencyValueAndMapping(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	txn := transaction.Transaction{
		ID:                       uuid.New().String(),
		ParentTransactionID:      nil,
		OrganizationID:           organizationID.String(),
		LedgerID:                 ledgerID.String(),
		Description:              "Test transaction",
		ChartOfAccountsGroupName: "test-group",
		Status:                   transaction.Status{Code: "COMMITTED"},
	}
	resultTTL := 10 * time.Second
	mappingTTL := 5 * time.Second

	t.Run("success with explicit key", func(t *testing.T) {
		key := "test-idempotency-key"
		hash := "hash-value"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, txn.ID)
		expectedValue, marshalErr := json.Marshal(txn)
		require.NoError(t, marshalErr)

		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, string(expectedValue), resultTTL).
			Return(nil).
			Times(1)
		mockRedisRepo.EXPECT().
			Set(gomock.Any(), reverseKey, key, mappingTTL).
			Return(nil).
			Times(1)

		err := uc.SetIdempotencyValueAndMapping(ctx, organizationID, ledgerID, key, hash, txn, resultTTL, mappingTTL)
		assert.NoError(t, err)
	})

	t.Run("uses hash when key is empty", func(t *testing.T) {
		hash := "hash-value"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, hash)
		reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, txn.ID)
		expectedValue, marshalErr := json.Marshal(txn)
		require.NoError(t, marshalErr)

		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, string(expectedValue), resultTTL).
			Return(nil).
			Times(1)
		mockRedisRepo.EXPECT().
			Set(gomock.Any(), reverseKey, hash, mappingTTL).
			Return(nil).
			Times(1)

		err := uc.SetIdempotencyValueAndMapping(ctx, organizationID, ledgerID, "", hash, txn, resultTTL, mappingTTL)
		assert.NoError(t, err)
	})

	t.Run("redis set error logs but does not panic", func(t *testing.T) {
		key := "test-idempotency-key"
		hash := "hash-value"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, txn.ID)
		expectedValue, marshalErr := json.Marshal(txn)
		require.NoError(t, marshalErr)

		mockRedisRepo.EXPECT().
			Set(gomock.Any(), internalKey, string(expectedValue), resultTTL).
			Return(assert.AnError).
			Times(1)
		mockRedisRepo.EXPECT().
			Set(gomock.Any(), reverseKey, key, mappingTTL).
			Return(assert.AnError).
			Times(1)

		err := uc.SetIdempotencyValueAndMapping(ctx, organizationID, ledgerID, key, hash, txn, resultTTL, mappingTTL)
		assert.Error(t, err)
	})

	t.Run("uses pipeline when available", func(t *testing.T) {
		key := "test-idempotency-key"
		hash := "hash-value"
		internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
		reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, txn.ID)

		pipelineCalled := false
		ucWithPipeline := &UseCase{
			RedisRepo: &redisRepoWithPipeline{
				MockRedisRepository: mockRedisRepo,
				setPipelineFn: func(_ context.Context, keys, values []string, ttls []time.Duration) error {
					pipelineCalled = true
					assert.Equal(t, []string{internalKey, reverseKey}, keys)
					assert.Equal(t, key, values[1])
					assert.Equal(t, []time.Duration{resultTTL, mappingTTL}, ttls)
					assert.NotEmpty(t, values[0])
					return nil
				},
			},
		}

		err := ucWithPipeline.SetIdempotencyValueAndMapping(ctx, organizationID, ledgerID, key, hash, txn, resultTTL, mappingTTL)
		assert.NoError(t, err)
		assert.True(t, pipelineCalled)
	})
}

func TestWaitForInFlightIdempotencyValue(t *testing.T) {
	t.Run("returns nil on context cancellation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := &UseCase{RedisRepo: redis.NewMockRedisRepository(ctrl)}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		value, err := uc.waitForInFlightIdempotencyValue(ctx, "idempotency-key")

		assert.NoError(t, err)
		assert.Nil(t, value)
	})

	t.Run("returns nil on timeout when value remains empty", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockRedisRepo.EXPECT().
			Get(gomock.Any(), "idempotency-key").
			Return("", nil).
			AnyTimes()

		uc := &UseCase{RedisRepo: mockRedisRepo}

		value, err := uc.waitForInFlightIdempotencyValue(context.Background(), "idempotency-key")

		assert.NoError(t, err)
		assert.Nil(t, value)
	})
}

func TestCreateOrCheckIdempotencyKey_UsesLuaPathThroughUseCase(t *testing.T) {
	mini, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mini.Close)

	client := redisv9.NewClient(&redisv9.Options{Addr: mini.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	conn := &libRedis.RedisConnection{Client: client, Connected: true}
	repo, err := redis.NewConsumerRedis(conn, false, nil)
	require.NoError(t, err)

	uc := &UseCase{RedisRepo: repo}
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	key := "lua-path-key"
	hash := "lua-path-hash"
	ttl := 2 * time.Second

	value, err := uc.CreateOrCheckIdempotencyKey(ctx, orgID, ledgerID, key, hash, ttl)
	require.NoError(t, err)
	require.Nil(t, value)

	txn := transaction.Transaction{
		ID:             uuid.NewString(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Description:    "lua idempotency test",
		Status:         transaction.Status{Code: "CREATED"},
	}

	require.NoError(t, uc.SetIdempotencyValueAndMapping(ctx, orgID, ledgerID, key, hash, txn, ttl, ttl))

	value, err = uc.CreateOrCheckIdempotencyKey(ctx, orgID, ledgerID, key, hash, ttl)
	require.NoError(t, err)
	require.NotNil(t, value)
	assert.Contains(t, *value, txn.ID)
}

func TestCreateOrCheckIdempotencyKey_ConcurrentRace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := &raceAwareIdempotencyRepo{
		MockRedisRepository: redis.NewMockRedisRepository(ctrl),
		values:              make(map[string]string),
	}

	uc := &UseCase{RedisRepo: repo}

	orgID := uuid.New()
	ledgerID := uuid.New()
	ttl := 2 * time.Second

	start := make(chan struct{})
	type result struct {
		value *string
		err   error
	}

	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			value, err := uc.CreateOrCheckIdempotencyKey(context.Background(), orgID, ledgerID, "same-key", "same-hash", ttl)
			results <- result{value: value, err: err}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0
	inFlightConflicts := 0
	for item := range results {
		if item.err == nil {
			successes++
			assert.Nil(t, item.value)
			continue
		}

		assert.Contains(t, item.err.Error(), "already in use")
		inFlightConflicts++
	}

	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, inFlightConflicts)
}
