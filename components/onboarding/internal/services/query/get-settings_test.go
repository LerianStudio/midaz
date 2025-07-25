package query

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

func TestGetSettings(t *testing.T) {
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
		Settings:        mmodel.JSON{"key1": "value1", "key2": "value2"},
		Enabled:         true,
		CreatedAt:       time.Now().Add(-time.Hour),
		UpdatedAt:       time.Now().Add(-time.Minute),
	}

	tests := []struct {
		name      string
		mockSetup func()
		expectErr bool
		expected  *mmodel.Settings
	}{
		{
			name: "Success getting existing settings",
			mockSetup: func() {
				mockRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, applicationName).
					Return(existingSettings, nil).
					Times(1)
			},
			expectErr: false,
			expected:  existingSettings,
		},
		{
			name: "Settings not found",
			mockSetup: func() {
				mockRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, applicationName).
					Return(nil, nil).
					Times(1)
			},
			expectErr: true,
			expected:  nil,
		},
		{
			name: "Error during find",
			mockSetup: func() {
				mockRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, applicationName).
					Return(nil, assert.AnError).
					Times(1)
			},
			expectErr: true,
			expected:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			tt.mockSetup()

			// Act
			result, err := uc.GetSettings(ctx, organizationID, ledgerID, applicationName)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected.ID, result.ID)
				assert.Equal(t, tt.expected.OrganizationID, result.OrganizationID)
				assert.Equal(t, tt.expected.LedgerID, result.LedgerID)
				assert.Equal(t, tt.expected.ApplicationName, result.ApplicationName)
				assert.Equal(t, tt.expected.Settings, result.Settings)
				assert.Equal(t, tt.expected.Enabled, result.Enabled)
				assert.Equal(t, tt.expected.CreatedAt, result.CreatedAt)
				assert.Equal(t, tt.expected.UpdatedAt, result.UpdatedAt)
			}
		})
	}
}