package query

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetSettingsByIDSuccess tests getting a setting by ID successfully
func TestGetSettingsByIDSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	active := true
	expectedSetting := &mmodel.Settings{
		ID:             settingID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            "accounting_validation_enabled",
		Active:         &active,
		Description:    "Controls whether strict accounting validation rules are enforced",
	}

	mockRepo := settings.NewMockRepository(ctrl)
	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, settingID).
		Return(expectedSetting, nil).
		Times(1)

	result, err := uc.GetSettingsByID(context.Background(), organizationID, ledgerID, settingID)

	assert.NoError(t, err)
	assert.Equal(t, expectedSetting, result)
}

// TestGetSettingsByIDError tests getting a setting by ID with database error
func TestGetSettingsByIDError(t *testing.T) {
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

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, settingID).
		Return(nil, expectedError).
		Times(1)

	result, err := uc.GetSettingsByID(context.Background(), organizationID, ledgerID, settingID)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, result)
}

// TestGetSettingsByIDNotFound tests getting a setting by ID when not found
func TestGetSettingsByIDNotFound(t *testing.T) {
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
		FindByID(gomock.Any(), organizationID, ledgerID, settingID).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.GetSettingsByID(context.Background(), organizationID, ledgerID, settingID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "The provided setting does not exist in our records")
	assert.Nil(t, result)
}
