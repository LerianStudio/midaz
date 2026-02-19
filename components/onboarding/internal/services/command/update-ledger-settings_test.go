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
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query"
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
	cacheKey := query.BuildLedgerSettingsCacheKey(orgID, ledgerID)

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
	cacheKey := query.BuildLedgerSettingsCacheKey(orgID, ledgerID)

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

func TestUpdateLedgerSettings_NilInput(t *testing.T) {
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

	// Nil input should be handled gracefully - returns existing settings unchanged
	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, map[string]any(nil)).
		Return(map[string]any{"existingKey": "existingValue"}, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, nil)

	require.NoError(t, err)
	assert.NotNil(t, settings)
	assert.Equal(t, "existingValue", settings["existingKey"])
}

func TestUpdateLedgerSettings_NullValueInSettings(t *testing.T) {
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
	// Per TRD: "To remove a key, set it to null explicitly"
	inputSettings := map[string]any{
		"keyToRemove": nil,
	}
	// Note: PostgreSQL || operator keeps the null value, it does not remove the key
	// This tests documents the current behavior
	mergedSettings := map[string]any{
		"existingKey": "existingValue",
		"keyToRemove": nil,
	}

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(mergedSettings, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.Equal(t, mergedSettings, settings)
	assert.Contains(t, settings, "keyToRemove", "null key should be present in merged result")
	assert.Nil(t, settings["keyToRemove"], "null key should have nil value")
}

func TestUpdateLedgerSettings_DeeplyNestedMerge(t *testing.T) {
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
	// Test deeply nested update
	inputSettings := map[string]any{
		"accounting": map[string]any{
			"newValidation": false,
		},
	}
	// Note: PostgreSQL || performs shallow merge at top level only
	// Nested objects are replaced entirely, not merged
	// This test documents the expected behavior per TRD Section 5.4
	mergedSettings := map[string]any{
		"accounting": map[string]any{
			"newValidation": false,
		},
	}

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(mergedSettings, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.Equal(t, mergedSettings, settings)
	// Verify nested structure is present
	accounting, ok := settings["accounting"].(map[string]any)
	require.True(t, ok, "accounting should be a map")
	assert.Equal(t, false, accounting["newValidation"])
}
