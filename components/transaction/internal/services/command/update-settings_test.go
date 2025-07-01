package command

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateSettingsSuccess tests updating a setting successfully
func TestUpdateSettingsSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	active := false
	updatedSetting := &mmodel.Settings{
		ID:             settingID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            "test_setting_key",
		Active:         &active,
		Description:    "Updated test setting description",
	}

	mockRepo := settings.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
		RedisRepo:    mockRedisRepo,
	}

	input := &mmodel.UpdateSettingsInput{
		Active:      &active,
		Description: "Updated test setting description",
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, settingID, gomock.Any()).
		Return(updatedSetting, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), "false", gomock.Any()).
		Return(nil).
		Times(1)

	result, err := uc.UpdateSettings(context.Background(), organizationID, ledgerID, settingID, input)

	assert.NoError(t, err)
	assert.Equal(t, updatedSetting, result)
}

// TestUpdateSettingsNotFound tests updating a setting that is not found
func TestUpdateSettingsNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	mockRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	input := &mmodel.UpdateSettingsInput{
		Description: "Updated test setting description",
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, settingID, gomock.Any()).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.UpdateSettings(context.Background(), organizationID, ledgerID, settingID, input)

	assert.Error(t, err)
	assert.Nil(t, result)

	businessError := pkg.ValidateBusinessError(constant.ErrSettingsNotFound, reflect.TypeOf(mmodel.Settings{}).Name())
	assert.Equal(t, businessError, err)
}

// TestUpdateSettingsRepositoryError tests updating a setting with repository error
func TestUpdateSettingsRepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	expectedError := errors.New("database error")

	mockRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	input := &mmodel.UpdateSettingsInput{
		Description: "Updated test setting description",
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, settingID, gomock.Any()).
		Return(nil, expectedError).
		Times(1)

	result, err := uc.UpdateSettings(context.Background(), organizationID, ledgerID, settingID, input)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, result)
}

// TestUpdateSettingsPartialUpdate tests updating a setting with partial input (only description)
func TestUpdateSettingsPartialUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	active := true
	updatedSetting := &mmodel.Settings{
		ID:             settingID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            "test_setting_key",
		Active:         &active,
		Description:    "Updated description only",
	}

	mockRepo := settings.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
		RedisRepo:    mockRedisRepo,
	}

	input := &mmodel.UpdateSettingsInput{
		Description: "Updated description only",
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, settingID, gomock.Any()).
		Return(updatedSetting, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), "true", gomock.Any()).
		Return(nil).
		Times(1)

	result, err := uc.UpdateSettings(context.Background(), organizationID, ledgerID, settingID, input)

	assert.NoError(t, err)
	assert.Equal(t, updatedSetting, result)
}

// TestUpdateSettingsCacheError tests that cache errors don't break the update operation
func TestUpdateSettingsCacheError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	active := true
	updatedSetting := &mmodel.Settings{
		ID:             settingID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            "cache_error_test_setting",
		Active:         &active,
		Description:    "Test setting for cache error",
	}

	mockRepo := settings.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
		RedisRepo:    mockRedisRepo,
	}

	input := &mmodel.UpdateSettingsInput{
		Active:      &active,
		Description: "Test setting for cache error",
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, settingID, gomock.Any()).
		Return(updatedSetting, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), gomock.Any(), "true", gomock.Any()).
		Return(errors.New("redis connection error")).
		Times(1)

	result, err := uc.UpdateSettings(context.Background(), organizationID, ledgerID, settingID, input)

	assert.NoError(t, err)
	assert.Equal(t, updatedSetting, result)
}
