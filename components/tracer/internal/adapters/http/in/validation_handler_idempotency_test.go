// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	"tracer/pkg/model"
)

// =============================================================================
// Handler HTTP Status Codes for Idempotency Tests
// =============================================================================
// These tests verify the HTTP status codes based on IsDuplicate flag:
// - New request (IsDuplicate=false): HTTP 201 Created
// - Duplicate request (IsDuplicate=true): HTTP 200 OK
// =============================================================================

// TestValidationHandler_Validate_ReturnsCorrectStatusCodes tests that the handler
// returns HTTP 201 for new requests and HTTP 200 for duplicate requests (DD-9).
func TestValidationHandler_Validate_ReturnsCorrectStatusCodes(t *testing.T) {
	validRequestID := testutil.MustDeterministicUUID(7001)
	accountID := testutil.MustDeterministicUUID(7002)
	now := testutil.DefaultTestTime

	validRequest := model.ValidationRequest{
		RequestID:            validRequestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID: accountID,
		},
	}

	tests := []struct {
		name           string
		mockSetup      func(ctrl *gomock.Controller) *mocks.MockValidationService
		expectedStatus int
		description    string
	}{
		{
			name: "new request returns HTTP 201 Created",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				mockService := mocks.NewMockValidationService(ctrl)

				// Service returns ValidateResult{Response, IsDuplicate: false} for new requests
				mockService.EXPECT().
					Validate(gomock.Any(), gomock.Any()).
					Return(&services.ValidateResult{
						Response: &model.ValidationResponse{
							ValidationID: testutil.MustDeterministicUUID(7010),
							RequestID:    validRequestID,
							EvaluationResult: model.EvaluationResult{
								Decision:         model.DecisionAllow,
								MatchedRuleIDs:   []uuid.UUID{},
								EvaluatedRuleIDs: []uuid.UUID{testutil.MustDeterministicUUID(7011)},
								Reason:           "No matching rules found",
							},
							LimitUsageDetails: []model.LimitUsageDetail{},
							ProcessingTimeMs:  15,
						},
						IsDuplicate: false,
					}, nil)

				return mockService
			},
			// New requests should return 201 Created
			expectedStatus: http.StatusCreated,
			description:    "Handler should return 201 for new (non-duplicate) requests",
		},
		{
			name: "duplicate request returns HTTP 200 OK",
			mockSetup: func(ctrl *gomock.Controller) *mocks.MockValidationService {
				mockService := mocks.NewMockValidationService(ctrl)

				// Service returns ValidateResult{Response, IsDuplicate: true} for duplicate requests
				mockService.EXPECT().
					Validate(gomock.Any(), gomock.Any()).
					Return(&services.ValidateResult{
						Response: &model.ValidationResponse{
							ValidationID: testutil.MustDeterministicUUID(7020),
							RequestID:    validRequestID,
							EvaluationResult: model.EvaluationResult{
								Decision:         model.DecisionAllow,
								MatchedRuleIDs:   []uuid.UUID{},
								EvaluatedRuleIDs: []uuid.UUID{testutil.MustDeterministicUUID(7021)},
								Reason:           "No matching rules found",
							},
							LimitUsageDetails: []model.LimitUsageDetail{},
							ProcessingTimeMs:  10,
						},
						IsDuplicate: true,
					}, nil)

				return mockService
			},
			// Duplicate requests should return 200 OK
			expectedStatus: http.StatusOK,
			description:    "Handler should return 200 for duplicate (idempotent) requests",
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

			body, err := json.Marshal(validRequest)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/v1/validations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// ASSERTION: Verify correct status code based on IsDuplicate
			assert.Equal(t, tt.expectedStatus, resp.StatusCode, tt.description)
		})
	}
}

// TestValidationHandler_Validate_IdempotencyHeader tests that the handler works
// correctly for both new and duplicate requests without requiring special headers.
func TestValidationHandler_Validate_IdempotencyHeader(t *testing.T) {
	validRequestID := testutil.MustDeterministicUUID(8001)
	accountID := testutil.MustDeterministicUUID(8002)
	now := testutil.DefaultTestTime

	validRequest := model.ValidationRequest{
		RequestID:            validRequestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: now,
		Account: model.AccountContext{
			ID:     accountID,
			Type:   "checking",
			Status: "active",
		},
	}

	ctrl := gomock.NewController(t)

	mockService := mocks.NewMockValidationService(ctrl)

	// Simulate duplicate request scenario
	mockService.EXPECT().
		Validate(gomock.Any(), gomock.Any()).
		Return(&services.ValidateResult{
			Response: &model.ValidationResponse{
				ValidationID: testutil.MustDeterministicUUID(8010),
				RequestID:    validRequestID,
				EvaluationResult: model.EvaluationResult{
					Decision:         model.DecisionAllow,
					MatchedRuleIDs:   []uuid.UUID{},
					EvaluatedRuleIDs: []uuid.UUID{},
					Reason:           "Approved",
				},
				LimitUsageDetails: []model.LimitUsageDetail{},
				ProcessingTimeMs:  5,
				EvaluatedAt:       now,
			},
			IsDuplicate: true,
		}, nil)

	clk := clock.New()
	handler, handlerErr := NewValidationHandler(mockService, clk)
	require.NoError(t, handlerErr)

	app := fiber.New()
	app.Post("/v1/validations", handler.Validate)

	body, err := json.Marshal(validRequest)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/validations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Duplicate requests should return 200 OK
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Duplicate response should return 200 OK")
}
