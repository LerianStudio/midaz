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
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"tracer/internal/adapters/http/in/mocks"
	"tracer/internal/services/query"
	"tracer/internal/testutil"
	"tracer/pkg/constant"
	"tracer/pkg/model"
)

// validationErrorResponse represents the standard error response format for structured assertions.
// Note: libHTTP uses "message" while local pkg/net/http uses different fields.
type validationErrorResponse struct {
	Code    string `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"` // Used by libHTTP NotFound/InternalServerError
	Detail  string `json:"detail"`  // Used by some error handlers
}

// parseStructuredErrorResponse parses the response body into a structured error response.
// Returns the parsed response and asserts no parse errors.
// The message field contains the detailed error description from libHTTP.
func parseStructuredErrorResponse(t *testing.T, body []byte) validationErrorResponse {
	t.Helper()
	var errResp validationErrorResponse
	err := json.Unmarshal(body, &errResp)
	require.NoError(t, err, "failed to parse error response: %s", string(body))
	return errResp
}

// assertStringErrorContains asserts the body contains the expected string.
// Used for BadRequest responses that return simple string messages.
func assertStringErrorContains(t *testing.T, body []byte, expected string) {
	t.Helper()
	bodyStr := string(body)
	assert.Contains(t, bodyStr, expected, "error message should contain: %s", expected)
}

