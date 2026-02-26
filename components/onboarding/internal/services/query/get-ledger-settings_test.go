// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
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
	// Result should be merged with defaults
	accounting, ok := settings["accounting"].(map[string]any)
	require.True(t, ok, "accounting section should exist")
	assert.Equal(t, true, accounting["validateAccountType"], "persisted value should be preserved")
	assert.Equal(t, false, accounting["validateRoutes"], "missing value should come from defaults")
}

func TestGetLedgerSettings_EmptySettings_ReturnsDefaults(t *testing.T) {
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
	// Should return default settings, not empty map
	expectedDefaults := mmodel.DefaultLedgerSettingsMap()
	assert.Equal(t, expectedDefaults, settings)
}

func TestGetLedgerSettings_NilSettings_ReturnsDefaults(t *testing.T) {
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
	assert.NotNil(t, settings)
	// Should return default settings, not empty map
	expectedDefaults := mmodel.DefaultLedgerSettingsMap()
	assert.Equal(t, expectedDefaults, settings)
}

func TestGetLedgerSettings_PartialSettings_MergedWithDefaults(t *testing.T) {
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
	// Only validateAccountType is set, validateRoutes should come from defaults
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

	// Should have merged settings: validateAccountType=true (persisted), validateRoutes=false (default)
	accounting, ok := settings["accounting"].(map[string]any)
	require.True(t, ok, "accounting section should exist")
	assert.Equal(t, true, accounting["validateAccountType"], "persisted value should be preserved")
	assert.Equal(t, false, accounting["validateRoutes"], "missing value should come from defaults")
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
	// Settings with extra keys not in defaults
	settingsWithExtras := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
			"customField":         "customValue",
		},
		"customSection": map[string]any{
			"key": "value",
		},
	}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(settingsWithExtras, nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.NotNil(t, settings)

	// accounting section should be merged
	accounting, ok := settings["accounting"].(map[string]any)
	require.True(t, ok, "accounting section should exist")
	assert.Equal(t, true, accounting["validateAccountType"])
	assert.Equal(t, false, accounting["validateRoutes"], "default should be added")
	assert.Equal(t, "customValue", accounting["customField"], "extra fields should be preserved")

	// Extra section should be preserved
	customSection, ok := settings["customSection"].(map[string]any)
	require.True(t, ok, "customSection should be preserved")
	assert.Equal(t, "value", customSection["key"])
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

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.Error(t, err)
	assert.Nil(t, settings)
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
		Return(nil, errors.New("database error"))

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.Error(t, err)
	assert.Nil(t, settings)
	assert.Contains(t, err.Error(), "database error")
}

// Cache-specific tests

func TestGetLedgerSettings_CacheHit_MergesWithDefaults(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
		RedisRepo:  mockRedisRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	// Cached settings only have validateAccountType
	cachedSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}
	cachedJSON, err := json.Marshal(cachedSettings)
	require.NoError(t, err, "test setup: failed to marshal cached settings")
	cacheKey := BuildLedgerSettingsCacheKey(orgID, ledgerID)

	// Cache hit - should NOT call database
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), cacheKey).
		Return(string(cachedJSON), nil)

	// Database should NOT be called on cache hit

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.NotNil(t, settings)

	// Should have merged settings from cache with defaults
	accounting, ok := settings["accounting"].(map[string]any)
	require.True(t, ok, "accounting section should exist")
	assert.Equal(t, true, accounting["validateAccountType"], "cached value should be preserved")
	assert.Equal(t, false, accounting["validateRoutes"], "missing value should come from defaults")
}

func TestGetLedgerSettings_CacheMiss_PopulatesCache(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
		RedisRepo:  mockRedisRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	persistedSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}
	cacheKey := BuildLedgerSettingsCacheKey(orgID, ledgerID)

	// Cache miss - empty string returned
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), cacheKey).
		Return("", nil)

	// Database should be called
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(persistedSettings, nil)

	// Cache should be populated with merged settings
	mockRedisRepo.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), DefaultSettingsCacheTTL).
		Return(nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	assert.NotNil(t, settings)
	// Result should be merged with defaults
	accounting, ok := settings["accounting"].(map[string]any)
	require.True(t, ok, "accounting section should exist")
	assert.Equal(t, true, accounting["validateAccountType"], "persisted value should be preserved")
	assert.Equal(t, false, accounting["validateRoutes"], "missing value should come from defaults")
}

