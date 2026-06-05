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
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func TestHandler_CreateRule(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    interface{}
		mockSetup      func(ctrl *gomock.Controller) *MockRuleService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success - creates rule",
			requestBody: map[string]interface{}{
				"name":        "Test Rule",
				"description": "Test Description",
				"expression":  "amount > 1000",
				"action":      "DENY",
				"scopes": []map[string]interface{}{
					{"accountId": "550e8400-e29b-41d4-a716-446655440000"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					CreateRule(gomock.Any(), gomock.Any()).
					Return(&model.Rule{
						ID:         testutil.MustDeterministicUUID(1),
						Name:       "Test Rule",
						Expression: "amount > 1000",
						Action:     model.DecisionDeny,
						Status:     model.RuleStatusDraft,
						CreatedAt:  testutil.FixedTime(),
						UpdatedAt:  testutil.FixedTime(),
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusCreated,
			expectedBody: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Test Rule", response["name"])
				assert.Equal(t, "DENY", response["action"])
				assert.Equal(t, "DRAFT", response["status"])
			},
		},
		{
			name: "success - creates rule without scopes (global rule)",
			requestBody: map[string]interface{}{
				"name":       "Global Rule",
				"expression": "amount > 5000",
				"action":     "REVIEW",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					CreateRule(gomock.Any(), gomock.Any()).
					Return(&model.Rule{
						ID:         testutil.MustDeterministicUUID(2),
						Name:       "Global Rule",
						Expression: "amount > 5000",
						Action:     model.DecisionReview,
						Status:     model.RuleStatusDraft,
						Scopes:     []model.Scope{},
						CreatedAt:  testutil.FixedTime(),
						UpdatedAt:  testutil.FixedTime(),
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusCreated,
			expectedBody: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "Global Rule", response["name"])
				assert.Equal(t, "REVIEW", response["action"])
			},
		},
		{
			name: "error - missing required field name",
			requestBody: map[string]interface{}{
				"expression": "amount > 1000",
				"action":     "DENY",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				return NewMockRuleService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "name")
			},
		},
		{
			name: "error - missing required field expression",
			requestBody: map[string]interface{}{
				"name":   "Test Rule",
				"action": "DENY",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				return NewMockRuleService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "expression")
			},
		},
		{
			name: "error - invalid action value",
			requestBody: map[string]interface{}{
				"name":       "Test Rule",
				"expression": "amount > 1000",
				"action":     "INVALID",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				return NewMockRuleService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "action")
			},
		},
		{
			name: "error - service returns name already exists",
			requestBody: map[string]interface{}{
				"name":       "Existing Rule",
				"expression": "amount > 1000",
				"action":     "DENY",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					CreateRule(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrRuleNameAlreadyExists)
				return mockService
			},
			expectedStatus: http.StatusConflict,
			expectedBody:   func(t *testing.T, body []byte) {},
		},
		{
			name: "error - service returns name already exists in context (TRC-0303)",
			requestBody: map[string]interface{}{
				"name":       "Duplicate Rule In Context",
				"expression": "amount > 1000",
				"action":     "DENY",
				"scopes": []map[string]interface{}{
					{"segmentId": "550e8400-e29b-41d4-a716-446655440000"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					CreateRule(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrRuleNameAlreadyExistsInCtx)
				return mockService
			},
			expectedStatus: http.StatusConflict,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "TRC-0303")
				assert.Contains(t, string(body), "already exists in this context")
			},
		},
		{
			name: "error - service returns CEL syntax error",
			requestBody: map[string]interface{}{
				"name":       "Bad Expression Rule",
				"expression": "invalid >>> cel",
				"action":     "DENY",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					CreateRule(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrExpressionSyntax)
				return mockService
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   func(t *testing.T, body []byte) {},
		},
		{
			name: "error - service returns internal error",
			requestBody: map[string]interface{}{
				"name":       "Test Rule",
				"expression": "amount > 1000",
				"action":     "DENY",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					CreateRule(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))
				return mockService
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   func(t *testing.T, body []byte) {},
		},
		{
			name:        "error - invalid JSON body",
			requestBody: "invalid json",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				return NewMockRuleService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   func(t *testing.T, body []byte) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)

			handler := NewHandler(mockService)

			app := fiber.New()
			app.Post("/v1/rules", handler.CreateRule)

			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/rules", bytes.NewReader(body))
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

func TestToServiceInput(t *testing.T) {
	accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	txType := model.TransactionTypeCard

	input := &CreateRuleInput{
		Name:        "Test Rule",
		Description: "Test Description",
		Expression:  "amount > 1000",
		Action:      model.DecisionDeny,
		Scopes: []model.Scope{
			{
				AccountID:       &accountID,
				TransactionType: &txType,
			},
		},
	}

	result := toServiceInput(input)

	assert.Equal(t, input.Name, result.Name)
	assert.Equal(t, input.Description, result.Description)
	assert.Equal(t, input.Expression, result.Expression)
	assert.Equal(t, input.Action, result.Action)
	assert.Len(t, result.Scopes, 1)
	assert.Equal(t, accountID, *result.Scopes[0].AccountID)
	assert.Equal(t, &txType, result.Scopes[0].TransactionType)
}

func TestHandler_UpdateRule(t *testing.T) {
	ruleID := testutil.MustDeterministicUUID(10)

	tests := []struct {
		name           string
		ruleIDParam    string
		requestBody    interface{}
		mockSetup      func(ctrl *gomock.Controller) *MockRuleService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "success - updates rule",
			ruleIDParam: ruleID.String(),
			requestBody: map[string]interface{}{
				"name":        "Updated Rule",
				"description": "Updated Description",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					UpdateRule(gomock.Any(), ruleID, gomock.Any()).
					Return(&model.Rule{
						ID:         ruleID,
						Name:       "updated rule",
						Expression: "amount > 1000",
						Action:     model.DecisionDeny,
						Status:     model.RuleStatusDraft,
						CreatedAt:  testutil.FixedTime(),
						UpdatedAt:  testutil.FixedTime(),
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "updated rule", response["name"])
			},
		},
		{
			name:        "success - updates expression only",
			ruleIDParam: ruleID.String(),
			requestBody: map[string]interface{}{
				"expression": "amount > 5000",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					UpdateRule(gomock.Any(), ruleID, gomock.Any()).
					Return(&model.Rule{
						ID:         ruleID,
						Name:       "existing rule",
						Expression: "amount > 5000",
						Action:     model.DecisionDeny,
						Status:     model.RuleStatusDraft,
						CreatedAt:  testutil.FixedTime(),
						UpdatedAt:  testutil.FixedTime(),
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "amount > 5000", response["expression"])
			},
		},
		{
			name:        "error - invalid UUID",
			ruleIDParam: "invalid-uuid",
			requestBody: map[string]interface{}{
				"name": "Updated Rule",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				return NewMockRuleService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "Invalid rule ID format")
			},
		},
		{
			name:        "error - empty body (no fields to update)",
			ruleIDParam: ruleID.String(),
			requestBody: map[string]interface{}{},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				return NewMockRuleService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "At least one field must be provided")
			},
		},
		{
			name:        "error - rule not found",
			ruleIDParam: ruleID.String(),
			requestBody: map[string]interface{}{
				"name": "Updated Rule",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					UpdateRule(gomock.Any(), ruleID, gomock.Any()).
					Return(nil, constant.ErrRuleNotFound)
				return mockService
			},
			expectedStatus: http.StatusNotFound,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "Rule not found")
			},
		},
		{
			name:        "error - name already exists in context",
			ruleIDParam: ruleID.String(),
			requestBody: map[string]interface{}{
				"name": "Existing Rule",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				// Service returns ErrRuleNameAlreadyExistsInCtx on unique violation
				mockService.EXPECT().
					UpdateRule(gomock.Any(), ruleID, gomock.Any()).
					Return(nil, constant.ErrRuleNameAlreadyExistsInCtx)
				return mockService
			},
			expectedStatus: http.StatusConflict,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "TRC-0303")
				assert.Contains(t, string(body), "already exists in this context")
			},
		},
		{
			name:        "error - invalid CEL expression",
			ruleIDParam: ruleID.String(),
			requestBody: map[string]interface{}{
				"expression": "invalid cel >>>",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					UpdateRule(gomock.Any(), ruleID, gomock.Any()).
					Return(nil, constant.ErrExpressionSyntax)
				return mockService
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "Invalid CEL expression")
			},
		},
		{
			name:        "error - service returns internal error",
			ruleIDParam: ruleID.String(),
			requestBody: map[string]interface{}{
				"name": "Updated Rule",
			},
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					UpdateRule(gomock.Any(), ruleID, gomock.Any()).
					Return(nil, errors.New("database error"))
				return mockService
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "unexpected error")
			},
		},
		{
			name:        "error - invalid JSON body",
			ruleIDParam: ruleID.String(),
			requestBody: "invalid json",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				return NewMockRuleService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)

			handler := NewHandler(mockService)

			app := fiber.New()
			app.Patch("/v1/rules/:id", handler.UpdateRule)

			var body []byte
			var err error
			switch v := tt.requestBody.(type) {
			case string:
				body = []byte(v)
			default:
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPatch, "/v1/rules/"+tt.ruleIDParam, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedBody != nil {
				respBody, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.expectedBody(t, respBody)
			}
		})
	}
}

