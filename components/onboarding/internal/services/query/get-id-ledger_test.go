package query

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestGetLedgerByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo:   mockLedgerRepo,
		MetadataRepo: mockMetadataRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		mockSetup      func()
		expectErr      bool
		expectedResult *mmodel.Ledger
	}{
		{
			name:           "Success - Retrieve ledger with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			mockSetup: func() {
				ledgerID := uuid.New()
				mockLedgerRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Ledger{ID: ledgerID.String(), Name: "Test Ledger", Status: mmodel.Status{Code: "active"}}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"key": "value"}}, nil)
			},
			expectErr: false,
			expectedResult: &mmodel.Ledger{
				ID:       "valid-uuid",
				Name:     "Test Ledger",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
		},
		{
			name:           "Error - Ledger not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			mockSetup: func() {
				mockLedgerRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr:      true,
			expectedResult: nil,
		},
		{
			name:           "Error - Failed to retrieve metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			mockSetup: func() {
				ledgerID := uuid.New()
				mockLedgerRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Ledger{ID: ledgerID.String(), Name: "Test Ledger", Status: mmodel.Status{Code: "active"}}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("metadata retrieval error"))
			},
			expectErr:      true,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.GetLedgerByID(ctx, tt.organizationID, tt.ledgerID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
