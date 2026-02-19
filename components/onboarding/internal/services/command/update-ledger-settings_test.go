// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/redis"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUpdateLedgerSettings_Success(t *testing.T) {
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
	inputSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}
	expectedSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
		"existing": "value",
	}

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(expectedSettings, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.NotNil(t, settings)
	assert.Equal(t, expectedSettings, settings)
}

func TestUpdateLedgerSettings_MergeWithExisting(t *testing.T) {
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
	inputSettings := map[string]any{
		"newKey": "newValue",
	}
	mergedSettings := map[string]any{
		"existingKey": "existingValue",
		"newKey":      "newValue",
	}

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(mergedSettings, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.Equal(t, mergedSettings, settings)
}

func TestUpdateLedgerSettings_EmptyInput(t *testing.T) {
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
	inputSettings := map[string]any{}
	existingSettings := map[string]any{
		"existingKey": "existingValue",
	}

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(existingSettings, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.Equal(t, existingSettings, settings)
}

func TestUpdateLedgerSettings_LedgerNotFound(t *testing.T) {
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
	inputSettings := map[string]any{
		"key": "value",
	}

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(nil, errors.New("ledger not found"))

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.Error(t, err)
	assert.Nil(t, settings)
}

func TestUpdateLedgerSettings_DatabaseError(t *testing.T) {
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
	inputSettings := map[string]any{
		"key": "value",
	}

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(nil, errors.New("database error"))

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.Error(t, err)
	assert.Nil(t, settings)
	assert.Contains(t, err.Error(), "database error")
}

func TestUpdateLedgerSettings_NilResultReturnsEmptyMap(t *testing.T) {
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
	inputSettings := map[string]any{
		"key": "value",
	}

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(nil, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.NotNil(t, settings)
	assert.Empty(t, settings)
}

// Cache invalidation tests

func TestUpdateLedgerSettings_InvalidatesCache(t *testing.T) {
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
	inputSettings := map[string]any{
		"key": "value",
	}
	updatedSettings := map[string]any{
		"key": "value",
	}
	cacheKey := buildLedgerSettingsCacheKey(orgID, ledgerID)

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(updatedSettings, nil)

	// Cache should be invalidated after successful update
	mockRedisRepo.EXPECT().
		Del(gomock.Any(), cacheKey).
		Return(nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.Equal(t, updatedSettings, settings)
}

func TestUpdateLedgerSettings_CacheInvalidationError_DoesNotFailOperation(t *testing.T) {
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
	inputSettings := map[string]any{
		"key": "value",
	}
	updatedSettings := map[string]any{
		"key": "value",
	}
	cacheKey := buildLedgerSettingsCacheKey(orgID, ledgerID)

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(updatedSettings, nil)

	// Cache invalidation fails - operation should still succeed
	mockRedisRepo.EXPECT().
		Del(gomock.Any(), cacheKey).
		Return(errors.New("redis connection error"))

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.Equal(t, updatedSettings, settings)
}

func TestUpdateLedgerSettings_DatabaseError_NoCacheInvalidation(t *testing.T) {
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
	inputSettings := map[string]any{
		"key": "value",
	}

	// Database update fails
	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(nil, errors.New("database error"))

	// Cache should NOT be invalidated when database fails
	// mockRedisRepo.Del should NOT be called

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.Error(t, err)
	assert.Nil(t, settings)
}

func TestBuildLedgerSettingsCacheKey_Command(t *testing.T) {
	t.Parallel()

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	key := buildLedgerSettingsCacheKey(orgID, ledgerID)

	expected := "ledger_settings:11111111-1111-1111-1111-111111111111:22222222-2222-2222-2222-222222222222"
	assert.Equal(t, expected, key)
}
