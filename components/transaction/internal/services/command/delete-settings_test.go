package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteSettingsByIDSuccess tests successful deletion of a setting
func TestDeleteSettingsByIDSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	existingSetting := &mmodel.Settings{
		ID:             settingID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            "test_setting_key",
		Active:         &[]bool{true}[0],
		Description:    "Test setting",
	}

	mockRepo := settings.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
		RedisRepo:    mockRedisRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, settingID).
		Return(existingSetting, nil).
		Times(1)

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, settingID).
		Return(nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	err := uc.DeleteSettingsByID(context.Background(), organizationID, ledgerID, settingID)

	assert.NoError(t, err)
}

// TestDeleteSettingsByIDNotFoundOnFind tests deletion when setting is not found during FindByID
func TestDeleteSettingsByIDNotFoundOnFind(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	mockRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
		// RedisRepo not needed since FindByID fails first
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, settingID).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	// Delete should not be called since FindByID failed

	err := uc.DeleteSettingsByID(context.Background(), organizationID, ledgerID, settingID)

	assert.Error(t, err)

	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, "0108", entityNotFoundError.Code)
}

// TestDeleteSettingsByIDNotFoundOnDelete tests deletion when setting is not found during Delete
func TestDeleteSettingsByIDNotFoundOnDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	existingSetting := &mmodel.Settings{
		ID:             settingID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            "test_setting_key",
		Active:         &[]bool{false}[0],
		Description:    "Test setting",
	}

	mockRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
		// RedisRepo not needed since Delete fails
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, settingID).
		Return(existingSetting, nil).
		Times(1)

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, settingID).
		Return(services.ErrDatabaseItemNotFound).
		Times(1)

	// Cache invalidation should not happen since Delete failed

	err := uc.DeleteSettingsByID(context.Background(), organizationID, ledgerID, settingID)

	assert.Error(t, err)

	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, "0108", entityNotFoundError.Code)
}

// TestDeleteSettingsByIDError tests deletion with database error
func TestDeleteSettingsByIDError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	databaseError := errors.New("database connection error")

	mockRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
		// RedisRepo not needed since FindByID fails
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, settingID).
		Return(nil, databaseError).
		Times(1)

	// Delete should not be called since FindByID failed

	err := uc.DeleteSettingsByID(context.Background(), organizationID, ledgerID, settingID)

	assert.Error(t, err)
	assert.Equal(t, databaseError, err)
}

// TestDeleteSettingsByIDCacheError tests that cache errors don't break the delete operation
func TestDeleteSettingsByIDCacheError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	existingSetting := &mmodel.Settings{
		ID:             settingID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            "cache_error_test_setting",
		Active:         &[]bool{true}[0],
		Description:    "Test setting for cache error",
	}

	mockRepo := settings.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
		RedisRepo:    mockRedisRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, settingID).
		Return(existingSetting, nil).
		Times(1)

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, settingID).
		Return(nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(errors.New("redis connection error")).
		Times(1)

	err := uc.DeleteSettingsByID(context.Background(), organizationID, ledgerID, settingID)

	assert.NoError(t, err)
}
