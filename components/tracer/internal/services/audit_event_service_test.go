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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestNewAuditEventService(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := query.NewMockAuditEventRepository(ctrl)
	getQuery, err := query.NewGetAuditEventQuery(mockRepo)
	require.NoError(t, err)
	listQuery, err := query.NewListAuditEventsQuery(mockRepo)
	require.NoError(t, err)
	verifyQuery, err := query.NewVerifyAuditEventQuery(mockRepo)
	require.NoError(t, err)

	service, err := NewAuditEventService(getQuery, listQuery, verifyQuery)
	require.NoError(t, err)

	assert.NotNil(t, service)
	assert.Equal(t, getQuery, service.getQuery)
	assert.Equal(t, listQuery, service.listQuery)
	assert.Equal(t, verifyQuery, service.verifyQuery)
}

func TestNewAuditEventService_NilDependencies(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRepo := query.NewMockAuditEventRepository(ctrl)

	validGetQuery, err := query.NewGetAuditEventQuery(mockRepo)
	require.NoError(t, err, "NewGetAuditEventQuery should not fail in test setup")

	validListQuery, err := query.NewListAuditEventsQuery(mockRepo)
	require.NoError(t, err, "NewListAuditEventsQuery should not fail in test setup")

	validVerifyQuery, err := query.NewVerifyAuditEventQuery(mockRepo)
	require.NoError(t, err, "NewVerifyAuditEventQuery should not fail in test setup")

	tests := []struct {
		name        string
		getQuery    *query.GetAuditEventQuery
		listQuery   *query.ListAuditEventsQuery
		verifyQuery *query.VerifyAuditEventQuery
		expectError bool
		errContains string
	}{
		{
			name:        "nil getQuery returns error",
			getQuery:    nil,
			listQuery:   validListQuery,
			verifyQuery: validVerifyQuery,
			expectError: true,
			errContains: "getQuery cannot be nil",
		},
		{
			name:        "nil listQuery returns error",
			getQuery:    validGetQuery,
			listQuery:   nil,
			verifyQuery: validVerifyQuery,
			expectError: true,
			errContains: "listQuery cannot be nil",
		},
		{
			name:        "nil verifyQuery returns error",
			getQuery:    validGetQuery,
			listQuery:   validListQuery,
			verifyQuery: nil,
			expectError: true,
			errContains: "verifyQuery cannot be nil",
		},
		{
			name:        "all nil returns error for getQuery first",
			getQuery:    nil,
			listQuery:   nil,
			verifyQuery: nil,
			expectError: true,
			errContains: "getQuery cannot be nil",
		},
		{
			name:        "all valid dependencies succeeds",
			getQuery:    validGetQuery,
			listQuery:   validListQuery,
			verifyQuery: validVerifyQuery,
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, err := NewAuditEventService(tc.getQuery, tc.listQuery, tc.verifyQuery)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
				assert.Nil(t, service)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, service)
			}
		})
	}
}

