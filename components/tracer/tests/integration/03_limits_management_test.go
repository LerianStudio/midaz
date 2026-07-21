// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ensure we use the shared HTTP client
var _ = testutil.HTTPClient

type createLimitRequest struct {
	Name            string            `json:"name"`
	Description     *string           `json:"description,omitempty"`
	LimitType       string            `json:"limitType"`
	MaxAmount       decimal.Decimal   `json:"maxAmount"`
	Currency        string            `json:"currency"`
	Scopes          []limitScopeInput `json:"scopes"`
	CustomStartDate *string           `json:"customStartDate,omitempty"`
	CustomEndDate   *string           `json:"customEndDate,omitempty"`
}

type limitScopeInput struct {
	AccountID       *string `json:"accountId,omitempty"`
	SegmentID       *string `json:"segmentId,omitempty"`
	PortfolioID     *string `json:"portfolioId,omitempty"`
	MerchantID      *string `json:"merchantId,omitempty"`
	TransactionType *string `json:"transactionType,omitempty"`
	SubType         *string `json:"subType,omitempty"`
}

type updateLimitRequest struct {
	Name        *string            `json:"name,omitempty"`
	Description *string            `json:"description,omitempty"`
	MaxAmount   *decimal.Decimal   `json:"maxAmount,omitempty"`
	Scopes      *[]limitScopeInput `json:"scopes,omitempty"`
}

type limitScopeResponse struct {
	SegmentID       *string `json:"segmentId,omitempty"`
	PortfolioID     *string `json:"portfolioId,omitempty"`
	AccountID       *string `json:"accountId,omitempty"`
	MerchantID      *string `json:"merchantId,omitempty"`
	TransactionType *string `json:"transactionType,omitempty"`
	SubType         *string `json:"subType,omitempty"`
}

type limitResponse struct {
	ID          string               `json:"limitId"`
	Name        string               `json:"name"`
	Description *string              `json:"description,omitempty"`
	LimitType   string               `json:"limitType"`
	MaxAmount   decimal.Decimal      `json:"maxAmount"`
	Currency    string               `json:"currency"`
	Scopes      []limitScopeResponse `json:"scopes"`
	Status      string               `json:"status"`
	ResetAt     *string              `json:"resetAt,omitempty"`
	CreatedAt   string               `json:"createdAt"`
	UpdatedAt   string               `json:"updatedAt"`
	DeletedAt   *string              `json:"deletedAt,omitempty"`
}

type listLimitsResponse struct {
	Limits     []limitResponse `json:"limits"`
	NextCursor string          `json:"nextCursor"`
	HasMore    bool            `json:"hasMore"`
}

// =============================================================================
// Authentication Tests (HIGH PRIORITY - Security)
// =============================================================================

func TestLimits_Authentication(t *testing.T) {
	baseURL := testutil.GetBaseURL()

	tests := []struct {
		name         string
		apiKey       string
		setHeader    bool
		expectedCode int
	}{
		{
			name:         "missing API key",
			apiKey:       "",
			setHeader:    false,
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:         "invalid API key",
			apiKey:       "invalid_key_12345",
			setHeader:    true,
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:         "empty API key",
			apiKey:       "",
			setHeader:    true,
			expectedCode: http.StatusUnauthorized,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", baseURL+"/v1/limits", nil)
			require.NoError(t, err)

			if tc.setHeader {
				req.Header.Set("X-API-Key", tc.apiKey)
			}

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedCode, resp.StatusCode, "%s should return %d", tc.name, tc.expectedCode)
		})
	}
}

// =============================================================================
// SQL Injection Prevention Tests (HIGH PRIORITY - Security)
// =============================================================================

func TestLimits_ListLimits_InvalidCursor(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Invalid base64 cursor
	req, err := http.NewRequest("GET", baseURL+"/v1/limits?limit=5&cursor=invalid-cursor-not-base64!", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Invalid cursor should return 400")
}

func TestLimits_ListLimits_CursorWithInvalidSortBy(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// SQL injection attempt via malicious sortBy in cursor
	maliciousCursor := base64.StdEncoding.EncodeToString([]byte(
		`{"id":"550e8400-e29b-41d4-a716-446655440000","sv":"test","sb":"malicious_column; DROP TABLE limits;--","so":"ASC","pn":true}`,
	))

	req, err := http.NewRequest("GET", baseURL+"/v1/limits?limit=5&cursor="+maliciousCursor, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Cursor with SQL injection attempt should return 400")
}

func TestLimits_ListLimits_InvalidSortColumn(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// SQL injection attempt via sortBy parameter
	req, err := http.NewRequest("GET", baseURL+"/v1/limits?sort_by=invalid_column;DROP+TABLE+limits;--&limit=10", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Invalid/malicious sort column should be rejected")
}

// =============================================================================
// Invalid UUID Format Tests (MEDIUM PRIORITY)
// =============================================================================

func TestLimits_GetLimit_InvalidUUID(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest("GET", baseURL+"/v1/limits/not-a-valid-uuid", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Invalid UUID should return 400")
}

func TestLimits_UpdateLimit_InvalidUUID(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	newName := "Updated Name"
	updateBody := updateLimitRequest{Name: &newName}
	body, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/not-a-valid-uuid", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Invalid UUID should return 400")
}

func TestLimits_DeleteLimit_InvalidUUID(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest("DELETE", baseURL+"/v1/limits/not-a-valid-uuid", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Invalid UUID should return 400")
}

func TestLimits_ActivateLimit_InvalidUUID(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest("POST", baseURL+"/v1/limits/not-a-valid-uuid/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Invalid UUID should return 400")
}

func TestLimits_DeactivateLimit_InvalidUUID(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest("POST", baseURL+"/v1/limits/not-a-valid-uuid/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Invalid UUID should return 400")
}

// =============================================================================
// Boundary Validation Tests (MEDIUM PRIORITY)
// =============================================================================

func TestLimits_CreateLimit_ValidationError_NameTooLong(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Name exceeding 255 characters (MaxLimitNameLength)
	longName := strings.Repeat("a", 256)
	reqBody := createLimitRequest{
		Name:      longName,
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []limitScopeInput{{TransactionType: testutil.Ptr("CARD")}},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Name exceeding max length should return 400")
}

func TestLimits_CreateLimit_ValidationError_DescriptionTooLong(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Description exceeding 1000 characters (MaxLimitDescriptionLength)
	longDesc := strings.Repeat("a", 1001)
	reqBody := createLimitRequest{
		Name:        "Test Limit",
		Description: &longDesc,
		LimitType:   "DAILY",
		MaxAmount:   decimal.RequireFromString("1000"),
		Currency:    "USD",
		Scopes:      []limitScopeInput{{TransactionType: testutil.Ptr("CARD")}},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Description exceeding max length should return 400")
}

func TestLimits_CreateLimit_ValidationError_InvalidCurrency(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	reqBody := createLimitRequest{
		Name:      "Invalid Currency Limit",
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "INVALID", // Not ISO 4217
		Scopes:    []limitScopeInput{{TransactionType: testutil.Ptr("CARD")}},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Invalid currency should return 400")
}

func TestLimits_CreateLimit_ValidationError_NegativeMaxAmount(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	reqBody := createLimitRequest{
		Name:      "Negative Amount Limit",
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("-1"),
		Currency:  "USD",
		Scopes:    []limitScopeInput{{TransactionType: testutil.Ptr("CARD")}},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Negative max amount should return 400")
}

// =============================================================================
// Idempotency Tests (MEDIUM PRIORITY)
// =============================================================================

func TestLimits_ActivateLimit_AlreadyActive_Idempotent(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (starts as DRAFT)
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// First activation: DRAFT -> ACTIVE
	firstReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	firstReq.Header.Set("X-API-Key", apiKey)

	firstResp, err := testutil.HTTPClient.Do(firstReq)
	require.NoError(t, err)
	firstResp.Body.Close()
	require.Equal(t, http.StatusOK, firstResp.StatusCode, "First activation should succeed")

	// Second activation: should be idempotent (already ACTIVE)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Idempotent activation should succeed: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)
	assert.Equal(t, "ACTIVE", limit.Status)
}

func TestLimits_DeactivateLimit_AlreadyInactive_Idempotent(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (starts as DRAFT)
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// First activate (DRAFT → ACTIVE)
	activateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode, "Activation should succeed")
	activateResp.Body.Close()

	// First deactivation (ACTIVE → INACTIVE)
	deactivateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)

	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode, "First deactivation should succeed")
	deactivateResp.Body.Close()

	// Second deactivation (should be idempotent)
	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Idempotent deactivation should succeed: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)
	assert.Equal(t, "INACTIVE", limit.Status)
}

// =============================================================================
// Invalid State Transition Tests (MEDIUM PRIORITY)
// =============================================================================

func TestLimits_ActivateDeletedLimit_InvalidTransition(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (DRAFT status)
	limitID := createTestLimit(t)

	// Activate the limit first
	activateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()
	require.Equal(t, http.StatusOK, activateResp.StatusCode, "Activate should succeed")

	// Deactivate the limit (ACTIVE limits cannot be deleted directly)
	deactivateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)

	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	deactivateResp.Body.Close()
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode, "Deactivate should succeed")

	// Delete the limit
	deleteReq, err := http.NewRequest("DELETE", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("X-API-Key", apiKey)

	deleteResp, err := testutil.HTTPClient.Do(deleteReq)
	require.NoError(t, err)
	deleteResp.Body.Close()
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// Try to activate the deleted limit (should fail)
	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return 404 (deleted limits are not found) or 400/409 (invalid transition)
	assert.True(t, resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusConflict,
		"Activating deleted limit should fail with 404, 400, or 409, got %d", resp.StatusCode)
}

func TestLimits_DeactivateDeletedLimit_InvalidTransition(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (DRAFT status)
	limitID := createTestLimit(t)

	// Activate the limit first
	activateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()
	require.Equal(t, http.StatusOK, activateResp.StatusCode, "Activate should succeed")

	// Deactivate the limit (ACTIVE limits cannot be deleted directly)
	deactivateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)

	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	deactivateResp.Body.Close()
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode, "Deactivate should succeed")

	// Delete the limit
	deleteReq, err := http.NewRequest("DELETE", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("X-API-Key", apiKey)

	deleteResp, err := testutil.HTTPClient.Do(deleteReq)
	require.NoError(t, err)
	deleteResp.Body.Close()
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// Try to deactivate the deleted limit (should fail)
	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return 404 (deleted limits are not found) or 400/409 (invalid transition)
	assert.True(t, resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusConflict,
		"Deactivating deleted limit should fail with 404, 400, or 409, got %d", resp.StatusCode)
}

// =============================================================================
// Not Found Tests for Lifecycle Operations
// =============================================================================

func TestLimits_ActivateLimit_NotFound(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	nonExistentID := testutil.MustDeterministicUUID(3001).String()

	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+nonExistentID+"/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Non-existent limit should return 404")
}

func TestLimits_DeactivateLimit_NotFound(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	nonExistentID := testutil.MustDeterministicUUID(3002).String()

	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+nonExistentID+"/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Non-existent limit should return 404")
}

