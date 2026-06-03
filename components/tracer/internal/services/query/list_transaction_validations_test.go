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

	"tracer/internal/services/query/mocks"
	"tracer/internal/testutil"
	"tracer/pkg/constant"
	"tracer/pkg/model"
)

// newTestTransactionValidationList creates a fresh list of TransactionValidation instances for test isolation.
// Each call returns new structs to prevent cross-test contamination.
func newTestTransactionValidationList() []*model.TransactionValidation {
	now := testutil.FixedTime().UTC()
	return []*model.TransactionValidation{
		{
			ID: testutil.MustDeterministicUUID(1),
			EvaluationResult: model.EvaluationResult{
				Decision:         model.DecisionAllow,
				MatchedRuleIDs:   []uuid.UUID{},
				EvaluatedRuleIDs: []uuid.UUID{testutil.MustDeterministicUUID(2)},
				Reason:           "All checks passed",
			},
			LimitUsageDetails: []model.LimitUsageDetail{},
			ProcessingTimeMs:  35,
			CreatedAt:         now.Add(-time.Hour),
		},
		{
			ID: testutil.MustDeterministicUUID(3),
			EvaluationResult: model.EvaluationResult{
				Decision:         model.DecisionDeny,
				MatchedRuleIDs:   []uuid.UUID{testutil.MustDeterministicUUID(4)},
				EvaluatedRuleIDs: []uuid.UUID{testutil.MustDeterministicUUID(5)},
				Reason:           "Blocked by rule",
			},
			LimitUsageDetails: []model.LimitUsageDetail{},
			ProcessingTimeMs:  42,
			CreatedAt:         now.Add(-2 * time.Hour),
		},
	}
}

