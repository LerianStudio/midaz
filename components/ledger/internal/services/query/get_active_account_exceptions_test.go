// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accountexception"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/onboarding"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestGetActiveAccountExceptions_CacheHit: a populated cache entry is served
// directly; the Postgres loader is never touched.
func TestGetActiveAccountExceptions_CacheHit(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	cacheKey := utils.AccountExceptionsInternalKey(orgID, ledgerID, accountID)

	cached := []*mmodel.AccountException{
		{ID: "exc-1", AccountID: accountID.String(), OperationalTypeCodes: []string{"PIX_IN"}},
	}
	payload, err := json.Marshal(cached)
	require.NoError(t, err)

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().Get(gomock.Any(), cacheKey).Return(string(payload), nil)

	// ListByAccountID must NOT be called on a hit — no expectation set.
	mockExc := accountexception.NewMockRepository(ctrl)

	uc := &UseCase{OnboardingRedisRepo: mockRedis, AccountExceptionRepo: mockExc}

	result, err := uc.GetActiveAccountExceptions(context.Background(), orgID, ledgerID, accountID)

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "exc-1", result[0].ID)
}

// TestGetActiveAccountExceptions_MissPopulatesWithTTL: a miss ("" from Get) reads
// Postgres and repopulates the cache with the 5-minute TTL and a JSON array value.
func TestGetActiveAccountExceptions_MissPopulatesWithTTL(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	cacheKey := utils.AccountExceptionsInternalKey(orgID, ledgerID, accountID)

	stored := []*mmodel.AccountException{
		{ID: "exc-1", AccountID: accountID.String(), OperationalTypeCodes: []string{"PIX_IN"}},
	}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().Get(gomock.Any(), cacheKey).Return("", nil)

	mockExc := accountexception.NewMockRepository(ctrl)
	mockExc.EXPECT().
		ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
		Return(stored, nil)

	var capturedValue string

	var capturedTTL time.Duration

	mockRedis.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, value string, ttl time.Duration) error {
			capturedValue = value
			capturedTTL = ttl

			return nil
		})

	uc := &UseCase{OnboardingRedisRepo: mockRedis, AccountExceptionRepo: mockExc}

	result, err := uc.GetActiveAccountExceptions(context.Background(), orgID, ledgerID, accountID)

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, accountExceptionsCacheTTL, capturedTTL)
	assert.Equal(t, 5*time.Minute, capturedTTL)

	var back []*mmodel.AccountException
	require.NoError(t, json.Unmarshal([]byte(capturedValue), &back))
	require.Len(t, back, 1)
	assert.Equal(t, "exc-1", back[0].ID)
}

// TestGetActiveAccountExceptions_CachedEmptyList: a literal "[]" is a cached
// empty result, distinct from a miss — Postgres is NOT queried.
func TestGetActiveAccountExceptions_CachedEmptyList(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	cacheKey := utils.AccountExceptionsInternalKey(orgID, ledgerID, accountID)

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().Get(gomock.Any(), cacheKey).Return("[]", nil)

	// Neither ListByAccountID nor Set is expected — cached-empty short-circuits both.
	mockExc := accountexception.NewMockRepository(ctrl)

	uc := &UseCase{OnboardingRedisRepo: mockRedis, AccountExceptionRepo: mockExc}

	result, err := uc.GetActiveAccountExceptions(context.Background(), orgID, ledgerID, accountID)

	require.NoError(t, err)
	assert.Empty(t, result)
}

// TestGetActiveAccountExceptions_RedisDownFallsBackToPostgres: a Get error is a
// graceful-degradation path — the loader is queried directly and the result returned.
func TestGetActiveAccountExceptions_RedisDownFallsBackToPostgres(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	cacheKey := utils.AccountExceptionsInternalKey(orgID, ledgerID, accountID)

	stored := []*mmodel.AccountException{{ID: "exc-1", AccountID: accountID.String()}}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().Get(gomock.Any(), cacheKey).Return("", errors.New("redis down"))
	// The best-effort repopulate may still be attempted and may still fail.
	mockRedis.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), gomock.Any()).
		Return(errors.New("redis still down")).
		AnyTimes()

	mockExc := accountexception.NewMockRepository(ctrl)
	mockExc.EXPECT().
		ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
		Return(stored, nil)

	uc := &UseCase{OnboardingRedisRepo: mockRedis, AccountExceptionRepo: mockExc}

	result, err := uc.GetActiveAccountExceptions(context.Background(), orgID, ledgerID, accountID)

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "exc-1", result[0].ID)
}

