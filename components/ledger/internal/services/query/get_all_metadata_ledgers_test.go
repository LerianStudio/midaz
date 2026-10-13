// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetAllMetadataLedgers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo:             mockLedgerRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
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
					FindEntityIDs(gomock.Any(), gomock.Any(), gomock.Any(), "", metadataListBatchSize).
					Return([]string{validUUID.String()}, nil)
				mockLedgerRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Ledger{
						{ID: validUUID.String(), Name: "Test Ledger", Status: mmodel.Status{Code: "active"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Ledger", []string{validUUID.String()}).
					Return([]*mongodb.Metadata{{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}}}, nil)
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
					FindEntityIDs(gomock.Any(), gomock.Any(), gomock.Any(), "", metadataListBatchSize).
					Return([]string{validUUID.String()}, nil)
				mockLedgerRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))
			},
			expectErr:      true,
			expectedResult: nil,
		},
		{
			name:           "Success - Metadata filter combined with status filter",
			organizationID: uuid.New(),
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
				mockLedgerRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Ledger{
						{ID: validUUID.String(), Name: "LATAM Ledger", Status: mmodel.Status{Code: "ACTIVE"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Ledger", []string{validUUID.String()}).
					Return([]*mongodb.Metadata{{EntityID: validUUID.String(), Data: map[string]any{"region": "LATAM"}}}, nil)
			},
			expectErr:      false,
			expectedResult: nil,
		},
		{
			name:           "Success - Metadata filter combined with name filter",
			organizationID: uuid.New(),
			filter: http.QueryHeader{
				Limit:       10,
				Page:        1,
				UseMetadata: true,
				Name:        func() *string { s := "Main"; return &s }(),
			},
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), gomock.Any(), gomock.Any(), "", metadataListBatchSize).
					Return([]string{validUUID.String()}, nil)
				mockLedgerRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Ledger{
						{ID: validUUID.String(), Name: "Main Ledger", Status: mmodel.Status{Code: "ACTIVE"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Ledger", []string{validUUID.String()}).
					Return([]*mongodb.Metadata{{EntityID: validUUID.String(), Data: map[string]any{"purpose": "operations"}}}, nil)
			},
			expectErr:      false,
			expectedResult: nil,
		},
		{
			name:           "Success - Metadata filter combined with multiple filters (status + name)",
			organizationID: uuid.New(),
			filter: http.QueryHeader{
				Limit:       10,
				Page:        1,
				UseMetadata: true,
				Status:      func() *string { s := "ACTIVE"; return &s }(),
				Name:        func() *string { s := "Production"; return &s }(),
			},
			mockSetup: func() {
				validUUID := uuid.New()
				mockMetadataRepo.EXPECT().
					FindEntityIDs(gomock.Any(), gomock.Any(), gomock.Any(), "", metadataListBatchSize).
					Return([]string{validUUID.String()}, nil)
				mockLedgerRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Ledger{
						{ID: validUUID.String(), Name: "Production Ledger", Status: mmodel.Status{Code: "ACTIVE"}},
					}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Ledger", []string{validUUID.String()}).
					Return([]*mongodb.Metadata{{EntityID: validUUID.String(), Data: map[string]any{"env": "prod"}}}, nil)
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

func TestGetAllMetadataLedgers_EmptyResults(t *testing.T) {
	t.Run("no matching metadata returns an empty non-nil slice without reaching PostgreSQL", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		mockLedgerRepo := ledger.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		organizationID := uuid.New()

		mockMetadataRepo.EXPECT().
			FindEntityIDs(gomock.Any(), "Ledger", gomock.Any(), "", metadataListBatchSize).
			Return([]string{}, nil)

		uc := &UseCase{LedgerRepo: mockLedgerRepo, OnboardingMetadataRepo: mockMetadataRepo}

		result, err := uc.GetAllMetadataLedgers(context.Background(), organizationID, http.QueryHeader{UseMetadata: true, Limit: 10, Page: 1})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("no PostgreSQL row for the matched ids returns an empty non-nil slice", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		mockLedgerRepo := ledger.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		organizationID := uuid.New()
		entityID := uuid.New()

		mockMetadataRepo.EXPECT().
			FindEntityIDs(gomock.Any(), "Ledger", gomock.Any(), "", metadataListBatchSize).
			Return([]string{entityID.String()}, nil)
		mockLedgerRepo.EXPECT().
			FindAll(gomock.Any(), organizationID, gomock.Cond(func(qh http.QueryHeader) bool {
				return len(qh.EntityIDs) == 1 && qh.EntityIDs[0] == entityID
			})).
			Return(nil, nil)

		uc := &UseCase{LedgerRepo: mockLedgerRepo, OnboardingMetadataRepo: mockMetadataRepo}

		result, err := uc.GetAllMetadataLedgers(context.Background(), organizationID, http.QueryHeader{UseMetadata: true, Limit: 10, Page: 1})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result)
	})
}
