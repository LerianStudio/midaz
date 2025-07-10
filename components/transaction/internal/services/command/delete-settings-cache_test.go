package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteSettingsCacheSuccess tests successful cache deletion
func TestDeleteSettingsCacheSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingID := libCommons.GenerateUUIDv7()

	expectedSetting := &mmodel.Settings{
		ID:             settingID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            "test_setting_key",
		Active:         &[]bool{true}[0],
		Description:    "Test setting for cache deletion",
	}

	mockSettingsRepo := settings.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockSettingsRepo,
		RedisRepo:    mockRedisRepo,
	}

	mockSettingsRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, settingID).
		Return(expectedSetting, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	err := uc.DeleteSettingsCache(context.Background(), organizationID, ledgerID, settingID)

	assert.NoError(t, err)
}

// TestDeleteSettingsCacheSettingsNotFound tests cache deletion when setting is not found
func TestDeleteSettingsCacheSettingsNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingID := libCommons.GenerateUUIDv7()

	mockSettingsRepo := settings.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockSettingsRepo,
		RedisRepo:    mockRedisRepo,
	}

	mockSettingsRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, settingID).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	// RedisRepo.Del should not be called since FindByID failed

	err := uc.DeleteSettingsCache(context.Background(), organizationID, ledgerID, settingID)

	assert.Error(t, err)
	assert.Equal(t, services.ErrDatabaseItemNotFound, err)
}

// TestDeleteSettingsCacheSettingsRepoError tests cache deletion with settings repository error
func TestDeleteSettingsCacheSettingsRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingID := libCommons.GenerateUUIDv7()
	settingsError := errors.New("settings repository error")

	mockSettingsRepo := settings.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockSettingsRepo,
		RedisRepo:    mockRedisRepo,
	}

	mockSettingsRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, settingID).
		Return(nil, settingsError).
		Times(1)

	// RedisRepo.Del should not be called since FindByID failed

	err := uc.DeleteSettingsCache(context.Background(), organizationID, ledgerID, settingID)

	assert.Error(t, err)
	assert.Equal(t, settingsError, err)
}

// TestDeleteSettingsCacheRedisError tests cache deletion with Redis error
func TestDeleteSettingsCacheRedisError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingID := libCommons.GenerateUUIDv7()
	redisError := errors.New("redis connection error")

	expectedSetting := &mmodel.Settings{
		ID:             settingID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            "test_setting_redis_error",
		Active:         &[]bool{false}[0],
		Description:    "Test setting for Redis error",
	}

	mockSettingsRepo := settings.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockSettingsRepo,
		RedisRepo:    mockRedisRepo,
	}

	mockSettingsRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, settingID).
		Return(expectedSetting, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(redisError).
		Times(1)

	err := uc.DeleteSettingsCache(context.Background(), organizationID, ledgerID, settingID)

	assert.Error(t, err)
	assert.Equal(t, redisError, err)
}

// TestDeleteSettingsCacheWithDifferentKeys tests cache deletion with various setting keys
func TestDeleteSettingsCacheWithDifferentKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	settingID := libCommons.GenerateUUIDv7()

	expectedSetting := &mmodel.Settings{
		ID:             settingID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            "complex.setting.key_with-special-chars123",
		Active:         &[]bool{true}[0],
		Description:    "Test setting with complex key",
	}

	mockSettingsRepo := settings.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockSettingsRepo,
		RedisRepo:    mockRedisRepo,
	}

	mockSettingsRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, settingID).
		Return(expectedSetting, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	err := uc.DeleteSettingsCache(context.Background(), organizationID, ledgerID, settingID)

	assert.NoError(t, err)
}