func TestAuditEventService_GetByID(t *testing.T) {
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	eventID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)

	tests := []struct {
		name        string
		eventID     uuid.UUID
		setupMock   func(*query.MockAuditEventRepository)
		expectError bool
		errorIs     error
		validate    func(*testing.T, *model.AuditEvent)
	}{
		{
			name:    "Success - get existing audit event",
			eventID: eventID,
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().GetByID(gomock.Any(), eventID).Return(&model.AuditEvent{
					EventID:      eventID,
					EventType:    model.AuditEventTransactionValidated,
					CreatedAt:    fixedTime,
					Action:       model.AuditActionValidate,
					Result:       model.AuditResultAllow,
					ResourceID:   eventID.String(),
					ResourceType: model.ResourceTypeTransaction,
					Actor: model.Actor{
						ActorType: model.ActorTypeSystem,
						ID:        "system",
						IPAddress: "127.0.0.1",
					},
					Context: map[string]any{
						"request": map[string]any{
							"account": map[string]any{
								"id": accountID.String(),
							},
						},
					},
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, event *model.AuditEvent) {
				assert.Equal(t, eventID, event.EventID)
				assert.Equal(t, model.AuditEventTransactionValidated, event.EventType)
				assert.Equal(t, model.AuditActionValidate, event.Action)
				assert.Equal(t, model.AuditResultAllow, event.Result)
				assert.Equal(t, model.ResourceTypeTransaction, event.ResourceType)
				assert.Equal(t, model.ActorTypeSystem, event.Actor.ActorType)
				assert.NotNil(t, event.Context)
			},
		},
		{
			name:    "Success - get rule creation audit event",
			eventID: eventID,
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().GetByID(gomock.Any(), eventID).Return(&model.AuditEvent{
					EventID:      eventID,
					EventType:    model.AuditEventRuleCreated,
					CreatedAt:    fixedTime,
					Action:       model.AuditActionCreate,
					Result:       model.AuditResultSuccess,
					ResourceID:   eventID.String(),
					ResourceType: model.ResourceTypeRule,
					Actor: model.Actor{
						ActorType: model.ActorTypeUser,
						ID:        "user-123",
						Name:      "Admin User",
						IPAddress: "192.168.1.1",
					},
					Context: map[string]any{
						"after": map[string]any{
							"name": "New Rule",
						},
					},
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, event *model.AuditEvent) {
				assert.Equal(t, model.AuditEventRuleCreated, event.EventType)
				assert.Equal(t, model.AuditActionCreate, event.Action)
				assert.Equal(t, model.AuditResultSuccess, event.Result)
				assert.Equal(t, model.ResourceTypeRule, event.ResourceType)
				assert.Equal(t, model.ActorTypeUser, event.Actor.ActorType)
			},
		},
		{
			name:    "Error - audit event not found",
			eventID: testutil.MustDeterministicUUID(999),
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().GetByID(gomock.Any(), gomock.Any()).Return(nil, constant.ErrAuditEventNotFound)
			},
			expectError: true,
			errorIs:     constant.ErrAuditEventNotFound,
		},
		{
			name:    "Error - database error",
			eventID: eventID,
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().GetByID(gomock.Any(), eventID).Return(nil, errors.New("database connection failed"))
			},
			expectError: true,
		},
		{
			name:    "Error - context canceled",
			eventID: eventID,
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().GetByID(gomock.Any(), eventID).Return(nil, context.Canceled)
			},
			expectError: true,
			errorIs:     context.Canceled,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := query.NewMockAuditEventRepository(ctrl)
			tc.setupMock(mockRepo)

			getQuery, err := query.NewGetAuditEventQuery(mockRepo)
			require.NoError(t, err)
			listQuery, err := query.NewListAuditEventsQuery(mockRepo)
			require.NoError(t, err)
			verifyQuery, err := query.NewVerifyAuditEventQuery(mockRepo)
			require.NoError(t, err)

			service, err := NewAuditEventService(getQuery, listQuery, verifyQuery)
			require.NoError(t, err)
			result, err := service.GetAuditEvent(context.Background(), tc.eventID)

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

func TestAuditEventService_List(t *testing.T) {
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	eventID1 := testutil.MustDeterministicUUID(1)
	eventID2 := testutil.MustDeterministicUUID(2)

	tests := []struct {
		name        string
		filters     *model.AuditEventFilters
		setupMock   func(*query.MockAuditEventRepository)
		expectError bool
		errorIs     error
		validate    func(*testing.T, *model.ListAuditEventsResult)
	}{
		{
			name:    "Success - list all audit events with default filters",
			filters: nil,
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(&model.ListAuditEventsResult{
					AuditEvents: []*model.AuditEvent{
						{
							EventID:      eventID1,
							EventType:    model.AuditEventTransactionValidated,
							CreatedAt:    fixedTime,
							Action:       model.AuditActionValidate,
							Result:       model.AuditResultAllow,
							ResourceID:   eventID1.String(),
							ResourceType: model.ResourceTypeTransaction,
							Actor: model.Actor{
								ActorType: model.ActorTypeSystem,
								ID:        "system",
								IPAddress: "127.0.0.1",
							},
						},
						{
							EventID:      eventID2,
							EventType:    model.AuditEventRuleCreated,
							CreatedAt:    fixedTime.Add(time.Hour),
							Action:       model.AuditActionCreate,
							Result:       model.AuditResultSuccess,
							ResourceID:   eventID2.String(),
							ResourceType: model.ResourceTypeRule,
							Actor: model.Actor{
								ActorType: model.ActorTypeUser,
								ID:        "user-123",
								IPAddress: "192.168.1.1",
							},
						},
					},
					HasMore:    false,
					NextCursor: "",
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListAuditEventsResult) {
				require.Len(t, result.AuditEvents, 2)
				assert.Equal(t, eventID1, result.AuditEvents[0].EventID)
				assert.Equal(t, eventID2, result.AuditEvents[1].EventID)
				assert.False(t, result.HasMore)
			},
		},
		{
			name: "Success - list with event type filter",
			filters: &model.AuditEventFilters{
				EventType: testutil.Ptr(model.AuditEventTransactionValidated),
			},
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(&model.ListAuditEventsResult{
					AuditEvents: []*model.AuditEvent{
						{
							EventID:      eventID1,
							EventType:    model.AuditEventTransactionValidated,
							CreatedAt:    fixedTime,
							Action:       model.AuditActionValidate,
							Result:       model.AuditResultAllow,
							ResourceID:   eventID1.String(),
							ResourceType: model.ResourceTypeTransaction,
							Actor: model.Actor{
								ActorType: model.ActorTypeSystem,
								ID:        "system",
								IPAddress: "127.0.0.1",
							},
						},
					},
					HasMore:    false,
					NextCursor: "",
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListAuditEventsResult) {
				require.Len(t, result.AuditEvents, 1)
				assert.Equal(t, model.AuditEventTransactionValidated, result.AuditEvents[0].EventType)
			},
		},
		{
			name: "Success - paginated results with next page token",
			filters: &model.AuditEventFilters{
				Limit: 10,
			},
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(&model.ListAuditEventsResult{
					AuditEvents: []*model.AuditEvent{
						{
							EventID:      eventID1,
							EventType:    model.AuditEventTransactionValidated,
							CreatedAt:    fixedTime,
							Action:       model.AuditActionValidate,
							Result:       model.AuditResultAllow,
							ResourceID:   eventID1.String(),
							ResourceType: model.ResourceTypeTransaction,
							Actor: model.Actor{
								ActorType: model.ActorTypeSystem,
								ID:        "system",
								IPAddress: "127.0.0.1",
							},
						},
					},
					HasMore:    true,
					NextCursor: "next-page-token",
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListAuditEventsResult) {
				assert.True(t, result.HasMore)
				assert.Equal(t, "next-page-token", result.NextCursor)
			},
		},
		{
			name:    "Success - empty results",
			filters: &model.AuditEventFilters{},
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(&model.ListAuditEventsResult{
					AuditEvents: []*model.AuditEvent{},
					HasMore:     false,
					NextCursor:  "",
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListAuditEventsResult) {
				assert.Len(t, result.AuditEvents, 0)
				assert.False(t, result.HasMore)
			},
		},
		{
			name:    "Error - database error",
			filters: &model.AuditEventFilters{},
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errors.New("database connection failed"))
			},
			expectError: true,
		},
		{
			name:    "Error - context canceled",
			filters: &model.AuditEventFilters{},
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, context.Canceled)
			},
			expectError: true,
			errorIs:     context.Canceled,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := query.NewMockAuditEventRepository(ctrl)
			tc.setupMock(mockRepo)

			getQuery, err := query.NewGetAuditEventQuery(mockRepo)
			require.NoError(t, err)

			listQuery, err := query.NewListAuditEventsQuery(mockRepo)
			require.NoError(t, err)

			verifyQuery, err := query.NewVerifyAuditEventQuery(mockRepo)
			require.NoError(t, err)

			service, err := NewAuditEventService(getQuery, listQuery, verifyQuery)
			require.NoError(t, err)
			result, err := service.ListAuditEvents(context.Background(), tc.filters)

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

func TestAuditEventService_ListValidations(t *testing.T) {
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	eventID1 := testutil.MustDeterministicUUID(1)
	eventID2 := testutil.MustDeterministicUUID(2)

	tests := []struct {
		name        string
		filters     *model.AuditEventFilters
		setupMock   func(*query.MockAuditEventRepository)
		expectError bool
		errorIs     error
		validate    func(*testing.T, *model.ListAuditEventsResult)
	}{
		{
			name:    "Success - list validation events only",
			filters: nil,
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, filters *model.AuditEventFilters) (*model.ListAuditEventsResult, error) {
						// Verify that EventType was set to TRANSACTION_VALIDATED
						require.NotNil(t, filters.EventType)
						assert.Equal(t, model.AuditEventTransactionValidated, *filters.EventType)

						return &model.ListAuditEventsResult{
							AuditEvents: []*model.AuditEvent{
								{
									EventID:      eventID1,
									EventType:    model.AuditEventTransactionValidated,
									CreatedAt:    fixedTime,
									Action:       model.AuditActionValidate,
									Result:       model.AuditResultAllow,
									ResourceID:   eventID1.String(),
									ResourceType: model.ResourceTypeTransaction,
									Actor: model.Actor{
										ActorType: model.ActorTypeSystem,
										ID:        "system",
										IPAddress: "127.0.0.1",
									},
								},
								{
									EventID:      eventID2,
									EventType:    model.AuditEventTransactionValidated,
									CreatedAt:    fixedTime.Add(time.Hour),
									Action:       model.AuditActionValidate,
									Result:       model.AuditResultDeny,
									ResourceID:   eventID2.String(),
									ResourceType: model.ResourceTypeTransaction,
									Actor: model.Actor{
										ActorType: model.ActorTypeSystem,
										ID:        "system",
										IPAddress: "127.0.0.1",
									},
								},
							},
							HasMore:    false,
							NextCursor: "",
						}, nil
					})
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListAuditEventsResult) {
				require.Len(t, result.AuditEvents, 2)
				// All events should be validation events
				for _, event := range result.AuditEvents {
					assert.Equal(t, model.AuditEventTransactionValidated, event.EventType)
					assert.Equal(t, model.AuditActionValidate, event.Action)
				}
			},
		},
		{
			name: "Success - list validations with filters",
			filters: &model.AuditEventFilters{
				Result: testutil.Ptr(model.AuditResultAllow),
			},
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(&model.ListAuditEventsResult{
					AuditEvents: []*model.AuditEvent{
						{
							EventID:      eventID1,
							EventType:    model.AuditEventTransactionValidated,
							CreatedAt:    fixedTime,
							Action:       model.AuditActionValidate,
							Result:       model.AuditResultAllow,
							ResourceID:   eventID1.String(),
							ResourceType: model.ResourceTypeTransaction,
							Actor: model.Actor{
								ActorType: model.ActorTypeSystem,
								ID:        "system",
								IPAddress: "127.0.0.1",
							},
						},
					},
					HasMore:    false,
					NextCursor: "",
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListAuditEventsResult) {
				require.Len(t, result.AuditEvents, 1)
				assert.Equal(t, model.AuditResultAllow, result.AuditEvents[0].Result)
			},
		},
		{
			name:    "Success - empty validation results",
			filters: &model.AuditEventFilters{},
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(&model.ListAuditEventsResult{
					AuditEvents: []*model.AuditEvent{},
					HasMore:     false,
					NextCursor:  "",
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.ListAuditEventsResult) {
				assert.Len(t, result.AuditEvents, 0)
				assert.False(t, result.HasMore)
			},
		},
		{
			name:    "Error - database error",
			filters: &model.AuditEventFilters{},
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errors.New("database connection failed"))
			},
			expectError: true,
		},
		{
			name:    "Error - context canceled",
			filters: &model.AuditEventFilters{},
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, context.Canceled)
			},
			expectError: true,
			errorIs:     context.Canceled,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := query.NewMockAuditEventRepository(ctrl)
			tc.setupMock(mockRepo)

			getQuery, err := query.NewGetAuditEventQuery(mockRepo)
			require.NoError(t, err)

			listQuery, err := query.NewListAuditEventsQuery(mockRepo)
			require.NoError(t, err)

			verifyQuery, err := query.NewVerifyAuditEventQuery(mockRepo)
			require.NoError(t, err)

			service, err := NewAuditEventService(getQuery, listQuery, verifyQuery)
			require.NoError(t, err)
			result, err := service.ListValidations(context.Background(), tc.filters)

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

func TestAuditEventService_VerifyHashChain(t *testing.T) {
	eventID := testutil.MustDeterministicUUID(1)

	tests := []struct {
		name        string
		eventID     uuid.UUID
		setupMock   func(*query.MockAuditEventRepository)
		expectError bool
		errorIs     error
		validate    func(*testing.T, *model.HashChainVerificationResult)
	}{
		{
			name:    "Success - valid hash chain",
			eventID: eventID,
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().VerifyHashChain(gomock.Any(), eventID).Return(&model.HashChainVerificationResult{
					IsValid:        true,
					FirstInvalidID: nil,
					TotalChecked:   10,
					Message:        "Hash chain is valid",
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.HashChainVerificationResult) {
				assert.True(t, result.IsValid)
				assert.Nil(t, result.FirstInvalidID)
				assert.Equal(t, int64(10), result.TotalChecked)
				assert.Equal(t, "Hash chain is valid", result.Message)
			},
		},
		{
			name:    "Success - invalid hash chain detected",
			eventID: eventID,
			setupMock: func(m *query.MockAuditEventRepository) {
				invalidID := int64(5)
				m.EXPECT().VerifyHashChain(gomock.Any(), eventID).Return(&model.HashChainVerificationResult{
					IsValid:        false,
					FirstInvalidID: &invalidID,
					TotalChecked:   5,
					Message:        "Hash chain broken at ID 5",
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.HashChainVerificationResult) {
				assert.False(t, result.IsValid)
				require.NotNil(t, result.FirstInvalidID)
				assert.Equal(t, int64(5), *result.FirstInvalidID)
				assert.Equal(t, int64(5), result.TotalChecked)
				assert.Contains(t, result.Message, "broken")
			},
		},
		{
			name:    "Success - single event verified",
			eventID: eventID,
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().VerifyHashChain(gomock.Any(), eventID).Return(&model.HashChainVerificationResult{
					IsValid:        true,
					FirstInvalidID: nil,
					TotalChecked:   1,
					Message:        "Single event verified",
				}, nil)
			},
			expectError: false,
			validate: func(t *testing.T, result *model.HashChainVerificationResult) {
				assert.True(t, result.IsValid)
				assert.Equal(t, int64(1), result.TotalChecked)
			},
		},
		{
			name:    "Error - audit event not found",
			eventID: testutil.MustDeterministicUUID(999),
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().VerifyHashChain(gomock.Any(), gomock.Any()).Return(nil, constant.ErrAuditEventNotFound)
			},
			expectError: true,
			errorIs:     constant.ErrAuditEventNotFound,
		},
		{
			name:    "Error - database error",
			eventID: eventID,
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().VerifyHashChain(gomock.Any(), eventID).Return(nil, errors.New("database connection failed"))
			},
			expectError: true,
		},
		{
			name:    "Error - context canceled",
			eventID: eventID,
			setupMock: func(m *query.MockAuditEventRepository) {
				m.EXPECT().VerifyHashChain(gomock.Any(), eventID).Return(nil, context.Canceled)
			},
			expectError: true,
			errorIs:     context.Canceled,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockRepo := query.NewMockAuditEventRepository(ctrl)
			tc.setupMock(mockRepo)

			getQuery, err := query.NewGetAuditEventQuery(mockRepo)
			require.NoError(t, err)
			listQuery, err := query.NewListAuditEventsQuery(mockRepo)
			require.NoError(t, err)
			verifyQuery, err := query.NewVerifyAuditEventQuery(mockRepo)
			require.NoError(t, err)

			service, err := NewAuditEventService(getQuery, listQuery, verifyQuery)
			require.NoError(t, err)
			result, err := service.VerifyHashChain(context.Background(), tc.eventID)

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
