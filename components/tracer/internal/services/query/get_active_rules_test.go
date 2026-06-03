// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

func TestGetActiveRulesQuery_Execute(t *testing.T) {
	rule1ID := uuid.MustParse("110e8400-e29b-41d4-a716-446655440010")
	rule2ID := uuid.MustParse("220e8400-e29b-41d4-a716-446655440020")
	testAccountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	testPortfolioID := uuid.MustParse("660e8400-e29b-41d4-a716-446655440002")
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	activeRule1 := &model.Rule{
		ID:         rule1ID,
		Name:       "High amount fraud rule",
		Expression: "amount > 100000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes: []model.Scope{
			{AccountID: testutil.UUIDPtr(testAccountID)},
		},
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now.Add(-1 * time.Hour),
	}

	activeRule2 := &model.Rule{
		ID:         rule2ID,
		Name:       "International wire review",
		Expression: "transactionType == 'WIRE' && subType == 'international'",
		Action:     model.DecisionReview,
		Status:     model.RuleStatusActive,
		Scopes: []model.Scope{
			{PortfolioID: testutil.UUIDPtr(testPortfolioID)},
		},
		CreatedAt: now.Add(-48 * time.Hour),
		UpdatedAt: now.Add(-2 * time.Hour),
	}

	tests := []struct {
		name          string
		mockSetup     func(ctrl *gomock.Controller) *MockActiveRulesRepository
		wantErr       bool
		expectedRules []*model.Rule
	}{
		{
			name: "returns active rules successfully",
			mockSetup: func(ctrl *gomock.Controller) *MockActiveRulesRepository {
				mockRepo := NewMockActiveRulesRepository(ctrl)
				mockRepo.EXPECT().
					GetActiveRules(gomock.Any(), gomock.Any()).
					Return([]*model.Rule{activeRule1, activeRule2}, nil)
				return mockRepo
			},
			wantErr:       false,
			expectedRules: []*model.Rule{activeRule1, activeRule2},
		},
		{
			name: "returns empty list when no active rules",
			mockSetup: func(ctrl *gomock.Controller) *MockActiveRulesRepository {
				mockRepo := NewMockActiveRulesRepository(ctrl)
				mockRepo.EXPECT().
					GetActiveRules(gomock.Any(), gomock.Any()).
					Return([]*model.Rule{}, nil)
				return mockRepo
			},
			wantErr:       false,
			expectedRules: []*model.Rule{},
		},
		{
			name: "returns error when repository fails",
			mockSetup: func(ctrl *gomock.Controller) *MockActiveRulesRepository {
				mockRepo := NewMockActiveRulesRepository(ctrl)
				mockRepo.EXPECT().
					GetActiveRules(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database connection failed"))
				return mockRepo
			},
			wantErr:       true,
			expectedRules: nil,
		},
		{
			name: "returns nil from repository (normalized to empty slice)",
			mockSetup: func(ctrl *gomock.Controller) *MockActiveRulesRepository {
				mockRepo := NewMockActiveRulesRepository(ctrl)
				mockRepo.EXPECT().
					GetActiveRules(gomock.Any(), gomock.Any()).
					Return(nil, nil)
				return mockRepo
			},
			wantErr:       false,
			expectedRules: []*model.Rule{},
		},
		{
			name: "returns error when context is canceled",
			mockSetup: func(ctrl *gomock.Controller) *MockActiveRulesRepository {
				mockRepo := NewMockActiveRulesRepository(ctrl)
				mockRepo.EXPECT().
					GetActiveRules(gomock.Any(), gomock.Any()).
					Return(nil, context.Canceled)
				return mockRepo
			},
			wantErr:       true,
			expectedRules: nil,
		},
	}

	// Tests with non-nil scope filtering
	scopeFilterTests := []struct {
		name          string
		scope         *model.Scope
		mockSetup     func(ctrl *gomock.Controller) *MockActiveRulesRepository
		wantErr       bool
		expectedRules []*model.Rule
	}{
		{
			name: "returns scope-filtered rules when AccountID scope provided",
			scope: &model.Scope{
				AccountID: testutil.UUIDPtr(testAccountID),
			},
			mockSetup: func(ctrl *gomock.Controller) *MockActiveRulesRepository {
				mockRepo := NewMockActiveRulesRepository(ctrl)
				expectedScope := &model.Scope{AccountID: testutil.UUIDPtr(testAccountID)}
				mockRepo.EXPECT().
					GetActiveRules(gomock.Any(), expectedScope).
					Return([]*model.Rule{activeRule1}, nil)
				return mockRepo
			},
			wantErr:       false,
			expectedRules: []*model.Rule{activeRule1},
		},
		{
			name: "returns scope-filtered rules when PortfolioID scope provided",
			scope: &model.Scope{
				PortfolioID: testutil.UUIDPtr(testPortfolioID),
			},
			mockSetup: func(ctrl *gomock.Controller) *MockActiveRulesRepository {
				mockRepo := NewMockActiveRulesRepository(ctrl)
				expectedScope := &model.Scope{PortfolioID: testutil.UUIDPtr(testPortfolioID)}
				mockRepo.EXPECT().
					GetActiveRules(gomock.Any(), expectedScope).
					Return([]*model.Rule{activeRule2}, nil)
				return mockRepo
			},
			wantErr:       false,
			expectedRules: []*model.Rule{activeRule2},
		},
		{
			name: "returns empty list when no rules match scope",
			scope: &model.Scope{
				AccountID: testutil.UUIDPtr(uuid.MustParse("999e8400-e29b-41d4-a716-446655440099")),
			},
			mockSetup: func(ctrl *gomock.Controller) *MockActiveRulesRepository {
				mockRepo := NewMockActiveRulesRepository(ctrl)
				mockRepo.EXPECT().
					GetActiveRules(gomock.Any(), gomock.Any()).
					Return([]*model.Rule{}, nil)
				return mockRepo
			},
			wantErr:       false,
			expectedRules: []*model.Rule{},
		},
		{
			name: "returns error when repository fails with scope",
			scope: &model.Scope{
				AccountID: testutil.UUIDPtr(testAccountID),
			},
			mockSetup: func(ctrl *gomock.Controller) *MockActiveRulesRepository {
				mockRepo := NewMockActiveRulesRepository(ctrl)
				mockRepo.EXPECT().
					GetActiveRules(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("scope filtering query failed"))
				return mockRepo
			},
			wantErr:       true,
			expectedRules: nil,
		},
	}

	for _, tt := range scopeFilterTests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupTestTracing(t)

			ctrl := gomock.NewController(t)

			mockRepo := tt.mockSetup(ctrl)

			query, err := NewGetActiveRulesQuery(mockRepo)
			require.NoError(t, err, "NewGetActiveRulesQuery should not return error with valid repo")

			ctx := context.Background()
			result, err := query.Execute(ctx, tt.scope)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result, "result should never be nil, use empty slice instead")
				require.Len(t, result, len(tt.expectedRules))

				for i, expectedRule := range tt.expectedRules {
					assert.Equal(t, expectedRule.ID, result[i].ID)
					assert.Equal(t, expectedRule.Name, result[i].Name)
					assert.Equal(t, expectedRule.Expression, result[i].Expression)
					assert.Equal(t, expectedRule.Action, result[i].Action)
					assert.Equal(t, model.RuleStatusActive, result[i].Status)
					assert.Equal(t, expectedRule.Scopes, result[i].Scopes)
					assert.Equal(t, expectedRule.CreatedAt, result[i].CreatedAt)
					assert.Equal(t, expectedRule.UpdatedAt, result[i].UpdatedAt)
				}
			}
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupTestTracing(t)

			ctrl := gomock.NewController(t)

			mockRepo := tt.mockSetup(ctrl)

			query, err := NewGetActiveRulesQuery(mockRepo)
			require.NoError(t, err, "NewGetActiveRulesQuery should not return error with valid repo")

			ctx := context.Background()
			result, err := query.Execute(ctx, nil)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				// Result should never be nil - always return empty slice for consistency
				require.NotNil(t, result, "result should never be nil, use empty slice instead")
				require.Len(t, result, len(tt.expectedRules))

				for i, expectedRule := range tt.expectedRules {
					assert.Equal(t, expectedRule.ID, result[i].ID)
					assert.Equal(t, expectedRule.Name, result[i].Name)
					assert.Equal(t, expectedRule.Expression, result[i].Expression)
					assert.Equal(t, expectedRule.Action, result[i].Action)
					assert.Equal(t, model.RuleStatusActive, result[i].Status)
					assert.Equal(t, expectedRule.Scopes, result[i].Scopes)
					assert.Equal(t, expectedRule.CreatedAt, result[i].CreatedAt)
					assert.Equal(t, expectedRule.UpdatedAt, result[i].UpdatedAt)
				}
			}
		})
	}
}

func TestGetActiveRulesQuery_Execute_NilQuery(t *testing.T) {
	t.Run("returns error when query is nil", func(t *testing.T) {
		var query *GetActiveRulesQuery = nil
		ctx := context.Background()
		result, err := query.Execute(ctx, nil)

		require.Error(t, err)
		require.ErrorIs(t, err, ErrNilQuery, "should return sentinel error for nil query")
		assert.Nil(t, result)
	})
}

func TestNewGetActiveRulesQuery_NilRepository(t *testing.T) {
	t.Run("returns error when repository is nil", func(t *testing.T) {
		query, err := NewGetActiveRulesQuery(nil)

		require.Error(t, err)
		require.ErrorIs(t, err, ErrNilActiveRulesRepository, "should return sentinel error for nil repository")
		assert.Nil(t, query)
	})
}
