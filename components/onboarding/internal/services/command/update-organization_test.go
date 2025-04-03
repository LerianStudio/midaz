package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// \1 performs an operation
func TestUpdateOrganizationByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrganizationRepo := organization.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OrganizationRepo: mockOrganizationRepo,
		MetadataRepo:     mockMetadataRepo,
	}

	tests := []struct {
		name      string
		orgID     uuid.UUID
		input     *mmodel.UpdateOrganizationInput
		mockSetup func()
		expectErr bool
	}{
		{
			name:  "Success - Organization updated with metadata",
			orgID: uuid.New(),
			input: &mmodel.UpdateOrganizationInput{
				ParentOrganizationID: nil,
				LegalName:            "Updated Organization",
				DoingBusinessAs:      "Updated DBA",
				Address:              mmodel.Address{Country: "US"},
				Status:               mmodel.Status{Code: "active"},
				Metadata:             map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockOrganizationRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Organization{
						ID:                   "123",
						LegalName:            "Updated Organization",
						DoingBusinessAs:      ptr.StringPtr("Updated DBA"),
						Address:              mmodel.Address{Country: "US"},
						Status:               mmodel.Status{Code: "active"},
						ParentOrganizationID: nil,
					}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectErr: false,
		},
		{
			name:  "Error - Invalid address",
			orgID: uuid.New(),
			input: &mmodel.UpdateOrganizationInput{
				Address: mmodel.Address{Country: "INVALID"},
			},
			mockSetup: func() {},
			expectErr: true,
		},
		{
			name:  "Error - Organization not found",
			orgID: uuid.New(),
			input: &mmodel.UpdateOrganizationInput{
				LegalName: "Nonexistent Organization",
				Address:   mmodel.Address{Country: "US"},
			},
			mockSetup: func() {
				mockOrganizationRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr: true,
		},
		{
			name:  "Error - Failed to update metadata",
			orgID: uuid.New(),
			input: &mmodel.UpdateOrganizationInput{
				LegalName: "Organization with Metadata Error",
				Address:   mmodel.Address{Country: "US"},
				Metadata:  map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockOrganizationRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Organization{ID: "123"}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("metadata update error"))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.UpdateOrganizationByID(ctx, tt.orgID, tt.input)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.input.LegalName, result.LegalName)
				assert.Equal(t, tt.input.Address, result.Address)
				assert.Equal(t, tt.input.Status, result.Status)
			}
		})
	}
}
