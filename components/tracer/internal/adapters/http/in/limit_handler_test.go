// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

func TestLimitHandler_CreateLimit(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    any
		mockSetup      func(ctrl *gomock.Controller) *MockLimitService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success - creates limit",
			requestBody: map[string]any{
				"name":        "Daily Limit",
				"description": "Daily spending limit",
				"limitType":   "DAILY",
				"maxAmount":   "1000.00",
				"currency":    "BRL",
				"scopes": []map[string]any{
					{"accountId": "550e8400-e29b-41d4-a716-446655440000"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					CreateLimit(gomock.Any(), gomock.Any()).
					Return(&model.Limit{
						ID:        testutil.MustDeterministicUUID(1),
						Name:      "Daily Limit",
						LimitType: model.LimitTypeDaily,
						MaxAmount: decimal.RequireFromString("1000"),
						Currency:  "BRL",
						Status:    model.LimitStatusActive,
						CreatedAt: testutil.FixedTime(),
						UpdatedAt: testutil.FixedTime(),
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusCreated,
			expectedBody: func(t *testing.T, body []byte) {
				var response map[string]any
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Daily Limit", response["name"])
				assert.Equal(t, "DAILY", response["limitType"])
				assert.Equal(t, "ACTIVE", response["status"])
			},
		},
		{
			name: "error - missing required field name",
			requestBody: map[string]any{
				"limitType": "DAILY",
				"maxAmount": "1000.00",
				"currency":  "BRL",
				"scopes": []map[string]any{
					{"accountId": "550e8400-e29b-41d4-a716-446655440000"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "name")
			},
		},
		{
			name: "error - missing required field limitType",
			requestBody: map[string]any{
				"name":      "Test Limit",
				"maxAmount": "1000.00",
				"currency":  "BRL",
				"scopes": []map[string]any{
					{"accountId": "550e8400-e29b-41d4-a716-446655440000"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "limitType")
			},
		},
		{
			name: "error - invalid limitType value",
			requestBody: map[string]any{
				"name":      "Test Limit",
				"limitType": "INVALID",
				"maxAmount": "1000.00",
				"currency":  "BRL",
				"scopes": []map[string]any{
					{"accountId": "550e8400-e29b-41d4-a716-446655440000"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "limitType")
			},
		},
		{
			name: "error - invalid currency (not 3 chars)",
			requestBody: map[string]any{
				"name":      "Test Limit",
				"limitType": "DAILY",
				"maxAmount": "1000.00",
				"currency":  "BR",
				"scopes": []map[string]any{
					{"accountId": "550e8400-e29b-41d4-a716-446655440000"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "currency")
			},
		},
		{
			name: "error - maxAmount must be positive",
			requestBody: map[string]any{
				"name":      "Test Limit",
				"limitType": "DAILY",
				"maxAmount": "0",
				"currency":  "BRL",
				"scopes": []map[string]any{
					{"accountId": "550e8400-e29b-41d4-a716-446655440000"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "maxAmount")
			},
		},
		{
			name: "error - empty scopes",
			requestBody: map[string]any{
				"name":      "Test Limit",
				"limitType": "DAILY",
				"maxAmount": "1000.00",
				"currency":  "BRL",
				"scopes":    []map[string]any{},
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "scopes")
			},
		},
		{
			name: "error - service returns internal error",
			requestBody: map[string]any{
				"name":      "Test Limit",
				"limitType": "DAILY",
				"maxAmount": "1000.00",
				"currency":  "BRL",
				"scopes": []map[string]any{
					{"accountId": "550e8400-e29b-41d4-a716-446655440000"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					CreateLimit(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))

				return mockService
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   func(t *testing.T, body []byte) {},
		},
		{
			name: "error - service returns limit name already exists (TRC-0304)",
			requestBody: map[string]any{
				"name":      "Duplicate Limit Name",
				"limitType": "DAILY",
				"maxAmount": "1000.00",
				"currency":  "BRL",
				"scopes": []map[string]any{
					{"accountId": "550e8400-e29b-41d4-a716-446655440000"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					CreateLimit(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrLimitNameAlreadyExists)

				return mockService
			},
			expectedStatus: http.StatusConflict,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "TRC-0304")
				assert.Contains(t, string(body), "already exists")
			},
		},
		{
			name:        "error - invalid JSON body",
			requestBody: "invalid json",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   func(t *testing.T, body []byte) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)
			handler := NewLimitHandler(mockService)

			app := fiber.New()
			app.Post("/limits", handler.CreateLimit)

			var bodyBytes []byte
			switch v := tt.requestBody.(type) {
			case string:
				bodyBytes = []byte(v)
			default:
				var err error
				bodyBytes, err = json.Marshal(v)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/limits", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			tt.expectedBody(t, body)
		})
	}
}

func TestLimitHandler_GetLimit(t *testing.T) {
	validID := testutil.MustDeterministicUUID(10)

	tests := []struct {
		name           string
		limitID        string
		mockSetup      func(ctrl *gomock.Controller) *MockLimitService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:    "success - gets limit by ID",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					GetLimit(gomock.Any(), validID).
					Return(&model.Limit{
						ID:        validID,
						Name:      "Daily Limit",
						LimitType: model.LimitTypeDaily,
						MaxAmount: decimal.RequireFromString("1000"),
						Currency:  "BRL",
						Status:    model.LimitStatusActive,
						CreatedAt: testutil.FixedTime(),
						UpdatedAt: testutil.FixedTime(),
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response map[string]any
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Daily Limit", response["name"])
			},
		},
		{
			name:    "error - invalid UUID",
			limitID: "invalid-uuid",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "Invalid limit ID")
			},
		},
		{
			name:    "error - limit not found",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					GetLimit(gomock.Any(), validID).
					Return(nil, constant.ErrLimitNotFound)

				return mockService
			},
			expectedStatus: http.StatusNotFound,
			expectedBody:   func(t *testing.T, body []byte) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)
			handler := NewLimitHandler(mockService)

			app := fiber.New()
			app.Get("/limits/:id", handler.GetLimit)

			req := httptest.NewRequest(http.MethodGet, "/limits/"+tt.limitID, nil)

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			tt.expectedBody(t, body)
		})
	}
}

func TestLimitHandler_ListLimits(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(ctrl *gomock.Controller) *MockLimitService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "lists limits with default parameters and empty result",
			queryParams: "",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					ListLimits(gomock.Any(), gomock.Any()).
					Return(&model.ListLimitsResult{
						Limits:     []model.Limit{},
						NextCursor: "",
						HasMore:    false,
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListLimitsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Empty(t, response.Limits)
				assert.False(t, response.HasMore)
				assert.Empty(t, response.NextCursor)
			},
		},
		{
			name:        "status filter ACTIVE returns matching limits with all fields",
			queryParams: "?status=ACTIVE",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					ListLimits(gomock.Any(), gomock.Any()).
					Return(&model.ListLimitsResult{
						Limits: []model.Limit{
							{
								ID:        testutil.MustDeterministicUUID(20),
								Name:      "Active Limit",
								LimitType: model.LimitTypeDaily,
								Status:    model.LimitStatusActive,
							},
						},
						NextCursor: "",
						HasMore:    false,
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListLimitsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Len(t, response.Limits, 1)
				assert.Equal(t, testutil.MustDeterministicUUID(20), response.Limits[0].ID)
				assert.Equal(t, "Active Limit", response.Limits[0].Name)
				assert.Equal(t, model.LimitStatusActive, response.Limits[0].Status)
				assert.False(t, response.HasMore)
			},
		},
		{
			name:        "status filter DRAFT returns limits with correct status",
			queryParams: "?status=DRAFT",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					ListLimits(gomock.Any(), gomock.Any()).
					Return(&model.ListLimitsResult{
						Limits: []model.Limit{
							{
								ID:        testutil.MustDeterministicUUID(21),
								Name:      "Draft Limit",
								LimitType: model.LimitTypeDaily,
								Status:    model.LimitStatusDraft,
							},
						},
						NextCursor: "",
						HasMore:    false,
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListLimitsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Len(t, response.Limits, 1)
				assert.Equal(t, testutil.MustDeterministicUUID(21), response.Limits[0].ID)
				assert.Equal(t, model.LimitStatusDraft, response.Limits[0].Status)
				assert.False(t, response.HasMore)
			},
		},
		{
			name:        "invalid status filter returns validation error",
			queryParams: "?status=INVALID",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "status")
			},
		},
		{
			name:        "name filter passes value to service filter",
			queryParams: "?name=Daily",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					ListLimits(gomock.Any(), gomock.Cond(func(x any) bool {
						f, ok := x.(*model.ListLimitsFilter)
						return ok && f.Name != nil && *f.Name == "Daily"
					})).
					Return(&model.ListLimitsResult{
						Limits:  []model.Limit{},
						HasMore: false,
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListLimitsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Empty(t, response.Limits)
				assert.False(t, response.HasMore)
			},
		},
		{
			name:        "accountId scope filter passes UUID to service filter",
			queryParams: "?account_id=" + testutil.MustDeterministicUUID(1).String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				expectedAccountID := testutil.MustDeterministicUUID(1)
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					ListLimits(gomock.Any(), gomock.Cond(func(x any) bool {
						f, ok := x.(*model.ListLimitsFilter)
						return ok && f.ScopeFilter != nil && f.ScopeFilter.AccountID != nil &&
							*f.ScopeFilter.AccountID == expectedAccountID
					})).
					Return(&model.ListLimitsResult{
						Limits:  []model.Limit{},
						HasMore: false,
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListLimitsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Empty(t, response.Limits)
				assert.False(t, response.HasMore)
			},
		},
		{
			name:        "combined name, scope, and status filters pass all values to service",
			queryParams: "?name=Monthly&transaction_type=PIX&status=ACTIVE",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					ListLimits(gomock.Any(), gomock.Cond(func(x any) bool {
						f, ok := x.(*model.ListLimitsFilter)
						return ok && f.Name != nil && *f.Name == "Monthly" &&
							f.ScopeFilter != nil && f.ScopeFilter.TransactionType != nil &&
							string(*f.ScopeFilter.TransactionType) == "PIX" &&
							f.Status != nil && *f.Status == model.LimitStatusActive
					})).
					Return(&model.ListLimitsResult{
						Limits:  []model.Limit{},
						HasMore: false,
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListLimitsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Empty(t, response.Limits)
				assert.False(t, response.HasMore)
			},
		},
		{
			name:        "name filter at max length boundary (255 chars) is accepted",
			queryParams: "?name=" + strings.Repeat("a", MaxLimitNameFilterLength),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				expectedName := strings.Repeat("a", MaxLimitNameFilterLength)
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					ListLimits(gomock.Any(), gomock.Cond(func(x any) bool {
						f, ok := x.(*model.ListLimitsFilter)
						return ok && f.Name != nil && *f.Name == expectedName
					})).
					Return(&model.ListLimitsResult{
						Limits:  []model.Limit{},
						HasMore: false,
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListLimitsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Empty(t, response.Limits)
				assert.False(t, response.HasMore)
			},
		},
		{
			name:        "name filter exceeding max length (256 chars) returns validation error",
			queryParams: "?name=" + strings.Repeat("a", MaxLimitNameFilterLength+1),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "TRC-0006")
				assert.Contains(t, string(body), "name filter exceeds maximum length")
			},
		},
		{
			name:        "invalid accountId UUID in scope filter returns validation error",
			queryParams: "?account_id=not-a-uuid",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "TRC-0006")
				assert.Contains(t, string(body), "account_id")
			},
		},
		{
			name:        "invalid transactionType enum in scope filter returns validation error",
			queryParams: "?transaction_type=INVALID_TYPE",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "TRC-0006")
				assert.Contains(t, string(body), "transaction_type")
			},
		},
		{
			name:        "limit_type filter WEEKLY passes value to service filter",
			queryParams: "?limit_type=WEEKLY",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					ListLimits(gomock.Any(), gomock.Cond(func(x any) bool {
						f, ok := x.(*model.ListLimitsFilter)
						return ok && f.LimitType != nil && *f.LimitType == model.LimitTypeWeekly
					})).
					Return(&model.ListLimitsResult{
						Limits: []model.Limit{
							{
								ID:        testutil.MustDeterministicUUID(25),
								Name:      "Weekly Limit",
								LimitType: model.LimitTypeWeekly,
								Status:    model.LimitStatusActive,
							},
						},
						HasMore: false,
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListLimitsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				require.Len(t, response.Limits, 1)
				assert.Equal(t, model.LimitTypeWeekly, response.Limits[0].LimitType)
			},
		},
		{
			name:        "limit_type filter CUSTOM passes value to service filter",
			queryParams: "?limit_type=CUSTOM",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					ListLimits(gomock.Any(), gomock.Cond(func(x any) bool {
						f, ok := x.(*model.ListLimitsFilter)
						return ok && f.LimitType != nil && *f.LimitType == model.LimitTypeCustom
					})).
					Return(&model.ListLimitsResult{
						Limits: []model.Limit{
							{
								ID:        testutil.MustDeterministicUUID(26),
								Name:      "Custom Limit",
								LimitType: model.LimitTypeCustom,
								Status:    model.LimitStatusActive,
							},
						},
						HasMore: false,
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListLimitsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				require.Len(t, response.Limits, 1)
				assert.Equal(t, model.LimitTypeCustom, response.Limits[0].LimitType)
			},
		},
		{
			name:        "invalid cursor returns bad request error",
			queryParams: "?cursor=invalid",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					ListLimits(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInvalidCursor)

				return mockService
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "cursor")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)
			handler := NewLimitHandler(mockService)

			app := fiber.New()
			app.Get("/limits", handler.ListLimits)

			req := httptest.NewRequest(http.MethodGet, "/limits"+tt.queryParams, nil)

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			tt.expectedBody(t, body)
		})
	}
}