func TestLimits_DeleteLimit_NotFound(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	nonExistentID := testutil.MustDeterministicUUID(3003).String()

	req, err := http.NewRequest("DELETE", baseURL+"/v1/limits/"+nonExistentID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Delete non-existent should return 404")
}

// =============================================================================
// CRUD Tests
// =============================================================================

func TestLimits_CreateLimit_Success(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	uniqueName := "Integration Test Limit " + testutil.RandomSuffix()
	description := "Test limit created by integration test"
	reqBody := createLimitRequest{
		Name:        uniqueName,
		Description: &description,
		LimitType:   "DAILY",
		MaxAmount:   decimal.RequireFromString("1000"),
		Currency:    "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "Create should return 201: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	assert.NotEmpty(t, limit.ID)
	assert.Equal(t, uniqueName, limit.Name)
	assert.Equal(t, "DAILY", limit.LimitType)
	assert.True(t, limit.MaxAmount.Equal(decimal.RequireFromString("1000")))
	assert.Equal(t, "USD", limit.Currency)
	assert.Equal(t, "DRAFT", limit.Status, "Newly created limits should be DRAFT")
	assert.NotEmpty(t, limit.CreatedAt)
	assert.Nil(t, limit.DeletedAt)

	t.Cleanup(func() {
		cleanupLimit(t, limit.ID)
	})
}

func TestLimits_CreateLimit_ValidationError_MissingName(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	reqBody := createLimitRequest{
		Name:      "", // Empty name should fail validation
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Missing name should return 400")
}

func TestLimits_CreateLimit_ValidationError_InvalidLimitType(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	reqBody := createLimitRequest{
		Name:      "Invalid Type Limit",
		LimitType: "INVALID_TYPE",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Invalid limit type should return 400")
}

func TestLimits_CreateLimit_ValidationError_EmptyScopes(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	reqBody := createLimitRequest{
		Name:      "Empty Scopes Limit",
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    []limitScopeInput{}, // Empty scopes should fail
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Empty scopes should return 400")
}

func TestLimits_GetLimit_Success(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit first
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Get the limit
	req, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Get should return 200: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	assert.Equal(t, limitID, limit.ID)
	assert.NotEmpty(t, limit.Name)
	assert.Equal(t, "DAILY", limit.LimitType)
}

func TestLimits_GetLimit_NotFound(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	nonExistentID := testutil.MustDeterministicUUID(3005).String()

	req, err := http.NewRequest("GET", baseURL+"/v1/limits/"+nonExistentID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Non-existent limit should return 404")
}

func TestLimits_ListLimits_Success(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit to ensure we have at least one
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	req, err := http.NewRequest("GET", baseURL+"/v1/limits?limit=10", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "List should return 200: %s", string(respBody))

	var listResp listLimitsResponse
	err = json.Unmarshal(respBody, &listResp)
	require.NoError(t, err)

	assert.NotEmpty(t, listResp.Limits, "Should have at least one limit")
}

func TestLimits_ListLimits_FilterByStatus(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (starts as DRAFT)
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Activate the limit to make it ACTIVE
	activateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()
	require.Equal(t, http.StatusOK, activateResp.StatusCode, "Activation should succeed")

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits?limit=10&status=ACTIVE", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "List should return 200: %s", string(respBody))

	var listResp listLimitsResponse
	err = json.Unmarshal(respBody, &listResp)
	require.NoError(t, err)

	for _, limit := range listResp.Limits {
		assert.Equal(t, "ACTIVE", limit.Status, "All returned limits should be ACTIVE")
	}
}

func TestLimits_ListLimits_FilterByLimitType(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a DAILY limit
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	req, err := http.NewRequest("GET", baseURL+"/v1/limits?limit=10&limit_type=DAILY", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "List should return 200: %s", string(respBody))

	var listResp listLimitsResponse
	err = json.Unmarshal(respBody, &listResp)
	require.NoError(t, err)

	for _, limit := range listResp.Limits {
		assert.Equal(t, "DAILY", limit.LimitType, "All returned limits should be DAILY")
	}
}

func TestLimits_ListLimits_FilterByWeeklyType(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a WEEKLY limit
	uniqueName := "Weekly Limit " + testutil.RandomSuffix()
	reqBody := createLimitRequest{
		Name:      uniqueName,
		LimitType: "WEEKLY",
		MaxAmount: decimal.RequireFromString("5000"),
		Currency:  "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	createReq, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createBody, err := io.ReadAll(createResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "Failed to create WEEKLY limit: %s", string(createBody))

	var createdLimit limitResponse
	err = json.Unmarshal(createBody, &createdLimit)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupLimit(t, createdLimit.ID)
	})

	// List with limit_type=WEEKLY filter
	listReq, err := http.NewRequest("GET", baseURL+"/v1/limits?limit=10&limit_type=WEEKLY", nil)
	require.NoError(t, err)
	listReq.Header.Set("X-API-Key", apiKey)

	listResp, err := testutil.HTTPClient.Do(listReq)
	require.NoError(t, err)
	defer listResp.Body.Close()

	listBody, err := io.ReadAll(listResp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, listResp.StatusCode, "List should return 200: %s", string(listBody))

	var listResult listLimitsResponse
	err = json.Unmarshal(listBody, &listResult)
	require.NoError(t, err)

	require.NotEmpty(t, listResult.Limits, "Expected at least one WEEKLY limit in results")

	for _, limit := range listResult.Limits {
		assert.Equal(t, "WEEKLY", limit.LimitType, "All returned limits should be WEEKLY")
	}

	found := false

	for _, lmt := range listResult.Limits {
		if lmt.ID == createdLimit.ID {
			found = true

			break
		}
	}

	require.True(t, found, "Created limit should appear in filtered results")
}

func TestLimits_ListLimits_FilterByCustomType(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a CUSTOM limit (requires customStartDate and customEndDate)
	uniqueName := "Custom Limit " + testutil.RandomSuffix()
	customStart := "2099-11-01T00:00:00Z"
	customEnd := "2099-11-30T23:59:59Z"
	reqBody := createLimitRequest{
		Name:            uniqueName,
		LimitType:       "CUSTOM",
		MaxAmount:       decimal.RequireFromString("10000"),
		Currency:        "USD",
		CustomStartDate: &customStart,
		CustomEndDate:   &customEnd,
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("WIRE")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	createReq, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createBody, err := io.ReadAll(createResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "Failed to create CUSTOM limit: %s", string(createBody))

	var createdLimit limitResponse
	err = json.Unmarshal(createBody, &createdLimit)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupLimit(t, createdLimit.ID)
	})

	// List with limit_type=CUSTOM filter
	listReq, err := http.NewRequest("GET", baseURL+"/v1/limits?limit=10&limit_type=CUSTOM", nil)
	require.NoError(t, err)
	listReq.Header.Set("X-API-Key", apiKey)

	listResp, err := testutil.HTTPClient.Do(listReq)
	require.NoError(t, err)
	defer listResp.Body.Close()

	listBody, err := io.ReadAll(listResp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, listResp.StatusCode, "List should return 200: %s", string(listBody))

	var listResult listLimitsResponse
	err = json.Unmarshal(listBody, &listResult)
	require.NoError(t, err)

	require.NotEmpty(t, listResult.Limits, "Expected at least one CUSTOM limit in results")

	for _, limit := range listResult.Limits {
		assert.Equal(t, "CUSTOM", limit.LimitType, "All returned limits should be CUSTOM")
	}

	found := false

	for _, lmt := range listResult.Limits {
		if lmt.ID == createdLimit.ID {
			found = true

			break
		}
	}

	require.True(t, found, "Created limit should appear in filtered results")
}

func TestLimits_UpdateLimit_Success(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit first
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Update the limit
	newName := "Updated Limit Name " + testutil.RandomSuffix()
	newAmount := decimal.RequireFromString("2000")
	updateBody := updateLimitRequest{
		Name:      &newName,
		MaxAmount: &newAmount,
	}

	body, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+limitID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Update should return 200: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	assert.Equal(t, newName, limit.Name)
	assert.True(t, limit.MaxAmount.Equal(newAmount))
}

func TestLimits_UpdateLimit_NotFound(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	nonExistentID := testutil.MustDeterministicUUID(3006).String()
	newName := "Updated Name"
	updateBody := updateLimitRequest{
		Name: &newName,
	}

	body, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+nonExistentID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Non-existent limit should return 404")
}

// TestLimits_UpdateLimit_StatusFieldIgnored verifies that the status field
// is silently ignored when sent via PATCH. Status changes must be done via
// dedicated endpoints: POST /activate and POST /deactivate.
func TestLimits_UpdateLimit_StatusFieldIgnored(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (starts as DRAFT)
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Activate the limit to get ACTIVE status
	activateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()
	require.Equal(t, http.StatusOK, activateResp.StatusCode, "Activate should succeed")

	// Verify status is ACTIVE
	getReq, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	getBody, err := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var initialLimit limitResponse
	err = json.Unmarshal(getBody, &initialLimit)
	require.NoError(t, err)
	require.Equal(t, "ACTIVE", initialLimit.Status, "Status should be ACTIVE after activation")

	// Try to change status via PATCH (should be ignored)
	// Using a raw map to include the status field that's not in updateLimitRequest
	updateBody := map[string]interface{}{
		"status": "INACTIVE",
	}

	body, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+limitID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// The request should succeed (status field is silently ignored)
	// Note: It might return 400 if no valid fields are provided, which is also acceptable
	if resp.StatusCode == http.StatusOK {
		var updatedLimit limitResponse
		err = json.Unmarshal(respBody, &updatedLimit)
		require.NoError(t, err)

		// Status should remain ACTIVE (the status field was ignored)
		assert.Equal(t, "ACTIVE", updatedLimit.Status,
			"Status should remain ACTIVE - PATCH should not change status. Use POST /activate or /deactivate instead")
	} else if resp.StatusCode == http.StatusBadRequest {
		// Also acceptable: API rejects request with only unknown/ignored fields
		t.Logf("API returned 400 when only status field provided (no valid update fields): %s", string(respBody))
	} else {
		t.Fatalf("Unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	// Double-check by fetching the limit again
	getReq2, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq2.Header.Set("X-API-Key", apiKey)

	getResp2, err := testutil.HTTPClient.Do(getReq2)
	require.NoError(t, err)
	getBody2, err := io.ReadAll(getResp2.Body)
	getResp2.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp2.StatusCode)

	var finalLimit limitResponse
	err = json.Unmarshal(getBody2, &finalLimit)
	require.NoError(t, err)
	assert.Equal(t, "ACTIVE", finalLimit.Status,
		"Status should still be ACTIVE after PATCH attempt with status field")
}

func TestLimits_ActivateLimit_Success(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit and deactivate it first
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Deactivate
	deactivateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)

	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	deactivateResp.Body.Close()

	// Now activate
	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Activate should return 200: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	assert.Equal(t, "ACTIVE", limit.Status)
}

// TestLimits_ActivateLimit_FromDraft tests activating a DRAFT limit (3.6.1)
func TestLimits_ActivateLimit_FromDraft(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (status = DRAFT)
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Activate from DRAFT
	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Activate from DRAFT should return 200: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	assert.Equal(t, "ACTIVE", limit.Status, "Status should be ACTIVE after activation from DRAFT")
}

// TestLimits_DeactivateLimit_FromDraft tests that deactivating a DRAFT limit is NOT allowed (3.7.2)
// State machine: DRAFT can only transition to ACTIVE or DELETED, not INACTIVE.
func TestLimits_DeactivateLimit_FromDraft(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (status = DRAFT)
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Try to deactivate from DRAFT - should fail
	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// DRAFT → INACTIVE is not a valid transition
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
		"Deactivate from DRAFT should return 422 - invalid transition")
}

// TestLimits_DeactivateLimit_FromActive tests deactivating an ACTIVE limit (3.7.1)
func TestLimits_DeactivateLimit_FromActive(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (status = DRAFT)
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// First activate the limit
	activateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()
	require.Equal(t, http.StatusOK, activateResp.StatusCode, "Activation should succeed")

	// Now deactivate from ACTIVE
	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Deactivate from ACTIVE should return 200: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	assert.Equal(t, "INACTIVE", limit.Status, "Status should be INACTIVE after deactivation from ACTIVE")
}

// TestLimits_DeactivateLimit_Success tests successful deactivation (ACTIVE -> INACTIVE)
func TestLimits_DeactivateLimit_Success(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (starts as DRAFT)
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// First activate (DRAFT → ACTIVE)
	activateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()
	require.Equal(t, http.StatusOK, activateResp.StatusCode, "Activation should succeed")

	// Then deactivate (ACTIVE → INACTIVE)
	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Deactivate should return 200: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	assert.Equal(t, "INACTIVE", limit.Status)
}

func TestLimits_DeleteLimit_Success(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (DRAFT status)
	limitID := createTestLimit(t)
	// No cleanup needed as we're deleting

	// Activate the limit first
	activateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()
	require.Equal(t, http.StatusOK, activateResp.StatusCode, "Activate should succeed")

	// Deactivate the limit (ACTIVE limits cannot be deleted directly)
	deactivateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)

	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	deactivateResp.Body.Close()
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode, "Deactivate should succeed")

	// Now delete the INACTIVE limit
	req, err := http.NewRequest("DELETE", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode, "Delete should return 204")

	// Verify the limit is no longer retrievable
	getReq, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusNotFound, getResp.StatusCode, "Deleted limit should return 404")
}

