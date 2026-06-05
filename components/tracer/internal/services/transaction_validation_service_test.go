// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

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

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	queryMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func TestNewTransactionValidationService(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	getQuery := query.NewGetTransactionValidationQuery(mockRepo)
	listQuery := query.NewListTransactionValidationsQuery(mockRepo)

	service, err := NewTransactionValidationService(getQuery, listQuery)
	require.NoError(t, err)

	assert.NotNil(t, service)
	assert.Equal(t, getQuery, service.getQuery)
	assert.Equal(t, listQuery, service.listQuery)
}

func TestNewTransactionValidationService_NilDependencies(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

	validGetQuery := query.NewGetTransactionValidationQuery(mockRepo)
	require.NotNil(t, validGetQuery, "NewGetTransactionValidationQuery should not return nil in test setup")

	validListQuery := query.NewListTransactionValidationsQuery(mockRepo)
	require.NotNil(t, validListQuery, "NewListTransactionValidationsQuery should not return nil in test setup")

	tests := []struct {
		name        string
		getQuery    *query.GetTransactionValidationQuery
		listQuery   *query.ListTransactionValidationsQuery
		expectError error
	}{
		{
			name:        "nil getQuery returns error",
			getQuery:    nil,
			listQuery:   validListQuery,
			expectError: ErrNilGetQuery,
		},
		{
			name:        "nil listQuery returns error",
			getQuery:    validGetQuery,
			listQuery:   nil,
			expectError: ErrNilListQuery,
		},
		{
			name:        "all nil returns error for getQuery first",
			getQuery:    nil,
			listQuery:   nil,
			expectError: ErrNilGetQuery,
		},
		{
			name:        "all valid dependencies succeeds",
			getQuery:    validGetQuery,
			listQuery:   validListQuery,
			expectError: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, err := NewTransactionValidationService(tc.getQuery, tc.listQuery)

			if tc.expectError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tc.expectError)
				assert.Nil(t, service)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, service)
			}
		})
	}
}

