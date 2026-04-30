// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAllMetadataOrganizations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrganizationRepo := organization.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OrganizationRepo:       mockOrganizationRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
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
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}},
					}, nil)
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Test Organization", Status: mmodel.Status{Code: "active"}},
					}, nil)
			},
			expectErr: false,
			expectedResult: []*mmodel.Organization{
				{ID: "valid-uuid", LegalName: "Test Organization", Status: mmodel.Status{Code: "active"}, Metadata: map[string]any{"key": "value"}},
			},
		},
		{
			name: "Error - Failed to retrieve organizations",
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}},
					}, nil)
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))
			},
			expectErr:      true,
			expectedResult: nil,
		},
		{
			name: "Success - Metadata filter combined with status filter",
			filter: http.QueryHeader{
				UseMetadata: true,
				Status:      func() *string { s := "ACTIVE"; return &s }(),
			},
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: validUUID.String(), Data: map[string]any{"tier": "enterprise"}},
					}, nil)
				// entityIDs AND status filter are both passed to FindAll
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Enterprise Org", Status: mmodel.Status{Code: "ACTIVE"}},
					}, nil)
			},
			expectErr:      false,
			expectedResult: nil,
		},
		{
			name: "Success - Metadata filter combined with legal_name filter",
			filter: http.QueryHeader{
				UseMetadata: true,
				LegalName:   func() *string { s := "Acme"; return &s }(),
			},
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: validUUID.String(), Data: map[string]any{"industry": "tech"}},
					}, nil)
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Acme Corporation", Status: mmodel.Status{Code: "ACTIVE"}},
					}, nil)
			},
			expectErr:      false,
			expectedResult: nil,
		},
		{
			name: "Success - Metadata filter combined with doing_business_as filter",
			filter: http.QueryHeader{
				UseMetadata:     true,
				DoingBusinessAs: func() *string { s := "TechCorp"; return &s }(),
			},
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: validUUID.String(), Data: map[string]any{"region": "LATAM"}},
					}, nil)
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Tech Corporation LLC", DoingBusinessAs: func() *string { s := "TechCorp"; return &s }()},
					}, nil)
			},
			expectErr:      false,
			expectedResult: nil,
		},
		{
			name: "Success - Metadata filter combined with multiple filters (status + legal_name)",
			filter: http.QueryHeader{
				UseMetadata: true,
				Status:      func() *string { s := "ACTIVE"; return &s }(),
				LegalName:   func() *string { s := "Global"; return &s }(),
			},
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: validUUID.String(), Data: map[string]any{"size": "large"}},
					}, nil)
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Global Industries", Status: mmodel.Status{Code: "ACTIVE"}},
					}, nil)
			},
			expectErr:      false,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.GetAllMetadataOrganizations(ctx, tt.filter)

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