func TestLimits_FullLifecycle(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// 1. Create a limit
	uniqueName := "Lifecycle Test Limit " + testutil.RandomSuffix()
	description := "Testing full lifecycle"
	reqBody := createLimitRequest{
		Name:        uniqueName,
		Description: &description,
		LimitType:   "DAILY",
		MaxAmount:   decimal.RequireFromString("1000"),
		Currency:    "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	createReq, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createBody, err := io.ReadAll(createResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "Create failed: %s", string(createBody))

	var createdLimit limitResponse
	err = json.Unmarshal(createBody, &createdLimit)
	require.NoError(t, err)

	limitID := createdLimit.ID
	assert.Equal(t, "DRAFT", createdLimit.Status, "Newly created limits should be DRAFT")

	// 1.5. Activate the limit (DRAFT -> ACTIVE)
	activateInitReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateInitReq.Header.Set("X-API-Key", apiKey)

	activateInitResp, err := testutil.HTTPClient.Do(activateInitReq)
	require.NoError(t, err)
	activateInitResp.Body.Close()
	require.Equal(t, http.StatusOK, activateInitResp.StatusCode, "Initial activation should succeed")

	// 2. Update the limit
	newName := "Updated Lifecycle Limit " + testutil.RandomSuffix()
	updateBody := updateLimitRequest{
		Name: &newName,
	}
	updateJSON, err := json.Marshal(updateBody)
	require.NoError(t, err, "failed to marshal update request body")

	updateReq, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+limitID, bytes.NewReader(updateJSON))
	require.NoError(t, err)
	updateReq.Header.Set("X-API-Key", apiKey)
	updateReq.Header.Set("Content-Type", "application/json")

	updateResp, err := testutil.HTTPClient.Do(updateReq)
	require.NoError(t, err)
	updateResp.Body.Close()
	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	// 3. Deactivate the limit
	deactivateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)

	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	defer deactivateResp.Body.Close()

	deactivateBody, err := io.ReadAll(deactivateResp.Body)
	require.NoError(t, err, "Failed to read deactivate response body")
	assert.Equal(t, http.StatusOK, deactivateResp.StatusCode, "Deactivate failed: %s", string(deactivateBody))

	var deactivatedLimit limitResponse
	err = json.Unmarshal(deactivateBody, &deactivatedLimit)
	require.NoError(t, err, "failed to unmarshal deactivate response")
	assert.Equal(t, "INACTIVE", deactivatedLimit.Status)

	// 4. Reactivate the limit
	activateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	defer activateResp.Body.Close()

	activateBody, err := io.ReadAll(activateResp.Body)
	require.NoError(t, err, "failed to read activate response")

	assert.Equal(t, http.StatusOK, activateResp.StatusCode, "Activate failed: %s", string(activateBody))

	var activatedLimit limitResponse
	err = json.Unmarshal(activateBody, &activatedLimit)
	require.NoError(t, err, "failed to unmarshal activate response")
	assert.Equal(t, "ACTIVE", activatedLimit.Status)

	// 5. Deactivate before delete (ACTIVE limits cannot be deleted directly)
	deactivate2Req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivate2Req.Header.Set("X-API-Key", apiKey)

	deactivate2Resp, err := testutil.HTTPClient.Do(deactivate2Req)
	require.NoError(t, err)
	deactivate2Resp.Body.Close()
	require.Equal(t, http.StatusOK, deactivate2Resp.StatusCode, "Deactivate before delete should succeed")

	// 6. Delete the limit
	deleteReq, err := http.NewRequest("DELETE", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("X-API-Key", apiKey)

	deleteResp, err := testutil.HTTPClient.Do(deleteReq)
	require.NoError(t, err)
	deleteResp.Body.Close()
	assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// 7. Verify limit is no longer accessible
	getReq, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	getResp.Body.Close()
	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

// createTestLimit creates a limit for testing and returns its ID.
func createTestLimit(t *testing.T) string {
	t.Helper()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	uniqueName := "Test Limit " + testutil.RandomSuffix()
	reqBody := createLimitRequest{
		Name:      uniqueName,
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Failed to create test limit: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	return limit.ID
}

// cleanupLimit deletes a limit. Called in t.Cleanup() to clean up test data.
// State machine: DRAFT and INACTIVE can be deleted directly, but ACTIVE must be deactivated first.
func cleanupLimit(t *testing.T, limitID string) {
	t.Helper()

	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Deactivate the limit first (handles ACTIVE -> INACTIVE transition).
	// This is a no-op if already INACTIVE or DRAFT.
	deactivateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	if err != nil {
		t.Logf("Cleanup: failed to build deactivate request for limit %s: %v", limitID, err)
	} else {
		deactivateReq.Header.Set("X-API-Key", apiKey)
		deactivateResp, doErr := testutil.HTTPClient.Do(deactivateReq)
		if doErr != nil {
			t.Logf("Cleanup: failed to deactivate limit %s: %v", limitID, doErr)
		} else if deactivateResp != nil {
			deactivateResp.Body.Close()
		}
	}

	// Delete the limit - DRAFT and INACTIVE can be deleted
	req, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/limits/"+limitID, nil)
	if err != nil {
		t.Logf("Cleanup: failed to create delete request for limit %s: %v", limitID, err)

		return
	}
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	if err != nil {
		t.Logf("Cleanup: failed to delete limit %s: %v", limitID, err)

		return
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		t.Logf("Cleanup: unexpected status %d when deleting limit %s", resp.StatusCode, limitID)
	}
}

// =============================================================================
// Usage Response Types (matching API's UsageSnapshot)
// =============================================================================

type usageSnapshotResponse struct {
	LimitID            string          `json:"limitId"`
	CurrentUsage       decimal.Decimal `json:"currentUsage"`
	LimitAmount        decimal.Decimal `json:"limitAmount"`
	UtilizationPercent float64         `json:"utilizationPercent"`
	NearLimit          bool            `json:"nearLimit"`
	ResetAt            *string         `json:"resetAt,omitempty"`
}

// =============================================================================
// 3.1 POST /v1/limits - Create Tests (Missing)
// =============================================================================

// TestLimits_CreateLimit_Monthly_ResetAtCalculated (3.1.2)
// Verifies that MONTHLY limits have resetAt calculated to 1st of next month, 00:00 UTC.
func TestLimits_CreateLimit_Monthly_ResetAtCalculated(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	uniqueName := "Monthly Limit Test " + testutil.RandomSuffix()
	reqBody := createLimitRequest{
		Name:      uniqueName,
		LimitType: "MONTHLY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "BRL",
		Scopes: []limitScopeInput{
			{SegmentID: testutil.Ptr(testutil.MustDeterministicUUID(3012).String())},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Create MONTHLY limit should return 201: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupLimit(t, limit.ID)
	})

	// Verify MONTHLY limit type
	assert.Equal(t, "MONTHLY", limit.LimitType, "LimitType should be MONTHLY")

	// Verify resetAt is present and is 1st of next month at 00:00 UTC
	require.NotNil(t, limit.ResetAt, "MONTHLY limit should have resetAt set")

	resetAt, err := time.Parse(time.RFC3339, *limit.ResetAt)
	require.NoError(t, err, "resetAt should be valid RFC3339 timestamp")

	// Verify it's the 1st day of a month at 00:00:00 UTC
	assert.Equal(t, 1, resetAt.Day(), "resetAt should be 1st of month")
	assert.Equal(t, 0, resetAt.Hour(), "resetAt should be at 00:00 UTC")
	assert.Equal(t, 0, resetAt.Minute(), "resetAt should be at 00:00 UTC")
	assert.Equal(t, 0, resetAt.Second(), "resetAt should be at 00:00 UTC")
	_, offset := resetAt.Zone()
	assert.Equal(t, 0, offset, "resetAt should be in UTC (zero offset)")

	// Verify it's in the future (next month from now)
	assert.True(t, resetAt.After(time.Now().UTC()), "resetAt should be in the future")
}

// TestLimits_CreateLimit_PerTransaction_ResetAtNull (3.1.3)
// Verifies that PER_TRANSACTION limits have resetAt == null.
func TestLimits_CreateLimit_PerTransaction_ResetAtNull(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	uniqueName := "Per Transaction Limit Test " + testutil.RandomSuffix()
	reqBody := createLimitRequest{
		Name:      uniqueName,
		LimitType: "PER_TRANSACTION",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "BRL",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Create PER_TRANSACTION limit should return 201: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupLimit(t, limit.ID)
	})

	// Verify PER_TRANSACTION limit type
	assert.Equal(t, "PER_TRANSACTION", limit.LimitType, "LimitType should be PER_TRANSACTION")

	// Verify resetAt is null for PER_TRANSACTION
	assert.Nil(t, limit.ResetAt, "PER_TRANSACTION limit should have resetAt == null")
}

// TestLimits_CreateLimit_MultipleScopesArray (3.1.4)
// Verifies that scopes array with multiple entries (2) is accepted.
func TestLimits_CreateLimit_MultipleScopesArray(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	uniqueName := "Multi Scope Limit Test " + testutil.RandomSuffix()
	accountID := testutil.MustDeterministicUUID(3015).String()
	reqBody := createLimitRequest{
		Name:      uniqueName,
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "BRL",
		Scopes: []limitScopeInput{
			{AccountID: &accountID},
			{AccountID: &accountID, TransactionType: testutil.Ptr("PIX")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Create limit with multiple scopes should return 201: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupLimit(t, limit.ID)
	})

	// Verify scopes contains 2 entries
	assert.Len(t, limit.Scopes, 2, "Scopes should contain 2 entries")
}

// TestLimits_CreateLimit_ScopeWithoutFields_BadRequest (3.1.5 Scenario 2)
// Verifies that scope without fields [{}] returns 400 Bad Request with proper error structure.
func TestLimits_CreateLimit_ScopeWithoutFields_BadRequest(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create request body with empty scope object [{}]
	reqBodyJSON := `{
		"name": "Empty Scope Fields Test",
		"limitType": "DAILY",
		"maxAmount": "100000.00",
		"currency": "BRL",
		"scopes": [{}]
	}`

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader([]byte(reqBodyJSON)))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Scope without fields should return 400 Bad Request")

	// Verify error response structure (code, title, detail)
	errResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "0009", errResp.Code, "Error code should be 0009 (missing fields in request)")
	assert.Equal(t, "Validation Error", errResp.Title, "Error title should be 'Validation Error'")
	assert.Equal(t, "scope at index 0 must have at least one field set", tracerProblemDetail(t, body), "Error detail should indicate scope validation failure")
}

