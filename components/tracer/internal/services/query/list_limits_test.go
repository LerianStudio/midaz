// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"tracer/internal/testutil"
	"tracer/pkg/constant"
	"tracer/pkg/model"
)

func TestNewListLimitsQuery(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	query, err := NewListLimitsQuery(mockRepo)

	require.NoError(t, err)
	assert.NotNil(t, query)
	assert.Equal(t, mockRepo, query.repo)
}

func TestNewListLimitsQuery_NilRepository(t *testing.T) {
	query, err := NewListLimitsQuery(nil)

	require.ErrorIs(t, err, ErrNilListLimitsRepository)
	assert.Nil(t, query)
}

func TestListLimitsQuery_Execute(t *testing.T) {
	now := testutil.FixedTime().UTC()

	createLimit := func(name string, limitType model.LimitType, status model.LimitStatus) model.Limit {
		return model.Limit{
			ID:        testutil.MustDeterministicUUID(1),
			Name:      name,
			LimitType: limitType,
			MaxAmount: decimal.RequireFromString("1000"),
			Currency:  "USD",
			Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(2))}},
			Status:    status,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	sampleLimits := []model.Limit{
		createLimit("Daily Limit 1", model.LimitTypeDaily, model.LimitStatusActive),
		createLimit("Daily Limit 2", model.LimitTypeDaily, model.LimitStatusActive),
		createLimit("Monthly Limit", model.LimitTypeMonthly, model.LimitStatusInactive),
	}

	// filterMatcher creates a gomock.Cond matcher that verifies filter normalization.
	// After ApplyDefaults(), filters should have: Limit set, SortBy="created_at", SortOrder="desc".
	filterMatcher := func(expectedLimit int, expectedSortBy, expectedSortOrder string, extraChecks ...func(*model.ListLimitsFilter) bool) gomock.Matcher {
		return gomock.Cond(func(x any) bool {
			f, ok := x.(*model.ListLimitsFilter)
			if !ok {
				return false
			}
			if f.Limit != expectedLimit || f.SortBy != expectedSortBy || f.SortOrder != expectedSortOrder {
				return false
			}
			for _, check := range extraChecks {
				if !check(f) {
					return false
				}
			}
			return true
		})
	}

	// defaultFilterMatcher verifies filter has default values applied
	// ApplyDefaults normalizes SortOrder to lowercase
	defaultFilterMatcher := filterMatcher(constant.DefaultPaginationLimit, "created_at", string(constant.Desc))

	tests := []struct {
		name        string
		filter      *model.ListLimitsFilter
		setupMock   func(*MockLimitRepository)
		expectError bool
		errorIs     error
		validate    func(*testing.T, *model.ListLimitsResult)
	}{
		{
			name:   "Success - list all limits with default filter",
			filter: &model.ListLimitsFilter{},
			setupMock: func(m *MockLimitRepository) {
				result := &model.ListLimitsResult{
					Limits:  sampleLimits,
					HasMore: false,
				}
				// Verify defaults are applied: Limit=10, SortBy="created_at", SortOrder="desc"
				m.EXPECT().List(gomock.Any(), defaultFilterMatcher).Return(result, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListLimitsResult) {
				assert.Len(t, result.Limits, 3)
				assert.False(t, result.HasMore)
				assert.Empty(t, result.NextCursor)
			},
		},
		{
			name: "Success - filter by status ACTIVE",
			filter: &model.ListLimitsFilter{
				Status: testutil.Ptr(model.LimitStatusActive),
			},
			setupMock: func(m *MockLimitRepository) {
				activeLimits := []model.Limit{sampleLimits[0], sampleLimits[1]}
				result := &model.ListLimitsResult{
					Limits:  activeLimits,
					HasMore: false,
				}
				// Verify defaults applied AND status filter preserved
				statusCheck := func(f *model.ListLimitsFilter) bool {
					return f.Status != nil && *f.Status == model.LimitStatusActive
				}
				// ApplyDefaults normalizes SortOrder to lowercase
				m.EXPECT().List(gomock.Any(), filterMatcher(constant.DefaultPaginationLimit, "created_at", string(constant.Desc), statusCheck)).Return(result, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListLimitsResult) {
				assert.Len(t, result.Limits, 2)
				for _, limit := range result.Limits {
					assert.Equal(t, model.LimitStatusActive, limit.Status)
				}
			},
		},
		{
			name: "Success - filter by limitType DAILY",
			filter: &model.ListLimitsFilter{
				LimitType: testutil.Ptr(model.LimitTypeDaily),
			},
			setupMock: func(m *MockLimitRepository) {
				dailyLimits := []model.Limit{sampleLimits[0], sampleLimits[1]}
				result := &model.ListLimitsResult{
					Limits:  dailyLimits,
					HasMore: false,
				}
				// Verify defaults applied AND limitType filter preserved
				limitTypeCheck := func(f *model.ListLimitsFilter) bool {
					return f.LimitType != nil && *f.LimitType == model.LimitTypeDaily
				}
				// ApplyDefaults normalizes SortOrder to lowercase
				m.EXPECT().List(gomock.Any(), filterMatcher(constant.DefaultPaginationLimit, "created_at", string(constant.Desc), limitTypeCheck)).Return(result, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListLimitsResult) {
				assert.Len(t, result.Limits, 2)
				for _, limit := range result.Limits {
					assert.Equal(t, model.LimitTypeDaily, limit.LimitType)
				}
			},
		},
		{
			name: "Success - filter by status and limitType",
			filter: &model.ListLimitsFilter{
				Status:    testutil.Ptr(model.LimitStatusActive),
				LimitType: testutil.Ptr(model.LimitTypeDaily),
			},
			setupMock: func(m *MockLimitRepository) {
				filteredLimits := []model.Limit{sampleLimits[0], sampleLimits[1]}
				result := &model.ListLimitsResult{
					Limits:  filteredLimits,
					HasMore: false,
				}
				// Verify both filters preserved with defaults
				combinedCheck := func(f *model.ListLimitsFilter) bool {
					return f.Status != nil && *f.Status == model.LimitStatusActive &&
						f.LimitType != nil && *f.LimitType == model.LimitTypeDaily
				}
				// ApplyDefaults normalizes SortOrder to lowercase
				m.EXPECT().List(gomock.Any(), filterMatcher(constant.DefaultPaginationLimit, "created_at", string(constant.Desc), combinedCheck)).Return(result, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListLimitsResult) {
				assert.Len(t, result.Limits, 2)
			},
		},
		{
			name: "Success - pagination with limit",
			filter: &model.ListLimitsFilter{
				Limit: 2,
			},
			setupMock: func(m *MockLimitRepository) {
				result := &model.ListLimitsResult{
					Limits:     sampleLimits[:2],
					NextCursor: "encoded_cursor_value",
					HasMore:    true,
				}
				// Verify custom limit preserved with default sort
				// ApplyDefaults normalizes SortOrder to lowercase
				m.EXPECT().List(gomock.Any(), filterMatcher(2, "created_at", string(constant.Desc))).Return(result, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListLimitsResult) {
				assert.Len(t, result.Limits, 2)
				assert.True(t, result.HasMore)
				assert.NotEmpty(t, result.NextCursor)
			},
		},
		{
			name: "Success - pagination with cursor",
			filter: &model.ListLimitsFilter{
				Limit:  10,
				Cursor: "encoded_cursor_value",
			},
			setupMock: func(m *MockLimitRepository) {
				result := &model.ListLimitsResult{
					Limits:  sampleLimits[2:],
					HasMore: false,
				}
				// Verify cursor is passed through with defaults
				cursorCheck := func(f *model.ListLimitsFilter) bool {
					return f.Cursor == "encoded_cursor_value"
				}
				// ApplyDefaults normalizes SortOrder to lowercase
				m.EXPECT().List(gomock.Any(), filterMatcher(10, "created_at", string(constant.Desc), cursorCheck)).Return(result, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListLimitsResult) {
				assert.Len(t, result.Limits, 1)
				assert.False(t, result.HasMore)
			},
		},
		{
			name: "Success - sort by name ascending",
			filter: &model.ListLimitsFilter{
				SortBy:    "name",
				SortOrder: string(constant.Asc),
			},
			setupMock: func(m *MockLimitRepository) {
				result := &model.ListLimitsResult{
					Limits:  sampleLimits,
					HasMore: false,
				}
				// Verify custom sort preserved with default limit
				// ApplyDefaults normalizes SortOrder to lowercase
				m.EXPECT().List(gomock.Any(), filterMatcher(constant.DefaultPaginationLimit, "name", string(constant.Asc))).Return(result, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListLimitsResult) {
				assert.Len(t, result.Limits, 3)
			},
		},
		{
			name: "Success - sort by createdAt descending",
			filter: &model.ListLimitsFilter{
				SortBy:    "created_at",
				SortOrder: string(constant.Desc),
			},
			setupMock: func(m *MockLimitRepository) {
				result := &model.ListLimitsResult{
					Limits:  sampleLimits,
					HasMore: false,
				}
				// Verify explicit sort values preserved
				// ApplyDefaults normalizes SortOrder to lowercase
				m.EXPECT().List(gomock.Any(), filterMatcher(constant.DefaultPaginationLimit, "created_at", string(constant.Desc))).Return(result, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListLimitsResult) {
				assert.Len(t, result.Limits, 3)
			},
		},
		{
			name:   "Success - empty result",
			filter: &model.ListLimitsFilter{},
			setupMock: func(m *MockLimitRepository) {
				result := &model.ListLimitsResult{
					Limits:  []model.Limit{},
					HasMore: false,
				}
				// Verify defaults applied even for empty result
				m.EXPECT().List(gomock.Any(), defaultFilterMatcher).Return(result, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListLimitsResult) {
				assert.Empty(t, result.Limits)
				assert.False(t, result.HasMore)
			},
		},
		{
			name: "Failure - invalid status filter",
			filter: &model.ListLimitsFilter{
				Status: testutil.Ptr(model.LimitStatus("INVALID")),
			},
			setupMock:   func(m *MockLimitRepository) {},
			expectError: true,
		},
		{
			name: "Failure - invalid limitType filter",
			filter: &model.ListLimitsFilter{
				LimitType: testutil.Ptr(model.LimitType("INVALID")),
			},
			setupMock:   func(m *MockLimitRepository) {},
			expectError: true,
		},
		{
			name: "Failure - invalid sortBy field",
			filter: &model.ListLimitsFilter{
				SortBy: "invalid_field",
			},
			setupMock:   func(m *MockLimitRepository) {},
			expectError: true,
		},
		{
			name: "Failure - invalid sortOrder",
			filter: &model.ListLimitsFilter{
				SortOrder: "random",
			},
			setupMock:   func(m *MockLimitRepository) {},
			expectError: true,
		},
		{
			name:   "Failure - repository error",
			filter: &model.ListLimitsFilter{},
			setupMock: func(m *MockLimitRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errors.New("database error"))
			},
			expectError: true,
		},
		{
			name: "Failure - invalid cursor",
			filter: &model.ListLimitsFilter{
				Cursor: "invalid_cursor_format",
			},
			setupMock: func(m *MockLimitRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, constant.ErrInvalidCursor)
			},
			expectError: true,
			errorIs:     constant.ErrInvalidCursor,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := NewMockLimitRepository(ctrl)
			tc.setupMock(mockRepo)

			query, err := NewListLimitsQuery(mockRepo)
			require.NoError(t, err)
			result, err := query.Execute(context.Background(), tc.filter)

			if tc.expectError {
				require.Error(t, err)
				if tc.errorIs != nil {
					assert.ErrorIs(t, err, tc.errorIs)
				}
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				if tc.validate != nil {
					tc.validate(t, result)
				}
			}
		})
	}
}

func TestListLimitsQuery_Execute_NilFilter(t *testing.T) {
	ctrl := gomock.NewController(t)

	// filterMatcher verifies nil input gets defaults: Limit=10, SortBy="created_at", SortOrder="desc"
	filterMatcher := gomock.Cond(func(x any) bool {
		f, ok := x.(*model.ListLimitsFilter)
		if !ok {
			return false
		}
		// ApplyDefaults normalizes SortOrder to lowercase
		return f.Limit == constant.DefaultPaginationLimit &&
			f.SortBy == "created_at" &&
			f.SortOrder == string(constant.Desc)
	})

	mockRepo := NewMockLimitRepository(ctrl)
	// Nil filter should apply defaults and call repo with exact default values
	mockRepo.EXPECT().List(gomock.Any(), filterMatcher).Return(&model.ListLimitsResult{
		Limits:  []model.Limit{},
		HasMore: false,
	}, nil)

	query, err := NewListLimitsQuery(mockRepo)
	require.NoError(t, err)
	result, err := query.Execute(context.Background(), nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Limits)
}

func TestListLimitsQuery_Execute_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	// No mock expectation - should short-circuit before repository call
	// gomock will fail if List is unexpectedly called

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	query, err := NewListLimitsQuery(mockRepo)
	require.NoError(t, err)
	result, err := query.Execute(ctx, &model.ListLimitsFilter{})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
}

func TestListLimitsQuery_Execute_LimitDefaults(t *testing.T) {
	// Test that negative limit gets default after ApplyDefaults
	filter := &model.ListLimitsFilter{
		Limit: -10,
	}

	filter.ApplyDefaults()
	err := filter.Validate()
	require.NoError(t, err)
	assert.Equal(t, constant.DefaultPaginationLimit, filter.Limit)

	// Test that limit exceeding max gets capped after ApplyDefaults
	filter = &model.ListLimitsFilter{
		Limit: 200,
	}

	filter.ApplyDefaults()
	err = filter.Validate()
	require.NoError(t, err)
	assert.Equal(t, constant.MaxPaginationLimit, filter.Limit)
}

func TestListLimitsQuery_Execute_SortDefaults(t *testing.T) {
	// Test that empty sort fields get defaults after ApplyDefaults
	filter := &model.ListLimitsFilter{}

	filter.ApplyDefaults()
	err := filter.Validate()
	require.NoError(t, err)
	assert.Equal(t, "created_at", filter.SortBy)
	// ApplyDefaults normalizes SortOrder to lowercase
	assert.Equal(t, string(constant.Desc), filter.SortOrder)
}
