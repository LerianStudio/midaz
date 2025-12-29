package query

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

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

func TestGetOrganizationByID_NilID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetOrganizationByID(ctx, uuid.Nil)
}