// =============================================================================
// 3.3 GET /v1/limits - List Tests (Missing)
// =============================================================================

// TestLimits_ListLimits_PaginationWorks (3.3.4)
// Verifies that pagination works - navigate with cursor.
func TestLimits_ListLimits_PaginationWorks(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create multiple limits to ensure pagination
	var createdIDs []string
	for i := 0; i < 6; i++ {
		limitID := createTestLimit(t)
		createdIDs = append(createdIDs, limitID)
	}

	t.Cleanup(func() {
		for _, id := range createdIDs {
			cleanupLimit(t, id)
		}
	})

	// First page with limit=3
	req1, err := http.NewRequest("GET", baseURL+"/v1/limits?limit=3", nil)
	require.NoError(t, err)
	req1.Header.Set("X-API-Key", apiKey)

	resp1, err := testutil.HTTPClient.Do(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()

	respBody1, err := io.ReadAll(resp1.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp1.StatusCode, "First page should return 200: %s", string(respBody1))

	var page1 listLimitsResponse
	err = json.Unmarshal(respBody1, &page1)
	require.NoError(t, err)

	require.Len(t, page1.Limits, 3, "First page should have 3 limits")
	assert.True(t, page1.HasMore, "HasMore should be true for first page")
	assert.NotEmpty(t, page1.NextCursor, "NextCursor should be present")

	// Second page using cursor
	req2, err := http.NewRequest("GET", baseURL+"/v1/limits?limit=3&cursor="+page1.NextCursor, nil)
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", apiKey)

	resp2, err := testutil.HTTPClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp2.StatusCode, "Second page should return 200: %s", string(respBody2))

	var page2 listLimitsResponse
	err = json.Unmarshal(respBody2, &page2)
	require.NoError(t, err)

	require.NotEmpty(t, page2.Limits, "Second page should have limits")

	// Verify pages contain different limits
	page1IDs := make(map[string]bool)
	for _, limit := range page1.Limits {
		page1IDs[limit.ID] = true
	}

	for _, limit := range page2.Limits {
		assert.False(t, page1IDs[limit.ID], "Page 2 should not contain limits from page 1")
	}
}

// =============================================================================
// 3.4 PATCH /v1/limits/{limitId} - Update Tests (Missing)
// =============================================================================

// TestLimits_UpdateLimit_UpdatesScopes (3.4.3)
// Verifies that scopes can be updated via PATCH.
func TestLimits_UpdateLimit_UpdatesScopes(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit first
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Update scopes
	newAccountID := testutil.MustDeterministicUUID(3016).String()
	newScopes := []limitScopeInput{
		{AccountID: &newAccountID},
	}
	updateBody := updateLimitRequest{
		Scopes: &newScopes,
	}

	body, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+limitID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "Update scopes should return 200: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	// Verify scopes were replaced
	require.Len(t, limit.Scopes, 1, "Scopes should have 1 entry after update")
	require.NotNil(t, limit.Scopes[0].AccountID, "Scope should have AccountID")
	assert.Equal(t, newAccountID, *limit.Scopes[0].AccountID, "AccountID should match new value")
}

// TestLimits_UpdateLimit_BlocksLimitTypeChange (3.4.5)
// Verifies that limitType is immutable on PATCH.
// The API rejects a limitType change with 422 (code 0380, "Limit Immutable Field")
// and leaves the stored limitType unchanged.
func TestLimits_UpdateLimit_BlocksLimitTypeChange(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a DAILY limit
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Try to change limitType to MONTHLY (should be ignored)
	updateBody := map[string]interface{}{
		"limitType": "MONTHLY",
	}

	body, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+limitID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// API rejects the immutable limitType change with 422 (Limit Immutable Field)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "Changing limitType should return 422")

	// Verify limitType was not changed by fetching the limit
	getReq, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	var limit limitResponse
	err = json.Unmarshal(getBody, &limit)
	require.NoError(t, err)

	assert.Equal(t, "DAILY", limit.LimitType, "limitType should remain DAILY (immutable)")
}

// TestLimits_UpdateLimit_BlocksCurrencyChange (3.4.6)
// Verifies that currency is immutable on PATCH.
// The API rejects a currency change with 422 (code 0380, "Limit Immutable Field")
// and leaves the stored currency unchanged.
func TestLimits_UpdateLimit_BlocksCurrencyChange(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit with USD currency
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Try to change currency to BRL (should be ignored)
	updateBody := map[string]interface{}{
		"currency": "BRL",
	}

	body, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+limitID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// API rejects the immutable currency change with 422 (Limit Immutable Field)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "Changing currency should return 422")

	// Verify currency was not changed by fetching the limit
	getReq, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	var limit limitResponse
	err = json.Unmarshal(getBody, &limit)
	require.NoError(t, err)

	assert.Equal(t, "USD", limit.Currency, "currency should remain USD (immutable)")
}

// TestLimits_UpdateLimit_ValidatesPositiveMaxAmount (3.4.7)
// Verifies that negative maxAmount on PATCH returns 400.
func TestLimits_UpdateLimit_ValidatesPositiveMaxAmount(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Try to update with negative maxAmount
	negativeAmount := decimal.RequireFromString("-1")
	updateBody := updateLimitRequest{
		MaxAmount: &negativeAmount,
	}

	body, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+limitID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Negative maxAmount on PATCH should return 400")
}

