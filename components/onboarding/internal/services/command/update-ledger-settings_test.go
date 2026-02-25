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

func TestUpdateLedgerSettings_NestedObjectPassedToRepository(t *testing.T) {
	// This test verifies that the service layer correctly passes nested objects
	// to the repository without modification. The actual merge behavior is tested
	// in integration tests (ledger.postgresql_integration_test.go).
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

	// Input with nested structure (3+ levels)
	inputSettings := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"deep": "value",
				},
			},
		},
	}

	// The repository returns the merged result
	// Nested objects are REPLACED entirely, not deep-merged.
	expectedResult := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"deep": "value",
				},
			},
		},
	}

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(expectedResult, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.Equal(t, expectedResult, settings)
}

func TestUpdateLedgerSettings_SpecialCharacterKeys(t *testing.T) {
	// Test that special characters in keys are passed through correctly
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
		"key.with.dots":   "value1",
		"key-with-dashes": "value2",
		"key with spaces": "value3",
		"unicode_キー":      "value4",
	}

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(inputSettings, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	assert.Equal(t, "value1", settings["key.with.dots"])
	assert.Equal(t, "value2", settings["key-with-dashes"])
	assert.Equal(t, "value3", settings["key with spaces"])
	assert.Equal(t, "value4", settings["unicode_キー"])
}

func TestUpdateLedgerSettings_EmptyNestedObject(t *testing.T) {
	// Test that empty nested objects are handled correctly
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
		"emptyConfig": map[string]any{},
	}

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(inputSettings, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)
	emptyConfig, ok := settings["emptyConfig"].(map[string]any)
	require.True(t, ok, "emptyConfig should be a map")
	assert.Empty(t, emptyConfig, "emptyConfig should be empty")
}

func TestUpdateLedgerSettings_ArrayValues(t *testing.T) {
	// Test that array values are passed through correctly
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
		"stringArray": []any{"a", "b", "c"},
		"mixedArray":  []any{"string", 123, true, nil},
		"objectArray": []any{
			map[string]any{"name": "first"},
			map[string]any{"name": "second"},
		},
	}

	mockLedgerRepo.EXPECT().
		UpdateSettings(gomock.Any(), orgID, ledgerID, inputSettings).
		Return(inputSettings, nil)

	settings, err := uc.UpdateLedgerSettings(ctx, orgID, ledgerID, inputSettings)

	require.NoError(t, err)

	stringArray, ok := settings["stringArray"].([]any)
	require.True(t, ok, "stringArray should be an array")
	assert.Len(t, stringArray, 3)

	objectArray, ok := settings["objectArray"].([]any)
	require.True(t, ok, "objectArray should be an array")
	assert.Len(t, objectArray, 2)
}
