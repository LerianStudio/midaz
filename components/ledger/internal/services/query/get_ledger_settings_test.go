// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/ledger"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/onboarding"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetLedgerSettings_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	persistedSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(persistedSettings, nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.NotNil(t, settings)
	// Raw map returned as-is from DB; no defaults injected
	accounting, ok := settings["accounting"].(map[string]any)
	require.True(t, ok, "accounting section should exist")
	assert.Equal(t, true, accounting["validateAccountType"], "persisted value should be preserved")
	_, hasRoutes := accounting["validateRoutes"]
	assert.False(t, hasRoutes, "missing field should NOT be injected into raw map")
}

func TestGetLedgerSettings_EmptySettings_ReturnsRawEmpty(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(map[string]any{}, nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.NotNil(t, settings)
	// Raw empty map returned as-is; ParseLedgerSettings handles defaults
	assert.Empty(t, settings, "empty DB result should be returned as-is")
}

func TestGetLedgerSettings_NilSettings_ReturnsRawNil(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(nil, nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	// nil from DB is returned as-is; ParseLedgerSettings handles defaults
	assert.Nil(t, settings, "nil DB result should be returned as-is")
}

func TestGetLedgerSettings_PartialSettings_ReturnedAsIs(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	partialSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(partialSettings, nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.NotNil(t, settings)

	// Raw map returned without defaults injected
	accounting, ok := settings["accounting"].(map[string]any)
	require.True(t, ok, "accounting section should exist")
	assert.Equal(t, true, accounting["validateAccountType"], "persisted value should be preserved")
	_, hasRoutes := accounting["validateRoutes"]
	assert.False(t, hasRoutes, "missing value should NOT be injected into raw map")
}

func TestGetLedgerSettings_ExtraSettings_Preserved(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	persistedSettings := map[string]any{
		"customKey": "customValue",
	}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(persistedSettings, nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.Equal(t, "customValue", settings["customKey"], "extra keys should be preserved")
	_, hasAccounting := settings["accounting"]
	assert.False(t, hasAccounting, "accounting should NOT be injected if not in DB")
}

func TestGetLedgerSettings_LedgerNotFound(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(nil, errors.New("ledger not found"))

	_, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ledger not found")
}

func TestGetLedgerSettings_DatabaseError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(nil, errors.New("connection refused"))

	_, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.Error(t, err)
}

func TestGetLedgerSettings_CacheHit_ReturnsRawCachedValue(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		LedgerRepo:          mockLedgerRepo,
		OnboardingRedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	cachedSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}
	cachedJSON, err := json.Marshal(cachedSettings)
	require.NoError(t, err)
	cacheKey := utils.LedgerSettingsInternalKey(orgID, ledgerID)

	// Cache hit
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), cacheKey).
		Return(string(cachedJSON), nil)

	// Database should NOT be called
	// (no mockLedgerRepo expectation)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.NotNil(t, settings)

	// Raw cached map returned without defaults injected
	accounting, ok := settings["accounting"].(map[string]any)
	require.True(t, ok, "accounting section should exist")
	assert.Equal(t, true, accounting["validateAccountType"], "cached value should be preserved")
	_, hasRoutes := accounting["validateRoutes"]
	assert.False(t, hasRoutes, "missing value should NOT be injected into raw map")
}

func TestGetLedgerSettings_CacheMiss_PopulatesCache(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		LedgerRepo:          mockLedgerRepo,
		OnboardingRedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	persistedSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}
	cacheKey := utils.LedgerSettingsInternalKey(orgID, ledgerID)

	// Cache miss
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), cacheKey).
		Return("", nil)

	// Database returns data
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(persistedSettings, nil)

	// Cache should be populated with the raw DB value (no defaults injected)
	mockRedisRepo.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), SettingsCacheTTL).
		Return(nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.NotNil(t, settings)
	accounting, ok := settings["accounting"].(map[string]any)
	require.True(t, ok, "accounting section should exist")
	assert.Equal(t, true, accounting["validateAccountType"], "persisted value should be preserved")
}

func TestGetLedgerSettings_CacheErrorOnRead_FallsBackToDatabase(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		LedgerRepo:          mockLedgerRepo,
		OnboardingRedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	persistedSettings := map[string]any{
		"customKey": "customValue",
	}
	cacheKey := utils.LedgerSettingsInternalKey(orgID, ledgerID)

	// Cache error on read
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), cacheKey).
		Return("", errors.New("redis connection error"))

	// Should fall back to database
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(persistedSettings, nil)

	// Should try to populate cache with raw DB value
	mockRedisRepo.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), SettingsCacheTTL).
		Return(nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.Equal(t, "customValue", settings["customKey"], "persisted custom key should be preserved")
}

func TestGetLedgerSettings_InvalidCacheJSON_FallsBackToDatabase(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		LedgerRepo:          mockLedgerRepo,
		OnboardingRedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	persistedSettings := map[string]any{
		"customKey": "customValue",
	}
	cacheKey := utils.LedgerSettingsInternalKey(orgID, ledgerID)

	// Cache returns invalid JSON
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), cacheKey).
		Return("invalid json {{{", nil)

	// Should fall back to database
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(persistedSettings, nil)

	// Should try to populate cache with raw DB value
	mockRedisRepo.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), SettingsCacheTTL).
		Return(nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.Equal(t, "customValue", settings["customKey"], "persisted custom key should be preserved")
}

func TestGetLedgerSettings_CacheSetError_DoesNotFailOperation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		LedgerRepo:          mockLedgerRepo,
		OnboardingRedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	persistedSettings := map[string]any{
		"customKey": "customValue",
	}
	cacheKey := utils.LedgerSettingsInternalKey(orgID, ledgerID)

	// Cache miss
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), cacheKey).
		Return("", nil)

	// Database returns data
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(persistedSettings, nil)

	// Cache set fails -- operation should still succeed
	mockRedisRepo.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), SettingsCacheTTL).
		Return(errors.New("redis write error"))

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.Equal(t, "customValue", settings["customKey"], "persisted custom key should be preserved")
}
