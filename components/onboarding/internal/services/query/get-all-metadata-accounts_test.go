// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetAllMetadataAccounts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo:  mockAccountRepo,
		MetadataRepo: mockMetadataRepo,
	}

	// Pre-generate UUIDs for deterministic assertions
	acc1ID := uuid.New()
	acc2ID := uuid.New()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		portfolioID    *uuid.UUID
		filter         http.QueryHeader
		mockSetup      func()
		expectErr      bool
		errContains    string
		validate       func(t *testing.T, result []*mmodel.Account)
	}{
		{
			name:           "success - retrieves accounts with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"group": "cash"}},
						{EntityID: acc2ID.String(), Data: map[string]any{"group": "ops"}},
					}, nil)
				mockAccountRepo.EXPECT().
					ListByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq([]uuid.UUID{acc1ID, acc2ID})).
					Return([]*mmodel.Account{
						{ID: acc1ID.String(), Name: "Account 1", Status: mmodel.Status{Code: "ACTIVE"}},
						{ID: acc2ID.String(), Name: "Account 2", Status: mmodel.Status{Code: "ACTIVE"}},
					}, nil)
			},
			expectErr: false,
			validate: func(t *testing.T, result []*mmodel.Account) {
				require.Len(t, result, 2)

				// Verify metadata was attached correctly
				accMap := make(map[string]*mmodel.Account)
				for _, acc := range result {
					accMap[acc.ID] = acc
				}

				assert.Equal(t, "cash", accMap[acc1ID.String()].Metadata["group"])
				assert.Equal(t, "ops", accMap[acc2ID.String()].Metadata["group"])
			},
		},
		{
			name:           "success - single account with nested metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{
							EntityID: acc1ID.String(),
							Data: map[string]any{
								"user": map[string]any{"role": "admin"},
								"tags": []string{"vip"},
							},
						},
					}, nil)
				mockAccountRepo.EXPECT().
					ListByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Account{
						{ID: acc1ID.String(), Name: "Admin Account"},
					}, nil)
			},
			expectErr: false,
			validate: func(t *testing.T, result []*mmodel.Account) {
				require.Len(t, result, 1)
				assert.NotNil(t, result[0].Metadata["user"])
			},
		},
		{
			name:           "success - filters by portfolio when provided",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    func() *uuid.UUID { id := uuid.New(); return &id }(),
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"key": "value"}},
					}, nil)
				mockAccountRepo.EXPECT().
					ListByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Not(gomock.Nil()), gomock.Any()).
					Return([]*mmodel.Account{
						{ID: acc1ID.String(), Name: "Portfolio Account"},
					}, nil)
			},
			expectErr: false,
			validate: func(t *testing.T, result []*mmodel.Account) {
				require.Len(t, result, 1)
				assert.Equal(t, acc1ID.String(), result[0].ID)
				assert.Equal(t, "value", result[0].Metadata["key"])
			},
		},
		{
			name:           "error - metadata not found returns nil",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return(nil, nil) // nil metadata triggers error
			},
			expectErr:   true,
			errContains: "No accounts were found",
		},
		{
			name:           "error - metadata repository returns error",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return(nil, errors.New("mongodb connection failed"))
			},
			expectErr:   true,
			errContains: "No accounts were found",
		},
		{
			name:           "error - account repository returns ErrDatabaseItemNotFound",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"key": "value"}},
					}, nil)
				mockAccountRepo.EXPECT().
					ListByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr:   true,
			errContains: "No accounts were found",
		},
		{
			name:           "error - account repository returns generic error",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"key": "value"}},
					}, nil)
				mockAccountRepo.EXPECT().
					ListByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database connection timeout"))
			},
			expectErr:   true,
			errContains: "database connection timeout",
		},
		{
			name:           "success - metadata attached only to matching accounts",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			mockSetup: func() {
				// Metadata has 2 entries, but repo only returns 1 account
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"found": true}},
						{EntityID: acc2ID.String(), Data: map[string]any{"found": true}},
					}, nil)
				mockAccountRepo.EXPECT().
					ListByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Account{
						{ID: acc1ID.String(), Name: "Only This One"},
					}, nil)
			},
			expectErr: false,
			validate: func(t *testing.T, result []*mmodel.Account) {
				require.Len(t, result, 1, "should return only accounts found in postgres")
				assert.Equal(t, acc1ID.String(), result[0].ID)
				assert.Equal(t, true, result[0].Metadata["found"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.GetAllMetadataAccounts(ctx, tt.organizationID, tt.ledgerID, tt.portfolioID, tt.filter)

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, result)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}
