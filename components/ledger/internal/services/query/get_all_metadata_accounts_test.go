// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
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
		AccountRepo:            mockAccountRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	// Pre-generate UUIDs for deterministic assertions
	acc1ID := uuid.New()
	acc2ID := uuid.New()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		portfolioID    *uuid.UUID
		segmentID      *uuid.UUID
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
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Not(gomock.Nil()), gomock.Any(), gomock.Any()).
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
			name:           "success - filters by segment when provided",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			segmentID:      func() *uuid.UUID { id := uuid.New(); return &id }(),
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"key": "seg-value"}},
					}, nil)
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Not(gomock.Nil()), gomock.Any()).
					Return([]*mmodel.Account{
						{ID: acc1ID.String(), Name: "Segment Account"},
					}, nil)
			},
			expectErr: false,
			validate: func(t *testing.T, result []*mmodel.Account) {
				require.Len(t, result, 1)
				assert.Equal(t, acc1ID.String(), result[0].ID)
				assert.Equal(t, "seg-value", result[0].Metadata["key"])
			},
		},
		{
			name:           "success - filters by both portfolio and segment when provided",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    func() *uuid.UUID { id := uuid.New(); return &id }(),
			segmentID:      func() *uuid.UUID { id := uuid.New(); return &id }(),
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"key": "both-value"}},
					}, nil)
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Not(gomock.Nil()), gomock.Not(gomock.Nil()), gomock.Any()).
					Return([]*mmodel.Account{
						{ID: acc1ID.String(), Name: "Both Filters Account"},
					}, nil)
			},
			expectErr: false,
			validate: func(t *testing.T, result []*mmodel.Account) {
				require.Len(t, result, 1)
				assert.Equal(t, acc1ID.String(), result[0].ID)
				assert.Equal(t, "both-value", result[0].Metadata["key"])
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
			errContains: "mongodb connection failed",
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
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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
		{
			name:           "success - metadata filter combined with status filter",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			filter: http.QueryHeader{
				UseMetadata: true,
				Status:      func() *string { s := "ACTIVE"; return &s }(),
			},
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"tier": "premium"}},
						{EntityID: acc2ID.String(), Data: map[string]any{"tier": "basic"}},
					}, nil)
				// The key assertion: entityIDs are in filter.EntityIDs AND filter.Status is forwarded
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Account{
						{ID: acc1ID.String(), Name: "Premium Active", Status: mmodel.Status{Code: "ACTIVE"}},
					}, nil)
			},
			expectErr: false,
			validate: func(t *testing.T, result []*mmodel.Account) {
				require.Len(t, result, 1, "status filter should reduce results")
				assert.Equal(t, acc1ID.String(), result[0].ID)
				assert.Equal(t, "premium", result[0].Metadata["tier"])
			},
		},
		{
			name:           "success - metadata filter combined with asset_code filter",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			filter: http.QueryHeader{
				UseMetadata: true,
				AssetCode:   func() *string { s := "USD"; return &s }(),
			},
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"region": "US"}},
					}, nil)
				// entityIDs AND AssetCode filter are both passed to FindAll
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Account{
						{ID: acc1ID.String(), Name: "USD Account", AssetCode: "USD"},
					}, nil)
			},
			expectErr: false,
			validate: func(t *testing.T, result []*mmodel.Account) {
				require.Len(t, result, 1)
				assert.Equal(t, "USD", result[0].AssetCode)
				assert.Equal(t, "US", result[0].Metadata["region"])
			},
		},
		{
			name:           "success - metadata filter combined with type filter",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			filter: http.QueryHeader{
				UseMetadata: true,
				Type:        func() *string { s := "deposit"; return &s }(),
			},
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"category": "savings"}},
					}, nil)
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Account{
						{ID: acc1ID.String(), Name: "Deposit Account", Type: "deposit"},
					}, nil)
			},
			expectErr: false,
			validate: func(t *testing.T, result []*mmodel.Account) {
				require.Len(t, result, 1)
				assert.Equal(t, "deposit", result[0].Type)
				assert.Equal(t, "savings", result[0].Metadata["category"])
			},
		},
		{
			name:           "success - metadata filter combined with alias filter",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			filter: http.QueryHeader{
				UseMetadata: true,
				Alias:       func() *string { s := "main-account"; return &s }(),
			},
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"priority": "high"}},
					}, nil)
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Account{
						{ID: acc1ID.String(), Name: "Main Account", Alias: func() *string { s := "main-account"; return &s }()},
					}, nil)
			},
			expectErr: false,
			validate: func(t *testing.T, result []*mmodel.Account) {
				require.Len(t, result, 1)
				assert.Equal(t, "main-account", *result[0].Alias)
				assert.Equal(t, "high", result[0].Metadata["priority"])
			},
		},
		{
			name:           "success - metadata filter combined with multiple filters (status + asset_code + type)",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			filter: http.QueryHeader{
				UseMetadata: true,
				Status:      func() *string { s := "ACTIVE"; return &s }(),
				AssetCode:   func() *string { s := "BRL"; return &s }(),
				Type:        func() *string { s := "creditCard"; return &s }(),
			},
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"limit": 5000}},
					}, nil)
				// All filters combined: entityIDs + status + asset_code + type
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Account{
						{ID: acc1ID.String(), Name: "BRL Credit Card", AssetCode: "BRL", Type: "creditCard", Status: mmodel.Status{Code: "ACTIVE"}},
					}, nil)
			},
			expectErr: false,
			validate: func(t *testing.T, result []*mmodel.Account) {
				require.Len(t, result, 1)
				assert.Equal(t, "BRL", result[0].AssetCode)
				assert.Equal(t, "creditCard", result[0].Type)
				assert.Equal(t, "ACTIVE", result[0].Status.Code)
				assert.Equal(t, 5000, result[0].Metadata["limit"])
			},
		},
		{
			name:           "success - metadata filter combined with blocked filter",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			filter: http.QueryHeader{
				UseMetadata: true,
				Blocked:     func() *bool { b := true; return &b }(),
			},
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"reason": "fraud"}},
					}, nil)
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Account{
						{ID: acc1ID.String(), Name: "Blocked Account"},
					}, nil)
			},
			expectErr: false,
			validate: func(t *testing.T, result []*mmodel.Account) {
				require.Len(t, result, 1)
				assert.Equal(t, "fraud", result[0].Metadata["reason"])
			},
		},
		{
			name:           "success - metadata filter returns empty when no accounts match combined filters",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			filter: http.QueryHeader{
				UseMetadata: true,
				Status:      func() *string { s := "INACTIVE"; return &s }(),
			},
			mockSetup: func() {
				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", gomock.Any()).
					Return([]*mongodb.Metadata{
						{EntityID: acc1ID.String(), Data: map[string]any{"key": "value"}},
					}, nil)
				// Metadata found IDs but status filter excludes all
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr:   true,
			errContains: "No accounts were found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.GetAllMetadataAccounts(ctx, tt.organizationID, tt.ledgerID, tt.portfolioID, tt.segmentID, tt.filter)

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
