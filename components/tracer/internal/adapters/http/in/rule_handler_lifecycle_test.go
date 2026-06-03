// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"tracer/internal/testutil"
	"tracer/pkg/constant"
	"tracer/pkg/model"
)

func TestActivateRuleHandler_Success(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	rule := &model.Rule{
		ID:     ruleID,
		Name:   "Test Rule",
		Status: model.RuleStatusActive,
	}

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		ActivateRule(gomock.Any(), ruleID).
		Return(rule, nil)

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/activate", handler.ActivateRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+ruleID.String()+"/activate", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body model.Rule
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, ruleID, body.ID)
	assert.Equal(t, model.RuleStatusActive, body.Status)
}

func TestActivateRuleHandler_InvalidUUID(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()

	mockService := NewMockRuleService(ctrl)

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/activate", handler.ActivateRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/invalid-uuid/activate", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0007", body["code"])
	assert.Equal(t, "Invalid Path Parameter", body["title"])
	assert.Equal(t, "Invalid rule ID format", body["message"])
}

func TestActivateRuleHandler_ServiceError(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		ActivateRule(gomock.Any(), ruleID).
		Return(nil, model.NewInvalidTransitionError(model.RuleStatusDeleted, model.RuleStatusActive))

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/activate", handler.ActivateRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+ruleID.String()+"/activate", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0102", body["code"])
	assert.Equal(t, "Invalid State Transition", body["title"])
	assert.Contains(t, body["message"], "DELETED to ACTIVE")
}

func TestActivateRuleHandler_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		ActivateRule(gomock.Any(), ruleID).
		Return(nil, constant.ErrRuleNotFound)

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/activate", handler.ActivateRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+ruleID.String()+"/activate", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0100", body["code"])
	assert.Equal(t, "Not Found", body["title"])
	assert.Equal(t, "Rule not found", body["message"])
}

func TestActivateRuleHandler_InternalError(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		ActivateRule(gomock.Any(), ruleID).
		Return(nil, errors.New("database connection failed"))

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/activate", handler.ActivateRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+ruleID.String()+"/activate", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0004", body["code"])
	assert.Equal(t, "Internal Server Error", body["title"])
	assert.Equal(t, "An unexpected error occurred", body["message"])
}

func TestDeactivateRuleHandler_Success(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	rule := &model.Rule{
		ID:     ruleID,
		Name:   "Test Rule",
		Status: model.RuleStatusInactive,
	}

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		DeactivateRule(gomock.Any(), ruleID).
		Return(rule, nil)

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/deactivate", handler.DeactivateRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+ruleID.String()+"/deactivate", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body model.Rule
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, ruleID, body.ID)
	assert.Equal(t, model.RuleStatusInactive, body.Status)
}

func TestDeactivateRuleHandler_InvalidUUID(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()

	mockService := NewMockRuleService(ctrl)

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/deactivate", handler.DeactivateRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/invalid-uuid/deactivate", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0007", body["code"])
	assert.Equal(t, "Invalid Path Parameter", body["title"])
	assert.Equal(t, "Invalid rule ID format", body["message"])
}

func TestDeactivateRuleHandler_ServiceError(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		DeactivateRule(gomock.Any(), ruleID).
		Return(nil, model.NewInvalidTransitionError(model.RuleStatusDeleted, model.RuleStatusInactive))

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/deactivate", handler.DeactivateRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+ruleID.String()+"/deactivate", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0102", body["code"])
	assert.Equal(t, "Invalid State Transition", body["title"])
	assert.Contains(t, body["message"], "DELETED to INACTIVE")
}

func TestDeactivateRuleHandler_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		DeactivateRule(gomock.Any(), ruleID).
		Return(nil, constant.ErrRuleNotFound)

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/deactivate", handler.DeactivateRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+ruleID.String()+"/deactivate", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0100", body["code"])
	assert.Equal(t, "Not Found", body["title"])
	assert.Equal(t, "Rule not found", body["message"])
}

func TestDeactivateRuleHandler_InternalError(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		DeactivateRule(gomock.Any(), ruleID).
		Return(nil, errors.New("database connection failed"))

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/deactivate", handler.DeactivateRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+ruleID.String()+"/deactivate", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0004", body["code"])
	assert.Equal(t, "Internal Server Error", body["title"])
	assert.Equal(t, "An unexpected error occurred", body["message"])
}

