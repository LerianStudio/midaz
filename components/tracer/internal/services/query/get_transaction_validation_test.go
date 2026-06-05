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
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// newTestTransactionValidation creates a fresh TransactionValidation instance for test isolation.
// Each call returns a new struct to prevent cross-test contamination.
func newTestTransactionValidation(id uuid.UUID) *model.TransactionValidation {
	return &model.TransactionValidation{
		ID:                   id,
		RequestID:            testutil.MustDeterministicUUID(100),
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: testutil.FixedTime().Add(-time.Hour),
		Account: model.AccountContext{
			ID:     testutil.MustDeterministicUUID(101),
			Type:   "checking",
			Status: "active",
		},
		EvaluationResult: model.EvaluationResult{
			Decision:         model.DecisionAllow,
			MatchedRuleIDs:   []uuid.UUID{},
			EvaluatedRuleIDs: []uuid.UUID{testutil.MustDeterministicUUID(102)},
			Reason:           "All checks passed",
		},
		LimitUsageDetails: []model.LimitUsageDetail{},
		ProcessingTimeMs:  42,
		CreatedAt:         testutil.FixedTime().Add(-time.Hour),
	}
}

func TestGetTransactionValidationQuery_Execute(t *testing.T) {
	tvID := testutil.MustDeterministicUUID(10)

	tests := []struct {
		name      string
		tvID      uuid.UUID
		mockSetup func(ctrl *gomock.Controller, audit *model.TransactionValidation) *mocks.MockTransactionValidationRepository
		wantErr   bool
		errIs     error
	}{
		{
			name: "success - returns audit",
			tvID: tvID,
			mockSetup: func(ctrl *gomock.Controller, audit *model.TransactionValidation) *mocks.MockTransactionValidationRepository {
				mockRepo := mocks.NewMockTransactionValidationRepository(ctrl)
				mockRepo.EXPECT().
					GetByID(gomock.Any(), audit.ID).
					Return(audit, nil)
				return mockRepo
			},
			wantErr: false,
		},
		{
			name: "error - audit not found",
			tvID: tvID,
			mockSetup: func(ctrl *gomock.Controller, audit *model.TransactionValidation) *mocks.MockTransactionValidationRepository {
				mockRepo := mocks.NewMockTransactionValidationRepository(ctrl)
				mockRepo.EXPECT().
					GetByID(gomock.Any(), audit.ID).
					Return(nil, constant.ErrTransactionValidationNotFound)
				return mockRepo
			},
			wantErr: true,
			errIs:   constant.ErrTransactionValidationNotFound,
		},
		{
			name: "error - repository error",
			tvID: tvID,
			mockSetup: func(ctrl *gomock.Controller, audit *model.TransactionValidation) *mocks.MockTransactionValidationRepository {
				mockRepo := mocks.NewMockTransactionValidationRepository(ctrl)
				mockRepo.EXPECT().
					GetByID(gomock.Any(), audit.ID).
					Return(nil, errors.New("database error"))
				return mockRepo
			},
			wantErr: true,
		},
		{
			name: "error - nil UUID returns invalid path parameter",
			tvID: uuid.Nil,
			mockSetup: func(ctrl *gomock.Controller, _ *model.TransactionValidation) *mocks.MockTransactionValidationRepository {
				// Repository is not called - validation fails before reaching repo
				return mocks.NewMockTransactionValidationRepository(ctrl)
			},
			wantErr: true,
			errIs:   constant.ErrInvalidPathParameter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			// Create fresh audit for each test case
			testTV := newTestTransactionValidation(tt.tvID)
			mockRepo := tt.mockSetup(ctrl, testTV)

			query := NewGetTransactionValidationQuery(mockRepo)

			ctx := context.Background()
			result, err := query.Execute(ctx, tt.tvID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.True(t, errors.Is(err, tt.errIs), "expected error %v, got %v", tt.errIs, err)
				}
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.tvID, result.ID)
				assert.Equal(t, testTV.EvaluationResult.Decision, result.EvaluationResult.Decision)
				assert.Equal(t, testTV.EvaluationResult.Reason, result.EvaluationResult.Reason)
				assert.Equal(t, testTV.ProcessingTimeMs, result.ProcessingTimeMs)
			}
		})
	}
}

func TestGetTransactionValidationQuery_Execute_ContextCancellation(t *testing.T) {
	tests := []struct {
		name      string
		setupCtx  func() (context.Context, context.CancelFunc)
		mockSetup func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository
		wantErr   bool
	}{
		{
			name: "cancelled context returns error",
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx, cancel
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationRepository {
				mockRepo := mocks.NewMockTransactionValidationRepository(ctrl)
				// Repository should receive cancelled context and return context error
				mockRepo.EXPECT().
					GetByID(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, id uuid.UUID) (*model.TransactionValidation, error) {
						return nil, ctx.Err()
					})
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
				// Repository should receive expired context and return context error
				mockRepo.EXPECT().
					GetByID(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, id uuid.UUID) (*model.TransactionValidation, error) {
						return nil, ctx.Err()
					})
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
			query := NewGetTransactionValidationQuery(mockRepo)

			result, err := query.Execute(ctx, testutil.MustDeterministicUUID(999))

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded),
					"expected context cancellation error, got: %v", err)
				assert.Nil(t, result)
			}
		})
	}
}