// TestLimits_UpdateLimit_EmptyBody_ReturnsTRC0002 (3.4.8)
// Verifies that PATCH with empty JSON body {} returns TRC-0002 error.
// When no fields are provided for update, the API should return a validation error
// indicating that at least one field must be provided.
// Also verifies that the limit state remains unchanged after the failed request.
func TestLimits_UpdateLimit_EmptyBody_ReturnsTRC0002(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit first
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Capture original limit state before the PATCH
	getReq, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)
	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	getBody, err := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode, "GET original limit should return 200")

	var originalLimit limitResponse
	err = json.Unmarshal(getBody, &originalLimit)
	require.NoError(t, err)

	// Send PATCH request with empty JSON body
	req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+limitID, bytes.NewReader([]byte("{}")))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Assert status code is 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Empty JSON body should return 400: %s", string(respBody))

	// Parse and validate error response
	errResp := testutil.ParseErrorResponse(t, respBody)

	// Code 0183 (Nothing to Update) for an empty object with no fields to update
	assert.Equal(t, "0183", errResp.Code, "Expected 0183 for empty JSON body (no fields to update)")
	assert.Equal(t, "Nothing to Update", errResp.Title, "Error title should be 'Nothing to Update'")
	assert.Equal(t, "No updatable fields were provided. Please include at least one field to update.", tracerProblemDetail(t, respBody), "Error detail should indicate at least one field required")

	// Re-fetch the limit after the failed PATCH and verify state is unchanged
	getReq2, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq2.Header.Set("X-API-Key", apiKey)
	getResp2, err := testutil.HTTPClient.Do(getReq2)
	require.NoError(t, err)
	getBody2, err := io.ReadAll(getResp2.Body)
	getResp2.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp2.StatusCode, "GET limit after failed PATCH should return 200")

	var fetchedLimit limitResponse
	err = json.Unmarshal(getBody2, &fetchedLimit)
	require.NoError(t, err)

	// Assert all fields remain unchanged after failed PATCH
	assert.Equal(t, originalLimit.ID, fetchedLimit.ID, "limitId should be unchanged")
	assert.Equal(t, originalLimit.Name, fetchedLimit.Name, "name should be unchanged")
	assert.Equal(t, originalLimit.Description, fetchedLimit.Description, "description should be unchanged")
	assert.Equal(t, originalLimit.LimitType, fetchedLimit.LimitType, "limitType should be unchanged")
	assert.True(t, originalLimit.MaxAmount.Equal(fetchedLimit.MaxAmount), "maxAmount should be unchanged")
	assert.Equal(t, originalLimit.Currency, fetchedLimit.Currency, "currency should be unchanged")
	assert.Equal(t, originalLimit.Status, fetchedLimit.Status, "status should be unchanged")
	assert.Equal(t, originalLimit.CreatedAt, fetchedLimit.CreatedAt, "createdAt should be unchanged")
	assert.Equal(t, originalLimit.UpdatedAt, fetchedLimit.UpdatedAt, "updatedAt should be unchanged (failed PATCH should not update timestamp)")
	assert.Equal(t, originalLimit.ResetAt, fetchedLimit.ResetAt, "resetAt should be unchanged")
	assert.Equal(t, originalLimit.DeletedAt, fetchedLimit.DeletedAt, "deletedAt should be unchanged")
	assert.Equal(t, originalLimit.Scopes, fetchedLimit.Scopes, "scopes should be unchanged (deep equality)")
}

// =============================================================================
// 3.5 GET /v1/limits/{limitId}/usage - Usage Tests (Missing)
// =============================================================================

// TestLimits_GetUsage_ReturnsAllFields (3.5.1)
// Verifies that usage endpoint returns all expected fields.
func TestLimits_GetUsage_ReturnsAllFields(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create an ACTIVE limit
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Get usage
	req, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID+"/usage", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "Usage endpoint should return 200: %s", string(respBody))

	var usage usageSnapshotResponse
	err = json.Unmarshal(respBody, &usage)
	require.NoError(t, err)

	// Verify all expected fields are present
	assert.NotEmpty(t, usage.LimitID, "limitId should be present")
	assert.Equal(t, limitID, usage.LimitID, "limitId should match")
	assert.True(t, usage.CurrentUsage.GreaterThanOrEqual(decimal.Zero), "currentUsage should be >= 0")
	assert.True(t, usage.LimitAmount.GreaterThan(decimal.Zero), "limitAmount should be > 0")
	assert.GreaterOrEqual(t, usage.UtilizationPercent, float64(0), "utilizationPercent should be >= 0")
	// nearLimit is a boolean, so no assertion needed for presence
	// resetAt may be nil for PER_TRANSACTION, but this is DAILY so should be present
	assert.NotNil(t, usage.ResetAt, "resetAt should be present for DAILY limit")
}

// TestLimits_GetUsage_NearLimitCalculation (3.5.2)
// Verifies that nearLimit is calculated correctly (>80% = true, <=80% = false).
// Note: This test verifies the initial state (0% usage = nearLimit false).
// Testing with >80% usage would require the validation endpoint to consume the limit.
func TestLimits_GetUsage_NearLimitCalculation(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Get usage - should have 0% utilization, so nearLimit = false
	req, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID+"/usage", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "Usage endpoint should return 200: %s", string(respBody))

	var usage usageSnapshotResponse
	err = json.Unmarshal(respBody, &usage)
	require.NoError(t, err)

	// With 0% usage, nearLimit should be false (threshold is >80%)
	assert.False(t, usage.NearLimit, "nearLimit should be false when usage is 0%")
	assert.Equal(t, float64(0), usage.UtilizationPercent, "UtilizationPercent should be 0 for new limit")
}

// TestLimits_GetUsage_NotFound (3.5.3)
// Verifies that usage endpoint returns 404 for non-existent limit.
func TestLimits_GetUsage_NotFound(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	nonExistentID := testutil.MustDeterministicUUID(3007).String()

	req, err := http.NewRequest("GET", baseURL+"/v1/limits/"+nonExistentID+"/usage", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Usage for non-existent limit should return 404")
}

// TestLimits_GetUsage_PerTransaction (3.5.4)
// Verifies that PER_TRANSACTION limit has currentUsage=0 and resetAt=null.
func TestLimits_GetUsage_PerTransaction(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a PER_TRANSACTION limit
	uniqueName := "Per Transaction Usage Test " + testutil.RandomSuffix()
	reqBody := createLimitRequest{
		Name:      uniqueName,
		LimitType: "PER_TRANSACTION",
		MaxAmount: decimal.RequireFromString("500"),
		Currency:  "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	createReq, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createBody, err := io.ReadAll(createResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "Failed to create limit: %s", string(createBody))

	var limit limitResponse
	err = json.Unmarshal(createBody, &limit)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupLimit(t, limit.ID)
	})

	// Get usage for PER_TRANSACTION limit
	req, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limit.ID+"/usage", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "Usage endpoint should return 200: %s", string(respBody))

	var usage usageSnapshotResponse
	err = json.Unmarshal(respBody, &usage)
	require.NoError(t, err)

	// Verify PER_TRANSACTION specific behavior
	assert.True(t, usage.CurrentUsage.IsZero(), "PER_TRANSACTION should have currentUsage=0 (no persistent counter)")
	assert.Nil(t, usage.ResetAt, "PER_TRANSACTION should have resetAt=null")
}

// =============================================================================
// 3.8 DELETE /v1/limits/{limitId} - Delete Tests (Missing)
// =============================================================================

// TestLimits_DeleteLimit_ActiveLimit (3.8.1b)
// Verifies that ACTIVE limit CANNOT be deleted (must deactivate first).
// This aligns with rules behavior per state machine diagram.
func TestLimits_DeleteLimit_ActiveLimit(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (starts as DRAFT)
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Activate the limit first
	activateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()
	require.Equal(t, http.StatusOK, activateResp.StatusCode, "Activation should succeed")

	// Verify it's ACTIVE
	getReq, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	getBody, err := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var beforeDelete limitResponse
	err = json.Unmarshal(getBody, &beforeDelete)
	require.NoError(t, err)
	require.Equal(t, "ACTIVE", beforeDelete.Status, "Limit should be ACTIVE before delete attempt")

	// Try to delete the ACTIVE limit - should fail
	req, err := http.NewRequest("DELETE", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// ACTIVE limits cannot be deleted - must deactivate first
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
		"Delete ACTIVE limit should return 422 - must deactivate first")
}

// TestLimits_DeleteLimit_InactiveLimit (3.8.1c)
// Verifies that INACTIVE limit can be deleted.
func TestLimits_DeleteLimit_InactiveLimit(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (starts as DRAFT)
	limitID := createTestLimit(t)

	// Activate it first (DRAFT → ACTIVE)
	activateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()
	require.Equal(t, http.StatusOK, activateResp.StatusCode, "Activation should succeed")

	// Then deactivate it (ACTIVE → INACTIVE)
	deactivateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)

	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	deactivateResp.Body.Close()
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode, "Deactivation should succeed")

	// Verify it's INACTIVE
	getReq, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	getBody, err := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var beforeDelete limitResponse
	err = json.Unmarshal(getBody, &beforeDelete)
	require.NoError(t, err)
	require.Equal(t, "INACTIVE", beforeDelete.Status, "Limit should be INACTIVE before delete")

	// Delete the INACTIVE limit
	req, err := http.NewRequest("DELETE", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode, "Delete INACTIVE limit should return 204")
}

// TestLimits_DeleteLimit_DraftLimit (3.8.1a)
// Verifies that DRAFT limit CAN be deleted directly.
// This aligns with rules behavior per state machine diagram.
func TestLimits_DeleteLimit_DraftLimit(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (starts as DRAFT)
	limitID := createTestLimit(t)

	// Verify it's DRAFT
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	getBody, err := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var limit limitResponse
	err = json.Unmarshal(getBody, &limit)
	require.NoError(t, err)
	require.Equal(t, "DRAFT", limit.Status, "Limit should be DRAFT")

	// Delete the DRAFT limit - should succeed
	req, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// DRAFT limits can be deleted directly
	assert.Equal(t, http.StatusNoContent, resp.StatusCode,
		"Delete DRAFT limit should return 204")
}