func TestToUpdateServiceInput(t *testing.T) {
	name := "Updated Rule"
	description := "Updated Description"
	expression := "amount > 5000"
	action := model.DecisionReview
	accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	input := &UpdateRuleInput{
		Name:        &name,
		Description: &description,
		Expression:  &expression,
		Action:      &action,
		Scopes: &[]model.Scope{
			{AccountID: &accountID},
		},
	}

	result := toUpdateServiceInput(input)

	assert.Equal(t, &name, result.Name)
	assert.Equal(t, &description, result.Description)
	assert.Equal(t, &expression, result.Expression)
	assert.Equal(t, &action, result.Action)
	require.NotNil(t, result.Scopes)
	assert.Len(t, *result.Scopes, 1)
	assert.Equal(t, accountID, *(*result.Scopes)[0].AccountID)
}

func TestToUpdateServiceInput_NilScopes(t *testing.T) {
	name := "Updated Rule"

	input := &UpdateRuleInput{
		Name:   &name,
		Scopes: nil,
	}

	result := toUpdateServiceInput(input)

	assert.Equal(t, &name, result.Name)
	assert.Nil(t, result.Scopes)
}

func TestHandler_GetRule(t *testing.T) {
	ruleID := testutil.MustDeterministicUUID(20)

	tests := []struct {
		name           string
		ruleIDParam    string
		mockSetup      func(ctrl *gomock.Controller) *MockRuleService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "success - returns rule",
			ruleIDParam: ruleID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					GetRule(gomock.Any(), ruleID).
					Return(&model.Rule{
						ID:         ruleID,
						Name:       "test rule",
						Expression: "amount > 1000",
						Action:     model.DecisionDeny,
						Status:     model.RuleStatusActive,
						CreatedAt:  testutil.FixedTime(),
						UpdatedAt:  testutil.FixedTime(),
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, ruleID.String(), response["ruleId"])
				assert.Equal(t, "test rule", response["name"])
				assert.Equal(t, "DENY", response["action"])
				assert.Equal(t, "ACTIVE", response["status"])
			},
		},
		{
			name:        "error - invalid UUID",
			ruleIDParam: "invalid-uuid",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				return NewMockRuleService(ctrl)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "Invalid rule ID format")
			},
		},
		{
			name:        "error - rule not found",
			ruleIDParam: ruleID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					GetRule(gomock.Any(), ruleID).
					Return(nil, constant.ErrRuleNotFound)
				return mockService
			},
			expectedStatus: http.StatusNotFound,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "Rule not found")
			},
		},
		{
			name:        "error - service returns internal error",
			ruleIDParam: ruleID.String(),
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					GetRule(gomock.Any(), ruleID).
					Return(nil, errors.New("database error"))
				return mockService
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "unexpected error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)

			handler := NewHandler(mockService)

			app := fiber.New()
			app.Get("/v1/rules/:id", handler.GetRule)

			req := httptest.NewRequest(http.MethodGet, "/v1/rules/"+tt.ruleIDParam, nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedBody != nil {
				respBody, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.expectedBody(t, respBody)
			}
		})
	}
}

