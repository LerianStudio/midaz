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
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
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
	existingSettings := map[string]any{
		"accounting": map[string]any{
			"validateRoutes": false,
		},
	}
	mergedSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
			"validateRoutes":      false,
		},
	}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(existingSettings, nil)

	mockLedgerRepo.EXPECT().
		ReplaceSettings(gomock.Any(), orgID, ledgerID, mergedSettings).
		Return(mergedSettings, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.NotNil(t, settings)
	assert.Equal(t, mergedSettings, settings)
}

func TestUpdateLedgerSettings_DeepMergePreservesExistingNestedKeys(t *testing.T) {
	// This test verifies the deep merge behavior: when updating only validateRoutes,
	// the existing validateAccountType should be preserved.
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

	// Input: only updating validateRoutes
	inputSettings := map[string]any{
		"accounting": map[string]any{
			"validateRoutes": true,
		},
	}

	// Existing: has validateAccountType set to true
	existingSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
			"validateRoutes":      false,
		},
	}

	// Expected: validateAccountType preserved, validateRoutes updated
	expectedMerged := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
			"validateRoutes":      true,
		},
	}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(existingSettings, nil)

	mockLedgerRepo.EXPECT().
		ReplaceSettings(gomock.Any(), orgID, ledgerID, expectedMerged).
		Return(expectedMerged, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	accountingMap := settings["accounting"].(map[string]any)
	assert.True(t, accountingMap["validateAccountType"].(bool), "validateAccountType should be preserved")
	assert.True(t, accountingMap["validateRoutes"].(bool), "validateRoutes should be updated")
}

func TestUpdateLedgerSettings_ValidationError_UnknownTopLevelField(t *testing.T) {
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

	// Invalid input: unknown top-level field
	inputSettings := map[string]any{
		"unknownField": "value",
	}

	// Repository methods should NOT be called when validation fails
	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.Error(t, err)
	assert.Nil(t, settings)
	assert.Contains(t, err.Error(), "unknownField")
}

func TestUpdateLedgerSettings_ValidationError_UnknownNestedField(t *testing.T) {
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

	// Invalid input: unknown nested field under "accounting"
	inputSettings := map[string]any{
		"accounting": map[string]any{
			"unknownNestedField": true,
		},
	}

	// Repository methods should NOT be called when validation fails
	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.Error(t, err)
	assert.Nil(t, settings)
	assert.Contains(t, err.Error(), "accounting.unknownNestedField")
}

func TestUpdateLedgerSettings_ValidationError_WrongType(t *testing.T) {
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

	// Invalid input: validateAccountType should be bool, not string
	inputSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": "true", // string instead of bool
		},
	}

	// Repository methods should NOT be called when validation fails
	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.Error(t, err)
	assert.Nil(t, settings)
	assert.Contains(t, err.Error(), "validateAccountType")
}

