package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteSettingsByIDSuccess tests successful deletion of a setting
func TestDeleteSettingsByIDSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

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

	settingID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

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

// TestDeleteSettingsByIDError tests deletion with database error
func TestDeleteSettingsByIDError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
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