func TestTransactionValidationService_GetTransactionValidation(t *testing.T) {
	fixedTime := testutil.FixedTime()
	validationID := testutil.MustDeterministicUUID(1)
	requestID := testutil.MustDeterministicUUID(2)
	accountID := testutil.MustDeterministicUUID(3)

	tests := []struct {
		name          string
		validationID  string
		setupMock     func(*queryMocks.MockTransactionValidationRepository)
		expectError   bool
		expectedErr   error
		cancelContext bool
		validate      func(*testing.T, *model.TransactionValidation)
	}{
		{
			name:         "Success - get existing transaction validation",
			validationID: validationID.String(),
			setupMock: func(m *queryMocks.MockTransactionValidationRepository) {
				m.EXPECT().GetByID(gomock.Any(), validationID).Return(&model.TransactionValidation{
					ID:                   validationID,
					RequestID:            requestID,
					TransactionType:      model.TransactionTypeCard,
					Amount:               decimal.RequireFromString("100"),
					Currency:             "USD",
					TransactionTimestamp: fixedTime,
					Account:              model.AccountContext{ID: accountID},
					EvaluationResult: model.EvaluationResult{
						Decision: model.DecisionAllow,
						Reason:   "Transaction allowed",
					},
					ProcessingTimeMs: 45,
					CreatedAt:        fixedTime,
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.TransactionValidation) {
				assert.Equal(t, validationID, result.ID)
				assert.Equal(t, requestID, result.RequestID)
				assert.Equal(t, model.TransactionTypeCard, result.TransactionType)
				assert.Equal(t, decimal.RequireFromString("100").String(), result.Amount.String())
				assert.Equal(t, "USD", result.Currency)
				assert.Equal(t, model.DecisionAllow, result.EvaluationResult.Decision)
				assert.Equal(t, "Transaction allowed", result.EvaluationResult.Reason)
			},
		},
		{
			name:         "Error - transaction validation not found",
			validationID: testutil.MustDeterministicUUID(999).String(),
			setupMock: func(m *queryMocks.MockTransactionValidationRepository) {
				m.EXPECT().GetByID(gomock.Any(), gomock.Any()).Return(nil, constant.ErrTransactionValidationNotFound)
			},
			expectError: true,
			expectedErr: constant.ErrTransactionValidationNotFound,
		},
		{
			name:         "Error - database error",
			validationID: validationID.String(),
			setupMock: func(m *queryMocks.MockTransactionValidationRepository) {
				m.EXPECT().GetByID(gomock.Any(), validationID).Return(nil, errors.New("database connection failed"))
			},
			expectError: true,
		},
		{
			name:         "Error - context canceled before query",
			validationID: validationID.String(),
			setupMock: func(m *queryMocks.MockTransactionValidationRepository) {
				// No call expected when context is canceled
			},
			expectError:   true,
			expectedErr:   context.Canceled,
			cancelContext: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
			tc.setupMock(mockRepo)

			getQuery := query.NewGetTransactionValidationQuery(mockRepo)
			listQuery := query.NewListTransactionValidationsQuery(mockRepo)

			service, err := NewTransactionValidationService(getQuery, listQuery)
			require.NoError(t, err)

			ctx := context.Background()
			if tc.cancelContext {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel() // Cancel immediately
			}

			id := testutil.MustDeterministicUUID(1)
			if tc.validationID != "" {
				parsedID, parseErr := uuid.Parse(tc.validationID)
				if parseErr != nil {
					id = testutil.MustDeterministicUUID(999)
				} else {
					id = parsedID
				}
			}

			result, err := service.GetTransactionValidation(ctx, id)

			if tc.expectError {
				require.Error(t, err)
				if tc.expectedErr != nil {
					assert.ErrorIs(t, err, tc.expectedErr)
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

func TestTransactionValidationService_ListTransactionValidations(t *testing.T) {
	fixedTime := testutil.FixedTime()
	validationID1 := testutil.MustDeterministicUUID(1)
	validationID2 := testutil.MustDeterministicUUID(2)
	requestID1 := testutil.MustDeterministicUUID(10)
	requestID2 := testutil.MustDeterministicUUID(11)
	accountID := testutil.MustDeterministicUUID(20)

	tests := []struct {
		name          string
		filters       *model.TransactionValidationFilters
		setupMock     func(*queryMocks.MockTransactionValidationRepository)
		expectError   bool
		expectedErr   error
		cancelContext bool
		validate      func(*testing.T, *model.ListTransactionValidationsResult)
	}{
		{
			name:    "Success - list all validations with default filters",
			filters: nil,
			setupMock: func(m *queryMocks.MockTransactionValidationRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(&model.ListTransactionValidationsResult{
					TransactionValidations: []*model.TransactionValidation{
						{
							ID:                   validationID1,
							RequestID:            requestID1,
							TransactionType:      model.TransactionTypeCard,
							Amount:               decimal.RequireFromString("100"),
							Currency:             "USD",
							TransactionTimestamp: fixedTime,
							Account:              model.AccountContext{ID: accountID},
							EvaluationResult: model.EvaluationResult{
								Decision: model.DecisionAllow,
							},
							ProcessingTimeMs: 45,
							CreatedAt:        fixedTime,
						},
						{
							ID:                   validationID2,
							RequestID:            requestID2,
							TransactionType:      model.TransactionTypePix,
							Amount:               decimal.RequireFromString("50"),
							Currency:             "BRL",
							TransactionTimestamp: fixedTime.Add(time.Hour),
							Account:              model.AccountContext{ID: accountID},
							EvaluationResult: model.EvaluationResult{
								Decision: model.DecisionDeny,
								Reason:   "limit_exceeded",
							},
							ProcessingTimeMs: 30,
							CreatedAt:        fixedTime.Add(time.Hour),
						},
					},
					HasMore:    false,
					NextCursor: "",
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListTransactionValidationsResult) {
				require.Len(t, result.TransactionValidations, 2)
				assert.Equal(t, validationID1, result.TransactionValidations[0].ID)
				assert.Equal(t, validationID2, result.TransactionValidations[1].ID)
				assert.False(t, result.HasMore)
			},
		},
		{
			name: "Success - list with decision filter",
			filters: &model.TransactionValidationFilters{
				Decision: testutil.Ptr(model.DecisionAllow),
			},
			setupMock: func(m *queryMocks.MockTransactionValidationRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(&model.ListTransactionValidationsResult{
					TransactionValidations: []*model.TransactionValidation{
						{
							ID:                   validationID1,
							RequestID:            requestID1,
							TransactionType:      model.TransactionTypeCard,
							Amount:               decimal.RequireFromString("100"),
							Currency:             "USD",
							TransactionTimestamp: fixedTime,
							Account:              model.AccountContext{ID: accountID},
							EvaluationResult: model.EvaluationResult{
								Decision: model.DecisionAllow,
							},
							ProcessingTimeMs: 45,
							CreatedAt:        fixedTime,
						},
					},
					HasMore:    false,
					NextCursor: "",
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListTransactionValidationsResult) {
				require.Len(t, result.TransactionValidations, 1)
				assert.Equal(t, model.DecisionAllow, result.TransactionValidations[0].EvaluationResult.Decision)
			},
		},
		{
			name: "Success - paginated results",
			filters: &model.TransactionValidationFilters{
				Limit: 10,
			},
			setupMock: func(m *queryMocks.MockTransactionValidationRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(&model.ListTransactionValidationsResult{
					TransactionValidations: []*model.TransactionValidation{
						{
							ID: validationID1,
							EvaluationResult: model.EvaluationResult{
								Decision: model.DecisionAllow,
							},
							CreatedAt: fixedTime,
						},
					},
					HasMore:    true,
					NextCursor: "next-page-cursor",
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListTransactionValidationsResult) {
				assert.True(t, result.HasMore)
				assert.Equal(t, "next-page-cursor", result.NextCursor)
			},
		},
		{
			name:    "Success - empty results",
			filters: &model.TransactionValidationFilters{},
			setupMock: func(m *queryMocks.MockTransactionValidationRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(&model.ListTransactionValidationsResult{
					TransactionValidations: []*model.TransactionValidation{},
					HasMore:                false,
					NextCursor:             "",
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListTransactionValidationsResult) {
				assert.Len(t, result.TransactionValidations, 0)
				assert.False(t, result.HasMore)
			},
		},
		{
			name:    "Error - database error",
			filters: &model.TransactionValidationFilters{},
			setupMock: func(m *queryMocks.MockTransactionValidationRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errors.New("database connection failed"))
			},
			expectError: true,
		},
		{
			name:    "Error - context canceled before query",
			filters: &model.TransactionValidationFilters{},
			setupMock: func(m *queryMocks.MockTransactionValidationRepository) {
				// No call expected when context is canceled
			},
			expectError:   true,
			expectedErr:   context.Canceled,
			cancelContext: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
			tc.setupMock(mockRepo)

			getQuery := query.NewGetTransactionValidationQuery(mockRepo)
			listQuery := query.NewListTransactionValidationsQuery(mockRepo)

			service, err := NewTransactionValidationService(getQuery, listQuery)
			require.NoError(t, err)

			ctx := context.Background()
			if tc.cancelContext {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel() // Cancel immediately
			}

			result, err := service.ListTransactionValidations(ctx, tc.filters)

			if tc.expectError {
				require.Error(t, err)
				if tc.expectedErr != nil {
					assert.ErrorIs(t, err, tc.expectedErr)
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