func TestUpdateLedgerSettings_EmptyInputPreservesExisting(t *testing.T) {
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

	// Empty input
	inputSettings := map[string]any{}

	// Existing settings should be preserved
	existingSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(existingSettings, nil)

	// DeepMergeSettings with empty input returns existing
	mockLedgerRepo.EXPECT().
		ReplaceSettings(gomock.Any(), orgID, ledgerID, existingSettings).
		Return(existingSettings, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.Equal(t, existingSettings, settings)
}

func TestUpdateLedgerSettings_NullValueStoresNull(t *testing.T) {
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

	// Input with null value for a known key
	inputSettings := map[string]any{
		"accounting": nil,
	}

	existingSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}

	// Null should replace the existing accounting object
	expectedMerged := map[string]any{
		"accounting": nil,
	}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(existingSettings, nil)

	mockLedgerRepo.EXPECT().
		ReplaceSettings(gomock.Any(), orgID, ledgerID, expectedMerged).
		Return(expectedMerged, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.Nil(t, settings["accounting"], "accounting should be null")
}

func TestUpdateLedgerSettings_LedgerNotFoundOnGetSettings(t *testing.T) {
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

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(nil, errors.New("ledger not found"))

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.Error(t, err)
	assert.Nil(t, settings)
	assert.Contains(t, err.Error(), "ledger not found")
}

func TestUpdateLedgerSettings_DatabaseErrorOnReplaceSettings(t *testing.T) {
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
	existingSettings := map[string]any{}

	// Compute expected merged settings (mmodel.DeepMergeSettings behavior)
	expectedMerged := mmodel.DeepMergeSettings(existingSettings, inputSettings)

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(existingSettings, nil)

	mockLedgerRepo.EXPECT().
		ReplaceSettings(gomock.Any(), orgID, ledgerID, expectedMerged).
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
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}
	existingSettings := map[string]any{}
	expectedMerged := mmodel.DeepMergeSettings(existingSettings, inputSettings)

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(existingSettings, nil)

	mockLedgerRepo.EXPECT().
		ReplaceSettings(gomock.Any(), orgID, ledgerID, expectedMerged).
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
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}
	existingSettings := map[string]any{}
	expectedMerged := mmodel.DeepMergeSettings(existingSettings, inputSettings)
	cacheKey := query.BuildLedgerSettingsCacheKey(orgID, ledgerID)

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(existingSettings, nil)

	mockLedgerRepo.EXPECT().
		ReplaceSettings(gomock.Any(), orgID, ledgerID, expectedMerged).
		Return(expectedMerged, nil)

	// Cache should be invalidated after successful update
	mockRedisRepo.EXPECT().
		Del(gomock.Any(), cacheKey).
		Return(nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.Equal(t, expectedMerged, settings)
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
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}
	existingSettings := map[string]any{}
	expectedMerged := mmodel.DeepMergeSettings(existingSettings, inputSettings)
	cacheKey := query.BuildLedgerSettingsCacheKey(orgID, ledgerID)

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(existingSettings, nil)

	mockLedgerRepo.EXPECT().
		ReplaceSettings(gomock.Any(), orgID, ledgerID, expectedMerged).
		Return(expectedMerged, nil)

	// Cache invalidation fails - operation should still succeed
	mockRedisRepo.EXPECT().
		Del(gomock.Any(), cacheKey).
		Return(errors.New("redis connection error"))

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.Equal(t, expectedMerged, settings)
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
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}
	existingSettings := map[string]any{}
	expectedMerged := mmodel.DeepMergeSettings(existingSettings, inputSettings)

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(existingSettings, nil)

	// Database update fails
	mockLedgerRepo.EXPECT().
		ReplaceSettings(gomock.Any(), orgID, ledgerID, expectedMerged).
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

	// Existing settings should be preserved when input is nil
	existingSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(existingSettings, nil)

	// DeepMergeSettings with nil returns existing
	mockLedgerRepo.EXPECT().
		ReplaceSettings(gomock.Any(), orgID, ledgerID, existingSettings).
		Return(existingSettings, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, nil)

	require.NoError(t, err)
	assert.NotNil(t, settings)
	assert.Equal(t, existingSettings, settings)
}

func TestUpdateLedgerSettings_ValidationSkippedForNilInput(t *testing.T) {
	// Validates that nil input passes validation (ValidateSettings returns nil for nil input)
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

	existingSettings := map[string]any{}

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(existingSettings, nil)

	mockLedgerRepo.EXPECT().
		ReplaceSettings(gomock.Any(), orgID, ledgerID, existingSettings).
		Return(existingSettings, nil)

	// Nil input should be accepted (validation passes for nil)
	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, nil)

	require.NoError(t, err)
	assert.NotNil(t, settings)
}

func TestUpdateLedgerSettings_NoRedisRepo_SkipsCacheInvalidation(t *testing.T) {
	// Tests that cache invalidation is skipped when RedisRepo is nil
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
		RedisRepo:  nil, // No redis repo
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	inputSettings := map[string]any{
		"accounting": map[string]any{
			"validateAccountType": true,
		},
	}
	existingSettings := map[string]any{}
	expectedMerged := mmodel.DeepMergeSettings(existingSettings, inputSettings)

	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), orgID, ledgerID).
		Return(existingSettings, nil)

	mockLedgerRepo.EXPECT().
		ReplaceSettings(gomock.Any(), orgID, ledgerID, expectedMerged).
		Return(expectedMerged, nil)

	// No cache invalidation call expected since RedisRepo is nil

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.Equal(t, expectedMerged, settings)
}
