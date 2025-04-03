package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// \1 performs an operation
func TestGetOrganizationByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrganizationRepo := organization.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OrganizationRepo: mockOrganizationRepo,
		MetadataRepo:     mockMetadataRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		mockSetup      func()
		expectErr      bool
		expectedResult *mmodel.Organization
	}{
		{
			name:           "Success - Retrieve organization with metadata",
			organizationID: uuid.New(),
			mockSetup: func() {
				orgID := uuid.New()
				mockOrganizationRepo.EXPECT().
					Find(gomock.Any(), gomock.Any()).
					Return(&mmodel.Organization{ID: orgID.String(), LegalName: "Test Organization", Status: mmodel.Status{Code: "ACTIVE"}}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"key": "value"}}, nil)
			},
			expectErr: false,
			expectedResult: &mmodel.Organization{
				ID:        "valid-uuid",
				LegalName: "Test Organization",
				Status:    mmodel.Status{Code: "ACTIVE"},
				Metadata:  map[string]any{"key": "value"},
			},
		},
		{
			name:           "Error - Organization not found",
			organizationID: uuid.New(),
			mockSetup: func() {
				mockOrganizationRepo.EXPECT().
					Find(gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr:      true,
			expectedResult: nil,
		},
		{
			name:           "Error - Failed to retrieve metadata",
			organizationID: uuid.New(),
			mockSetup: func() {
				orgID := uuid.New()
				mockOrganizationRepo.EXPECT().
					Find(gomock.Any(), gomock.Any()).
					Return(&mmodel.Organization{ID: orgID.String(), LegalName: "Test Organization", Status: mmodel.Status{Code: "ACTIVE"}}, nil)
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
			result, err := uc.GetOrganizationByID(ctx, tt.organizationID)

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