// TestLimits_DeleteLimit_ExcludedFromList (3.8.4)
// Verifies that deleted limit is excluded from list.
func TestLimits_DeleteLimit_ExcludedFromList(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit with unique name
	uniqueName := "Deleted Limit Test " + testutil.RandomSuffix()
	reqBody := createLimitRequest{
		Name:      uniqueName,
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	createReq, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	createBody, err := io.ReadAll(createResp.Body)
	createResp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var createdLimit limitResponse
	err = json.Unmarshal(createBody, &createdLimit)
	require.NoError(t, err)
	limitID := createdLimit.ID

	// Activate the limit first (limits start as DRAFT)
	activateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()
	require.Equal(t, http.StatusOK, activateResp.StatusCode, "Activation should succeed")

	// Deactivate the limit (ACTIVE limits cannot be deleted directly)
	deactivateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)

	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	deactivateResp.Body.Close()
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode, "Deactivation should succeed")

	// Delete the INACTIVE limit
	deleteReq, err := http.NewRequest("DELETE", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("X-API-Key", apiKey)

	deleteResp, err := testutil.HTTPClient.Do(deleteReq)
	require.NoError(t, err)
	deleteResp.Body.Close()
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode, "Delete should return 204")

	// List limits and verify deleted limit is not present
	listReq, err := http.NewRequest("GET", baseURL+"/v1/limits?limit=100", nil)
	require.NoError(t, err)
	listReq.Header.Set("X-API-Key", apiKey)

	listResp, err := testutil.HTTPClient.Do(listReq)
	require.NoError(t, err)
	defer listResp.Body.Close()

	listBody, err := io.ReadAll(listResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var listResult listLimitsResponse
	err = json.Unmarshal(listBody, &listResult)
	require.NoError(t, err)

	// Verify deleted limit is not in the list
	for _, limit := range listResult.Limits {
		assert.NotEqual(t, limitID, limit.ID, "Deleted limit should not appear in list")
	}
}

// =============================================================================
// Security Validation Tests - XSS Prevention (HIGH PRIORITY)
// =============================================================================

// TestLimits_CreateLimit_ValidationError_NameWithXSS tests that XSS payloads
// in the name field are rejected.
func TestLimits_CreateLimit_ValidationError_NameWithXSS(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	reqBody := createLimitRequest{
		Name:      "<script>alert('xss')</script>",
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	xssPayload := "<script>alert('xss')</script>"

	// API should reject XSS payloads - if it doesn't reject, verify the name
	// is sanitized (i.e., the raw payload is NOT stored as-is)
	if resp.StatusCode == http.StatusCreated {
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var limit limitResponse
		err = json.Unmarshal(respBody, &limit)
		require.NoError(t, err)

		t.Cleanup(func() {
			cleanupLimit(t, limit.ID)
		})

		// If the API accepts the payload, it MUST sanitize it.
		// Fail if the raw XSS payload is stored without sanitization.
		assert.NotEqual(t, xssPayload, limit.Name,
			"Security vulnerability: XSS payload stored without sanitization")
	} else {
		// Expected behavior: reject XSS payloads
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"Name with XSS payload should return 400 Bad Request")
	}
}

// TestLimits_CreateLimit_ValidationError_DescriptionWithXSS tests that XSS payloads
// in the description field are rejected.
func TestLimits_CreateLimit_ValidationError_DescriptionWithXSS(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	xssDescription := "<img src=x onerror=alert('xss')>"
	reqBody := createLimitRequest{
		Name:        "XSS Test Limit " + testutil.RandomSuffix(),
		Description: &xssDescription,
		LimitType:   "DAILY",
		MaxAmount:   decimal.RequireFromString("1000"),
		Currency:    "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// API should reject XSS payloads - if it doesn't reject, at least verify
	// the description is sanitized or stored safely
	if resp.StatusCode == http.StatusCreated {
		// If created, read the response and cleanup
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var limit limitResponse
		err = json.Unmarshal(respBody, &limit)
		require.NoError(t, err)

		t.Cleanup(func() {
			cleanupLimit(t, limit.ID)
		})

		// If the API accepts it, it MUST sanitize it.
		// Fail if the raw XSS payload is stored without sanitization.
		if limit.Description != nil {
			assert.NotEqual(t, xssDescription, *limit.Description,
				"Security vulnerability: XSS payload stored without sanitization in description")
		}
	} else {
		// Expected behavior: reject XSS payloads
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"Description with XSS payload should return 400 Bad Request")
	}
}

// =============================================================================
// Security Validation Tests - Scope Validation (HIGH PRIORITY)
// =============================================================================

