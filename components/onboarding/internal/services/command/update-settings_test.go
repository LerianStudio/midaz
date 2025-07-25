package command

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"
)

func TestUpdateSettings(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mongodb.NewMockSettingsRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	ctx := context.Background()
	organizationID := "org-123"
	ledgerID := "ledger-456"
	applicationName := "test-app"

	existingSettings := &mmodel.Settings{
		ID:              primitive.NewObjectID(),
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		ApplicationName: applicationName,
		Settings:        mmodel.JSON{"oldKey": "oldValue"},
		Enabled:         false,
		CreatedAt:       time.Now().Add(-time.Hour),
		UpdatedAt:       time.Now().Add(-time.Hour),
	}

	tests := []struct {
		name      string
		input     *mmodel.UpdateSettingsInput
		mockSetup func()
		expectErr bool
	}{
		{
			name: "Success updating existing settings",
			input: &mmodel.UpdateSettingsInput{
				Settings: mmodel.JSON{"newKey": "newValue", "anotherKey": "anotherValue"},
				Enabled:  true,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, applicationName).
					Return(existingSettings, nil).
					Times(1)

				mockRepo.EXPECT().
					Upsert(gomock.Any(), false, gomock.Any()).
					Return(nil).
					Times(1)
			},
			expectErr: false,
		},
		{
			name: "Settings not found",
			input: &mmodel.UpdateSettingsInput{
				Settings: mmodel.JSON{"key": "value"},
				Enabled:  true,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, applicationName).
					Return(nil, nil).
					Times(1)
			},
			expectErr: true,
		},
		{
			name: "Error finding existing settings",
			input: &mmodel.UpdateSettingsInput{
				Settings: mmodel.JSON{"key": "value"},
				Enabled:  true,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, applicationName).
					Return(nil, assert.AnError).
					Times(1)
			},
			expectErr: true,
		},
		{
			name: "Error during upsert",
			input: &mmodel.UpdateSettingsInput{
				Settings: mmodel.JSON{"key": "value"},
				Enabled:  true,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, applicationName).
					Return(existingSettings, nil).
					Times(1)

				mockRepo.EXPECT().
					Upsert(gomock.Any(), false, gomock.Any()).
					Return(assert.AnError).
					Times(1)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			tt.mockSetup()

			// Act
			result, err := uc.UpdateSettings(ctx, organizationID, ledgerID, applicationName, tt.input)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, organizationID, result.OrganizationID)
				assert.Equal(t, ledgerID, result.LedgerID)
				assert.Equal(t, applicationName, result.ApplicationName)
				assert.Equal(t, tt.input.Settings, result.Settings)
				assert.Equal(t, tt.input.Enabled, result.Enabled)
				assert.Equal(t, existingSettings.CreatedAt, result.CreatedAt)
				assert.WithinDuration(t, time.Now(), result.UpdatedAt, time.Second)
			}
		})
	}
}