func TestHandler_ListRules(t *testing.T) {
	rules := []model.Rule{
		{
			ID:         testutil.MustDeterministicUUID(30),
			Name:       "rule 1",
			Expression: "amount > 1000",
			Action:     model.DecisionDeny,
			Status:     model.RuleStatusActive,
			CreatedAt:  testutil.FixedTime(),
			UpdatedAt:  testutil.FixedTime(),
		},
		{
			ID:         testutil.MustDeterministicUUID(31),
			Name:       "rule 2",
			Expression: "amount > 5000",
			Action:     model.DecisionReview,
			Status:     model.RuleStatusActive,
			CreatedAt:  testutil.FixedTime(),
			UpdatedAt:  testutil.FixedTime(),
		},
	}

	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(ctrl *gomock.Controller) *MockRuleService
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "success - returns rules with defaults",
			queryParams: "",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					ListRules(gomock.Any(), gomock.Any()).
					Return(&model.ListRulesResult{
						Rules:      rules,
						NextCursor: "next-cursor-123",
						HasMore:    true,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListRulesResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Len(t, response.Rules, 2)
				assert.Equal(t, "next-cursor-123", response.NextCursor)
				assert.True(t, response.HasMore)
			},
		},
		{
			name:        "success - with cursor parameter",
			queryParams: "?limit=20&cursor=abc123",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					ListRules(gomock.Any(), gomock.Any()).
					Return(&model.ListRulesResult{
						Rules:      []model.Rule{rules[1]},
						NextCursor: "",
						HasMore:    false,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListRulesResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Len(t, response.Rules, 1)
				assert.Empty(t, response.NextCursor)
				assert.False(t, response.HasMore)
			},
		},
		{
			name:        "success - with status filter",
			queryParams: "?status=ACTIVE",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					ListRules(gomock.Any(), gomock.Any()).
					Return(&model.ListRulesResult{
						Rules:   rules,
						HasMore: false,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListRulesResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Len(t, response.Rules, 2)
			},
		},
		{
			name:        "success - with action filter",
			queryParams: "?action=DENY",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					ListRules(gomock.Any(), gomock.Any()).
					Return(&model.ListRulesResult{
						Rules:   []model.Rule{rules[0]},
						HasMore: false,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListRulesResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Len(t, response.Rules, 1)
			},
		},
		{
			name:        "success - with sorting",
			queryParams: "?sort_by=name&sort_order=ASC",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					ListRules(gomock.Any(), gomock.Any()).
					Return(&model.ListRulesResult{
						Rules:   rules,
						HasMore: false,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListRulesResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Len(t, response.Rules, 2)
			},
		},
		{
			name:        "success - empty result",
			queryParams: "?status=INACTIVE",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					ListRules(gomock.Any(), gomock.Any()).
					Return(&model.ListRulesResult{
						Rules:   []model.Rule{},
						HasMore: false,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListRulesResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Len(t, response.Rules, 0)
				assert.False(t, response.HasMore)
			},
		},
		{
			name:        "error - DELETED status not allowed",
			queryParams: "?status=DELETED",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				// No mock expectations - validation fails before service call
				return mockService
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "DELETED")
			},
		},
		{
			name:        "error - service returns error",
			queryParams: "",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					ListRules(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))
				return mockService
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "unexpected error")
			},
		},
		{
			name:        "success - with accountId scope filter",
			queryParams: "?account_id=550e8400-e29b-41d4-a716-446655440000",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					ListRules(gomock.Any(), gomock.Any()).
					Return(&model.ListRulesResult{
						Rules:   rules,
						HasMore: false,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListRulesResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Len(t, response.Rules, 2)
			},
		},
		{
			name:        "success - with transactionType scope filter",
			queryParams: "?transaction_type=CARD",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					ListRules(gomock.Any(), gomock.Any()).
					Return(&model.ListRulesResult{
						Rules:   rules,
						HasMore: false,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListRulesResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Len(t, response.Rules, 2)
			},
		},
		{
			name:        "success - with multiple scope filters and existing filters combined",
			queryParams: "?status=ACTIVE&account_id=550e8400-e29b-41d4-a716-446655440000&transaction_type=PIX",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				mockService.EXPECT().
					ListRules(gomock.Any(), gomock.Any()).
					Return(&model.ListRulesResult{
						Rules:   []model.Rule{},
						HasMore: false,
					}, nil)
				return mockService
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var response ListRulesResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Empty(t, response.Rules)
			},
		},
		{
			name:        "error - invalid accountId UUID",
			queryParams: "?account_id=not-a-uuid",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				return mockService
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "account_id")
			},
		},
		{
			name:        "error - invalid transactionType enum",
			queryParams: "?transaction_type=INVALID",
			mockSetup: func(ctrl *gomock.Controller) *MockRuleService {
				mockService := NewMockRuleService(ctrl)
				return mockService
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "transaction_type")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockService := tt.mockSetup(ctrl)

			handler := NewHandler(mockService)

			app := fiber.New()
			app.Get("/v1/rules", handler.ListRules)

			req := httptest.NewRequest(http.MethodGet, "/v1/rules"+tt.queryParams, nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedBody != nil {
				respBody, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.expectedBody(t, respBody)
			}
		})
	}
}

