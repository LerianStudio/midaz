// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
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
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"tracer/internal/adapters/http/in/mocks"
	"tracer/internal/services"
	"tracer/internal/testutil"
	"tracer/pkg/clock"
	"tracer/pkg/constant"
	"tracer/pkg/model"
)

func TestValidationHandler_Validate(t *testing.T) {
	// Test fixtures - use deterministic values for reproducible tests
	validRequestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	now := testutil.DefaultTestTime

	validRequest := model.ValidationRequest{
		RequestID:            validRequestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"), // $100.00
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: accountID,
		},
	}

	tests := []struct {
		name           string
		requestBody    any
		mockSetup      func(ctrl *gomock.Controller) *mocks.MockValidationService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "success - returns validation response with ALLOW decision",
			requestBody: validRequest,
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				mockService := mocks.NewMockValidationService(ctrl)
				mockService.EXPECT().
					Validate(gomock.Any(), gomock.Any()).
					Return(&services.ValidateResult{
						Response: &model.ValidationResponse{
							ValidationID: testutil.MustDeterministicUUID(10),
							RequestID:    validRequestID,
							EvaluationResult: model.EvaluationResult{
								Decision:         model.DecisionAllow,
								MatchedRuleIDs:   []uuid.UUID{},
								EvaluatedRuleIDs: []uuid.UUID{testutil.MustDeterministicUUID(11)},
								Reason:           "No matching rules found",
							},
							LimitUsageDetails: []model.LimitUsageDetail{},
							ProcessingTimeMs:  15,
						},
						IsDuplicate: false,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusCreated,
			expectedBody: func(t *testing.T, body []byte) {
				var response model.ValidationResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.NotEqual(t, uuid.Nil, response.ValidationID)
				assert.Equal(t, validRequestID, response.RequestID)
				assert.Equal(t, model.DecisionAllow, response.Decision)
			},
		},
		{
			name:        "success - returns validation response with DENY decision",
			requestBody: validRequest,
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				mockService := mocks.NewMockValidationService(ctrl)
				matchedRuleID := testutil.MustDeterministicUUID(20)
				mockService.EXPECT().
					Validate(gomock.Any(), gomock.Any()).
					Return(&services.ValidateResult{
						Response: &model.ValidationResponse{
							ValidationID: testutil.MustDeterministicUUID(21),
							RequestID:    validRequestID,
							EvaluationResult: model.EvaluationResult{
								Decision:         model.DecisionDeny,
								MatchedRuleIDs:   []uuid.UUID{matchedRuleID},
								EvaluatedRuleIDs: []uuid.UUID{matchedRuleID},
								Reason:           "High-risk transaction blocked",
							},
							LimitUsageDetails: []model.LimitUsageDetail{},
							ProcessingTimeMs:  20,
						},
						IsDuplicate: false,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusCreated,
			expectedBody: func(t *testing.T, body []byte) {
				var response model.ValidationResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, model.DecisionDeny, response.Decision)
				require.Len(t, response.MatchedRuleIDs, 1)
			},
		},
		{
			name:        "success - returns validation response with limit usage details",
			requestBody: validRequest,
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				mockService := mocks.NewMockValidationService(ctrl)
				limitID := testutil.MustDeterministicUUID(30)
				mockService.EXPECT().
					Validate(gomock.Any(), gomock.Any()).
					Return(&services.ValidateResult{
						Response: &model.ValidationResponse{
							ValidationID: testutil.MustDeterministicUUID(31),
							RequestID:    validRequestID,
							EvaluationResult: model.EvaluationResult{
								Decision:         model.DecisionAllow,
								MatchedRuleIDs:   []uuid.UUID{},
								EvaluatedRuleIDs: []uuid.UUID{},
								Reason:           "Transaction approved",
							},
							LimitUsageDetails: []model.LimitUsageDetail{
								{
									LimitID:      limitID,
									LimitAmount:  decimal.RequireFromString("1000"), // $1000.00
									CurrentUsage: decimal.RequireFromString("500"),  // $500.00
									Exceeded:     false,
								},
							},
							ProcessingTimeMs: 25,
						},
						IsDuplicate: false,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusCreated,
			expectedBody: func(t *testing.T, body []byte) {
				var response model.ValidationResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				require.Len(t, response.LimitUsageDetails, 1)
				assert.False(t, response.LimitUsageDetails[0].Exceeded)
			},
		},
		{
			name: "error - payload too large (exceeds 100KB)",
			requestBody: func() any {
				// Create a payload larger than 100KB with a single large string
				req := validRequest
				req.Metadata = map[string]any{
					"blob": strings.Repeat("a", 110*1024),
				}
				return req
			}(),
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				// Service should NOT be called when payload is too large
				return mocks.NewMockValidationService(ctrl)
			},
			expectedStatus: http.StatusRequestEntityTooLarge,
			expectedBody: func(t *testing.T, body []byte) {
				// Verify standardized response format
				assert.Contains(t, string(body), "TRC-0011")
				assert.Contains(t, string(body), "Payload Too Large")
				assert.Contains(t, string(body), "payload too large: exceeds 100KB limit")
			},
		},
		{
			name:        "error - invalid JSON body",
			requestBody: "invalid json {{{",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				// Service should NOT be called when body parsing fails
				return mocks.NewMockValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   func(t *testing.T, body []byte) {},
		},
		{
			name: "error - missing required field requestId",
			requestBody: map[string]any{
				"transactionType":      "CARD",
				"amount":               100,
				"currency":             "USD",
				"transactionTimestamp": now.Format(time.RFC3339),
				"account":              map[string]any{"accountId": accountID.String()},
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				// Service should NOT be called when validation fails
				return mocks.NewMockValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "requestId")
			},
		},
		{
			name: "error - invalid transaction type",
			requestBody: map[string]any{
				"requestId":            validRequestID.String(),
				"transactionType":      "INVALID_TYPE",
				"amount":               100,
				"currency":             "USD",
				"transactionTimestamp": now.Format(time.RFC3339),
				"account":              map[string]any{"accountId": accountID.String()},
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				return mocks.NewMockValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "transactionType")
			},
		},
		{
			name: "error - amount non-positive",
			requestBody: map[string]any{
				"requestId":            validRequestID.String(),
				"transactionType":      "CARD",
				"amount":               0,
				"currency":             "USD",
				"transactionTimestamp": now.Format(time.RFC3339),
				"account":              map[string]any{"accountId": accountID.String()},
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				return mocks.NewMockValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "amount")
			},
		},
		{
			name: "error - negative amount",
			requestBody: map[string]any{
				"requestId":            validRequestID.String(),
				"transactionType":      "CARD",
				"amount":               -100,
				"currency":             "USD",
				"transactionTimestamp": now.Format(time.RFC3339),
				"account":              map[string]any{"accountId": accountID.String()},
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				return mocks.NewMockValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "amount")
			},
		},
		{
			name: "error - missing currency",
			requestBody: map[string]any{
				"requestId":            validRequestID.String(),
				"transactionType":      "CARD",
				"amount":               100,
				"transactionTimestamp": now.Format(time.RFC3339),
				"account":              map[string]any{"accountId": accountID.String()},
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				return mocks.NewMockValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "currency")
			},
		},
		{
			name: "error - missing timestamp",
			requestBody: map[string]any{
				"requestId":       validRequestID.String(),
				"transactionType": "CARD",
				"amount":          100,
				"currency":        "USD",
				"account":         map[string]any{"accountId": accountID.String()},
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				return mocks.NewMockValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "transactionTimestamp")
			},
		},
		{
			name:        "error - service returns rule evaluation failed",
			requestBody: validRequest,
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				mockService := mocks.NewMockValidationService(ctrl)
				mockService.EXPECT().
					Validate(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrRuleEvaluationFailed)
				return mockService
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "rule")
			},
		},
		{
			name:        "error - service returns limit check failed",
			requestBody: validRequest,
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				mockService := mocks.NewMockValidationService(ctrl)
				mockService.EXPECT().
					Validate(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrLimitCheckFailed)
				return mockService
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "limit")
			},
		},
		{
			name:        "error - service returns generic error",
			requestBody: validRequest,
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				mockService := mocks.NewMockValidationService(ctrl)
				mockService.EXPECT().
					Validate(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("unexpected database error"))
				return mockService
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody: func(t *testing.T, body []byte) {
				// Generic message returned to prevent info leakage
				assert.Contains(t, string(body), "validation processing failed")
			},
		},
		{
			name:        "error - service returns validation timeout",
			requestBody: validRequest,
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				mockService := mocks.NewMockValidationService(ctrl)
				mockService.EXPECT().
					Validate(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrValidationTimeout)
				return mockService
			},
			expectedStatus: http.StatusGatewayTimeout,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "timeout")
			},
		},
		{
			name:        "error - context cancelled",
			requestBody: validRequest,
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				mockService := mocks.NewMockValidationService(ctrl)
				mockService.EXPECT().
					Validate(gomock.Any(), gomock.Any()).
					Return(nil, context.Canceled)
				return mockService
			},
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody:   func(t *testing.T, body []byte) {},
		},
		{
			name: "error - missing account",
			requestBody: map[string]any{
				"requestId":            validRequestID.String(),
				"transactionType":      "CARD",
				"amount":               100,
				"currency":             "USD",
				"transactionTimestamp": now.Format("2006-01-02T15:04:05Z07:00"),
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				// Service should NOT be called when account is missing
				return mocks.NewMockValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "account")
			},
		},
		{
			name: "error - missing account ID (nil UUID)",
			requestBody: map[string]any{
				"requestId":            validRequestID.String(),
				"transactionType":      "CARD",
				"amount":               100,
				"currency":             "USD",
				"transactionTimestamp": now.Format("2006-01-02T15:04:05Z07:00"),
				"account":              map[string]any{"accountId": ""},
			},
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				// Service should NOT be called when account.id is empty
				return mocks.NewMockValidationService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "account")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)

			clk := clock.New()
			handler, handlerErr := NewValidationHandler(mockService, clk)
			require.NoError(t, handlerErr)

			app := fiber.New()
			app.Post("/v1/validations", handler.Validate)

			var body []byte
			var err error

			switch v := tt.requestBody.(type) {
			case string:
				body = []byte(v)
			default:
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/validations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if tt.expectedBody != nil {
				tt.expectedBody(t, respBody)
			}
		})
	}
}

