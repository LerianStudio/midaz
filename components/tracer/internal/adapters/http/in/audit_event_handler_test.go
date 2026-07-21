// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestAuditEventHandler_ListAuditEvents(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(ctrl *gomock.Controller) *MockAuditEventService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "success - lists audit events with default params",
			queryParams: "",
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				mockService := NewMockAuditEventService(ctrl)
				eventID := testutil.MustDeterministicUUID(1)
				mockService.EXPECT().
					ListAuditEvents(gomock.Any(), gomock.Any()).
					Return(&model.ListAuditEventsResult{
						AuditEvents: []*model.AuditEvent{
							{
								EventID:      eventID,
								EventType:    model.AuditEventRuleCreated,
								Action:       model.AuditActionCreate,
								Result:       model.AuditResultSuccess,
								ResourceID:   testutil.MustDeterministicUUID(2).String(),
								ResourceType: model.ResourceTypeRule,
								CreatedAt:    testutil.FixedTime(),
								Actor: model.Actor{
									ActorType: model.ActorTypeUser,
									ID:        "user-123",
									Name:      "Test User",
									IPAddress: "192.168.1.1",
								},
							},
						},
						HasMore:    false,
						NextCursor: "",
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListAuditEventsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Len(t, response.AuditEvents, 1)
				assert.Equal(t, model.AuditEventRuleCreated, response.AuditEvents[0].EventType)
				assert.False(t, response.HasMore)
			},
		},
		{
			name:        "success - lists with event type filter",
			queryParams: "?event_type=RULE_CREATED",
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				mockService := NewMockAuditEventService(ctrl)
				mockService.EXPECT().
					ListAuditEvents(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx any, filters *model.AuditEventFilters) (*model.ListAuditEventsResult, error) {
						// Verify filter was applied
						assert.NotNil(t, filters.EventType)
						assert.Equal(t, model.AuditEventRuleCreated, *filters.EventType)
						return &model.ListAuditEventsResult{
							AuditEvents: []*model.AuditEvent{},
							HasMore:     false,
							NextCursor:  "",
						}, nil
					})
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListAuditEventsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
			},
		},
		{
			name:        "success - returns all results without pagination (hasMore=false)",
			queryParams: "?limit=100",
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				mockService := NewMockAuditEventService(ctrl)
				mockService.EXPECT().
					ListAuditEvents(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx any, filters *model.AuditEventFilters) (*model.ListAuditEventsResult, error) {
						assert.Equal(t, 100, filters.Limit)
						// Return fewer results than limit (no more pages)
						return &model.ListAuditEventsResult{
							AuditEvents: []*model.AuditEvent{
								{
									EventID:      testutil.MustDeterministicUUID(10),
									EventType:    model.AuditEventRuleCreated,
									Action:       model.AuditActionCreate,
									Result:       model.AuditResultSuccess,
									ResourceID:   testutil.MustDeterministicUUID(11).String(),
									ResourceType: model.ResourceTypeRule,
									CreatedAt:    testutil.FixedTime(),
									Actor: model.Actor{
										ActorType: model.ActorTypeUser,
										ID:        "user-1",
										Name:      "User 1",
									},
								},
								{
									EventID:      testutil.MustDeterministicUUID(12),
									EventType:    model.AuditEventLimitActivated,
									Action:       model.AuditActionActivate,
									Result:       model.AuditResultSuccess,
									ResourceID:   testutil.MustDeterministicUUID(13).String(),
									ResourceType: model.ResourceTypeLimit,
									CreatedAt:    testutil.FixedTime(),
									Actor: model.Actor{
										ActorType: model.ActorTypeUser,
										ID:        "user-2",
										Name:      "User 2",
									},
								},
							},
							HasMore:    false,
							NextCursor: "",
						}, nil
					})
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListAuditEventsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Len(t, response.AuditEvents, 2, "should return all 2 events")
				assert.False(t, response.HasMore, "hasMore should be false when all results fit in one page")
				assert.Empty(t, response.NextCursor, "nextCursor should be empty when no more pages")
			},
		},
		{
			name:        "error - invalid event type",
			queryParams: "?event_type=INVALID_TYPE",
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				return NewMockAuditEventService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				// Validator returns "auditeventtype" (lowercase) in error message
				bodyStr := string(body)
				assert.True(t, len(bodyStr) > 0, "Expected non-empty error message")
			},
		},
		{
			name:        "error - invalid date format",
			queryParams: "?start_date=invalid-date",
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				return NewMockAuditEventService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				// Returns validation error with specific TRC-0020 for invalid date format
				var errorResp map[string]any
				err := json.Unmarshal(body, &errorResp)
				require.NoError(t, err, "response should be valid JSON object")
				assert.Equal(t, "0077", errorResp["code"], "error code should be TRC-0020 (Invalid Date Format)")
				assert.NotEmpty(t, errorResp["detail"], "error detail should not be empty")
			},
		},
		{
			name:        "error - invalid audit event filters",
			queryParams: "",
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				mockService := NewMockAuditEventService(ctrl)
				mockService.EXPECT().
					ListAuditEvents(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInvalidAuditEventFilters)
				return mockService
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				var errorResp map[string]any
				err := json.Unmarshal(body, &errorResp)
				require.NoError(t, err, "response should be valid JSON")

				assert.Equal(t, "0382", errorResp["code"], "error code should be TRC-0141 (invalid filters)")
			},
		},
		{
			name:        "error - invalid cursor",
			queryParams: "",
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				mockService := NewMockAuditEventService(ctrl)
				mockService.EXPECT().
					ListAuditEvents(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInvalidCursor)
				return mockService
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				var errorResp map[string]any
				err := json.Unmarshal(body, &errorResp)
				require.NoError(t, err, "response should be valid JSON")

				assert.Equal(t, "0333", errorResp["code"], "error code should be TRC-0044 (invalid cursor)")
			},
		},
		{
			name:        "error - invalid sort column",
			queryParams: "",
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				mockService := NewMockAuditEventService(ctrl)
				mockService.EXPECT().
					ListAuditEvents(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInvalidSortColumn)
				return mockService
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				var errorResp map[string]any
				err := json.Unmarshal(body, &errorResp)
				require.NoError(t, err, "response should be valid JSON")

				assert.Equal(t, "0332", errorResp["code"], "error code should be TRC-0043 (invalid sort column)")
			},
		},
		{
			name:        "error - service returns generic error (default case)",
			queryParams: "",
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				mockService := NewMockAuditEventService(ctrl)
				mockService.EXPECT().
					ListAuditEvents(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))
				return mockService
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody: func(t *testing.T, body []byte) {
				var errorResp map[string]any
				err := json.Unmarshal(body, &errorResp)
				require.NoError(t, err, "response should be valid JSON")

				assert.Equal(t, "0046", errorResp["code"], "error code should be TRC-0004 (internal server error)")
				assert.NotEmpty(t, errorResp["detail"], "error detail should be present")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)
			handler := NewAuditEventHandler(mockService)

			app := fiber.New()
			app.Get("/v1/audit-events", handler.ListAuditEvents)

			req := httptest.NewRequest(http.MethodGet, "/v1/audit-events"+tt.queryParams, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if tt.expectedBody != nil {
				tt.expectedBody(t, body)
			}
		})
	}
}

