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

func TestDeleteSettings(t *testing.T) {
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
		Settings:        mmodel.JSON{"key": "value"},
		Enabled:         true,
		CreatedAt:       time.Now().Add(-time.Hour),
		UpdatedAt:       time.Now().Add(-time.Minute),
	}

	tests := []struct {
		name      string
		mockSetup func()
		expectErr bool
	}{
		{
			name: "Success deleting existing settings",
			mockSetup: func() {
				mockRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID).
					Return(existingSettings, nil).
					Times(1)

				mockRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, applicationName).
					Return(nil).
					Times(1)
			},
			expectErr: false,
		},
		{
			name: "Settings not found",
			mockSetup: func() {
				mockRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID).
					Return(nil, nil).
					Times(1)
			},
			expectErr: true,
		},
		{
			name: "Error finding existing settings",
			mockSetup: func() {
				mockRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID).
					Return(nil, assert.AnError).
					Times(1)
			},
			expectErr: true,
		},
		{
			name: "Error during delete",
			mockSetup: func() {
				mockRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID).
					Return(existingSettings, nil).
					Times(1)

				mockRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, applicationName).
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
			err := uc.DeleteSettings(ctx, organizationID, ledgerID, applicationName)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
