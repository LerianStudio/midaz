package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateSettingsSuccess is responsible to test CreateSettings with success
func TestCreateSettingsSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	payload := &mmodel.CreateSettingsInput{
		Key:         "accounting_validation_enabled",
		Active:      true,
		Description: "Controls whether strict accounting validation rules are enforced",
	}

	expectedSettings := &mmodel.Settings{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            payload.Key,
		Active:         &payload.Active,
		Description:    payload.Description,
	}

	mockSettingsRepo := settings.NewMockRepository(ctrl)

	uc := UseCase{
		SettingsRepo: mockSettingsRepo,
	}

	mockSettingsRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedSettings, nil).
		Times(1)

	res, err := uc.CreateSettings(context.TODO(), organizationID, ledgerID, payload)

	assert.Equal(t, expectedSettings, res)
	assert.Nil(t, err)
}

// TestCreateSettingsError is responsible to test CreateSettings with error
func TestCreateSettingsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	errMSG := "err to create settings on database"

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	payload := &mmodel.CreateSettingsInput{
		Key:         "transaction_timeout_enabled",
		Active:      false,
		Description: "Controls whether transaction timeout is enabled",
	}

	mockSettingsRepo := settings.NewMockRepository(ctrl)

	uc := UseCase{
		SettingsRepo: mockSettingsRepo,
	}

	mockSettingsRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, errors.New(errMSG)).
		Times(1)

	res, err := uc.CreateSettings(context.TODO(), organizationID, ledgerID, payload)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}

// TestCreateSettingsWithDifferentActiveValues tests CreateSettings with various active values
func TestCreateSettingsWithDifferentActiveValues(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	testCases := []struct {
		name   string
		active bool
	}{
		{"ActiveTrue", true},
		{"ActiveFalse", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := &mmodel.CreateSettingsInput{
				Key:         "test_setting_" + tc.name,
				Active:      tc.active,
				Description: "Test setting for " + tc.name,
			}

			expectedSettings := &mmodel.Settings{
				ID:             libCommons.GenerateUUIDv7(),
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Key:            payload.Key,
				Active:         &payload.Active,
				Description:    payload.Description,
			}

			mockSettingsRepo := settings.NewMockRepository(ctrl)

			uc := UseCase{
				SettingsRepo: mockSettingsRepo,
			}

			mockSettingsRepo.EXPECT().
				Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
				Return(expectedSettings, nil).
				Times(1)

			res, err := uc.CreateSettings(context.TODO(), organizationID, ledgerID, payload)

			assert.NoError(t, err)
			assert.Equal(t, expectedSettings, res)
			assert.Equal(t, tc.active, *res.Active)
		})
	}
}