func TestLimitHandler_UpdateLimit(t *testing.T) {
	validID := testutil.MustDeterministicUUID(30)

	tests := []struct {
		name           string
		limitID        string
		requestBody    any
		mockSetup      func(ctrl *gomock.Controller) *MockLimitService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:    "success - updates limit name",
			limitID: validID.String(),
			requestBody: map[string]any{
				"name": "Updated Limit Name",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					UpdateLimit(gomock.Any(), validID, gomock.Any()).
					Return(&model.Limit{
						ID:        validID,
						Name:      "Updated Limit Name",
						LimitType: model.LimitTypeDaily,
						MaxAmount: decimal.RequireFromString("1000"),
						Currency:  "BRL",
						Status:    model.LimitStatusActive,
						CreatedAt: testutil.FixedTime(),
						UpdatedAt: testutil.FixedTime(),
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response map[string]any
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Updated Limit Name", response["name"])
			},
		},
		{
			name:    "success - updates maxAmount",
			limitID: validID.String(),
			requestBody: map[string]any{
				"maxAmount": "2000.00",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					UpdateLimit(gomock.Any(), validID, gomock.Any()).
					Return(&model.Limit{
						ID:        validID,
						Name:      "Test Limit",
						LimitType: model.LimitTypeDaily,
						MaxAmount: decimal.RequireFromString("2000"),
						Currency:  "BRL",
						Status:    model.LimitStatusActive,
						CreatedAt: testutil.FixedTime(),
						UpdatedAt: testutil.FixedTime(),
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response map[string]any
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "2000", response["maxAmount"])
			},
		},
		{
			name:        "error - empty body (no fields to update)",
			limitID:     validID.String(),
			requestBody: map[string]any{},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "At least one field")
			},
		},
		{
			name:    "error - invalid UUID",
			limitID: "invalid-uuid",
			requestBody: map[string]any{
				"name": "Test",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "Invalid limit ID")
			},
		},
		{
			name:    "error - limit not found",
			limitID: validID.String(),
			requestBody: map[string]any{
				"name": "Test",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					UpdateLimit(gomock.Any(), validID, gomock.Any()).
					Return(nil, constant.ErrLimitNotFound)

				return mockService
			},
			expectedStatus: http.StatusNotFound,
			expectedBody:   func(t *testing.T, body []byte) {},
		},
		{
			name:    "error - limit already deleted",
			limitID: validID.String(),
			requestBody: map[string]any{
				"name": "Test",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					UpdateLimit(gomock.Any(), validID, gomock.Any()).
					Return(nil, constant.ErrLimitAlreadyDeleted)

				return mockService
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "deleted")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)
			handler := NewLimitHandler(mockService)

			app := fiber.New()
			app.Patch("/limits/:id", handler.UpdateLimit)

			var bodyBytes []byte
			switch v := tt.requestBody.(type) {
			case string:
				bodyBytes = []byte(v)
			default:
				var err error
				bodyBytes, err = json.Marshal(v)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPatch, "/limits/"+tt.limitID, bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			tt.expectedBody(t, body)
		})
	}
}

