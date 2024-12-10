package command

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCreateOrganization(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := organization.NewMockRepository(ctrl)

	uc := &UseCase{
		OrganizationRepo: mockRepo,
	}

	tests := []struct {
		name        string
		input       *mmodel.CreateOrganizationInput
		mockSetup   func()
		expectErr   bool
		expectedOrg *mmodel.Organization
	}{
		{
			name: "Success with all fields provided",
			input: &mmodel.CreateOrganizationInput{
				LegalName:       "Test Org",
				DoingBusinessAs: ptr.StringPtr("Test DBA"),
				LegalDocument:   "123456789",
				Address: mmodel.Address{
					Country: "US",
				},
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Organization{
						ID:                   "123",
						LegalName:            "Test Org",
						DoingBusinessAs:      ptr.StringPtr("Test DBA"),
						LegalDocument:        "123456789",
						Address:              mmodel.Address{Country: "US"},
						Status:               mmodel.Status{Code: "ACTIVE"},
						CreatedAt:            time.Now(),
						UpdatedAt:            time.Now(),
						ParentOrganizationID: nil,
						Metadata:             nil,
					}, nil)
			},
			expectErr: false,
			expectedOrg: &mmodel.Organization{
				LegalName:       "Test Org",
				DoingBusinessAs: ptr.StringPtr("Test DBA"),
				LegalDocument:   "123456789",
				Address:         mmodel.Address{Country: "US"},
				Status:          mmodel.Status{Code: "ACTIVE"},
			},
		},
		{
			name: "Success with default status",
			input: &mmodel.CreateOrganizationInput{
				LegalName:       "Default Status Org",
				DoingBusinessAs: ptr.StringPtr("Default DBA"),
				LegalDocument:   "555555555",
				Address: mmodel.Address{
					Country: "CA",
				},
				Status:   mmodel.Status{}, // Empty status
				Metadata: nil,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Organization{
						ID:                   "124",
						LegalName:            "Default Status Org",
						DoingBusinessAs:      ptr.StringPtr("Default DBA"),
						LegalDocument:        "555555555",
						Address:              mmodel.Address{Country: "CA"},
						Status:               mmodel.Status{Code: "ACTIVE"},
						CreatedAt:            time.Now(),
						UpdatedAt:            time.Now(),
						ParentOrganizationID: nil,
						Metadata:             nil,
					}, nil)
			},
			expectErr: false,
			expectedOrg: &mmodel.Organization{
				LegalName:       "Default Status Org",
				DoingBusinessAs: ptr.StringPtr("Default DBA"),
				LegalDocument:   "555555555",
				Address:         mmodel.Address{Country: "CA"},
				Status:          mmodel.Status{Code: "ACTIVE"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.CreateOrganization(ctx, tt.input)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedOrg.LegalName, result.LegalName)
				assert.Equal(t, tt.expectedOrg.DoingBusinessAs, result.DoingBusinessAs)
				assert.Equal(t, tt.expectedOrg.LegalDocument, result.LegalDocument)
				assert.Equal(t, tt.expectedOrg.Status.Code, result.Status.Code)
			}
		})
	}
}