func TestDeleteRuleHandler_Success(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		DeleteRule(gomock.Any(), ruleID).
		Return(nil)

	handler := NewHandler(mockService)
	app.Delete("/v1/rules/:id", handler.DeleteRule)

	req := httptest.NewRequest(http.MethodDelete, "/v1/rules/"+ruleID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestDeleteRuleHandler_InvalidUUID(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()

	mockService := NewMockRuleService(ctrl)

	handler := NewHandler(mockService)
	app.Delete("/v1/rules/:id", handler.DeleteRule)

	req := httptest.NewRequest(http.MethodDelete, "/v1/rules/invalid-uuid", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0007", body["code"])
	assert.Equal(t, "Invalid Path Parameter", body["title"])
	assert.Equal(t, "Invalid rule ID format", body["message"])
}

func TestDeleteRuleHandler_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		DeleteRule(gomock.Any(), ruleID).
		Return(constant.ErrRuleNotFound)

	handler := NewHandler(mockService)
	app.Delete("/v1/rules/:id", handler.DeleteRule)

	req := httptest.NewRequest(http.MethodDelete, "/v1/rules/"+ruleID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0100", body["code"])
	assert.Equal(t, "Not Found", body["title"])
	assert.Equal(t, "Rule not found", body["message"])
}

func TestDeleteRuleHandler_InvalidTransition(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		DeleteRule(gomock.Any(), ruleID).
		Return(model.NewInvalidTransitionError(model.RuleStatusActive, model.RuleStatusDeleted))

	handler := NewHandler(mockService)
	app.Delete("/v1/rules/:id", handler.DeleteRule)

	req := httptest.NewRequest(http.MethodDelete, "/v1/rules/"+ruleID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0102", body["code"])
	assert.Equal(t, "Invalid State Transition", body["title"])
	assert.Contains(t, body["message"], "ACTIVE to DELETED")
}

func TestDeleteRuleHandler_InternalError(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		DeleteRule(gomock.Any(), ruleID).
		Return(errors.New("database connection failed"))

	handler := NewHandler(mockService)
	app.Delete("/v1/rules/:id", handler.DeleteRule)

	req := httptest.NewRequest(http.MethodDelete, "/v1/rules/"+ruleID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0004", body["code"])
	assert.Equal(t, "Internal Server Error", body["title"])
	assert.Equal(t, "An unexpected error occurred", body["message"])
}

func TestDraftRuleHandler_Success(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	rule := &model.Rule{
		ID:     ruleID,
		Name:   "Test Rule",
		Status: model.RuleStatusDraft,
	}

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		DraftRule(gomock.Any(), ruleID).
		Return(rule, nil)

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/draft", handler.DraftRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+ruleID.String()+"/draft", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body model.Rule
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, ruleID, body.ID)
	assert.Equal(t, model.RuleStatusDraft, body.Status)
}

func TestDraftRuleHandler_InvalidUUID(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()

	mockService := NewMockRuleService(ctrl)

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/draft", handler.DraftRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/invalid-uuid/draft", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0007", body["code"])
	assert.Equal(t, "Invalid Path Parameter", body["title"])
	assert.Equal(t, "Invalid rule ID format", body["message"])
}

func TestDraftRuleHandler_ServiceError(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		DraftRule(gomock.Any(), ruleID).
		Return(nil, model.NewInvalidTransitionError(model.RuleStatusActive, model.RuleStatusDraft))

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/draft", handler.DraftRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+ruleID.String()+"/draft", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0102", body["code"])
	assert.Equal(t, "Invalid State Transition", body["title"])
	assert.Contains(t, body["message"], "ACTIVE to DRAFT")
}

func TestDraftRuleHandler_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		DraftRule(gomock.Any(), ruleID).
		Return(nil, constant.ErrRuleNotFound)

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/draft", handler.DraftRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+ruleID.String()+"/draft", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0100", body["code"])
	assert.Equal(t, "Not Found", body["title"])
	assert.Equal(t, "Rule not found", body["message"])
}

func TestDraftRuleHandler_InternalError(t *testing.T) {
	ctrl := gomock.NewController(t)

	app := fiber.New()
	ruleID := testutil.MustDeterministicUUID(1)

	mockService := NewMockRuleService(ctrl)

	mockService.EXPECT().
		DraftRule(gomock.Any(), ruleID).
		Return(nil, errors.New("database connection failed"))

	handler := NewHandler(mockService)
	app.Post("/v1/rules/:id/draft", handler.DraftRule)

	req := httptest.NewRequest(http.MethodPost, "/v1/rules/"+ruleID.String()+"/draft", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "TRC-0004", body["code"])
	assert.Equal(t, "Internal Server Error", body["title"])
	assert.Equal(t, "An unexpected error occurred", body["message"])
}
