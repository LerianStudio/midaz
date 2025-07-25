package command

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCreateSettings(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mongodb.NewMockSettingsRepository(ctrl)

	uc := &UseCase{
		SettingsRepo: mockRepo,
	}

	ctx := context.Background()
	organizationID := "org-123"
	ledgerID := "ledger-456"

	tests := []struct {
		name      string
		input     *mmodel.CreateSettingsInput
		mockSetup func()
		expectErr bool
	}{
		{
			name: "Success creating settings",
			input: &mmodel.CreateSettingsInput{
				ApplicationName: "test-app",
				Settings:        mmodel.JSON{"key1": "value1", "key2": "value2"},
				Enabled:         true,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					Update(gomock.Any(), gomock.Any()).
					Return(nil).
					Times(1)
			},
			expectErr: false,
		},
		{
			name: "Error during upsert",
			input: &mmodel.CreateSettingsInput{
				Settings: mmodel.JSON{"key1": "value1"},
				Enabled:  false,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					Update(gomock.Any(), gomock.Any()).
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
			result, err := uc.CreateSettings(ctx, organizationID, ledgerID, tt.input)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, organizationID, result.OrganizationID)
				assert.Equal(t, ledgerID, result.LedgerID)
				assert.Equal(t, tt.input.ApplicationName, result.ApplicationName)
				assert.Equal(t, tt.input.Settings, result.Settings)
				assert.Equal(t, tt.input.Enabled, result.Enabled)
				assert.WithinDuration(t, time.Now(), result.CreatedAt, time.Second)
				assert.WithinDuration(t, time.Now(), result.UpdatedAt, time.Second)
			}
		})
	}
}