func TestTransactionValidationHandler_GetTransactionValidation(t *testing.T) {
	auditID := testutil.MustDeterministicUUID(1)
	now := testutil.FixedTime().UTC()

	tests := []struct {
		name           string
		auditID        string
		mockSetup      func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:    "success - returns audit",
			auditID: auditID.String(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				mockService := mocks.NewMockTransactionValidationService(ctrl)
				mockService.EXPECT().
					GetTransactionValidation(gomock.Any(), auditID).
					Return(&model.TransactionValidation{
						ID: auditID,
						EvaluationResult: model.EvaluationResult{
							Decision:         model.DecisionAllow,
							MatchedRuleIDs:   []uuid.UUID{},
							EvaluatedRuleIDs: []uuid.UUID{testutil.MustDeterministicUUID(2)},
							Reason:           "All checks passed",
						},
						LimitUsageDetails: []model.LimitUsageDetail{},
						ProcessingTimeMs:  42,
						CreatedAt:         now,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response model.TransactionValidation
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, auditID, response.ID)
				assert.Equal(t, model.DecisionAllow, response.Decision)
				// Additional response validation for completeness
				assert.Equal(t, float64(42), response.ProcessingTimeMs)
				assert.Equal(t, "All checks passed", response.Reason)
				assert.False(t, response.CreatedAt.IsZero(), "CreatedAt should not be zero")
				assert.Equal(t, now.Year(), response.CreatedAt.Year())
				assert.Equal(t, now.Month(), response.CreatedAt.Month())
				assert.Equal(t, now.Day(), response.CreatedAt.Day())
			},
		},
		{
			name:    "error - audit not found",
			auditID: auditID.String(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				mockService := mocks.NewMockTransactionValidationService(ctrl)
				mockService.EXPECT().
					GetTransactionValidation(gomock.Any(), auditID).
					Return(nil, constant.ErrTransactionValidationNotFound)
				return mockService
			},
			expectedStatus: http.StatusNotFound,
			expectedBody: func(t *testing.T, body []byte) {
				errResp := parseStructuredErrorResponse(t, body)
				assert.Equal(t, "TRC-0251", errResp.Code)
				assert.Equal(t, "Not Found", errResp.Title)
				assert.Equal(t, "Transaction validation not found", errResp.Message)
			},
		},
		{
			name:    "error - invalid UUID format",
			auditID: "invalid-uuid",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				return mocks.NewMockTransactionValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				// BadRequest returns simple string, not structured response
				assertStringErrorContains(t, body, "Invalid transaction validation ID format")
			},
		},
		{
			name:    "error - internal server error",
			auditID: auditID.String(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				mockService := mocks.NewMockTransactionValidationService(ctrl)
				mockService.EXPECT().
					GetTransactionValidation(gomock.Any(), auditID).
					Return(nil, errors.New("database connection lost"))
				return mockService
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody: func(t *testing.T, body []byte) {
				errResp := parseStructuredErrorResponse(t, body)
				assert.Equal(t, "TRC-0004", errResp.Code)
				assert.Equal(t, "Internal Server Error", errResp.Title)
				assert.Equal(t, "An unexpected error occurred", errResp.Message)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)

			handler := NewTransactionValidationHandler(mockService)

			app := fiber.New()
			app.Get("/v1/validations/:id", handler.GetTransactionValidation)

			req := httptest.NewRequest(http.MethodGet, "/v1/validations/"+tt.auditID, nil)
			req.Header.Set("Content-Type", "application/json")

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

func TestTransactionValidationHandler_ListTransactionValidations(t *testing.T) {
	now := testutil.FixedTime().UTC()
	audits := []*model.TransactionValidation{
		{
			ID: testutil.MustDeterministicUUID(10),
			EvaluationResult: model.EvaluationResult{
				Decision:         model.DecisionAllow,
				MatchedRuleIDs:   []uuid.UUID{},
				EvaluatedRuleIDs: []uuid.UUID{testutil.MustDeterministicUUID(11)},
			},
			LimitUsageDetails: []model.LimitUsageDetail{},
			ProcessingTimeMs:  35,
			CreatedAt:         now.Add(-time.Hour),
		},
		{
			ID: testutil.MustDeterministicUUID(12),
			EvaluationResult: model.EvaluationResult{
				Decision:         model.DecisionDeny,
				MatchedRuleIDs:   []uuid.UUID{testutil.MustDeterministicUUID(13)},
				EvaluatedRuleIDs: []uuid.UUID{testutil.MustDeterministicUUID(14)},
			},
			LimitUsageDetails: []model.LimitUsageDetail{},
			ProcessingTimeMs:  42,
			CreatedAt:         now.Add(-2 * time.Hour),
		},
	}

	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "success - returns audits with defaults",
			queryParams: "",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				mockService := mocks.NewMockTransactionValidationService(ctrl)
				mockService.EXPECT().
					ListTransactionValidations(gomock.Any(), gomock.Any()).
					Return(&query.ListTransactionValidationsResult{
						TransactionValidations: audits,
						NextCursor:             "",
						HasMore:                false,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListTransactionValidationsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				require.Len(t, response.TransactionValidations, 2)
				assert.False(t, response.HasMore)
				assert.Empty(t, response.NextCursor)
			},
		},
		{
			name:        "success - with limit and cursor",
			queryParams: "?limit=50&cursor=test-cursor",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				mockService := mocks.NewMockTransactionValidationService(ctrl)
				mockService.EXPECT().
					ListTransactionValidations(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx interface{}, filters *model.TransactionValidationFilters) (*query.ListTransactionValidationsResult, error) {
						assert.Equal(t, 50, filters.Limit)
						assert.Equal(t, "test-cursor", filters.Cursor)
						return &query.ListTransactionValidationsResult{
							TransactionValidations: audits,
							NextCursor:             "next-page-cursor",
							HasMore:                true,
						}, nil
					})
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListTransactionValidationsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.True(t, response.HasMore)
				assert.Equal(t, "next-page-cursor", response.NextCursor)
			},
		},
		{
			name:        "success - with sortBy and sortOrder",
			queryParams: "?sort_by=created_at&sort_order=ASC",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				mockService := mocks.NewMockTransactionValidationService(ctrl)
				mockService.EXPECT().
					ListTransactionValidations(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx interface{}, filters *model.TransactionValidationFilters) (*query.ListTransactionValidationsResult, error) {
						assert.Equal(t, "created_at", filters.SortBy)
						assert.Equal(t, "ASC", filters.SortOrder)
						return &query.ListTransactionValidationsResult{
							TransactionValidations: audits,
							NextCursor:             "",
							HasMore:                false,
						}, nil
					})
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListTransactionValidationsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				require.Len(t, response.TransactionValidations, 2)
			},
		},
		{
			name:        "success - with decision filter",
			queryParams: "?decision=DENY",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				mockService := mocks.NewMockTransactionValidationService(ctrl)
				mockService.EXPECT().
					ListTransactionValidations(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx interface{}, filters *model.TransactionValidationFilters) (*query.ListTransactionValidationsResult, error) {
						require.NotNil(t, filters.Decision)
						assert.Equal(t, model.DecisionDeny, *filters.Decision)
						return &query.ListTransactionValidationsResult{
							TransactionValidations: []*model.TransactionValidation{audits[1]},
							NextCursor:             "",
							HasMore:                false,
						}, nil
					})
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListTransactionValidationsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				require.Len(t, response.TransactionValidations, 1)
			},
		},
		{
			name:        "error - invalid decision value",
			queryParams: "?decision=INVALID",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				return mocks.NewMockTransactionValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				// BadRequest returns simple string, not structured response
				assertStringErrorContains(t, body, "decision must be one of")
			},
		},
		{
			name:        "error - negative limit",
			queryParams: "?limit=-1",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				return mocks.NewMockTransactionValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				// BadRequest returns simple string, not structured response
				assertStringErrorContains(t, body, "limit must be at least 1")
			},
		},
		{
			name:        "error - limit too high",
			queryParams: "?limit=2000",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				return mocks.NewMockTransactionValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				// BadRequest returns simple string, not structured response
				assertStringErrorContains(t, body, "limit must not exceed 1000")
			},
		},
		{
			name:        "error - invalid sortBy",
			queryParams: "?sort_by=invalid_field",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				return mocks.NewMockTransactionValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assertStringErrorContains(t, body, "sort_by must be one of")
			},
		},
		{
			name:        "error - invalid sortOrder",
			queryParams: "?sort_order=INVALID",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				return mocks.NewMockTransactionValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assertStringErrorContains(t, body, "sort_order must be ASC or DESC")
			},
		},
		{
			name:        "error - invalid date format",
			queryParams: "?start_date=invalid-date",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				return mocks.NewMockTransactionValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				// BadRequest returns simple string, not structured response
				assertStringErrorContains(t, body, "start_date must be in RFC3339 format")
			},
		},
		{
			name:        "error - invalid accountId UUID",
			queryParams: "?account_id=invalid-uuid",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				return mocks.NewMockTransactionValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				// BadRequest returns simple string, not structured response
				assertStringErrorContains(t, body, "account_id must be a valid UUID")
			},
		},
		{
			name:        "success - with segmentId filter",
			queryParams: "?segment_id=" + testutil.MustDeterministicUUID(100).String(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				mockService := mocks.NewMockTransactionValidationService(ctrl)
				mockService.EXPECT().
					ListTransactionValidations(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx interface{}, filters *model.TransactionValidationFilters) (*query.ListTransactionValidationsResult, error) {
						require.NotNil(t, filters.SegmentID)
						return &query.ListTransactionValidationsResult{
							TransactionValidations: audits,
							NextCursor:             "",
							HasMore:                false,
						}, nil
					})
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListTransactionValidationsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				require.Len(t, response.TransactionValidations, 2)
			},
		},
		{
			name:        "success - with portfolioId filter",
			queryParams: "?portfolio_id=" + testutil.MustDeterministicUUID(101).String(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				mockService := mocks.NewMockTransactionValidationService(ctrl)
				mockService.EXPECT().
					ListTransactionValidations(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx interface{}, filters *model.TransactionValidationFilters) (*query.ListTransactionValidationsResult, error) {
						require.NotNil(t, filters.PortfolioID)
						return &query.ListTransactionValidationsResult{
							TransactionValidations: audits,
							NextCursor:             "",
							HasMore:                false,
						}, nil
					})
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListTransactionValidationsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				require.Len(t, response.TransactionValidations, 2)
			},
		},
		{
			name:        "success - with transactionType filter",
			queryParams: "?transaction_type=CARD",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				mockService := mocks.NewMockTransactionValidationService(ctrl)
				mockService.EXPECT().
					ListTransactionValidations(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx interface{}, filters *model.TransactionValidationFilters) (*query.ListTransactionValidationsResult, error) {
						require.NotNil(t, filters.TransactionType)
						assert.Equal(t, model.TransactionTypeCard, *filters.TransactionType)
						return &query.ListTransactionValidationsResult{
							TransactionValidations: audits,
							NextCursor:             "",
							HasMore:                false,
						}, nil
					})
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListTransactionValidationsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				require.Len(t, response.TransactionValidations, 2)
			},
		},
		{
			name:        "error - invalid transactionType value",
			queryParams: "?transaction_type=INVALID",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				return mocks.NewMockTransactionValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assertStringErrorContains(t, body, "transaction_type must be one of")
			},
		},
		{
			name:        "error - invalid segmentId UUID",
			queryParams: "?segment_id=invalid-uuid",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				return mocks.NewMockTransactionValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assertStringErrorContains(t, body, "segment_id must be a valid UUID")
			},
		},
		{
			name:        "error - invalid portfolioId UUID",
			queryParams: "?portfolio_id=invalid-uuid",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				return mocks.NewMockTransactionValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assertStringErrorContains(t, body, "portfolio_id must be a valid UUID")
			},
		},
		{
			name:        "success - with sort_by=processing_time_ms",
			queryParams: "?sort_by=processing_time_ms",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockTransactionValidationService {
				mockService := mocks.NewMockTransactionValidationService(ctrl)
				mockService.EXPECT().
					ListTransactionValidations(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx interface{}, filters *model.TransactionValidationFilters) (*query.ListTransactionValidationsResult, error) {
						assert.Equal(t, "processing_time_ms", filters.SortBy)
						return &query.ListTransactionValidationsResult{
							TransactionValidations: audits,
							NextCursor:             "",
							HasMore:                false,
						}, nil
					})
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListTransactionValidationsResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				require.Len(t, response.TransactionValidations, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)

			handler := NewTransactionValidationHandler(mockService)

			app := fiber.New()
			app.Get("/v1/validations", handler.ListTransactionValidations)

			req := httptest.NewRequest(http.MethodGet, "/v1/validations"+tt.queryParams, nil)
			req.Header.Set("Content-Type", "application/json")

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

func TestToTransactionValidationFilters(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(200)
	matchedRuleID := testutil.MustDeterministicUUID(201)
	exceededLimitID := testutil.MustDeterministicUUID(202)
	segmentID := testutil.MustDeterministicUUID(203)
	portfolioID := testutil.MustDeterministicUUID(204)

	tests := []struct {
		name           string
		input          *ListTransactionValidationsInput
		expectedFilter func(t *testing.T, f *model.TransactionValidationFilters)
		wantErr        bool
	}{
		{
			name:  "empty input",
			input: &ListTransactionValidationsInput{},
			expectedFilter: func(t *testing.T, f *model.TransactionValidationFilters) {
				assert.Equal(t, model.DefaultTransactionValidationFilterLimit, f.Limit)
				assert.Empty(t, f.Cursor)
				assert.Empty(t, f.SortBy)
				assert.Empty(t, f.SortOrder)
				assert.True(t, f.StartDate.IsZero())
				assert.True(t, f.EndDate.IsZero())
				assert.Nil(t, f.Decision)
				assert.Nil(t, f.AccountID)
				assert.Nil(t, f.MatchedRuleID)
				assert.Nil(t, f.ExceededLimitID)
				assert.Nil(t, f.SegmentID)
				assert.Nil(t, f.PortfolioID)
				assert.Nil(t, f.TransactionType)
			},
			wantErr: false,
		},
		{
			name: "with limit and cursor",
			input: &ListTransactionValidationsInput{
				Limit:  testutil.Ptr(50),
				Cursor: "test-cursor",
			},
			expectedFilter: func(t *testing.T, f *model.TransactionValidationFilters) {
				assert.Equal(t, 50, f.Limit)
				assert.Equal(t, "test-cursor", f.Cursor)
			},
			wantErr: false,
		},
		{
			name: "with sortBy and sortOrder",
			input: &ListTransactionValidationsInput{
				SortBy:    "created_at",
				SortOrder: "ASC",
			},
			expectedFilter: func(t *testing.T, f *model.TransactionValidationFilters) {
				assert.Equal(t, "created_at", f.SortBy)
				assert.Equal(t, "ASC", f.SortOrder)
			},
			wantErr: false,
		},
		{
			name: "with start and end dates",
			input: &ListTransactionValidationsInput{
				StartDate: "2026-01-01T00:00:00Z",
				EndDate:   "2026-01-15T23:59:59Z",
			},
			expectedFilter: func(t *testing.T, f *model.TransactionValidationFilters) {
				assert.Equal(t, 2026, f.StartDate.Year())
				assert.Equal(t, time.January, f.StartDate.Month())
				assert.Equal(t, 1, f.StartDate.Day())
				assert.Equal(t, 2026, f.EndDate.Year())
				assert.Equal(t, time.January, f.EndDate.Month())
				assert.Equal(t, 15, f.EndDate.Day())
			},
			wantErr: false,
		},
		{
			name: "with decision filter",
			input: &ListTransactionValidationsInput{
				Decision: "DENY",
			},
			expectedFilter: func(t *testing.T, f *model.TransactionValidationFilters) {
				require.NotNil(t, f.Decision)
				assert.Equal(t, model.DecisionDeny, *f.Decision)
			},
			wantErr: false,
		},
		{
			name: "with accountId",
			input: &ListTransactionValidationsInput{
				AccountID: accountID.String(),
			},
			expectedFilter: func(t *testing.T, f *model.TransactionValidationFilters) {
				require.NotNil(t, f.AccountID)
				assert.Equal(t, accountID, *f.AccountID)
			},
			wantErr: false,
		},
		{
			name: "with matchedRuleId",
			input: &ListTransactionValidationsInput{
				MatchedRuleID: matchedRuleID.String(),
			},
			expectedFilter: func(t *testing.T, f *model.TransactionValidationFilters) {
				require.NotNil(t, f.MatchedRuleID)
				assert.Equal(t, matchedRuleID, *f.MatchedRuleID)
			},
			wantErr: false,
		},
		{
			name: "with exceededLimitId",
			input: &ListTransactionValidationsInput{
				ExceededLimitID: exceededLimitID.String(),
			},
			expectedFilter: func(t *testing.T, f *model.TransactionValidationFilters) {
				require.NotNil(t, f.ExceededLimitID)
				assert.Equal(t, exceededLimitID, *f.ExceededLimitID)
			},
			wantErr: false,
		},
		{
			name: "with segmentId",
			input: &ListTransactionValidationsInput{
				SegmentID: segmentID.String(),
			},
			expectedFilter: func(t *testing.T, f *model.TransactionValidationFilters) {
				require.NotNil(t, f.SegmentID)
				assert.Equal(t, segmentID, *f.SegmentID)
			},
			wantErr: false,
		},
		{
			name: "with portfolioId",
			input: &ListTransactionValidationsInput{
				PortfolioID: portfolioID.String(),
			},
			expectedFilter: func(t *testing.T, f *model.TransactionValidationFilters) {
				require.NotNil(t, f.PortfolioID)
				assert.Equal(t, portfolioID, *f.PortfolioID)
			},
			wantErr: false,
		},
		{
			name: "with transactionType",
			input: &ListTransactionValidationsInput{
				TransactionType: "WIRE",
			},
			expectedFilter: func(t *testing.T, f *model.TransactionValidationFilters) {
				require.NotNil(t, f.TransactionType)
				assert.Equal(t, model.TransactionTypeWire, *f.TransactionType)
			},
			wantErr: false,
		},
		{
			name: "all fields populated",
			input: &ListTransactionValidationsInput{
				Limit:           testutil.Ptr(100),
				Cursor:          "page-cursor",
				SortBy:          "created_at",
				SortOrder:       "DESC",
				StartDate:       "2026-01-01T00:00:00Z",
				EndDate:         "2026-01-31T23:59:59Z",
				Decision:        "ALLOW",
				AccountID:       accountID.String(),
				MatchedRuleID:   matchedRuleID.String(),
				ExceededLimitID: exceededLimitID.String(),
				SegmentID:       segmentID.String(),
				PortfolioID:     portfolioID.String(),
				TransactionType: "PIX",
			},
			expectedFilter: func(t *testing.T, f *model.TransactionValidationFilters) {
				assert.Equal(t, 100, f.Limit)
				assert.Equal(t, "page-cursor", f.Cursor)
				assert.Equal(t, "created_at", f.SortBy)
				assert.Equal(t, "DESC", f.SortOrder)
				assert.False(t, f.StartDate.IsZero())
				assert.False(t, f.EndDate.IsZero())
				require.NotNil(t, f.Decision)
				assert.Equal(t, model.DecisionAllow, *f.Decision)
				require.NotNil(t, f.AccountID)
				assert.Equal(t, accountID, *f.AccountID)
				require.NotNil(t, f.MatchedRuleID)
				assert.Equal(t, matchedRuleID, *f.MatchedRuleID)
				require.NotNil(t, f.ExceededLimitID)
				assert.Equal(t, exceededLimitID, *f.ExceededLimitID)
				require.NotNil(t, f.SegmentID)
				assert.Equal(t, segmentID, *f.SegmentID)
				require.NotNil(t, f.PortfolioID)
				assert.Equal(t, portfolioID, *f.PortfolioID)
				require.NotNil(t, f.TransactionType)
				assert.Equal(t, model.TransactionTypePix, *f.TransactionType)
			},
			wantErr: false,
		},
		{
			name: "invalid startDate format",
			input: &ListTransactionValidationsInput{
				StartDate: "invalid-date",
			},
			wantErr: true,
		},
		{
			name: "invalid endDate format",
			input: &ListTransactionValidationsInput{
				EndDate: "2026-01-01", // missing time component
			},
			wantErr: true,
		},
		{
			name: "invalid accountId UUID",
			input: &ListTransactionValidationsInput{
				AccountID: "not-a-uuid",
			},
			wantErr: true,
		},
		{
			name: "invalid matchedRuleId UUID",
			input: &ListTransactionValidationsInput{
				MatchedRuleID: "bad-uuid",
			},
			wantErr: true,
		},
		{
			name: "invalid exceededLimitId UUID",
			input: &ListTransactionValidationsInput{
				ExceededLimitID: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid segmentId UUID",
			input: &ListTransactionValidationsInput{
				SegmentID: "not-a-uuid",
			},
			wantErr: true,
		},
		{
			name: "invalid portfolioId UUID",
			input: &ListTransactionValidationsInput{
				PortfolioID: "bad-uuid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ToTransactionValidationFilters(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				tt.expectedFilter(t, result)
			}
		})
	}
}

func TestListTransactionValidationsInput_Validate(t *testing.T) {
	tests := []struct {
		name    string
		input   ListTransactionValidationsInput
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid - empty input",
			input:   ListTransactionValidationsInput{},
			wantErr: false,
		},
		{
			name: "valid - all fields",
			input: ListTransactionValidationsInput{
				Limit:  testutil.Ptr(100),
				Cursor: "test-cursor",
				// Note: SortBy and SortOrder omitted when cursor is present (TRC-0045)
				StartDate:       "2026-01-01T00:00:00Z",
				EndDate:         "2026-01-15T00:00:00Z",
				Decision:        "ALLOW",
				AccountID:       testutil.MustDeterministicUUID(300).String(),
				MatchedRuleID:   testutil.MustDeterministicUUID(301).String(),
				ExceededLimitID: testutil.MustDeterministicUUID(302).String(),
				SegmentID:       testutil.MustDeterministicUUID(303).String(),
				PortfolioID:     testutil.MustDeterministicUUID(304).String(),
				TransactionType: "CARD",
			},
			wantErr: false,
		},
		{
			name: "error - negative limit",
			input: ListTransactionValidationsInput{
				Limit: testutil.Ptr(-1),
			},
			wantErr: true,
			errMsg:  "limit must be at least 1",
		},
		{
			name: "error - limit too high",
			input: ListTransactionValidationsInput{
				Limit: testutil.Ptr(1001),
			},
			wantErr: true,
			errMsg:  "limit must not exceed 1000",
		},
		{
			name: "error - invalid sortBy",
			input: ListTransactionValidationsInput{
				SortBy: "invalid_field",
			},
			wantErr: true,
			errMsg:  "sort_by must be one of [created_at processing_time_ms]",
		},
		{
			name: "valid - sortBy processing_time_ms",
			input: ListTransactionValidationsInput{
				SortBy: "processing_time_ms",
			},
			wantErr: false,
		},
		{
			name: "error - invalid sortOrder",
			input: ListTransactionValidationsInput{
				SortOrder: "INVALID",
			},
			wantErr: true,
			errMsg:  "sort_order must be ASC or DESC",
		},
		{
			name: "error - invalid decision",
			input: ListTransactionValidationsInput{
				Decision: "INVALID",
			},
			wantErr: true,
			errMsg:  "decision must be one of",
		},
		{
			name: "error - invalid startDate format",
			input: ListTransactionValidationsInput{
				StartDate: "2026-01-01",
			},
			wantErr: true,
			errMsg:  "start_date must be in RFC3339 format",
		},
		{
			name: "error - invalid endDate format",
			input: ListTransactionValidationsInput{
				EndDate: "invalid",
			},
			wantErr: true,
			errMsg:  "end_date must be in RFC3339 format",
		},
		{
			name: "error - invalid accountId UUID",
			input: ListTransactionValidationsInput{
				AccountID: "not-a-uuid",
			},
			wantErr: true,
			errMsg:  "account_id must be a valid UUID",
		},
		{
			name: "error - invalid matchedRuleId UUID",
			input: ListTransactionValidationsInput{
				MatchedRuleID: "invalid",
			},
			wantErr: true,
			errMsg:  "matched_rule_id must be a valid UUID",
		},
		{
			name: "error - invalid exceededLimitId UUID",
			input: ListTransactionValidationsInput{
				ExceededLimitID: "bad-uuid",
			},
			wantErr: true,
			errMsg:  "exceeded_limit_id must be a valid UUID",
		},
		{
			name: "error - invalid segmentId UUID",
			input: ListTransactionValidationsInput{
				SegmentID: "not-a-uuid",
			},
			wantErr: true,
			errMsg:  "segment_id must be a valid UUID",
		},
		{
			name: "error - invalid portfolioId UUID",
			input: ListTransactionValidationsInput{
				PortfolioID: "bad-uuid",
			},
			wantErr: true,
			errMsg:  "portfolio_id must be a valid UUID",
		},
		{
			name: "error - invalid transactionType",
			input: ListTransactionValidationsInput{
				TransactionType: "INVALID",
			},
			wantErr: true,
			errMsg:  "transaction_type must be one of [CARD, WIRE, PIX, CRYPTO]",
		},
		{
			name: "valid - transactionType CRYPTO",
			input: ListTransactionValidationsInput{
				TransactionType: "CRYPTO",
			},
			wantErr: false,
		},
		{
			name: "error - startDate after endDate",
			input: ListTransactionValidationsInput{
				StartDate: "2026-01-15T00:00:00Z",
				EndDate:   "2026-01-01T00:00:00Z",
			},
			wantErr: true,
			errMsg:  "end_date must be on or after start_date",
		},
		{
			name: "valid - startDate equals endDate",
			input: ListTransactionValidationsInput{
				StartDate: "2026-01-15T00:00:00Z",
				EndDate:   "2026-01-15T00:00:00Z",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply defaults first, then validate (matches handler behavior)
			tt.input.SetDefaults()
			err := tt.input.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestToValidationSummary_NilSlices_ReturnEmptyArrays(t *testing.T) {
	t.Parallel()

	accountID := testutil.MustDeterministicUUID(400)
	validationID := testutil.MustDeterministicUUID(401)

	tv := &model.TransactionValidation{
		ID:              validationID,
		Amount:          decimal.RequireFromString("100"),
		Currency:        "BRL",
		TransactionType: model.TransactionTypePix,
		Account:         model.AccountContext{ID: accountID},
		EvaluationResult: model.EvaluationResult{
			Decision:       "ALLOW",
			Reason:         "no_rules_matched",
			MatchedRuleIDs: nil, // Explicitly nil
		},
		LimitUsageDetails: nil,
		ProcessingTimeMs:  5,
		CreatedAt:         testutil.FixedTime(),
	}

	summary := ToValidationSummary(tv)

	require.NotNil(t, summary)
	assert.NotNil(t, summary.MatchedRuleIDs, "MatchedRuleIDs should not be nil")
	assert.NotNil(t, summary.ExceededLimitIDs, "ExceededLimitIDs should not be nil")
	assert.Empty(t, summary.MatchedRuleIDs, "MatchedRuleIDs should be empty slice")
	assert.Empty(t, summary.ExceededLimitIDs, "ExceededLimitIDs should be empty slice")

	// Verify JSON serialization produces [] not null
	jsonBytes, err := json.Marshal(summary)
	require.NoError(t, err)

	jsonStr := string(jsonBytes)
	assert.Contains(t, jsonStr, `"matchedRuleIds":[]`, "JSON should serialize as empty array, not null")
	assert.Contains(t, jsonStr, `"exceededLimitIds":[]`, "JSON should serialize as empty array, not null")
}

func TestEnsureUUIDSlice_NilInput_ReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	result := ensureUUIDSlice(nil)

	assert.NotNil(t, result, "result should not be nil")
	assert.Empty(t, result, "result should be empty")
	assert.Equal(t, []uuid.UUID{}, result)
}

func TestEnsureUUIDSlice_NonNilInput_ReturnsSameSlice(t *testing.T) {
	t.Parallel()

	id1 := testutil.MustDeterministicUUID(500)
	id2 := testutil.MustDeterministicUUID(501)
	input := []uuid.UUID{id1, id2}

	result := ensureUUIDSlice(input)

	assert.Equal(t, input, result)
	assert.Len(t, result, 2)
}