func TestGetLedgerSettings_CacheErrorOnRead_FallsBackToDatabase(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
		RedisRepo:  mockRedisRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	persistedSettings := map[string]any{
		"customKey": "customValue",
	}
	cacheKey := BuildLedgerSettingsCacheKey(orgID, ledgerID)

	// Cache error on read
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), cacheKey).
		Return("", errors.New("redis connection error"))

	// Should fall back to database
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(persistedSettings, nil)

	// Should still try to populate cache
	mockRedisRepo.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), DefaultSettingsCacheTTL).
		Return(nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	// Should have merged with defaults
	assert.Equal(t, "customValue", settings["customKey"], "persisted custom key should be preserved")
	_, hasAccounting := settings["accounting"]
	assert.True(t, hasAccounting, "accounting defaults should be added")
}

func TestGetLedgerSettings_InvalidCacheJSON_FallsBackToDatabase(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
		RedisRepo:  mockRedisRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	persistedSettings := map[string]any{
		"customKey": "customValue",
	}
	cacheKey := BuildLedgerSettingsCacheKey(orgID, ledgerID)

	// Cache returns invalid JSON
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), cacheKey).
		Return("invalid json {{{", nil)

	// Should fall back to database
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(persistedSettings, nil)

	// Should try to populate cache with valid data
	mockRedisRepo.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), DefaultSettingsCacheTTL).
		Return(nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	// Should have merged with defaults
	assert.Equal(t, "customValue", settings["customKey"], "persisted custom key should be preserved")
	_, hasAccounting := settings["accounting"]
	assert.True(t, hasAccounting, "accounting defaults should be added")
}

func TestGetLedgerSettings_CacheSetError_DoesNotFailOperation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
		RedisRepo:  mockRedisRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	persistedSettings := map[string]any{
		"customKey": "customValue",
	}
	cacheKey := BuildLedgerSettingsCacheKey(orgID, ledgerID)

	// Cache miss
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), cacheKey).
		Return("", nil)

	// Database returns data
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(persistedSettings, nil)

	// Cache set fails - operation should still succeed
	mockRedisRepo.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), DefaultSettingsCacheTTL).
		Return(errors.New("redis write error"))

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	// Should have merged with defaults despite cache error
	assert.Equal(t, "customValue", settings["customKey"], "persisted custom key should be preserved")
	_, hasAccounting := settings["accounting"]
	assert.True(t, hasAccounting, "accounting defaults should be added")
}

func TestBuildLedgerSettingsCacheKey(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	key := BuildLedgerSettingsCacheKey(orgID, ledgerID)

	expected := "ledger_settings:11111111-1111-1111-1111-111111111111:22222222-2222-2222-2222-222222222222"
	assert.Equal(t, expected, key)
}

func TestGetLedgerSettings_CustomCacheTTL(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Custom TTL of 10 minutes instead of default 5 minutes
	customTTL := 2 * DefaultSettingsCacheTTL

	uc := &UseCase{
		LedgerRepo:       mockLedgerRepo,
		RedisRepo:        mockRedisRepo,
		SettingsCacheTTL: customTTL,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	persistedSettings := map[string]any{
		"customKey": "customValue",
	}
	cacheKey := BuildLedgerSettingsCacheKey(orgID, ledgerID)

	// Cache miss
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), cacheKey).
		Return("", nil)

	// Database returns data
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(persistedSettings, nil)

	// Cache should be set with custom TTL, not default
	mockRedisRepo.EXPECT().
		Set(gomock.Any(), cacheKey, gomock.Any(), customTTL).
		Return(nil)

	settings, err := uc.GetLedgerSettings(ctx, orgID, ledgerID)

	require.NoError(t, err)
	// Should have merged with defaults
	assert.Equal(t, "customValue", settings["customKey"], "persisted custom key should be preserved")
	_, hasAccounting := settings["accounting"]
	assert.True(t, hasAccounting, "accounting defaults should be added")
}

func TestGetSettingsCacheTTL_DefaultValue(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	ttl := uc.getSettingsCacheTTL()

	assert.Equal(t, DefaultSettingsCacheTTL, ttl)
}

func TestGetSettingsCacheTTL_CustomValue(t *testing.T) {
	t.Parallel()

	customTTL := 15 * DefaultSettingsCacheTTL

	uc := &UseCase{
		SettingsCacheTTL: customTTL,
	}

	ttl := uc.getSettingsCacheTTL()

	assert.Equal(t, customTTL, ttl)
}
