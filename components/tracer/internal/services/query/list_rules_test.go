// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func TestListRulesQuery_Execute(t *testing.T) {
	rules := []model.Rule{
		{
			ID:         testutil.MustDeterministicUUID(1),
			Name:       "rule 1",
			Expression: "amount > 1000",
			Action:     model.DecisionDeny,
			Status:     model.RuleStatusActive,
			CreatedAt:  testutil.FixedTime(),
			UpdatedAt:  testutil.FixedTime(),
		},
		{
			ID:         testutil.MustDeterministicUUID(2),
			Name:       "rule 2",
			Expression: "amount > 5000",
			Action:     model.DecisionReview,
			Status:     model.RuleStatusActive,
			CreatedAt:  testutil.FixedTime(),
			UpdatedAt:  testutil.FixedTime(),
		},
	}

	tests := []struct {
		name      string
		filter    *model.ListRulesFilter
		mockSetup func(ctrl *gomock.Controller) *MockListRulesRepository
		wantErr   bool
		validate  func(t *testing.T, result *model.ListRulesResult)
	}{
		{
			name:   "success - returns rules with defaults",
			filter: &model.ListRulesFilter{},
			mockSetup: func(ctrl *gomock.Controller) *MockListRulesRepository {
				mockRepo := NewMockListRulesRepository(ctrl)
				mockRepo.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, f *model.ListRulesFilter) (*model.ListRulesResult, error) {
						// SortBy uses snake_case
						if f.Limit != 10 || f.SortBy != "created_at" || f.SortOrder != "DESC" {
							t.Errorf("unexpected filter defaults: %+v", f)
						}
						return &model.ListRulesResult{
							Rules:      rules,
							NextCursor: "eyJpZCI6InJ1bGUtMiIsInBvaW50c05leHQiOnRydWV9",
							HasMore:    false,
						}, nil
					})
				return mockRepo
			},
			wantErr: false,
			validate: func(t *testing.T, result *model.ListRulesResult) {
				assert.Len(t, result.Rules, 2)
				assert.False(t, result.HasMore)
			},
		},
		{
			name: "success - with status filter",
			filter: &model.ListRulesFilter{
				Status: testutil.Ptr(model.RuleStatusActive),
				Limit:  20,
			},
			mockSetup: func(ctrl *gomock.Controller) *MockListRulesRepository {
				mockRepo := NewMockListRulesRepository(ctrl)
				mockRepo.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, f *model.ListRulesFilter) (*model.ListRulesResult, error) {
						if f.Status == nil || *f.Status != model.RuleStatusActive || f.Limit != 20 {
							t.Errorf("unexpected filter: %+v", f)
						}
						return &model.ListRulesResult{
							Rules:   rules,
							HasMore: false,
						}, nil
					})
				return mockRepo
			},
			wantErr: false,
			validate: func(t *testing.T, result *model.ListRulesResult) {
				assert.Len(t, result.Rules, 2)
			},
		},
		{
			name: "success - with action filter",
			filter: &model.ListRulesFilter{
				Action: testutil.Ptr(model.DecisionDeny),
				Limit:  10,
			},
			mockSetup: func(ctrl *gomock.Controller) *MockListRulesRepository {
				mockRepo := NewMockListRulesRepository(ctrl)
				mockRepo.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, f *model.ListRulesFilter) (*model.ListRulesResult, error) {
						if f.Action == nil || *f.Action != model.DecisionDeny {
							t.Errorf("unexpected action filter: %+v", f)
						}
						return &model.ListRulesResult{
							Rules:   []model.Rule{rules[0]},
							HasMore: false,
						}, nil
					})
				return mockRepo
			},
			wantErr: false,
			validate: func(t *testing.T, result *model.ListRulesResult) {
				assert.Len(t, result.Rules, 1)
			},
		},
		{
			name: "success - with sorting by name ASC",
			filter: &model.ListRulesFilter{
				Limit:     10,
				SortBy:    "name",
				SortOrder: "asc",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockListRulesRepository {
				mockRepo := NewMockListRulesRepository(ctrl)
				mockRepo.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, f *model.ListRulesFilter) (*model.ListRulesResult, error) {
						if f.SortBy != "name" || f.SortOrder != "ASC" {
							t.Errorf("unexpected sorting: sortBy=%s, sortOrder=%s", f.SortBy, f.SortOrder)
						}
						return &model.ListRulesResult{
							Rules:   rules,
							HasMore: false,
						}, nil
					})
				return mockRepo
			},
			wantErr: false,
			validate: func(t *testing.T, result *model.ListRulesResult) {
				assert.Len(t, result.Rules, 2)
			},
		},
		{
			name: "success - with cursor for next page",
			filter: &model.ListRulesFilter{
				Limit:  10,
				Cursor: "eyJpZCI6InJ1bGUtMSIsInBvaW50c05leHQiOnRydWV9",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockListRulesRepository {
				mockRepo := NewMockListRulesRepository(ctrl)
				mockRepo.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, f *model.ListRulesFilter) (*model.ListRulesResult, error) {
						if f.Cursor != "eyJpZCI6InJ1bGUtMSIsInBvaW50c05leHQiOnRydWV9" || f.Limit != 10 {
							t.Errorf("unexpected filter: %+v", f)
						}
						return &model.ListRulesResult{
							Rules:      []model.Rule{rules[1]},
							NextCursor: "",
							HasMore:    false,
						}, nil
					})
				return mockRepo
			},
			wantErr: false,
			validate: func(t *testing.T, result *model.ListRulesResult) {
				assert.Len(t, result.Rules, 1)
				assert.False(t, result.HasMore)
				assert.Empty(t, result.NextCursor)
			},
		},
		{
			name: "success - has more results",
			filter: &model.ListRulesFilter{
				Limit: 1,
			},
			mockSetup: func(ctrl *gomock.Controller) *MockListRulesRepository {
				mockRepo := NewMockListRulesRepository(ctrl)
				mockRepo.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, f *model.ListRulesFilter) (*model.ListRulesResult, error) {
						if f.Limit != 1 {
							t.Errorf("unexpected limit: %d", f.Limit)
						}
						return &model.ListRulesResult{
							Rules:      []model.Rule{rules[0]},
							NextCursor: "eyJpZCI6InJ1bGUtMSIsInBvaW50c05leHQiOnRydWV9",
							HasMore:    true,
						}, nil
					})
				return mockRepo
			},
			wantErr: false,
			validate: func(t *testing.T, result *model.ListRulesResult) {
				assert.Len(t, result.Rules, 1)
				assert.True(t, result.HasMore)
				assert.NotEmpty(t, result.NextCursor)
			},
		},
		{
			name: "success - filter by DELETED status",
			filter: &model.ListRulesFilter{
				Status: testutil.Ptr(model.RuleStatusDeleted),
				Limit:  10,
			},
			mockSetup: func(ctrl *gomock.Controller) *MockListRulesRepository {
				mockRepo := NewMockListRulesRepository(ctrl)
				mockRepo.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, f *model.ListRulesFilter) (*model.ListRulesResult, error) {
						if f.Status == nil || *f.Status != model.RuleStatusDeleted {
							t.Errorf("unexpected status filter: %+v", f)
						}
						return &model.ListRulesResult{
							Rules:   []model.Rule{},
							HasMore: false,
						}, nil
					})
				return mockRepo
			},
			wantErr: false,
			validate: func(t *testing.T, result *model.ListRulesResult) {
				assert.Len(t, result.Rules, 0)
				assert.False(t, result.HasMore)
			},
		},
		{
			name: "error - repository error",
			filter: &model.ListRulesFilter{
				Limit: 10,
			},
			mockSetup: func(ctrl *gomock.Controller) *MockListRulesRepository {
				mockRepo := NewMockListRulesRepository(ctrl)
				mockRepo.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))
				return mockRepo
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := tt.mockSetup(ctrl)

			query := NewListRulesQuery(mockRepo)

			ctx := context.Background()
			result, err := query.Execute(ctx, tt.filter)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
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
