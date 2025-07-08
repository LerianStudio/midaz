package command

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateSettingsSuccess tests creating settings successfully
func TestCreateSettingsSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	payload := &mmodel.CreateSettingsInput{
		Key:         constant.AccountingValidationEnabledKey,
		Active:      true,
		Description: "Controls whether strict accounting validation rules are enforced",
	}

	now := time.Now()

	expectedSettings := &mmodel.Settings{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            payload.Key,
		Active:         &payload.Active,
		Description:    payload.Description,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	mockSettingsRepo := settings.NewMockRepository(ctrl)

	uc := UseCase{
		SettingsRepo: mockSettingsRepo,
	}

	mockSettingsRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledID interface{}, settings *mmodel.Settings) (*mmodel.Settings, error) {
			assert.NotZero(t, settings.CreatedAt)
			assert.NotZero(t, settings.UpdatedAt)
			assert.Equal(t, settings.CreatedAt, settings.UpdatedAt)

			return expectedSettings, nil
		}).
		Times(1)

	result, err := uc.CreateSettings(context.Background(), organizationID, ledgerID, payload)

	assert.NoError(t, err)
	assert.Equal(t, expectedSettings, result)
}

// TestCreateSettingsError tests creating settings with database error
func TestCreateSettingsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	errMsg := "failed to create settings in database"
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
		Return(nil, errors.New(errMsg)).
		Times(1)

	result, err := uc.CreateSettings(context.Background(), organizationID, ledgerID, payload)

	assert.Error(t, err)
	assert.Equal(t, errMsg, err.Error())
	assert.Nil(t, result)
}

// TestCreateSettingsValidatesInput tests that input fields are properly validated and set
func TestCreateSettingsValidatesInput(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	testCases := []struct {
		name    string
		payload *mmodel.CreateSettingsInput
	}{
		{
			name: "all fields set",
			payload: &mmodel.CreateSettingsInput{
				Key:         "test_setting",
				Active:      true,
				Description: "Test description",
			},
		},
		{
			name: "active false",
			payload: &mmodel.CreateSettingsInput{
				Key:         "inactive_setting",
				Active:      false,
				Description: "Inactive setting test",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSettingsRepo := settings.NewMockRepository(ctrl)
			uc := UseCase{
				SettingsRepo: mockSettingsRepo,
			}

			mockSettingsRepo.EXPECT().
				Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
				DoAndReturn(func(ctx context.Context, orgID, ledID interface{}, settings *mmodel.Settings) (*mmodel.Settings, error) {
					assert.Equal(t, tc.payload.Key, settings.Key)
					assert.Equal(t, tc.payload.Active, *settings.Active)
					assert.Equal(t, tc.payload.Description, settings.Description)
					assert.Equal(t, organizationID, settings.OrganizationID)
					assert.Equal(t, ledgerID, settings.LedgerID)
					assert.NotZero(t, settings.ID)
					assert.NotZero(t, settings.CreatedAt)
					assert.NotZero(t, settings.UpdatedAt)

					return settings, nil
				}).
				Times(1)

			result, err := uc.CreateSettings(context.Background(), organizationID, ledgerID, tc.payload)

			assert.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}

// TestCreateSettingsDuplicateKey tests creating settings with a duplicate key
func TestCreateSettingsDuplicateKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	payload := &mmodel.CreateSettingsInput{
		Key:         "existing_key",
		Active:      true,
		Description: "This key already exists",
	}

	mockSettingsRepo := settings.NewMockRepository(ctrl)

	uc := UseCase{
		SettingsRepo: mockSettingsRepo,
	}

	expectedErr := pkg.ValidateBusinessError(constant.ErrDuplicateSettingsKey, reflect.TypeOf(mmodel.Settings{}).Name())

	mockSettingsRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, expectedErr).
		Times(1)

	result, err := uc.CreateSettings(context.Background(), organizationID, ledgerID, payload)

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Nil(t, result)
}