func TestAuditEventHandler_ListAuditEvents_ValidTransactionTypeFilter(t *testing.T) {
	validTypes := []string{"CARD", "WIRE", "PIX", "CRYPTO"}

	for _, txType := range validTypes {
		t.Run("valid transaction_type: "+txType, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := NewMockAuditEventService(ctrl)
			mockService.EXPECT().
				ListAuditEvents(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx any, filters *model.AuditEventFilters) (*model.ListAuditEventsResult, error) {
					assert.NotNil(t, filters.TransactionType)
					assert.Equal(t, model.TransactionType(txType), *filters.TransactionType)

					return &model.ListAuditEventsResult{
						AuditEvents: []*model.AuditEvent{},
						HasMore:     false,
						NextCursor:  "",
					}, nil
				})

			handler := NewAuditEventHandler(mockService)
			app := fiber.New()
			app.Get("/v1/audit-events", handler.ListAuditEvents)

			req := httptest.NewRequest(http.MethodGet, "/v1/audit-events?transaction_type="+txType, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestAuditEventHandler_ListAuditEvents_InvalidTransactionType(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockService := NewMockAuditEventService(ctrl)
	handler := NewAuditEventHandler(mockService)

	app := fiber.New()
	app.Get("/v1/audit-events", handler.ListAuditEvents)

	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events?transaction_type=INVALID_TYPE", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]any
	err = json.Unmarshal(body, &errResp)
	require.NoError(t, err, "Expected valid JSON error response")

	require.Equal(t, "0009", errResp["code"], "Expected canonical missing-fields code for invalid transaction_type")
	require.NotEmpty(t, errResp["detail"], "Expected non-empty error detail")
}

func TestListAuditEventsInput_ValidTransactionTypes(t *testing.T) {
	validTypes := []string{"CARD", "WIRE", "PIX", "CRYPTO"}

	for _, txType := range validTypes {
		t.Run(txType, func(t *testing.T) {
			tx := txType
			limit := 10
			input := ListAuditEventsInput{
				TransactionType: &tx,
				Limit:           &limit,
			}
			err := input.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestListAuditEventsInput_InvalidTransactionType(t *testing.T) {
	invalid := "INVALID"
	limit := 10
	input := ListAuditEventsInput{
		TransactionType: &invalid,
		Limit:           &limit,
	}
	err := input.Validate()
	assert.Error(t, err)
}

func TestListAuditEventsInput_DateRangeValidation(t *testing.T) {
	tests := []struct {
		name         string
		startDate    string
		endDate      string
		wantErr      bool
		errMsg       string
		expectedCode string
	}{
		{
			name:      "valid - start before end",
			startDate: "2026-01-01T00:00:00Z",
			endDate:   "2026-12-31T23:59:59Z",
			wantErr:   false,
		},
		{
			name:      "valid - same date",
			startDate: "2026-06-15T12:00:00Z",
			endDate:   "2026-06-15T12:00:00Z",
			wantErr:   false,
		},
		{
			name:         "invalid - start after end",
			startDate:    "2026-12-31T23:59:59Z",
			endDate:      "2026-01-01T00:00:00Z",
			wantErr:      true,
			errMsg:       "end_date must be on or after start_date",
			expectedCode: "0083",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit := 10
			input := ListAuditEventsInput{
				StartDate: tt.startDate,
				EndDate:   tt.endDate,
				Limit:     &limit,
			}
			err := input.Validate()
			if tt.wantErr {
				require.Error(t, err)

				if tt.expectedCode != "" {
					var valErr pkg.ValidationError
					require.ErrorAs(t, err, &valErr, "Expected pkg.ValidationError type")
					assert.Equal(t, tt.expectedCode, valErr.Code, "Expected error code %s", tt.expectedCode)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuditEventHandler_ListAuditEvents_MultiPagePagination(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockService := NewMockAuditEventService(ctrl)

	// Create unique event IDs for tracking across pages
	event1ID := testutil.MustDeterministicUUID(20)
	event2ID := testutil.MustDeterministicUUID(21)
	event3ID := testutil.MustDeterministicUUID(22)
	event4ID := testutil.MustDeterministicUUID(23)
	event5ID := testutil.MustDeterministicUUID(24)
	event6ID := testutil.MustDeterministicUUID(25)

	// Setup mock to return different pages based on cursor
	callCount := 0
	mockService.EXPECT().
		ListAuditEvents(gomock.Any(), gomock.Any()).
		Times(3).
		DoAndReturn(func(ctx any, filters *model.AuditEventFilters) (*model.ListAuditEventsResult, error) {
			callCount++
			assert.Equal(t, 2, filters.Limit, "limit should be 2 per page")

			switch callCount {
			case 1: // Page 1 - no cursor
				assert.Empty(t, filters.Cursor, "first page should have empty cursor")
				return &model.ListAuditEventsResult{
					AuditEvents: []*model.AuditEvent{
						{EventID: event1ID, EventType: model.AuditEventRuleCreated, Action: model.AuditActionCreate, Result: model.AuditResultSuccess, ResourceID: "res-1", ResourceType: model.ResourceTypeRule, CreatedAt: testutil.FixedTime().Add(-5 * time.Hour), Actor: model.Actor{ActorType: model.ActorTypeUser, ID: "user-1", Name: "User 1"}},
						{EventID: event2ID, EventType: model.AuditEventLimitActivated, Action: model.AuditActionActivate, Result: model.AuditResultSuccess, ResourceID: "res-2", ResourceType: model.ResourceTypeLimit, CreatedAt: testutil.FixedTime().Add(-4 * time.Hour), Actor: model.Actor{ActorType: model.ActorTypeUser, ID: "user-2", Name: "User 2"}},
					},
					HasMore:    true,
					NextCursor: "cursor-page-2",
				}, nil

			case 2: // Page 2 - cursor from page 1
				assert.Equal(t, "cursor-page-2", filters.Cursor, "second page should have cursor from page 1")
				return &model.ListAuditEventsResult{
					AuditEvents: []*model.AuditEvent{
						{EventID: event3ID, EventType: model.AuditEventRuleActivated, Action: model.AuditActionActivate, Result: model.AuditResultSuccess, ResourceID: "res-3", ResourceType: model.ResourceTypeRule, CreatedAt: testutil.FixedTime().Add(-3 * time.Hour), Actor: model.Actor{ActorType: model.ActorTypeUser, ID: "user-3", Name: "User 3"}},
						{EventID: event4ID, EventType: model.AuditEventLimitDeactivated, Action: model.AuditActionDeactivate, Result: model.AuditResultSuccess, ResourceID: "res-4", ResourceType: model.ResourceTypeLimit, CreatedAt: testutil.FixedTime().Add(-2 * time.Hour), Actor: model.Actor{ActorType: model.ActorTypeUser, ID: "user-4", Name: "User 4"}},
					},
					HasMore:    true,
					NextCursor: "cursor-page-3",
				}, nil

			case 3: // Page 3 - cursor from page 2 (last page)
				assert.Equal(t, "cursor-page-3", filters.Cursor, "third page should have cursor from page 2")
				return &model.ListAuditEventsResult{
					AuditEvents: []*model.AuditEvent{
						{EventID: event5ID, EventType: model.AuditEventRuleDeleted, Action: model.AuditActionDelete, Result: model.AuditResultSuccess, ResourceID: "res-5", ResourceType: model.ResourceTypeRule, CreatedAt: testutil.FixedTime().Add(-1 * time.Hour), Actor: model.Actor{ActorType: model.ActorTypeUser, ID: "user-5", Name: "User 5"}},
						{EventID: event6ID, EventType: model.AuditEventLimitUpdated, Action: model.AuditActionUpdate, Result: model.AuditResultSuccess, ResourceID: "res-6", ResourceType: model.ResourceTypeLimit, CreatedAt: testutil.FixedTime(), Actor: model.Actor{ActorType: model.ActorTypeUser, ID: "user-6", Name: "User 6"}},
					},
					HasMore:    false,
					NextCursor: "",
				}, nil

			default:
				t.Fatalf("unexpected call count: %d", callCount)
				return nil, errors.New("unexpected call")
			}
		})

	handler := NewAuditEventHandler(mockService)
	app := fiber.New()
	app.Get("/v1/audit-events", handler.ListAuditEvents)

	// Track all event IDs to verify no duplicates
	allEventIDs := make(map[uuid.UUID]bool)

	// Page 1: Initial request
	req1 := httptest.NewRequest(http.MethodGet, "/v1/audit-events?limit=2", nil)
	resp1, err := app.Test(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()

	assert.Equal(t, http.StatusOK, resp1.StatusCode)
	body1, err := io.ReadAll(resp1.Body)
	require.NoError(t, err)

	var response1 ListAuditEventsResponse
	err = json.Unmarshal(body1, &response1)
	require.NoError(t, err)
	assert.Len(t, response1.AuditEvents, 2, "page 1 should have 2 events")
	assert.True(t, response1.HasMore, "page 1 should have more results")
	assert.Equal(t, "cursor-page-2", response1.NextCursor, "page 1 should return cursor for page 2")

	// Track page 1 events
	for _, event := range response1.AuditEvents {
		allEventIDs[event.EventID] = true
	}

	// Page 2: Request with cursor from page 1
	req2 := httptest.NewRequest(http.MethodGet, "/v1/audit-events?limit=2&cursor=cursor-page-2", nil)
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	body2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	var response2 ListAuditEventsResponse
	err = json.Unmarshal(body2, &response2)
	require.NoError(t, err)
	assert.Len(t, response2.AuditEvents, 2, "page 2 should have 2 events")
	assert.True(t, response2.HasMore, "page 2 should have more results")
	assert.Equal(t, "cursor-page-3", response2.NextCursor, "page 2 should return cursor for page 3")

	// Verify no duplicates from page 1
	for _, event := range response2.AuditEvents {
		assert.False(t, allEventIDs[event.EventID], "event %s should not be duplicated from previous pages", event.EventID)
		allEventIDs[event.EventID] = true
	}

	// Page 3: Request with cursor from page 2 (last page)
	req3 := httptest.NewRequest(http.MethodGet, "/v1/audit-events?limit=2&cursor=cursor-page-3", nil)
	resp3, err := app.Test(req3)
	require.NoError(t, err)
	defer resp3.Body.Close()

	assert.Equal(t, http.StatusOK, resp3.StatusCode)
	body3, err := io.ReadAll(resp3.Body)
	require.NoError(t, err)

	var response3 ListAuditEventsResponse
	err = json.Unmarshal(body3, &response3)
	require.NoError(t, err)
	assert.Len(t, response3.AuditEvents, 2, "page 3 should have 2 events")
	assert.False(t, response3.HasMore, "page 3 should be the last page (hasMore=false)")
	assert.Empty(t, response3.NextCursor, "page 3 should have empty cursor (no more pages)")

	// Verify no duplicates from pages 1 and 2
	for _, event := range response3.AuditEvents {
		assert.False(t, allEventIDs[event.EventID], "event %s should not be duplicated from previous pages", event.EventID)
		allEventIDs[event.EventID] = true
	}

	// Verify total of 6 unique events collected across all pages
	assert.Len(t, allEventIDs, 6, "should have collected 6 unique events across 3 pages")
}

func TestAuditEventHandler_GetAuditEvent(t *testing.T) {
	tests := []struct {
		name           string
		eventID        string
		mockSetup      func(ctrl *gomock.Controller, eventID uuid.UUID) *MockAuditEventService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:    "success - retrieves audit event",
			eventID: testutil.MustDeterministicUUID(30).String(),
			mockSetup: func(ctrl *gomock.Controller, eventID uuid.UUID) *MockAuditEventService {
				mockService := NewMockAuditEventService(ctrl)
				mockService.EXPECT().
					GetAuditEvent(gomock.Any(), eventID).
					Return(&model.AuditEvent{
						EventID:      eventID,
						EventType:    model.AuditEventLimitActivated,
						Action:       model.AuditActionActivate,
						Result:       model.AuditResultSuccess,
						ResourceID:   testutil.MustDeterministicUUID(31).String(),
						ResourceType: model.ResourceTypeLimit,
						CreatedAt:    testutil.FixedTime(),
						Actor: model.Actor{
							ActorType: model.ActorTypeSystem,
							ID:        "system",
							Name:      "System",
							IPAddress: "127.0.0.1",
						},
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response model.AuditEvent
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, model.AuditEventLimitActivated, response.EventType)
				assert.Equal(t, model.AuditActionActivate, response.Action)
			},
		},
		{
			name:    "error - invalid event ID format",
			eventID: "invalid-uuid",
			mockSetup: func(ctrl *gomock.Controller, eventID uuid.UUID) *MockAuditEventService {
				return NewMockAuditEventService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				// Handler returns structured error response
				var errorResp map[string]any
				err := json.Unmarshal(body, &errorResp)
				require.NoError(t, err, "response should be valid JSON object")
				assert.Equal(t, "0065", errorResp["code"], "error code should be TRC-0007")
			},
		},
		{
			name:    "error - audit event not found",
			eventID: testutil.MustDeterministicUUID(33).String(),
			mockSetup: func(ctrl *gomock.Controller, eventID uuid.UUID) *MockAuditEventService {
				mockService := NewMockAuditEventService(ctrl)
				mockService.EXPECT().
					GetAuditEvent(gomock.Any(), eventID).
					Return(nil, constant.ErrAuditEventNotFound)
				return mockService
			},
			expectedStatus: http.StatusNotFound,
			expectedBody: func(t *testing.T, body []byte) {
				var errorResp map[string]any
				err := json.Unmarshal(body, &errorResp)
				require.NoError(t, err, "response should be valid JSON")

				assert.Equal(t, "0381", errorResp["code"], "error code should be TRC-0140 (ErrAuditEventNotFound)")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			var parsedID uuid.UUID
			if tt.eventID != "invalid-uuid" {
				parsedID, _ = uuid.Parse(tt.eventID)
			}

			mockService := tt.mockSetup(ctrl, parsedID)
			handler := NewAuditEventHandler(mockService)

			app := fiber.New()
			app.Get("/v1/audit-events/:id", handler.GetAuditEvent)

			req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/"+tt.eventID, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if tt.expectedBody != nil {
				tt.expectedBody(t, body)
			}
		})
	}
}

func TestAuditEventHandler_VerifyHashChain(t *testing.T) {
	tests := []struct {
		name           string
		eventID        string
		mockSetup      func(ctrl *gomock.Controller) *MockAuditEventService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:    "success - chain is valid",
			eventID: testutil.MustDeterministicUUID(40).String(),
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				mockService := NewMockAuditEventService(ctrl)
				mockService.EXPECT().
					VerifyHashChain(gomock.Any(), gomock.Any()).
					Return(&model.HashChainVerificationResult{
						IsValid:      true,
						TotalChecked: 100,
						Message:      "Hash chain is valid",
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response model.HashChainVerificationResult
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.True(t, response.IsValid)
				assert.Equal(t, int64(100), response.TotalChecked)
			},
		},
		{
			name:    "success - chain has tampering detected",
			eventID: testutil.MustDeterministicUUID(41).String(),
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				mockService := NewMockAuditEventService(ctrl)
				invalidID := int64(50)
				mockService.EXPECT().
					VerifyHashChain(gomock.Any(), gomock.Any()).
					Return(&model.HashChainVerificationResult{
						IsValid:        false,
						FirstInvalidID: &invalidID,
						TotalChecked:   100,
						Message:        "Hash chain tampering detected at event 50",
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response model.HashChainVerificationResult
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.False(t, response.IsValid)
				assert.NotNil(t, response.FirstInvalidID)
				assert.Equal(t, int64(50), *response.FirstInvalidID)
			},
		},
		{
			name:    "error - invalid event ID format",
			eventID: "invalid-uuid",
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				return NewMockAuditEventService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				// Handler returns structured error response
				var errorResp map[string]any
				err := json.Unmarshal(body, &errorResp)
				require.NoError(t, err, "response should be valid JSON object")
				assert.Equal(t, "0065", errorResp["code"], "error code should be TRC-0007")
			},
		},
		{
			name:    "error - audit event not found",
			eventID: testutil.MustDeterministicUUID(42).String(),
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				mockService := NewMockAuditEventService(ctrl)
				mockService.EXPECT().
					VerifyHashChain(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrAuditEventNotFound)
				return mockService
			},
			expectedStatus: http.StatusNotFound,
			expectedBody: func(t *testing.T, body []byte) {
				var errorResp map[string]any
				err := json.Unmarshal(body, &errorResp)
				require.NoError(t, err, "response should be valid JSON")

				assert.Equal(t, "0381", errorResp["code"], "error code should be TRC-0140 (audit event not found)")
			},
		},
		{
			name:    "error - service returns generic error (default case)",
			eventID: testutil.MustDeterministicUUID(43).String(),
			mockSetup: func(ctrl *gomock.Controller) *MockAuditEventService {
				mockService := NewMockAuditEventService(ctrl)
				mockService.EXPECT().
					VerifyHashChain(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("verification failed"))
				return mockService
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody: func(t *testing.T, body []byte) {
				var errorResp map[string]any
				err := json.Unmarshal(body, &errorResp)
				require.NoError(t, err, "response should be valid JSON")

				assert.Equal(t, "0046", errorResp["code"], "error code should be TRC-0004 (internal server error)")
				assert.NotEmpty(t, errorResp["detail"], "error detail should be present")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)
			handler := NewAuditEventHandler(mockService)

			app := fiber.New()
			app.Get("/v1/audit-events/:id/verify", handler.VerifyHashChain)

			req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/"+tt.eventID+"/verify", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if tt.expectedBody != nil {
				tt.expectedBody(t, body)
			}
		})
	}
}