// TestGetActiveAccountExceptions_PostgresDownReturnsError: when the cache cannot
// answer AND Postgres errors, the error propagates (caller owns fail-closed).
func TestGetActiveAccountExceptions_PostgresDownReturnsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	cacheKey := utils.AccountExceptionsInternalKey(orgID, ledgerID, accountID)

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().Get(gomock.Any(), cacheKey).Return("", errors.New("redis down"))

	mockExc := accountexception.NewMockRepository(ctrl)
	mockExc.EXPECT().
		ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
		Return(nil, errors.New("pg down"))

	uc := &UseCase{OnboardingRedisRepo: mockRedis, AccountExceptionRepo: mockExc}

	result, err := uc.GetActiveAccountExceptions(context.Background(), orgID, ledgerID, accountID)

	require.Error(t, err)
	assert.Nil(t, result)
}

// TestGetActiveAccountExceptions_CorruptCacheFallsBackToPostgres: a value that is
// not valid JSON is treated as a miss (fall through to Postgres), and the repopulate
// Set error is swallowed — the read result is unaffected.
func TestGetActiveAccountExceptions_CorruptCacheFallsBackToPostgres(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	cacheKey := utils.AccountExceptionsInternalKey(orgID, ledgerID, accountID)

	stored := []*mmodel.AccountException{{ID: "exc-1", AccountID: accountID.String()}}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().Get(gomock.Any(), cacheKey).Return("not-json", nil)
	mockRedis.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), gomock.Any()).
		Return(errors.New("redis write failed"))

	mockExc := accountexception.NewMockRepository(ctrl)
	mockExc.EXPECT().
		ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
		Return(stored, nil)

	uc := &UseCase{OnboardingRedisRepo: mockRedis, AccountExceptionRepo: mockExc}

	result, err := uc.GetActiveAccountExceptions(context.Background(), orgID, ledgerID, accountID)

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "exc-1", result[0].ID)
}

// TestGetActiveAccountExceptions_CachedNullNormalizesToEmpty: a JSON "null" value is
// a hit that normalizes to an empty slice; Postgres is NOT queried.
func TestGetActiveAccountExceptions_CachedNullNormalizesToEmpty(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	cacheKey := utils.AccountExceptionsInternalKey(orgID, ledgerID, accountID)

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().Get(gomock.Any(), cacheKey).Return("null", nil)

	// A hit must not touch Postgres.
	mockExc := accountexception.NewMockRepository(ctrl)

	uc := &UseCase{OnboardingRedisRepo: mockRedis, AccountExceptionRepo: mockExc}

	result, err := uc.GetActiveAccountExceptions(context.Background(), orgID, ledgerID, accountID)

	require.NoError(t, err)
	assert.Empty(t, result)
	assert.NotNil(t, result)
}

// TestGetActiveAccountExceptions_NilRedisFallsBackToPostgres: a binary with the
// cache disabled (nil repo) still serves reads straight from Postgres.
func TestGetActiveAccountExceptions_NilRedisFallsBackToPostgres(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	stored := []*mmodel.AccountException{{ID: "exc-1", AccountID: accountID.String()}}

	mockExc := accountexception.NewMockRepository(ctrl)
	mockExc.EXPECT().
		ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
		Return(stored, nil)

	uc := &UseCase{OnboardingRedisRepo: nil, AccountExceptionRepo: mockExc}

	result, err := uc.GetActiveAccountExceptions(context.Background(), orgID, ledgerID, accountID)

	require.NoError(t, err)
	require.Len(t, result, 1)
}