// TestValidationHandler_Validate_PayloadSizeCheck tests payload size validation
// with explicit size boundary testing.
func TestValidationHandler_Validate_PayloadSizeCheck(t *testing.T) {
	// Helper to create a valid JSON payload of exact size
	createValidPayloadOfSize := func(t *testing.T, targetSize int) []byte {
		t.Helper()

		// Base valid request
		validRequestID := testutil.MustDeterministicUUID(100)
		accountID := testutil.MustDeterministicUUID(101)
		now := testutil.FixedTime()

		baseRequest := model.ValidationRequest{
			RequestID:            validRequestID,
			TransactionType:      model.TransactionTypeCard,
			Amount:               decimal.RequireFromString("100"),
			Currency:             "USD",
			TransactionTimestamp: now,
			Account: model.AccountContext{
				ID: accountID,
			},
			Metadata: map[string]any{},
		}

		// Marshal to get base size
		baseJSON, err := json.Marshal(baseRequest)
		require.NoError(t, err)
		baseSize := len(baseJSON)

		// Calculate padding needed in metadata
		// We need to add a metadata field with enough padding
		// Account for JSON overhead: {"padding":"..."} adds ~14 chars
		paddingNeeded := targetSize - baseSize - 14

		if paddingNeeded > 0 {
			baseRequest.Metadata["padding"] = strings.Repeat("x", paddingNeeded)
		}

		result, err := json.Marshal(baseRequest)
		require.NoError(t, err)

		// Fine-tune to exact size by adjusting padding (with max iterations to prevent infinite loop)
		maxIterations := 100
		for i := 0; len(result) < targetSize && paddingNeeded > 0 && i < maxIterations; i++ {
			paddingNeeded++
			baseRequest.Metadata["padding"] = strings.Repeat("x", paddingNeeded)
			result, err = json.Marshal(baseRequest)
			require.NoError(t, err)
		}

		for i := 0; len(result) > targetSize && paddingNeeded > 0 && i < maxIterations; i++ {
			paddingNeeded--
			baseRequest.Metadata["padding"] = strings.Repeat("x", paddingNeeded)
			result, err = json.Marshal(baseRequest)
			require.NoError(t, err)
		}

		return result
	}

	tests := []struct {
		name           string
		payloadSize    int
		expectedStatus int
	}{
		{
			name:           "payload at limit (100KB) is accepted",
			payloadSize:    100 * 1024, // 100KB exactly
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "payload over limit (100KB+1) is rejected",
			payloadSize:    100*1024 + 1, // 100KB + 1 byte
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := mocks.NewMockValidationService(ctrl)

			// Only expect service call if payload is within limit
			if tt.expectedStatus == http.StatusCreated {
				mockService.EXPECT().
					Validate(gomock.Any(), gomock.Any()).
					Return(&services.ValidateResult{
						Response: &model.ValidationResponse{
							ValidationID: testutil.MustDeterministicUUID(110),
							RequestID:    testutil.MustDeterministicUUID(111),
							EvaluationResult: model.EvaluationResult{
								Decision:         model.DecisionAllow,
								MatchedRuleIDs:   []uuid.UUID{},
								EvaluatedRuleIDs: []uuid.UUID{},
								Reason:           "approved",
							},
							LimitUsageDetails: []model.LimitUsageDetail{},
						},
						IsDuplicate: false,
					}, nil)
			}

			clk := clock.New()
			handler, handlerErr := NewValidationHandler(mockService, clk)
			require.NoError(t, handlerErr)

			app := fiber.New()
			app.Post("/v1/validations", handler.Validate)

			// Create valid JSON payload of exact size
			payload := createValidPayloadOfSize(t, tt.payloadSize)

			// Assert payload has the expected size (validates the helper function)
			assert.Equal(t, tt.payloadSize, len(payload), "payload size should match expected")

			req := httptest.NewRequest(http.MethodPost, "/v1/validations", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}