func TestListTransactionValidationsQuery_Execute(t *testing.T) {
	tests := []struct {
		name      string
		filters   *model.TransactionValidationFilters
		mockSetup func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository
		wantErr   bool
		errIs     error
		validate  func(t *testing.T, result *ListTransactionValidationsResult)
	}{
		{
			name:    "success - returns audits with defaults",
			filters: nil,
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository {
				testAudits := newTestTransactionValidationList()
				mockRepo := mocks.NewMockTransactionValidationRepository(ctrl)
				mockRepo.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, f *model.TransactionValidationFilters) (*model.ListTransactionValidationsResult, error) {
						// Verify defaults were applied
						if f.Limit != model.DefaultTransactionValidationFilterLimit {
							t.Errorf("expected default limit %d, got %d", model.DefaultTransactionValidationFilterLimit, f.Limit)
						}
						if f.SortBy != "created_at" {
							t.Errorf("expected default sortBy 'created_at', got %s", f.SortBy)
						}
						if f.SortOrder != "DESC" {
							t.Errorf("expected default sortOrder 'DESC', got %s", f.SortOrder)
						}
						return &model.ListTransactionValidationsResult{
							TransactionValidations: testAudits,
							NextCursor:             "",
							HasMore:                false,
						}, nil
					})
				return mockRepo
			},
			wantErr: false,
			validate: func(t *testing.T, result *ListTransactionValidationsResult) {
				assert.Len(t, result.TransactionValidations, 2)
				assert.False(t, result.HasMore)
				assert.Empty(t, result.NextCursor)
			},
		},
		{
			name: "success - with explicit filters and cursor",
			filters: &model.TransactionValidationFilters{
				Limit:     50,
				Cursor:    "test-cursor",
				SortBy:    "created_at",
				SortOrder: "ASC",
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository {
				testAudits := newTestTransactionValidationList()
				mockRepo := mocks.NewMockTransactionValidationRepository(ctrl)
				mockRepo.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, f *model.TransactionValidationFilters) (*model.ListTransactionValidationsResult, error) {
						if f.Limit != 50 {
							t.Errorf("expected limit 50, got %d", f.Limit)
						}
						if f.Cursor != "test-cursor" {
							t.Errorf("expected cursor 'test-cursor', got %s", f.Cursor)
						}
						return &model.ListTransactionValidationsResult{
							TransactionValidations: testAudits,
							NextCursor:             "next-cursor",
							HasMore:                true,
						}, nil
					})
				return mockRepo
			},
			wantErr: false,
			validate: func(t *testing.T, result *ListTransactionValidationsResult) {
				assert.Len(t, result.TransactionValidations, 2)
				assert.True(t, result.HasMore)
				assert.Equal(t, "next-cursor", result.NextCursor)
			},
		},
		{
			name: "success - with decision filter",
			filters: &model.TransactionValidationFilters{
				Decision: func() *model.Decision { d := model.DecisionDeny; return &d }(),
				Limit:    100,
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository {
				testAudits := newTestTransactionValidationList()
				mockRepo := mocks.NewMockTransactionValidationRepository(ctrl)
				mockRepo.EXPECT().
					List(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, f *model.TransactionValidationFilters) (*model.ListTransactionValidationsResult, error) {
						if f.Decision == nil || *f.Decision != model.DecisionDeny {
							t.Errorf("expected decision filter DENY, got %v", f.Decision)
						}
						return &model.ListTransactionValidationsResult{
							TransactionValidations: []*model.TransactionValidation{testAudits[1]},
							NextCursor:             "",
							HasMore:                false,
						}, nil
					})
				return mockRepo
			},
			wantErr: false,
			validate: func(t *testing.T, result *ListTransactionValidationsResult) {
				assert.Len(t, result.TransactionValidations, 1)
				assert.False(t, result.HasMore)
			},
		},
		{
			name: "success - empty result",
			filters: &model.TransactionValidationFilters{
				Limit: 100,
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository {
				mockRepo := mocks.NewMockTransactionValidationRepository(ctrl)
				mockRepo.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(&model.ListTransactionValidationsResult{
						TransactionValidations: []*model.TransactionValidation{},
						NextCursor:             "",
						HasMore:                false,
					}, nil)
				return mockRepo
			},
			wantErr: false,
			validate: func(t *testing.T, result *ListTransactionValidationsResult) {
				assert.Len(t, result.TransactionValidations, 0)
				assert.False(t, result.HasMore)
			},
		},
		{
			name: "error - invalid filter (negative limit)",
			filters: &model.TransactionValidationFilters{
				Limit: -1,
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository {
				return mocks.NewMockTransactionValidationRepository(ctrl)
			},
			wantErr: true,
			errIs:   constant.ErrInvalidTransactionValidationFilters,
		},
		{
			name: "error - invalid filter (limit too high)",
			filters: &model.TransactionValidationFilters{
				Limit: model.MaxTransactionValidationFilterLimit + 1,
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository {
				return mocks.NewMockTransactionValidationRepository(ctrl)
			},
			wantErr: true,
			errIs:   constant.ErrInvalidTransactionValidationFilters,
		},
		{
			name: "error - invalid filter (invalid sortBy)",
			filters: &model.TransactionValidationFilters{
				Limit:  100,
				SortBy: "invalid_field",
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository {
				return mocks.NewMockTransactionValidationRepository(ctrl)
			},
			wantErr: true,
			errIs:   constant.ErrInvalidTransactionValidationFilters,
		},
		{
			name: "error - invalid filter (invalid sortOrder)",
			filters: &model.TransactionValidationFilters{
				Limit:     100,
				SortOrder: "INVALID",
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository {
				return mocks.NewMockTransactionValidationRepository(ctrl)
			},
			wantErr: true,
			errIs:   constant.ErrInvalidTransactionValidationFilters,
		},
		{
			name: "error - repository list error",
			filters: &model.TransactionValidationFilters{
				Limit: 100,
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository {
				mockRepo := mocks.NewMockTransactionValidationRepository(ctrl)
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

			query := NewListTransactionValidationsQuery(mockRepo)

			ctx := context.Background()
			result, err := query.Execute(ctx, tt.filters)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.True(t, errors.Is(err, tt.errIs), "expected error %v, got %v", tt.errIs, err)
				}
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

func TestFormatTimeOrNotSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{
			name:     "zero time returns 'not set'",
			input:    time.Time{},
			expected: "not set",
		},
		{
			name:     "non-zero time returns RFC3339 format",
			input:    time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			expected: "2025-01-15T10:30:00Z",
		},
		{
			name:     "time with offset returns RFC3339 format",
			input:    time.Date(2025, 6, 15, 14, 30, 0, 0, time.FixedZone("UTC-5", -5*60*60)),
			expected: "2025-06-15T14:30:00-05:00",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := formatTimeOrNotSet(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestListTransactionValidationsQuery_Execute_ContextCancellation(t *testing.T) {
	tests := []struct {
		name      string
		setupCtx  func() (context.Context, context.CancelFunc)
		mockSetup func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository
		wantErr   bool
	}{
		{
			name: "cancelled context on List returns error",
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx, cancel
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository {
				mockRepo := mocks.NewMockTransactionValidationRepository(ctrl)
				// Repository should NOT be called - context is checked BEFORE repo call
				// No EXPECT() calls since the early context check prevents repo invocation
				return mockRepo
			},
			wantErr: true,
		},
		{
			name: "deadline exceeded context returns error",
			setupCtx: func() (context.Context, context.CancelFunc) {
				// Create context with very short timeout that expires immediately
				ctx, cancel := context.WithTimeout(context.Background(), 0)
				return ctx, cancel
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository {
				mockRepo := mocks.NewMockTransactionValidationRepository(ctrl)
				// Repository should NOT be called - context is checked BEFORE repo call
				// No EXPECT() calls since the early context check prevents repo invocation
				return mockRepo
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			ctx, cancel := tt.setupCtx()
			defer cancel()

			mockRepo := tt.mockSetup(ctrl)
			query := NewListTransactionValidationsQuery(mockRepo)

			// Use valid filters (limit > 0 to pass validation)
			filters := &model.TransactionValidationFilters{Limit: 100}
			result, err := query.Execute(ctx, filters)

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded),
					"expected context cancellation error, got: %v", err)
				assert.Nil(t, result)
			}
		})
	}
}
