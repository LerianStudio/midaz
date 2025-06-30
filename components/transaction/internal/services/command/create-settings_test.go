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
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	payload := &mmodel.CreateSettingsInput{
		Key:         "accounting_validation_enabled",
		Value:       "true",
		Description: "Controls whether strict accounting validation rules are enforced",
	}

	expectedSettings := &mmodel.Settings{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Key:            payload.Key,
		Value:          payload.Value,
		Description:    payload.Description,
	}

	uc := UseCase{
		SettingsRepo: settings.NewMockRepository(gomock.NewController(t)),
	}

	uc.SettingsRepo.(*settings.MockRepository).
		EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedSettings, nil).
		Times(1)

	res, err := uc.CreateSettings(context.TODO(), organizationID, ledgerID, payload)

	assert.Equal(t, expectedSettings, res)
	assert.Nil(t, err)
}

// TestCreateSettingsError is responsible to test CreateSettings with error
func TestCreateSettingsError(t *testing.T) {
	errMSG := "err to create settings on database"

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	payload := &mmodel.CreateSettingsInput{
		Key:         "transaction_timeout_seconds",
		Value:       "300",
		Description: "Maximum time in seconds a transaction can remain in pending state",
	}

	uc := UseCase{
		SettingsRepo: settings.NewMockRepository(gomock.NewController(t)),
	}

	uc.SettingsRepo.(*settings.MockRepository).
		EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, errors.New(errMSG)).
		Times(1)

	res, err := uc.CreateSettings(context.TODO(), organizationID, ledgerID, payload)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
