// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
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

	filter := http.QueryHeader{
		Limit: 10,
		Page:  1,
	}

	tests := []struct {
		name           string
		filter         http.QueryHeader
		mockSetup      func()
		expectErr      bool
		expectedResult []*mmodel.Organization
	}{
		{
			name:   "Success - Retrieve organizations with metadata",
			filter: filter,
			mockSetup: func() {
				validUUID := uuid.New()
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), filter.ToOffsetPagination()).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Test Organization", Status: mmodel.Status{Code: "active"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), gomock.Any(), gomock.Any()).
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
					FindAll(gomock.Any(), filter.ToOffsetPagination()).
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
					FindAll(gomock.Any(), filter.ToOffsetPagination()).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Test Organization", Status: mmodel.Status{Code: "active"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errMetadataRetrievalError)
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
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
