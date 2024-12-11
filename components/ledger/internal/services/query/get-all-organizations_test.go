package query

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestGetAllOrganizations(t *testing.T) {
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
		filter         http.QueryHeader
		mockSetup      func()
		expectErr      bool
		expectedResult []*mmodel.Organization
	}{
		{
			name: "Success - Retrieve organizations with metadata",
			filter: http.QueryHeader{
				Limit: 10,
				Page:  1,
			},
			mockSetup: func() {
				validUUID := uuid.New()
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), 10, 1).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Test Organization", Status: mmodel.Status{Code: "active"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}},
					}, nil)
			},
			expectErr: false,
			expectedResult: []*mmodel.Organization{
				{ID: "valid-uuid", LegalName: "Test Organization", Status: mmodel.Status{Code: "active"}, Metadata: map[string]any{"key": "value"}},
			},
		},
		{
			name: "Error - No organizations found",
			filter: http.QueryHeader{
				Limit: 10,
				Page:  1,
			},
			mockSetup: func() {
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), 10, 1).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr:      true,
			expectedResult: nil,
		},
		{
			name: "Error - Failed to retrieve metadata",
			filter: http.QueryHeader{
				Limit: 10,
				Page:  1,
			},
			mockSetup: func() {
				validUUID := uuid.New()
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), 10, 1).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Test Organization", Status: mmodel.Status{Code: "active"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
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
			result, err := uc.GetAllOrganizations(ctx, tt.filter)

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
