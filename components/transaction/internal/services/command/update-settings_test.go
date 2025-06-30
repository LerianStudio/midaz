package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateSettingsSuccess tests updating a setting successfully
func TestUpdateSettingsSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateSettingsInput{
		Value:       "false",
		Description: "Updated description for the setting",
	}

	updatedSetting := &mmodel.Settings{
		ID:             settingID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            "accounting_validation_enabled",
		Value:          input.Value,
		Description:    input.Description,
	}

	mockRepo := settings.NewMockRepository(ctrl)
	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, settingID, gomock.Any()).
		Return(updatedSetting, nil).
		Times(1)

	result, err := uc.UpdateSettings(context.Background(), organizationID, ledgerID, settingID, input)

	assert.NoError(t, err)
	assert.Equal(t, updatedSetting, result)
	assert.Equal(t, input.Value, result.Value)
	assert.Equal(t, input.Description, result.Description)
}

// TestUpdateSettingsNotFound tests when setting is not found during update
func TestUpdateSettingsNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateSettingsInput{
		Value:       "false",
		Description: "Updated description for the setting",
	}

	mockRepo := settings.NewMockRepository(ctrl)
	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, settingID, gomock.Any()).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.UpdateSettings(context.Background(), organizationID, ledgerID, settingID, input)

	assert.Error(t, err)
	assert.Nil(t, result)

	expectedBusinessError := pkg.ValidateBusinessError(constant.ErrSettingsNotFound, "Settings")
	assert.Equal(t, expectedBusinessError, err)
}

// TestUpdateSettingsRepositoryError tests handling repository errors during update
func TestUpdateSettingsRepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	repositoryError := errors.New("database connection failed")

	input := &mmodel.UpdateSettingsInput{
		Value:       "true",
		Description: "Some description",
	}

	mockRepo := settings.NewMockRepository(ctrl)
	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, settingID, gomock.Any()).
		Return(nil, repositoryError).
		Times(1)

	result, err := uc.UpdateSettings(context.Background(), organizationID, ledgerID, settingID, input)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, repositoryError, err)
}

// TestUpdateSettingsPartialUpdate tests updating only some fields
func TestUpdateSettingsPartialUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	settingID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateSettingsInput{
		Description: "Updated description only",
		// Value is not provided
	}

	updatedSetting := &mmodel.Settings{
		ID:             settingID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            "max_transaction_amount",
		Value:          "", // Value not changed in input
		Description:    input.Description,
	}

	mockRepo := settings.NewMockRepository(ctrl)
	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, settingID, gomock.Any()).
		Return(updatedSetting, nil).
		Times(1)

	result, err := uc.UpdateSettings(context.Background(), organizationID, ledgerID, settingID, input)

	assert.NoError(t, err)
	assert.Equal(t, updatedSetting, result)
	assert.Equal(t, input.Description, result.Description)
}
