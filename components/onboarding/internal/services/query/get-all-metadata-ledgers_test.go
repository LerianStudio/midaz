package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestGetAllMetadataLedgers(t *testing.T) {
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
		filter         http.QueryHeader
		mockSetup      func()
		expectErr      bool
		expectedResult []*mmodel.Ledger
	}{
		{
			name:           "Success - Retrieve ledgers with metadata",
			organizationID: uuid.New(),
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}},
					}, nil)
				mockLedgerRepo.EXPECT().
					ListByIDs(gomock.Any(), gomock.Any(), gomock.Eq([]uuid.UUID{validUUID})).
					Return([]*mmodel.Ledger{
						{ID: validUUID.String(), Name: "Test Ledger", Status: mmodel.Status{Code: "active"}},
					}, nil)
			},
			expectErr: false,
			expectedResult: []*mmodel.Ledger{
				{ID: "valid-uuid", Name: "Test Ledger", Status: mmodel.Status{Code: "active"}, Metadata: map[string]any{"key": "value"}},
			},
		},
		{
			name:           "Error - Failed to retrieve ledgers",
			organizationID: uuid.New(),
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}},
					}, nil)
				mockLedgerRepo.EXPECT().
					ListByIDs(gomock.Any(), gomock.Any(), gomock.Eq([]uuid.UUID{validUUID})).
					Return(nil, errors.New("database error"))
			},
			expectErr:      true,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.GetAllMetadataLedgers(ctx, tt.organizationID, tt.filter)

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