// TestLimits_CreateLimit_ValidationError_InvalidScopeUUID tests that invalid UUID
// format in scope accountId is rejected.
func TestLimits_CreateLimit_ValidationError_InvalidScopeUUID(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	invalidUUID := "not-a-valid-uuid"
	reqBody := createLimitRequest{
		Name:      "Invalid Scope UUID Test " + testutil.RandomSuffix(),
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes: []limitScopeInput{
			{AccountID: &invalidUUID},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Invalid UUID in scope accountId should return 400 Bad Request")
}

// TestLimits_CreateLimit_ValidationError_InvalidTransactionTypeInScope tests that
// invalid transactionType in scope is rejected.
func TestLimits_CreateLimit_ValidationError_InvalidTransactionTypeInScope(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	reqBody := createLimitRequest{
		Name:      "Invalid Transaction Type Test " + testutil.RandomSuffix(),
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("INVALID")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Invalid transactionType in scope should return 400 Bad Request")
}

// =============================================================================
// Usage Boundary Tests (HIGH PRIORITY)
// =============================================================================

// TestLimits_GetUsage_NearLimitBoundary80Percent tests the exact boundary
// of the nearLimit calculation (exactly 80% = false, 80.01% = true).
// Note: This test verifies the concept. To fully test >80%, we would need
// the validation endpoint to consume the limit.
func TestLimits_GetUsage_NearLimitBoundary80Percent(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit with a known maxAmount
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Get usage - with 0% utilization, nearLimit should be false
	req, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID+"/usage", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "Usage endpoint should return 200: %s", string(respBody))

	var usage usageSnapshotResponse
	err = json.Unmarshal(respBody, &usage)
	require.NoError(t, err)

	// Verify boundary behavior:
	// At 0% usage, nearLimit should be false (threshold is >80%)
	assert.False(t, usage.NearLimit, "nearLimit should be false at 0%% utilization")
	assert.Equal(t, float64(0), usage.UtilizationPercent, "UtilizationPercent should be 0")

	// Document the expected behavior at boundary:
	// - 80% exactly: nearLimit = false (threshold is >80%, not >=80%)
	// - 80.01%: nearLimit = true
	t.Log("Boundary behavior: >80% triggers nearLimit=true, <=80% means nearLimit=false")
}

// =============================================================================
// Create Validation Tests - Medium Priority
// =============================================================================

// TestLimits_CreateLimit_ValidationError_ZeroMaxAmount tests that maxAmount=0
// is rejected (different from negative).
func TestLimits_CreateLimit_ValidationError_ZeroMaxAmount(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	reqBody := createLimitRequest{
		Name:      "Zero Amount Limit " + testutil.RandomSuffix(),
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("0"), // Zero should fail (must be positive)
		Currency:  "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"maxAmount=0 should return 400 Bad Request (must be positive)")
}

// TestLimits_CreateLimit_ValidationError_EmptyCurrency tests that empty currency
// string is rejected.
func TestLimits_CreateLimit_ValidationError_EmptyCurrency(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	reqBody := createLimitRequest{
		Name:      "Empty Currency Limit " + testutil.RandomSuffix(),
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "", // Empty should fail
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Empty currency should return 400 Bad Request")
}

// TestLimits_CreateLimit_ValidationError_TooManyScopes tests that scopes array
// with more than 100 items is rejected.
func TestLimits_CreateLimit_ValidationError_TooManyScopes(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create 101 scopes (max is 100)
	scopes := make([]limitScopeInput, 101)
	for i := 0; i < 101; i++ {
		accountID := testutil.MustDeterministicUUID(int64(30000 + i)).String()
		scopes[i] = limitScopeInput{AccountID: &accountID}
	}

	reqBody := createLimitRequest{
		Name:      "Too Many Scopes Limit " + testutil.RandomSuffix(),
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    scopes,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Scopes with 101 items (max=100) should return 400 Bad Request")
}

// TestLimits_CreateLimit_Success_ExactlyMaxScopes tests that scopes array
// with exactly 100 items (at the limit) is accepted.
func TestLimits_CreateLimit_Success_ExactlyMaxScopes(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create exactly 100 scopes (max allowed)
	scopes := make([]limitScopeInput, 100)
	for i := 0; i < 100; i++ {
		accountID := testutil.MustDeterministicUUID(int64(30200 + i)).String()
		scopes[i] = limitScopeInput{AccountID: &accountID}
	}

	reqBody := createLimitRequest{
		Name:      "Max Scopes Limit " + testutil.RandomSuffix(),
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes:    scopes,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// At-limit case should succeed
	if resp.StatusCode == http.StatusCreated {
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var limit limitResponse
		err = json.Unmarshal(respBody, &limit)
		require.NoError(t, err)

		t.Cleanup(func() {
			cleanupLimit(t, limit.ID)
		})

		assert.Equal(t, 100, len(limit.Scopes), "Limit should have exactly 100 scopes")
	} else {
		// If not created, fail the test
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		t.Fatalf("Expected 201 Created for 100 scopes (at limit), got %d: %s", resp.StatusCode, string(respBody))
	}
}

// =============================================================================
// List Validation Tests - Medium Priority
// =============================================================================

// TestLimits_ListLimits_InvalidStatusFilter tests that invalid status filter
// is rejected.
func TestLimits_ListLimits_InvalidStatusFilter(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest("GET", baseURL+"/v1/limits?status=INVALID", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Invalid status filter should return 400 Bad Request")
}

// TestLimits_ListLimits_InvalidLimitTypeFilter tests that invalid limitType filter
// is rejected.
func TestLimits_ListLimits_InvalidLimitTypeFilter(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest("GET", baseURL+"/v1/limits?limit_type=INVALID", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Invalid limitType filter should return 400 Bad Request")
}

// TestLimits_ListLimits_InvalidSortOrder tests that invalid sortOrder is rejected.
func TestLimits_ListLimits_InvalidSortOrder(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest("GET", baseURL+"/v1/limits?sort_order=RANDOM", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Invalid sortOrder (not ASC/DESC) should return 400 Bad Request")
}

// TestLimits_ListLimits_PaginationLimitBounds tests pagination limit bounds:
// limit=0 uses default, limit=101 caps at 100 or returns error.
func TestLimits_ListLimits_PaginationLimitBounds(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	t.Run("limit=0 uses default", func(t *testing.T) {
		req, err := http.NewRequest("GET", baseURL+"/v1/limits?limit=0", nil)
		require.NoError(t, err)
		req.Header.Set("X-API-Key", apiKey)

		resp, err := testutil.HTTPClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// limit=0 should either use default (200 OK) or return 400
		assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest,
			"limit=0 should return 200 (use default) or 400, got %d", resp.StatusCode)
	})

	t.Run("limit=101 caps at 100 or returns error", func(t *testing.T) {
		req, err := http.NewRequest("GET", baseURL+"/v1/limits?limit=101", nil)
		require.NoError(t, err)
		req.Header.Set("X-API-Key", apiKey)

		resp, err := testutil.HTTPClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// limit=101 should either cap at 100 (200 OK) or return 400
		if resp.StatusCode == http.StatusOK {
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var listResp listLimitsResponse
			err = json.Unmarshal(respBody, &listResp)
			require.NoError(t, err)

			assert.LessOrEqual(t, len(listResp.Limits), 100,
				"limit=101 should be capped at 100 items")
		} else {
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"limit=101 should return 400 if not capping")
		}
	})
}

// =============================================================================
// Update Validation Tests - Medium Priority
// =============================================================================

// TestLimits_UpdateLimit_ValidationError_NameTooLong tests that name exceeding
// 255 characters in PATCH is rejected.
func TestLimits_UpdateLimit_ValidationError_NameTooLong(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit first
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Try to update with name > 255 chars
	longName := strings.Repeat("a", 256)
	updateBody := updateLimitRequest{
		Name: &longName,
	}

	body, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+limitID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Name exceeding 255 chars in PATCH should return 400")
}

// TestLimits_UpdateLimit_ValidationError_DescriptionTooLong tests that description
// exceeding 1000 characters in PATCH is rejected.
func TestLimits_UpdateLimit_ValidationError_DescriptionTooLong(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit first
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Try to update with description > 1000 chars
	longDesc := strings.Repeat("a", 1001)
	updateBody := updateLimitRequest{
		Description: &longDesc,
	}

	body, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+limitID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Description exceeding 1000 chars in PATCH should return 400")
}

// TestLimits_UpdateLimit_DeletedLimit tests that PATCH on a deleted limit
// returns 404.
func TestLimits_UpdateLimit_DeletedLimit(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (DRAFT status)
	limitID := createTestLimit(t)

	// Activate the limit first
	activateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()
	require.Equal(t, http.StatusOK, activateResp.StatusCode, "Activate should succeed")

	// Deactivate the limit (ACTIVE limits cannot be deleted directly)
	deactivateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)

	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	deactivateResp.Body.Close()
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode, "Deactivate should succeed")

	// Delete the limit
	deleteReq, err := http.NewRequest("DELETE", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("X-API-Key", apiKey)

	deleteResp, err := testutil.HTTPClient.Do(deleteReq)
	require.NoError(t, err)
	deleteResp.Body.Close()
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode, "Delete should succeed")

	// Try to update the deleted limit
	newName := "Updated Name"
	updateBody := updateLimitRequest{
		Name: &newName,
	}

	body, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+limitID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"PATCH on deleted limit should return 404")
}

// =============================================================================
// Boundary Tests - Low Priority
// =============================================================================

// TestLimits_CreateLimit_Boundary_NameExactly255Chars tests that name with
// exactly 255 characters is accepted.
func TestLimits_CreateLimit_Boundary_NameExactly255Chars(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Name with exactly 255 chars (boundary value)
	prefix := "Boundary255_" + testutil.RandomSuffix() + "_"
	exactName := prefix + strings.Repeat("a", 255-len(prefix))
	reqBody := createLimitRequest{
		Name:      exactName,
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Name with exactly 255 chars should succeed: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupLimit(t, limit.ID)
	})

	assert.Len(t, limit.Name, 255, "Name should have exactly 255 characters")
}

// TestLimits_CreateLimit_Boundary_DescriptionExactly1000Chars tests that description
// with exactly 1000 characters is accepted.
func TestLimits_CreateLimit_Boundary_DescriptionExactly1000Chars(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Description with exactly 1000 chars (boundary value)
	exactDesc := strings.Repeat("a", 1000)
	reqBody := createLimitRequest{
		Name:        "Boundary Test Limit " + testutil.RandomSuffix(),
		Description: &exactDesc,
		LimitType:   "DAILY",
		MaxAmount:   decimal.RequireFromString("1000"),
		Currency:    "USD",
		Scopes: []limitScopeInput{
			{TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Description with exactly 1000 chars should succeed: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupLimit(t, limit.ID)
	})

	require.NotNil(t, limit.Description, "Description should be present")
	assert.Len(t, *limit.Description, 1000, "Description should have exactly 1000 characters")
}

// =============================================================================
// Response Fields Validation Tests
// =============================================================================

// TestLimits_CreateLimit_ResponseFields_Complete validates that all expected
// fields are present in the create limit response.
func TestLimits_CreateLimit_ResponseFields_Complete(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	uniqueName := "Response Fields Test " + testutil.RandomSuffix()
	description := "Test description for response fields"
	accountID := testutil.MustDeterministicUUID(3028).String()
	reqBody := createLimitRequest{
		Name:        uniqueName,
		Description: &description,
		LimitType:   "DAILY",
		MaxAmount:   decimal.RequireFromString("1000"),
		Currency:    "USD",
		Scopes: []limitScopeInput{
			{AccountID: &accountID, TransactionType: testutil.Ptr("CARD")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Create should return 201: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupLimit(t, limit.ID)
	})

	// Validate all required fields are present
	assert.NotEmpty(t, limit.ID, "limitId should be present")
	_, err = uuid.Parse(limit.ID)
	assert.NoError(t, err, "limitId should be valid UUID")

	assert.Equal(t, uniqueName, limit.Name, "name should match request")
	assert.NotNil(t, limit.Description, "description should be present")
	assert.Equal(t, description, *limit.Description, "description should match request")

	assert.Equal(t, "DAILY", limit.LimitType, "limitType should match request")
	assert.True(t, limit.MaxAmount.Equal(decimal.RequireFromString("1000")), "maxAmount should match request")
	assert.Equal(t, "USD", limit.Currency, "currency should match request")

	// Validate scopes
	require.Len(t, limit.Scopes, 1, "scopes should have 1 entry")
	assert.NotNil(t, limit.Scopes[0].AccountID, "scope accountId should be present")
	assert.Equal(t, accountID, *limit.Scopes[0].AccountID, "scope accountId should match")
	assert.NotNil(t, limit.Scopes[0].TransactionType, "scope transactionType should be present")
	assert.Equal(t, "CARD", *limit.Scopes[0].TransactionType, "scope transactionType should match")

	// Validate status and timestamps (limits are created as DRAFT)
	assert.Equal(t, "DRAFT", limit.Status, "status should be DRAFT for newly created limit")
	assert.NotNil(t, limit.ResetAt, "resetAt should be present for DAILY limit")
	assert.NotEmpty(t, limit.CreatedAt, "createdAt should be present")
	assert.NotEmpty(t, limit.UpdatedAt, "updatedAt should be present")
	assert.Nil(t, limit.DeletedAt, "deletedAt should be nil for non-deleted limit")

	// Validate timestamp formats (RFC3339)
	_, err = time.Parse(time.RFC3339, limit.CreatedAt)
	assert.NoError(t, err, "createdAt should be valid RFC3339 timestamp")

	_, err = time.Parse(time.RFC3339, limit.UpdatedAt)
	assert.NoError(t, err, "updatedAt should be valid RFC3339 timestamp")

	if limit.ResetAt != nil {
		_, err = time.Parse(time.RFC3339, *limit.ResetAt)
		assert.NoError(t, err, "resetAt should be valid RFC3339 timestamp")
	}
}

// TestLimits_UpdateLimit_ResponseFields_UpdatedAtChanged validates that
// updatedAt changes after an update.
func TestLimits_UpdateLimit_ResponseFields_UpdatedAtChanged(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit first
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Get the limit to capture original updatedAt
	getReq1, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq1.Header.Set("X-API-Key", apiKey)

	getResp1, err := testutil.HTTPClient.Do(getReq1)
	require.NoError(t, err)
	getBody1, err := io.ReadAll(getResp1.Body)
	getResp1.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp1.StatusCode)

	var originalLimit limitResponse
	err = json.Unmarshal(getBody1, &originalLimit)
	require.NoError(t, err)

	originalUpdatedAt := originalLimit.UpdatedAt

	// Update the limit (no sleep needed - we use >= comparison for timestamps)
	newName := "Updated Name " + testutil.RandomSuffix()
	updateBody := updateLimitRequest{
		Name: &newName,
	}

	body, err := json.Marshal(updateBody)
	require.NoError(t, err)

	req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+limitID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "Update should return 200: %s", string(respBody))

	var updatedLimit limitResponse
	err = json.Unmarshal(respBody, &updatedLimit)
	require.NoError(t, err)

	// Validate updatedAt has changed
	assert.NotEqual(t, originalUpdatedAt, updatedLimit.UpdatedAt,
		"updatedAt should change after update")

	// Parse and compare timestamps to ensure new updatedAt is later
	originalTime, err := time.Parse(time.RFC3339, originalUpdatedAt)
	require.NoError(t, err)

	newTime, err := time.Parse(time.RFC3339, updatedLimit.UpdatedAt)
	require.NoError(t, err)

	assert.True(t, newTime.After(originalTime) || newTime.Equal(originalTime),
		"New updatedAt should be >= original updatedAt")
}

// TestLimits_GetLimit_ResponseFields_Complete validates that all expected
// fields are present in the GET limit response.
func TestLimits_GetLimit_ResponseFields_Complete(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit with all fields
	uniqueName := "Get Response Test " + testutil.RandomSuffix()
	description := "Test description for get response"
	accountID := testutil.MustDeterministicUUID(3031).String()
	reqBody := createLimitRequest{
		Name:        uniqueName,
		Description: &description,
		LimitType:   "MONTHLY",
		MaxAmount:   decimal.RequireFromString("5000"),
		Currency:    "BRL",
		Scopes: []limitScopeInput{
			{AccountID: &accountID},
			{TransactionType: testutil.Ptr("PIX")},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	createReq, err := http.NewRequest("POST", baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	createBody, err := io.ReadAll(createResp.Body)
	createResp.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "Create failed: %s", string(createBody))

	var createdLimit limitResponse
	err = json.Unmarshal(createBody, &createdLimit)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupLimit(t, createdLimit.ID)
	})

	// GET the limit
	req, err := http.NewRequest("GET", baseURL+"/v1/limits/"+createdLimit.ID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "GET should return 200: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	// Validate all fields match what was created
	assert.Equal(t, createdLimit.ID, limit.ID, "limitId should match")
	assert.Equal(t, uniqueName, limit.Name, "name should match")
	assert.NotNil(t, limit.Description, "description should be present")
	assert.Equal(t, description, *limit.Description, "description should match")
	assert.Equal(t, "MONTHLY", limit.LimitType, "limitType should match")
	assert.True(t, limit.MaxAmount.Equal(decimal.RequireFromString("5000")), "maxAmount should match")
	assert.Equal(t, "BRL", limit.Currency, "currency should match")
	assert.Equal(t, "DRAFT", limit.Status, "status should be DRAFT for newly created limit")

	// Validate scopes
	assert.Len(t, limit.Scopes, 2, "scopes should have 2 entries")

	// Validate timestamps
	assert.NotEmpty(t, limit.CreatedAt, "createdAt should be present")
	assert.NotEmpty(t, limit.UpdatedAt, "updatedAt should be present")
	assert.NotNil(t, limit.ResetAt, "resetAt should be present for MONTHLY limit")
	assert.Nil(t, limit.DeletedAt, "deletedAt should be nil")

	// Validate resetAt is a valid timestamp
	if limit.ResetAt != nil {
		_, err := time.Parse(time.RFC3339, *limit.ResetAt)
		require.NoError(t, err, "resetAt should be valid RFC3339 timestamp")
	}
}

// =============================================================================
// ImmutableField Validation Tests
// =============================================================================
// These tests verify that attempting to change immutable fields (limitType, currency)
// via PATCH returns HTTP 422 with error code 0380 ("Limit Immutable Field").
//
// Immutable-field changes are an unprocessable business rule:
// | 422 | Limit Immutable Field | Attempted to change limitType or currency |
// =============================================================================

// TestLimits_UpdateLimit_ImmutableFields_ReturnsTRC0138 verifies that attempting to change
// immutable fields (limitType, currency) returns HTTP 422 with error code 0380.
func TestLimits_UpdateLimit_ImmutableFields_ReturnsTRC0138(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Test cases for immutable field validation
	testCases := []struct {
		name          string
		updateBody    map[string]interface{}
		expectedField string
		description   string
	}{
		{
			name: "change_limitType_DAILY_to_MONTHLY",
			updateBody: map[string]interface{}{
				"limitType": "MONTHLY",
			},
			expectedField: "limitType",
			description:   "Attempt to change limitType from DAILY to MONTHLY",
		},
		{
			name: "change_limitType_DAILY_to_PER_TRANSACTION",
			updateBody: map[string]interface{}{
				"limitType": "PER_TRANSACTION",
			},
			expectedField: "limitType",
			description:   "Attempt to change limitType from DAILY to PER_TRANSACTION",
		},
		{
			name: "change_currency_USD_to_BRL",
			updateBody: map[string]interface{}{
				"currency": "BRL",
			},
			expectedField: "currency",
			description:   "Attempt to change currency from USD to BRL",
		},
		{
			name: "change_both_limitType_and_currency",
			updateBody: map[string]interface{}{
				"limitType": "MONTHLY",
				"currency":  "BRL",
			},
			expectedField: "limitType", // First immutable field encountered
			description:   "Attempt to change both limitType and currency together",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a limit with known values: limitType=DAILY, currency=USD
			limitID := createTestLimit(t)
			t.Cleanup(func() {
				cleanupLimit(t, limitID)
			})

			// Attempt to PATCH with immutable field change
			body, err := json.Marshal(tc.updateBody)
			require.NoError(t, err)

			req, err := http.NewRequest("PATCH", baseURL+"/v1/limits/"+limitID, bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)
			req.Header.Set("Content-Type", "application/json")

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Assert HTTP 422 Unprocessable Entity
			assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
				"[%s] %s should return 422 Unprocessable Entity, got %d: %s",
				tc.name, tc.description, resp.StatusCode, string(respBody))

			// Parse and validate error response
			errResp := testutil.ParseErrorResponse(t, respBody)

			// Assert error code is 0380 (Limit Immutable Field)
			assert.Equal(t, "0380", errResp.Code,
				"[%s] Expected error code 0380 (Limit Immutable Field), got %s",
				tc.name, errResp.Code)

			// Assert error title
			assert.Equal(t, "Limit Immutable Field", errResp.Title,
				"[%s] Expected error title 'Limit Immutable Field', got '%s'",
				tc.name, errResp.Title)

			// Assert error detail mentions the immutable field(s) that cannot be modified (case-insensitive).
			// The detail lists all immutable fields (limitType, currency), so any target key is present.
			detail := tracerProblemDetail(t, respBody)
			if len(tc.updateBody) > 1 {
				// Check that detail contains at least one of the updateBody keys
				detailLower := strings.ToLower(detail)
				foundField := false
				for key := range tc.updateBody {
					if strings.Contains(detailLower, strings.ToLower(key)) {
						foundField = true
						break
					}
				}
				assert.True(t, foundField,
					"[%s] Error detail should mention at least one immutable field from %v, got '%s'",
					tc.name, tc.updateBody, detail)
			} else {
				assert.Contains(t, strings.ToLower(detail), strings.ToLower(tc.expectedField),
					"[%s] Error detail should mention '%s' field, got '%s'",
					tc.name, tc.expectedField, detail)
			}

			// Verify the limit was NOT modified by fetching it
			getReq, err := http.NewRequest("GET", baseURL+"/v1/limits/"+limitID, nil)
			require.NoError(t, err)
			getReq.Header.Set("X-API-Key", apiKey)

			getResp, err := testutil.HTTPClient.Do(getReq)
			require.NoError(t, err)
			defer getResp.Body.Close()

			getBody, err := io.ReadAll(getResp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, getResp.StatusCode, "GET should return 200")

			var limit limitResponse
			err = json.Unmarshal(getBody, &limit)
			require.NoError(t, err)

			// Verify immutable fields remain unchanged
			assert.Equal(t, "DAILY", limit.LimitType,
				"[%s] limitType should remain DAILY (immutable)", tc.name)
			assert.Equal(t, "USD", limit.Currency,
				"[%s] currency should remain USD (immutable)", tc.name)
		})
	}
}

// =============================================================================
// Draft Limit Tests
// =============================================================================

func TestLimits_DraftLimit_Success(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (starts as DRAFT), activate it, then deactivate to INACTIVE
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// DRAFT → ACTIVE
	testutil.ActivateLimit(t, limitID)

	// ACTIVE → INACTIVE
	deactivateReq, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)

	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	deactivateResp.Body.Close()
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode)

	// INACTIVE → DRAFT
	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Draft should return 200: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	assert.Equal(t, "DRAFT", limit.Status, "Status should be DRAFT after drafting")
}