func TestLimitHandler_ActivateLimit(t *testing.T) {
	validID := testutil.MustDeterministicUUID(40)

	tests := []struct {
		name           string
		limitID        string
		mockSetup      func(ctrl *gomock.Controller) *MockLimitService
		expectedStatus int
	}{
		{
			name:    "success - activates limit",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					ActivateLimit(gomock.Any(), validID).
					Return(&model.Limit{
						ID:     validID,
						Name:   "Test Limit",
						Status: model.LimitStatusActive,
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "error - invalid UUID",
			limitID: "invalid-uuid",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:    "error - limit not found",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					ActivateLimit(gomock.Any(), validID).
					Return(nil, constant.ErrLimitNotFound)

				return mockService
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:    "error - invalid status transition",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					ActivateLimit(gomock.Any(), validID).
					Return(nil, constant.ErrLimitInvalidStatusChange)

				return mockService
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)
			handler := NewLimitHandler(mockService)

			app := fiber.New()
			app.Post("/limits/:id/activate", handler.ActivateLimit)

			req := httptest.NewRequest(http.MethodPost, "/limits/"+tt.limitID+"/activate", nil)

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestLimitHandler_DeactivateLimit(t *testing.T) {
	validID := testutil.MustDeterministicUUID(50)

	tests := []struct {
		name           string
		limitID        string
		mockSetup      func(ctrl *gomock.Controller) *MockLimitService
		expectedStatus int
	}{
		{
			name:    "success - deactivates limit",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					DeactivateLimit(gomock.Any(), validID).
					Return(&model.Limit{
						ID:     validID,
						Name:   "Test Limit",
						Status: model.LimitStatusInactive,
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "error - invalid UUID",
			limitID: "invalid-uuid",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:    "error - limit not found",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					DeactivateLimit(gomock.Any(), validID).
					Return(nil, constant.ErrLimitNotFound)

				return mockService
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)
			handler := NewLimitHandler(mockService)

			app := fiber.New()
			app.Post("/limits/:id/deactivate", handler.DeactivateLimit)

			req := httptest.NewRequest(http.MethodPost, "/limits/"+tt.limitID+"/deactivate", nil)

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestLimitHandler_DeleteLimit(t *testing.T) {
	validID := testutil.MustDeterministicUUID(60)

	tests := []struct {
		name           string
		limitID        string
		mockSetup      func(ctrl *gomock.Controller) *MockLimitService
		expectedStatus int
	}{
		{
			name:    "success - deletes limit",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					DeleteLimit(gomock.Any(), validID).
					Return(nil)

				return mockService
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:    "error - invalid UUID",
			limitID: "invalid-uuid",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:    "error - limit not found",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					DeleteLimit(gomock.Any(), validID).
					Return(constant.ErrLimitNotFound)

				return mockService
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:    "error - limit already deleted",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					DeleteLimit(gomock.Any(), validID).
					Return(constant.ErrLimitAlreadyDeleted)

				return mockService
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)
			handler := NewLimitHandler(mockService)

			app := fiber.New()
			app.Delete("/limits/:id", handler.DeleteLimit)

			req := httptest.NewRequest(http.MethodDelete, "/limits/"+tt.limitID, nil)

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestLimitHandler_DraftLimit(t *testing.T) {
	validID := testutil.MustDeterministicUUID(65)

	tests := []struct {
		name           string
		limitID        string
		mockSetup      func(ctrl *gomock.Controller) *MockLimitService
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:    "success - transitions limit to draft",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					DraftLimit(gomock.Any(), validID).
					Return(&model.Limit{
						ID:     validID,
						Name:   "Test Limit",
						Status: model.LimitStatusDraft,
					}, nil)

				return mockService
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response map[string]any
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, validID.String(), response["limitId"])
				assert.Equal(t, "Test Limit", response["name"])
				assert.Equal(t, "DRAFT", response["status"])
			},
		},
		{
			name:    "error - invalid UUID",
			limitID: "invalid-uuid",
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				return NewMockLimitService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:    "error - limit not found",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					DraftLimit(gomock.Any(), validID).
					Return(nil, constant.ErrLimitNotFound)

				return mockService
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:    "error - invalid status transition",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					DraftLimit(gomock.Any(), validID).
					Return(nil, constant.ErrLimitInvalidStatusChange)

				return mockService
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:    "error - internal server error",
			limitID: validID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockLimitService {
				mockService := NewMockLimitService(ctrl)
				mockService.EXPECT().
					DraftLimit(gomock.Any(), validID).
					Return(nil, errors.New("unexpected error"))

				return mockService
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)
			handler := NewLimitHandler(mockService)

			app := fiber.New()
			app.Post("/limits/:id/draft", handler.DraftLimit)

			req := httptest.NewRequest(http.MethodPost, "/limits/"+tt.limitID+"/draft", nil)

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

func TestToCreateLimitServiceInput(t *testing.T) {
	accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	txType := model.TransactionTypeCard
	desc := "Test Description"

	input := &CreateLimitInput{
		Name:        "Test Limit",
		Description: &desc,
		LimitType:   model.LimitTypeDaily,
		MaxAmount:   decimal.RequireFromString("1000"),
		Currency:    "BRL",
		Scopes: []model.Scope{
			{
				AccountID:       &accountID,
				TransactionType: &txType,
			},
		},
	}

	result := ToCreateLimitServiceInput(input)

	assert.Equal(t, input.Name, result.Name)
	assert.Equal(t, input.Description, result.Description)
	assert.Equal(t, input.LimitType, result.LimitType)
	assert.Equal(t, input.MaxAmount, result.MaxAmount)
	assert.Equal(t, input.Currency, result.Currency)
	assert.Len(t, result.Scopes, 1)
	assert.Equal(t, accountID, *result.Scopes[0].AccountID)
	assert.Equal(t, &txType, result.Scopes[0].TransactionType)
}

func TestToUpdateLimitServiceInput(t *testing.T) {
	name := "Updated Name"
	amount := decimal.RequireFromString("2000")
	accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	input := &UpdateLimitInput{
		Name:      &name,
		MaxAmount: &amount,
		Scopes: &[]model.Scope{
			{AccountID: &accountID},
		},
	}

	result := ToUpdateLimitServiceInput(input)

	assert.Equal(t, &name, result.Name)
	assert.Equal(t, &amount, result.MaxAmount)
	assert.NotNil(t, result.Scopes)
	assert.Len(t, *result.Scopes, 1)
}

func TestToListLimitsFilter(t *testing.T) {
	input := &ListLimitsInput{
		Limit:     testutil.Ptr(20),
		Cursor:    "abc123",
		Status:    "ACTIVE",
		LimitType: "DAILY",
		SortBy:    "created_at",
		SortOrder: "desc",
	}

	result := ToListLimitsFilter(input)

	assert.Equal(t, 20, result.Limit)
	assert.Equal(t, "abc123", result.Cursor)
	assert.NotNil(t, result.Status)
	assert.Equal(t, model.LimitStatusActive, *result.Status)
	assert.NotNil(t, result.LimitType)
	assert.Equal(t, model.LimitTypeDaily, *result.LimitType)
	assert.Equal(t, "created_at", result.SortBy)
	assert.Equal(t, "DESC", result.SortOrder)
}

func TestToListLimitsResponse(t *testing.T) {
	result := &model.ListLimitsResult{
		Limits: []model.Limit{
			{ID: testutil.MustDeterministicUUID(70), Name: "Limit 1"},
			{ID: testutil.MustDeterministicUUID(71), Name: "Limit 2"},
		},
		NextCursor: "next123",
		HasMore:    true,
	}

	response := ToListLimitsResponse(result)

	assert.Len(t, response.Limits, 2)
	assert.Equal(t, "next123", response.NextCursor)
	assert.True(t, response.HasMore)
}

func TestCreateLimitInput_Validate(t *testing.T) {
	tests := []struct {
		name        string
		input       CreateLimitInput
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid input",
			input: CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "BRL",
				Scopes: []model.Scope{
					{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(80))},
				},
			},
			expectError: false,
		},
		{
			name: "scope with empty fields",
			input: CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "BRL",
				Scopes: []model.Scope{
					{}, // Empty scope
				},
			},
			expectError: true,
			errorMsg:    "scope",
		},
		{
			name: "name too long",
			input: CreateLimitInput{
				Name:      string(make([]byte, 300)),
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "BRL",
				Scopes: []model.Scope{
					{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(81))},
				},
			},
			expectError: true,
			errorMsg:    "name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdateLimitInput_Validate(t *testing.T) {
	tests := []struct {
		name        string
		input       UpdateLimitInput
		expectError bool
	}{
		{
			name: "valid partial update - name only",
			input: UpdateLimitInput{
				Name: func() *string { s := "Updated"; return &s }(),
			},
			expectError: false,
		},
		{
			name: "valid partial update - amount only",
			input: UpdateLimitInput{
				MaxAmount: func() *decimal.Decimal { a := decimal.RequireFromString("2000"); return &a }(),
			},
			expectError: false,
		},
		{
			name:        "empty update (all nil) is valid for validation",
			input:       UpdateLimitInput{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListLimitsInput_SetDefaults(t *testing.T) {
	input := &ListLimitsInput{}
	input.SetDefaults()

	assert.Equal(t, 10, *input.Limit) // constant.DefaultPaginationLimit
	assert.Equal(t, "created_at", input.SortBy)
	assert.Equal(t, "DESC", input.SortOrder)
}

func TestListLimitsInput_SetDefaults_WithCursor_DoesNotPopulateSortFields(t *testing.T) {
	input := &ListLimitsInput{
		Cursor: "some-cursor-value",
	}
	input.SetDefaults()

	assert.Equal(t, 10, *input.Limit, "Limit should be defaulted")
	assert.Empty(t, input.SortBy, "SortBy should remain empty when cursor is present")
	assert.Empty(t, input.SortOrder, "SortOrder should remain empty when cursor is present")
}

func TestListLimitsInput_Validate(t *testing.T) {
	tests := []struct {
		name        string
		input       ListLimitsInput
		expectError bool
	}{
		{
			name: "valid - status ACTIVE",
			input: ListLimitsInput{
				Status: "ACTIVE",
				Limit:  testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - status INACTIVE",
			input: ListLimitsInput{
				Status: "INACTIVE",
				Limit:  testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - limitType DAILY",
			input: ListLimitsInput{
				LimitType: "DAILY",
				Limit:     testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - limitType MONTHLY",
			input: ListLimitsInput{
				LimitType: "MONTHLY",
				Limit:     testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "valid - limitType PER_TRANSACTION",
			input: ListLimitsInput{
				LimitType: "PER_TRANSACTION",
				Limit:     testutil.Ptr(10),
			},
			expectError: false,
		},
		{
			name: "error - invalid sortBy",
			input: ListLimitsInput{
				SortBy: "invalid_column",
				Limit:  testutil.Ptr(10),
			},
			expectError: true,
		},
		{
			name: "error - invalid sortOrder",
			input: ListLimitsInput{
				SortOrder: "RANDOM",
				Limit:     testutil.Ptr(10),
			},
			expectError: true,
		},
		{
			name: "error - cursor with sortBy rejected (TRC-0045)",
			input: ListLimitsInput{
				Cursor: "abc123",
				SortBy: "created_at",
				Limit:  testutil.Ptr(10),
			},
			expectError: true,
		},
		{
			name: "error - cursor with sortOrder rejected (TRC-0045)",
			input: ListLimitsInput{
				Cursor:    "abc123",
				SortOrder: "DESC",
				Limit:     testutil.Ptr(10),
			},
			expectError: true,
		},
		{
			name: "error - cursor with both sortBy and sortOrder rejected (TRC-0045)",
			input: ListLimitsInput{
				Cursor:    "abc123",
				SortBy:    "created_at",
				SortOrder: "DESC",
				Limit:     testutil.Ptr(10),
			},
			expectError: true,
		},
		{
			name: "valid - cursor alone",
			input: ListLimitsInput{
				Cursor: "abc123",
				Limit:  testutil.Ptr(10),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdateLimitInput_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		input    UpdateLimitInput
		expected bool
	}{
		{
			name:     "empty input",
			input:    UpdateLimitInput{},
			expected: true,
		},
		{
			name: "input with name",
			input: UpdateLimitInput{
				Name: func() *string { s := "test"; return &s }(),
			},
			expected: false,
		},
		{
			name: "input with maxAmount",
			input: UpdateLimitInput{
				MaxAmount: func() *decimal.Decimal { a := decimal.RequireFromString("1"); return &a }(),
			},
			expected: false,
		},
		{
			name: "input with description",
			input: UpdateLimitInput{
				Description: func() *string { s := "desc"; return &s }(),
			},
			expected: false,
		},
		{
			name: "input with scopes",
			input: UpdateLimitInput{
				Scopes: &[]model.Scope{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.IsEmpty()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateScopeFieldErrors(t *testing.T) {
	// These tests trigger the scope field validation error formatting (formatScopeFieldError)
	tests := []struct {
		name        string
		input       CreateLimitInput
		expectError bool
		errorMsg    string
	}{
		{
			name: "scope with invalid transactionType triggers transactiontype validator",
			input: CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "BRL",
				Scopes: []model.Scope{
					{
						TransactionType: func() *model.TransactionType {
							t := model.TransactionType("INVALID_TX_TYPE")
							return &t
						}(),
					},
				},
			},
			expectError: true,
			errorMsg:    "transactionType",
		},
		{
			name: "scope at index 1 with empty fields",
			input: CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "BRL",
				Scopes: []model.Scope{
					{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(90))}, // Valid scope at index 0
					{}, // Empty scope at index 1
				},
			},
			expectError: true,
			errorMsg:    "scope at index 1",
		},
		{
			name: "scope with subType too long",
			input: CreateLimitInput{
				Name:      "Test Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "BRL",
				Scopes: []model.Scope{
					{
						AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(100)),
						SubType:   func() *string { s := string(make([]byte, 100)); return &s }(),
					},
				},
			},
			expectError: true,
			errorMsg:    "subType",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLimitHandler_ServiceErrorHandling(t *testing.T) {
	validID := testutil.MustDeterministicUUID(110)

	tests := []struct {
		name           string
		method         string // HTTP method: GET or POST
		setupMock      func(mockService *MockLimitService)
		request        func() *http.Request
		expectedStatus int
		expectedBody   string
	}{
		{
			name:   "ErrLimitNameRequired",
			method: "POST",
			setupMock: func(mockService *MockLimitService) {
				mockService.EXPECT().
					CreateLimit(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrLimitNameRequired)
			},
			request: func() *http.Request {
				body, err := json.Marshal(map[string]any{
					"name":      "Test",
					"limitType": "DAILY",
					"maxAmount": "1000.00",
					"currency":  "BRL",
					"scopes":    []map[string]any{{"accountId": testutil.MustDeterministicUUID(120).String()}},
				})
				require.NoError(t, err)

				return httptest.NewRequest(http.MethodPost, "/limits", bytes.NewReader(body))
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "name is required",
		},
		{
			name:   "ErrLimitInvalidType",
			method: "POST",
			setupMock: func(mockService *MockLimitService) {
				mockService.EXPECT().
					CreateLimit(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrLimitInvalidType)
			},
			request: func() *http.Request {
				body, err := json.Marshal(map[string]any{
					"name":      "Test",
					"limitType": "DAILY",
					"maxAmount": "1000.00",
					"currency":  "BRL",
					"scopes":    []map[string]any{{"accountId": testutil.MustDeterministicUUID(120).String()}},
				})
				require.NoError(t, err)

				return httptest.NewRequest(http.MethodPost, "/limits", bytes.NewReader(body))
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "limitType",
		},
		{
			name:   "ErrLimitInvalidMaxAmount",
			method: "POST",
			setupMock: func(mockService *MockLimitService) {
				mockService.EXPECT().
					CreateLimit(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrLimitInvalidMaxAmount)
			},
			request: func() *http.Request {
				body, err := json.Marshal(map[string]any{
					"name":      "Test",
					"limitType": "DAILY",
					"maxAmount": "1000.00",
					"currency":  "BRL",
					"scopes":    []map[string]any{{"accountId": testutil.MustDeterministicUUID(120).String()}},
				})
				require.NoError(t, err)

				return httptest.NewRequest(http.MethodPost, "/limits", bytes.NewReader(body))
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "maxAmount",
		},
		{
			name:   "ErrLimitInvalidCurrency",
			method: "POST",
			setupMock: func(mockService *MockLimitService) {
				mockService.EXPECT().
					CreateLimit(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrLimitInvalidCurrency)
			},
			request: func() *http.Request {
				body, err := json.Marshal(map[string]any{
					"name":      "Test",
					"limitType": "DAILY",
					"maxAmount": "1000.00",
					"currency":  "BRL",
					"scopes":    []map[string]any{{"accountId": testutil.MustDeterministicUUID(120).String()}},
				})
				require.NoError(t, err)

				return httptest.NewRequest(http.MethodPost, "/limits", bytes.NewReader(body))
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "currency",
		},
		{
			name:   "ErrLimitInvalidScope",
			method: "POST",
			setupMock: func(mockService *MockLimitService) {
				mockService.EXPECT().
					CreateLimit(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrLimitInvalidScope)
			},
			request: func() *http.Request {
				body, err := json.Marshal(map[string]any{
					"name":      "Test",
					"limitType": "DAILY",
					"maxAmount": "1000.00",
					"currency":  "BRL",
					"scopes":    []map[string]any{{"accountId": testutil.MustDeterministicUUID(120).String()}},
				})
				require.NoError(t, err)

				return httptest.NewRequest(http.MethodPost, "/limits", bytes.NewReader(body))
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "scopes",
		},
		{
			name:   "ErrLimitNameTooLong",
			method: "POST",
			setupMock: func(mockService *MockLimitService) {
				mockService.EXPECT().
					CreateLimit(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrLimitNameTooLong)
			},
			request: func() *http.Request {
				body, err := json.Marshal(map[string]any{
					"name":      "Test",
					"limitType": "DAILY",
					"maxAmount": "1000.00",
					"currency":  "BRL",
					"scopes":    []map[string]any{{"accountId": testutil.MustDeterministicUUID(120).String()}},
				})
				require.NoError(t, err)

				return httptest.NewRequest(http.MethodPost, "/limits", bytes.NewReader(body))
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "name",
		},
		{
			name:   "ErrLimitNameInvalidChars",
			method: "POST",
			setupMock: func(mockService *MockLimitService) {
				mockService.EXPECT().
					CreateLimit(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrLimitNameInvalidChars)
			},
			request: func() *http.Request {
				body, err := json.Marshal(map[string]any{
					"name":      "Test",
					"limitType": "DAILY",
					"maxAmount": "1000.00",
					"currency":  "BRL",
					"scopes":    []map[string]any{{"accountId": testutil.MustDeterministicUUID(120).String()}},
				})
				require.NoError(t, err)

				return httptest.NewRequest(http.MethodPost, "/limits", bytes.NewReader(body))
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "name",
		},
		{
			name:   "ErrLimitDescriptionInvalidChars",
			method: "POST",
			setupMock: func(mockService *MockLimitService) {
				mockService.EXPECT().
					CreateLimit(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrLimitDescriptionInvalidChars)
			},
			request: func() *http.Request {
				body, err := json.Marshal(map[string]any{
					"name":      "Test",
					"limitType": "DAILY",
					"maxAmount": "1000.00",
					"currency":  "BRL",
					"scopes":    []map[string]any{{"accountId": testutil.MustDeterministicUUID(120).String()}},
				})
				require.NoError(t, err)

				return httptest.NewRequest(http.MethodPost, "/limits", bytes.NewReader(body))
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "description",
		},
		{
			name:   "ErrInvalidSortOrder on ListLimits",
			method: "GET",
			setupMock: func(mockService *MockLimitService) {
				mockService.EXPECT().
					ListLimits(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInvalidSortOrder)
			},
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/limits", nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "ErrInvalidSortColumn on ListLimits",
			method: "GET",
			setupMock: func(mockService *MockLimitService) {
				mockService.EXPECT().
					ListLimits(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInvalidSortColumn)
			},
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/limits", nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := NewMockLimitService(ctrl)
			tt.setupMock(mockService)

			app := fiber.New()
			handler := NewLimitHandler(mockService)

			// Register route based on explicit method field
			if tt.method == "GET" {
				app.Get("/limits", handler.ListLimits)
			} else if tt.method == "POST" {
				app.Post("/limits", handler.CreateLimit)
			} else {
				t.Fatalf("unsupported method in test case: %s", tt.method)
			}

			req := tt.request()
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedBody != "" {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.Contains(t, string(body), tt.expectedBody)
			}
		})
	}

	// Test for deactivate with invalid status transition
	t.Run("ErrLimitInvalidStatusChange on Deactivate", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		mockService := NewMockLimitService(ctrl)
		mockService.EXPECT().
			DeactivateLimit(gomock.Any(), validID).
			Return(nil, constant.ErrLimitInvalidStatusChange)

		app := fiber.New()
		handler := NewLimitHandler(mockService)
		app.Post("/limits/:id/deactivate", handler.DeactivateLimit)

		req := httptest.NewRequest(http.MethodPost, "/limits/"+validID.String()+"/deactivate", nil)

		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestLimitHandler_GetLimitUsage(t *testing.T) {
	validID := testutil.MustDeterministicUUID(130)
	// Use a fixed time for deterministic tests
	resetAt := testutil.FixedTime().Add(24 * time.Hour)

	tests := []struct {
		name           string
		limitID        string
		setupMock      func(*MockLimitService)
		expectedStatus int
		expectedBody   string
		validateJSON   func(t *testing.T, body []byte)
	}{
		{
			name:    "success - gets usage snapshot with 50% utilization",
			limitID: validID.String(),
			setupMock: func(m *MockLimitService) {
				snapshot := &model.UsageSnapshot{
					LimitID:            validID,
					CurrentUsage:       decimal.RequireFromString("500"),
					LimitAmount:        decimal.RequireFromString("1000"),
					UtilizationPercent: 50.0,
					NearLimit:          false,
					ResetAt:            &resetAt,
				}
				m.EXPECT().
					GetLimitUsage(gomock.Any(), validID).
					Return(snapshot, nil)
			},
			expectedStatus: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var response map[string]any
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)

				// Verify snapshot fields
				assert.Equal(t, validID.String(), response["limitId"])
				assert.Equal(t, "500", response["currentUsage"])
				assert.Equal(t, "1000", response["limitAmount"])
				assert.Equal(t, 50.0, response["utilizationPercent"])
				assert.Equal(t, false, response["nearLimit"])
				assert.NotNil(t, response["resetAt"])
			},
		},
		{
			name:    "success - gets usage snapshot near limit (>80%)",
			limitID: validID.String(),
			setupMock: func(m *MockLimitService) {
				snapshot := &model.UsageSnapshot{
					LimitID:            validID,
					CurrentUsage:       decimal.RequireFromString("850"),
					LimitAmount:        decimal.RequireFromString("1000"),
					UtilizationPercent: 85.0,
					NearLimit:          true,
					ResetAt:            &resetAt,
				}
				m.EXPECT().
					GetLimitUsage(gomock.Any(), validID).
					Return(snapshot, nil)
			},
			expectedStatus: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var response map[string]any
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)

				// Verify nearLimit is true when >80%
				assert.Equal(t, validID.String(), response["limitId"])
				assert.Equal(t, "850", response["currentUsage"])
				assert.Equal(t, 85.0, response["utilizationPercent"])
				assert.Equal(t, true, response["nearLimit"])
			},
		},
		{
			name:    "success - gets usage snapshot for PER_TRANSACTION (zero usage, no resetAt)",
			limitID: validID.String(),
			setupMock: func(m *MockLimitService) {
				snapshot := &model.UsageSnapshot{
					LimitID:            validID,
					CurrentUsage:       decimal.RequireFromString("0"),
					LimitAmount:        decimal.RequireFromString("1000"),
					UtilizationPercent: 0.0,
					NearLimit:          false,
					ResetAt:            nil, // PER_TRANSACTION has no reset
				}
				m.EXPECT().
					GetLimitUsage(gomock.Any(), validID).
					Return(snapshot, nil)
			},
			expectedStatus: http.StatusOK,
			validateJSON: func(t *testing.T, body []byte) {
				var response map[string]any
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)

				// Verify PER_TRANSACTION: zero usage and no resetAt
				assert.Equal(t, validID.String(), response["limitId"])
				assert.Equal(t, "0", response["currentUsage"])
				assert.Equal(t, 0.0, response["utilizationPercent"])
				assert.Equal(t, false, response["nearLimit"])
				_, hasResetAt := response["resetAt"]
				assert.False(t, hasResetAt, "resetAt should be omitted for PER_TRANSACTION")
			},
		},
		{
			name:           "error - invalid UUID",
			limitID:        "not-a-uuid",
			setupMock:      func(m *MockLimitService) {},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid limit ID format",
		},
		{
			name:           "error - empty UUID string",
			limitID:        "",
			setupMock:      func(m *MockLimitService) {},
			expectedStatus: http.StatusNotFound, // Fiber returns 404 for /limits//usage as route doesn't match
			expectedBody:   "Cannot GET",
		},
		{
			name:    "error - limit not found",
			limitID: validID.String(),
			setupMock: func(m *MockLimitService) {
				m.EXPECT().
					GetLimitUsage(gomock.Any(), validID).
					Return(nil, constant.ErrLimitNotFound)
			},
			expectedStatus: http.StatusNotFound,
			expectedBody:   "Limit not found",
		},
		{
			name:    "error - internal server error",
			limitID: validID.String(),
			setupMock: func(m *MockLimitService) {
				m.EXPECT().
					GetLimitUsage(gomock.Any(), validID).
					Return(nil, errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "An unexpected error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := NewMockLimitService(ctrl)
			tt.setupMock(mockService)

			app := fiber.New()
			handler := NewLimitHandler(mockService)
			app.Get("/limits/:id/usage", handler.GetLimitUsage)

			req := httptest.NewRequest(http.MethodGet, "/limits/"+tt.limitID+"/usage", nil)

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if tt.validateJSON != nil {
				tt.validateJSON(t, body)
			} else if tt.expectedBody != "" {
				assert.Contains(t, string(body), tt.expectedBody)
			}
		})
	}
}
