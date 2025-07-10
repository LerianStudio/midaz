package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
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

	mockRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, settingID).
		Return(nil).
		Times(1)

	err := uc.DeleteSettingsByID(context.Background(), organizationID, ledgerID, settingID)

	assert.NoError(t, err)
}

// TestDeleteSettingsByIDNotFound tests deletion when setting is not found
func TestDeleteSettingsByIDNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	mockRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, settingID).
		Return(services.ErrDatabaseItemNotFound).
		Times(1)

	err := uc.DeleteSettingsByID(context.Background(), organizationID, ledgerID, settingID)

	assert.Error(t, err)

	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, "0108", entityNotFoundError.Code)
}

// TestDeleteSettingsByIDDatabaseError tests deletion with database error
func TestDeleteSettingsByIDDatabaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	databaseError := errors.New("database connection error")

	mockRepo := settings.NewMockRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, settingID).
		Return(databaseError).
		Times(1)

	err := uc.DeleteSettingsByID(context.Background(), organizationID, ledgerID, settingID)

	assert.Error(t, err)
	assert.Equal(t, databaseError, err)
}
