// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
					FindEntityIDs(gomock.Any(), gomock.Any(), gomock.Any(), "", metadataListBatchSize).
					Return([]string{validUUID.String()}, nil)
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Test Organization", Status: mmodel.Status{Code: "active"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Organization", []string{validUUID.String()}).
					Return([]*mongodb.Metadata{{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}}}, nil)
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
					FindEntityIDs(gomock.Any(), gomock.Any(), gomock.Any(), "", metadataListBatchSize).
					Return([]string{validUUID.String()}, nil)
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
				Limit:       10,
				Page:        1,
				UseMetadata: true,
				Status:      func() *string { s := "ACTIVE"; return &s }(),
			},
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), gomock.Any(), gomock.Any(), "", metadataListBatchSize).
					Return([]string{validUUID.String()}, nil)
				// entityIDs AND status filter are both passed to FindAll
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Enterprise Org", Status: mmodel.Status{Code: "ACTIVE"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Organization", []string{validUUID.String()}).
					Return([]*mongodb.Metadata{{EntityID: validUUID.String(), Data: map[string]any{"tier": "enterprise"}}}, nil)
			},
			expectErr:      false,
			expectedResult: nil,
		},
		{
			name: "Success - Metadata filter combined with legal_name filter",
			filter: http.QueryHeader{
				Limit:       10,
				Page:        1,
				UseMetadata: true,
				LegalName:   func() *string { s := "Acme"; return &s }(),
			},
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), gomock.Any(), gomock.Any(), "", metadataListBatchSize).
					Return([]string{validUUID.String()}, nil)
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Acme Corporation", Status: mmodel.Status{Code: "ACTIVE"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Organization", []string{validUUID.String()}).
					Return([]*mongodb.Metadata{{EntityID: validUUID.String(), Data: map[string]any{"industry": "tech"}}}, nil)
			},
			expectErr:      false,
			expectedResult: nil,
		},
		{
			name: "Success - Metadata filter combined with doing_business_as filter",
			filter: http.QueryHeader{
				Limit:           10,
				Page:            1,
				UseMetadata:     true,
				DoingBusinessAs: func() *string { s := "TechCorp"; return &s }(),
			},
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), gomock.Any(), gomock.Any(), "", metadataListBatchSize).
					Return([]string{validUUID.String()}, nil)
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Tech Corporation LLC", DoingBusinessAs: func() *string { s := "TechCorp"; return &s }()},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Organization", []string{validUUID.String()}).
					Return([]*mongodb.Metadata{{EntityID: validUUID.String(), Data: map[string]any{"region": "LATAM"}}}, nil)
			},
			expectErr:      false,
			expectedResult: nil,
		},
		{
			name: "Success - Metadata filter combined with multiple filters (status + legal_name)",
			filter: http.QueryHeader{
				Limit:       10,
				Page:        1,
				UseMetadata: true,
				Status:      func() *string { s := "ACTIVE"; return &s }(),
				LegalName:   func() *string { s := "Global"; return &s }(),
			},
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), gomock.Any(), gomock.Any(), "", metadataListBatchSize).
					Return([]string{validUUID.String()}, nil)
				mockOrganizationRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any()).
					Return([]*mmodel.Organization{
						{ID: validUUID.String(), LegalName: "Global Industries", Status: mmodel.Status{Code: "ACTIVE"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Organization", []string{validUUID.String()}).
					Return([]*mongodb.Metadata{{EntityID: validUUID.String(), Data: map[string]any{"size": "large"}}}, nil)
			},
			expectErr:      false,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.filter.Limit == 0 {
				tt.filter.Limit, tt.filter.Page = 10, 1
			}

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

func TestGetAllMetadataOrganizations_EmptyResults(t *testing.T) {
	t.Run("no matching metadata returns an empty non-nil slice without reaching PostgreSQL", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		mockOrganizationRepo := organization.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		mockMetadataRepo.EXPECT().
			FindEntityIDs(gomock.Any(), "Organization", gomock.Any(), "", metadataListBatchSize).
			Return([]string{}, nil)

		uc := &UseCase{OrganizationRepo: mockOrganizationRepo, OnboardingMetadataRepo: mockMetadataRepo}

		result, err := uc.GetAllMetadataOrganizations(context.Background(), http.QueryHeader{UseMetadata: true, Limit: 10, Page: 1})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("no PostgreSQL row for the matched ids returns an empty non-nil slice", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		mockOrganizationRepo := organization.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		entityID := uuid.New()

		mockMetadataRepo.EXPECT().
			FindEntityIDs(gomock.Any(), "Organization", gomock.Any(), "", metadataListBatchSize).
			Return([]string{entityID.String()}, nil)
		mockOrganizationRepo.EXPECT().
			FindAll(gomock.Any(), gomock.Cond(func(qh http.QueryHeader) bool {
				return len(qh.EntityIDs) == 1 && qh.EntityIDs[0] == entityID
			})).
			Return(nil, nil)

		uc := &UseCase{OrganizationRepo: mockOrganizationRepo, OnboardingMetadataRepo: mockMetadataRepo}

		result, err := uc.GetAllMetadataOrganizations(context.Background(), http.QueryHeader{UseMetadata: true, Limit: 10, Page: 1})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result)
	})
}
