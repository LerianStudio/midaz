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

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestGetRuleQuery_Execute(t *testing.T) {
	ruleID := testutil.MustDeterministicUUID(1)
	testAccountID := testutil.MustDeterministicUUID(2)
	existingRule := &model.Rule{
		ID:         ruleID,
		Name:       "test rule",
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes: []model.Scope{
			{AccountID: testutil.UUIDPtr(testAccountID)},
		},
		CreatedAt: testutil.FixedTime().Add(-time.Hour),
		UpdatedAt: testutil.FixedTime(),
	}

	tests := []struct {
		name      string
		ruleID    uuid.UUID
		mockSetup func(ctrl *gomock.Controller) *MockRuleRepository
		wantErr   bool
		errIs     error
	}{
		{
			name:   "success - returns rule",
			ruleID: ruleID,
			mockSetup: func(ctrl *gomock.Controller) *MockRuleRepository {
				mockRepo := NewMockRuleRepository(ctrl)
				mockRepo.EXPECT().
					GetByID(gomock.Any(), ruleID).
					Return(existingRule, nil)
				return mockRepo
			},
			wantErr: false,
		},
		{
			name:   "error - rule not found",
			ruleID: ruleID,
			mockSetup: func(ctrl *gomock.Controller) *MockRuleRepository {
				mockRepo := NewMockRuleRepository(ctrl)
				mockRepo.EXPECT().
					GetByID(gomock.Any(), ruleID).
					Return(nil, constant.ErrRuleNotFound)
				return mockRepo
			},
			wantErr: true,
			errIs:   constant.ErrRuleNotFound,
		},
		{
			name:   "error - repository error",
			ruleID: ruleID,
			mockSetup: func(ctrl *gomock.Controller) *MockRuleRepository {
				mockRepo := NewMockRuleRepository(ctrl)
				mockRepo.EXPECT().
					GetByID(gomock.Any(), ruleID).
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

			query := NewGetRuleQuery(mockRepo)

			ctx := context.Background()
			result, err := query.Execute(ctx, tt.ruleID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.True(t, errors.Is(err, tt.errIs), "expected error %v, got %v", tt.errIs, err)
				}
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.ruleID, result.ID)
				assert.Equal(t, existingRule.Name, result.Name)
				assert.Equal(t, existingRule.Expression, result.Expression)
				assert.Equal(t, existingRule.Action, result.Action)
				assert.Equal(t, existingRule.Status, result.Status)
			}
		})
	}
}