// TestHandler_ListRules_ValidationErrors consolidates validation error tests
// that share the same mockSetup (no service calls) and assert http.StatusBadRequest.
func TestHandler_ListRules_ValidationErrors(t *testing.T) {
	validationTests := []struct {
		name            string
		queryParams     string
		expectedKeyword string
	}{
		{
			name:            "invalid limit (too high)",
			queryParams:     "?limit=101",
			expectedKeyword: "limit",
		},

		{
			name:            "invalid status",
			queryParams:     "?status=INVALID",
			expectedKeyword: "Status",
		},
		{
			name:            "invalid sort_by",
			queryParams:     "?sort_by=priority",
			expectedKeyword: "sort_by",
		},
		{
			name:            "invalid sort_order",
			queryParams:     "?sort_order=RANDOM",
			expectedKeyword: "sort_order",
		},
	}

	for _, tt := range validationTests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			// No service calls expected - validation fails before reaching service
			mockService := NewMockRuleService(ctrl)

			handler := NewHandler(mockService)

			app := fiber.New()
			app.Get("/v1/rules", handler.ListRules)

			req := httptest.NewRequest(http.MethodGet, "/v1/rules"+tt.queryParams, nil)

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Contains(t, string(respBody), tt.expectedKeyword)
		})
	}
}
