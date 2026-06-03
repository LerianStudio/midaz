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

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

func TestNewGetLimitQuery(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	query := NewGetLimitQuery(mockRepo)

	assert.NotNil(t, query)
}

func TestGetLimitQuery_Execute(t *testing.T) {
	limitID := testutil.MustDeterministicUUID(1)
	now := testutil.FixedTime().UTC()

	existingLimit := &model.Limit{
		ID:          limitID,
		Name:        "Test Limit",
		Description: testutil.StringPtr("A test limit description"),
		LimitType:   model.LimitTypeDaily,
		MaxAmount:   decimal.RequireFromString("1000"),
		Currency:    "USD",
		Scopes:      []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(2))}},
		Status:      model.LimitStatusActive,
		ResetAt:     testutil.Ptr(now.Add(24 * time.Hour)),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	tests := []struct {
		name        string
		limitID     uuid.UUID
		setupMock   func(*MockLimitRepository)
		expectError bool
		errorIs     error
		validate    func(*testing.T, *model.Limit)
	}{
		{
			name:    "Success - get existing limit",
			limitID: limitID,
			setupMock: func(m *MockLimitRepository) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(existingLimit, nil)
			},
			expectError: false,
			validate: func(t *testing.T, limit *model.Limit) {
				assert.Equal(t, limitID, limit.ID)
				assert.Equal(t, "Test Limit", limit.Name)
				assert.Equal(t, model.LimitTypeDaily, limit.LimitType)
				assert.True(t, decimal.RequireFromString("1000").Equal(limit.MaxAmount))
				assert.Equal(t, "USD", limit.Currency)
				assert.Equal(t, model.LimitStatusActive, limit.Status)
				assert.NotNil(t, limit.Description)
				assert.NotNil(t, limit.ResetAt)
			},
		},
		{
			name:    "Success - get inactive limit",
			limitID: limitID,
			setupMock: func(m *MockLimitRepository) {
				inactiveLimit := &model.Limit{
					ID:        limitID,
					Name:      "Inactive Limit",
					LimitType: model.LimitTypeMonthly,
					MaxAmount: decimal.RequireFromString("5000"),
					Currency:  "BRL",
					Scopes:    []model.Scope{{PortfolioID: testutil.UUIDPtr(testutil.MustDeterministicUUID(10))}},
					Status:    model.LimitStatusInactive,
					CreatedAt: now,
					UpdatedAt: now,
				}
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(inactiveLimit, nil)
			},
			expectError: false,
			validate: func(t *testing.T, limit *model.Limit) {
				assert.Equal(t, model.LimitStatusInactive, limit.Status)
			},
		},
		{
			name:    "Success - get per-transaction limit (no resetAt)",
			limitID: limitID,
			setupMock: func(m *MockLimitRepository) {
				perTxLimit := &model.Limit{
					ID:        limitID,
					Name:      "Per Transaction Limit",
					LimitType: model.LimitTypePerTransaction,
					MaxAmount: decimal.RequireFromString("100"),
					Currency:  "EUR",
					Scopes:    []model.Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(11))}},
					Status:    model.LimitStatusActive,
					ResetAt:   nil,
					CreatedAt: now,
					UpdatedAt: now,
				}
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(perTxLimit, nil)
			},
			expectError: false,
			validate: func(t *testing.T, limit *model.Limit) {
				assert.Equal(t, model.LimitTypePerTransaction, limit.LimitType)
				assert.Nil(t, limit.ResetAt)
			},
		},
		{
			name:    "Failure - limit not found",
			limitID: testutil.MustDeterministicUUID(20),
			setupMock: func(m *MockLimitRepository) {
				m.EXPECT().GetByID(gomock.Any(), gomock.Any()).Return(nil, constant.ErrLimitNotFound)
			},
			expectError: true,
			errorIs:     constant.ErrLimitNotFound,
		},
		{
			name:    "Failure - repository error",
			limitID: limitID,
			setupMock: func(m *MockLimitRepository) {
				m.EXPECT().GetByID(gomock.Any(), limitID).Return(nil, errors.New("database error"))
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := NewMockLimitRepository(ctrl)
			tc.setupMock(mockRepo)

			query := NewGetLimitQuery(mockRepo)
			result, err := query.Execute(context.Background(), tc.limitID)

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

func TestGetLimitQuery_Execute_NilUUID(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := NewMockLimitRepository(ctrl)
	// No mock setup needed - nil UUID is validated before repo call

	query := NewGetLimitQuery(mockRepo)
	result, err := query.Execute(context.Background(), uuid.Nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrLimitInvalidID)
	assert.Nil(t, result)
}

func TestGetLimitQuery_Execute_ContextCancellation(t *testing.T) {
	// This test verifies that context cancellation errors from the repository are properly
	// propagated by the query layer. The query itself does not check ctx.Err() before
	// calling the repository - it relies on the repository to respect context cancellation.
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(30)
	mockRepo := NewMockLimitRepository(ctrl)
	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(nil, context.Canceled)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	query := NewGetLimitQuery(mockRepo)
	result, err := query.Execute(ctx, limitID)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
}

func TestGetLimitQuery_Execute_DeletedLimit(t *testing.T) {
	// Contract: LimitRepository.GetByID filters out deleted limits at the database level
	// (WHERE deleted_at IS NULL). Deleted limits return ErrLimitNotFound, not the limit itself.
	// This test verifies the query layer correctly propagates that error.
	ctrl := gomock.NewController(t)

	limitID := testutil.MustDeterministicUUID(30)

	mockRepo := NewMockLimitRepository(ctrl)
	// Repository returns ErrLimitNotFound for deleted limits (filtered by WHERE deleted_at IS NULL)
	mockRepo.EXPECT().GetByID(gomock.Any(), limitID).Return(nil, constant.ErrLimitNotFound)

	query := NewGetLimitQuery(mockRepo)
	result, err := query.Execute(context.Background(), limitID)

	require.Error(t, err)
	assert.ErrorIs(t, err, constant.ErrLimitNotFound)
	assert.Nil(t, result)
}