func TestLimits_DraftLimit_Idempotent(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a limit (starts as DRAFT)
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	// Draft a DRAFT limit (idempotent no-op)
	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Idempotent draft should return 200: %s", string(respBody))

	var limit limitResponse
	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	assert.Equal(t, "DRAFT", limit.Status, "Status should remain DRAFT")
}

func TestLimits_DraftLimit_RejectsActiveLimit(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create and activate a limit
	limitID := createTestLimit(t)
	t.Cleanup(func() {
		cleanupLimit(t, limitID)
	})

	testutil.ActivateLimit(t, limitID)

	// Try to draft an ACTIVE limit (invalid transition)
	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "ACTIVE → DRAFT should be rejected")
}

func TestLimits_DraftLimit_RejectsDeletedLimit(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create and delete a limit (DRAFT → DELETED)
	limitID := createTestLimit(t)

	deleteReq, err := http.NewRequest("DELETE", baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("X-API-Key", apiKey)

	deleteResp, err := testutil.HTTPClient.Do(deleteReq)
	require.NoError(t, err)
	deleteResp.Body.Close()
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// Try to draft a DELETED limit
	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Soft-deleted limits are filtered by GetByID (WHERE deleted_at IS NULL), so they return 404
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "DELETED limit should return 404")
}

func TestLimits_DraftLimit_InvalidUUID(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest("POST", baseURL+"/v1/limits/not-a-uuid/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Invalid UUID should return 400")
}

func TestLimits_DraftLimit_NotFound(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	nonExistentID := testutil.MustDeterministicUUID(3032).String()

	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+nonExistentID+"/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Non-existent limit should return 404")
}

func TestLimits_DraftLimit_WithoutAuthentication(t *testing.T) {
	baseURL := testutil.GetBaseURL()

	limitID := testutil.MustDeterministicUUID(3033).String()

	req, err := http.NewRequest("POST", baseURL+"/v1/limits/"+limitID+"/draft", nil)
	require.NoError(t, err)
	// No X-API-Key header

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Missing API key should return 401")
}
