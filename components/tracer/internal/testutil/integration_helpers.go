// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

// RandomSuffix returns an 8-char random suffix for unique test names.
func RandomSuffix() string {
	return uuid.New().String()[:8]
}

// HTTPClient is a shared HTTP client with timeout to prevent CI hangs.
// Use this for all HTTP requests in integration tests.
var HTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

// GetAPIKey returns the API key from environment or default.
// Uses the same default as the service configuration.
func GetAPIKey() string {
	if key := os.Getenv("API_KEY"); key != "" {
		return key
	}

	return "dev_api_key_change_in_production"
}

// GetBaseURL returns the base URL for the service from environment or default.
// Handles full URLs (http://api.example.com:4020), host:port (localhost:4020),
// and bare port suffixes (":4020", matching the SERVER_ADDRESS convention from
// lib-commons — in that form the host is omitted and localhost is implied).
// Default port 4020 matches SERVER_PORT in .env and the docker-compose mapping.
func GetBaseURL() string {
	if addr := os.Getenv("SERVER_ADDRESS"); addr != "" {
		if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
			return addr
		}

		if strings.HasPrefix(addr, ":") {
			return "http://localhost" + addr
		}

		return "http://" + addr
	}

	if port := os.Getenv("SERVER_PORT"); port != "" {
		if strings.HasPrefix(port, ":") {
			return "http://localhost" + port
		}

		return "http://localhost:" + port
	}

	return constant.DefaultTestServerURL
}

// RuleRequest represents the request body for creating a rule.
type RuleRequest struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Expression  string       `json:"expression"`
	Action      string       `json:"action"`
	Scopes      []ScopeInput `json:"scopes,omitempty"`
}

// ScopeInput represents a scope in a rule request.
type ScopeInput struct {
	AccountID       *string `json:"accountId,omitempty"`
	SegmentID       *string `json:"segmentId,omitempty"`
	PortfolioID     *string `json:"portfolioId,omitempty"`
	MerchantID      *string `json:"merchantId,omitempty"`
	TransactionType *string `json:"transactionType,omitempty"`
	SubType         *string `json:"subType,omitempty"`
}

// RuleResponse represents the response body for a rule.
type RuleResponse struct {
	ID          string          `json:"ruleId"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Expression  string          `json:"expression"`
	Action      string          `json:"action"`
	Scopes      []ScopeResponse `json:"scopes"`
	Status      string          `json:"status"`
	CreatedAt   string          `json:"createdAt"`
	UpdatedAt   string          `json:"updatedAt"`
	DeletedAt   *string         `json:"deletedAt,omitempty"`
}

// ScopeResponse represents a scope in a rule response.
type ScopeResponse struct {
	AccountID       *string `json:"accountId,omitempty"`
	SegmentID       *string `json:"segmentId,omitempty"`
	PortfolioID     *string `json:"portfolioId,omitempty"`
	MerchantID      *string `json:"merchantId,omitempty"`
	TransactionType *string `json:"transactionType,omitempty"`
	SubType         *string `json:"subType,omitempty"`
}

// ListRulesResponse represents the response body for listing rules.
type ListRulesResponse struct {
	Rules      []RuleResponse `json:"rules"`
	NextCursor string         `json:"nextCursor"`
	HasMore    bool           `json:"hasMore"`
}

// CreateTestRule creates a test rule with the given name and default DENY action.
// Returns the rule ID.
func CreateTestRule(t *testing.T, name string) string {
	t.Helper()

	return CreateTestRuleWithAction(t, name, "DENY")
}

// CreateTestRuleWithAction creates a test rule with a specific action and returns its ID.
// Useful for test isolation when filtering by action.
func CreateTestRuleWithAction(t *testing.T, name string, action string) string {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	reqBody := RuleRequest{
		Name:       name,
		Expression: "amount > 1000",
		Action:     action,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }() // Intentionally ignored in test helper

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "failed to read response body")
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Failed to create rule: %s", string(respBody))

	var createdRule RuleResponse

	err = json.Unmarshal(respBody, &createdRule)
	require.NoError(t, err)

	return createdRule.ID
}

// CleanupRule deactivates and deletes a rule as part of test cleanup.
// Uses t.Helper() and logs errors but does not fail the test.
func CleanupRule(t *testing.T, ruleID string) {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	// First deactivate the rule (required before delete per state machine)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/deactivate", nil)
	if err != nil {
		t.Logf("Cleanup: failed to create deactivate request for rule %s: %v", ruleID, err)

		return
	}

	req.Header.Set("X-API-Key", apiKey)

	resp, err := HTTPClient.Do(req)
	if err != nil {
		t.Logf("Cleanup: failed to deactivate rule %s: %v", ruleID, err)

		return
	}

	_ = resp.Body.Close() // Intentionally ignored in test helper
	// Ignore status - rule might already be inactive or deleted

	// Now delete the rule
	req, err = http.NewRequest(http.MethodDelete, baseURL+"/v1/rules/"+ruleID, nil)
	if err != nil {
		t.Logf("Cleanup: failed to create delete request for rule %s: %v", ruleID, err)

		return
	}

	req.Header.Set("X-API-Key", apiKey)

	resp, err = HTTPClient.Do(req)
	if err != nil {
		t.Logf("Cleanup: failed to delete rule %s: %v", ruleID, err)

		return
	}

	_ = resp.Body.Close() // Intentionally ignored in test helper
	// Ignore status - rule might already be deleted
}

// DeleteRuleViaAPI deletes a rule using the DELETE /v1/rules/:id endpoint.
// State machine allows: DRAFT → DELETED and INACTIVE → DELETED.
// If the rule is ACTIVE, it will be deactivated first.
// Unlike CleanupRule, this function asserts on errors.
func DeleteRuleViaAPI(t *testing.T, ruleID string) {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	// Check current status
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := HTTPClient.Do(getReq)
	require.NoError(t, err)
	getBody, err := io.ReadAll(getResp.Body)
	_ = getResp.Body.Close() // Intentionally ignored in test helper

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode, "Failed to get rule status")

	var rule map[string]any

	err = json.Unmarshal(getBody, &rule)
	require.NoError(t, err)

	status, ok := rule["status"].(string)
	require.True(t, ok, "expected status to be a string in rule response")

	// If ACTIVE, need to deactivate first (ACTIVE → INACTIVE → DELETED)
	if status == "ACTIVE" {
		deactivateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/deactivate", nil)
		require.NoError(t, err)
		deactivateReq.Header.Set("X-API-Key", apiKey)

		deactivateResp, err := HTTPClient.Do(deactivateReq)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, deactivateResp.StatusCode, "Deactivate should succeed")
		_ = deactivateResp.Body.Close() // Intentionally ignored in test helper
	}

	// DRAFT and INACTIVE can be deleted directly
	req, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }() // Intentionally ignored in test helper

	require.Equal(t, http.StatusNoContent, resp.StatusCode, "Failed to delete rule via API")
}

// SkipIfRulesNotImplemented checks if the rules API is available.
// Skips the test if routes are not yet registered (returns 404 for base endpoint).
func SkipIfRulesNotImplemented(t *testing.T) {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules?limit=1", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }() // Intentionally ignored in test helper

	if resp.StatusCode == http.StatusNotFound {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			t.Fatalf("reading response body: %v", readErr)
		}

		if strings.Contains(string(body), "Cannot GET /v1/rules") {
			t.Fatal("Rules API not registered in routes.go - this is a critical bug! Register the API endpoint before running tests.")
		}
	}
}

// ValidationRequest represents the request body for transaction validation.
// Per API Design v1.3.1: Segment and Portfolio are separate context objects,
// NOT an explicit scopes array. The system derives matching criteria from contexts.
type ValidationRequest struct {
	RequestID            string            `json:"requestId,omitempty"`
	TransactionType      string            `json:"transactionType,omitempty"`
	SubType              string            `json:"subType,omitempty"`
	Amount               decimal.Decimal   `json:"amount"`
	Currency             string            `json:"currency,omitempty"`
	TransactionTimestamp string            `json:"transactionTimestamp,omitempty"`
	Account              *AccountContext   `json:"account,omitempty"`
	Segment              *SegmentContext   `json:"segment,omitempty"`
	Portfolio            *PortfolioContext `json:"portfolio,omitempty"`
	Merchant             *MerchantContext  `json:"merchant,omitempty"`
	Metadata             map[string]any    `json:"metadata,omitempty"`
}

// AccountContext represents account context in a validation request.
type AccountContext struct {
	ID     string `json:"accountId,omitempty"`
	Type   string `json:"type,omitempty"`
	Status string `json:"status,omitempty"`
}

// SegmentContext represents segment context in a validation request.
type SegmentContext struct {
	ID       string         `json:"segmentId,omitempty"`
	Name     string         `json:"name,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// PortfolioContext represents portfolio context in a validation request.
type PortfolioContext struct {
	ID       string         `json:"portfolioId,omitempty"`
	Name     string         `json:"name,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// MerchantContext represents merchant context in a validation request.
type MerchantContext struct {
	ID       string `json:"merchantId,omitempty"`
	Category string `json:"category,omitempty"`
	Country  string `json:"country,omitempty"`
	MCC      string `json:"mcc,omitempty"`
	Name     string `json:"name,omitempty"`
}

// LimitUsageDetail represents limit usage information in validation response.
type LimitUsageDetail struct {
	LimitID         string          `json:"limitId"`
	LimitAmount     decimal.Decimal `json:"limitAmount"`
	CurrentUsage    decimal.Decimal `json:"currentUsage"`
	Exceeded        bool            `json:"exceeded"`
	Period          string          `json:"period"`
	Scope           string          `json:"scope"`
	AttemptedAmount decimal.Decimal `json:"attemptedAmount"`
	Skipped         bool            `json:"skipped,omitempty"`
	SkipReason      string          `json:"skipReason,omitempty"`
}

// ValidationResponse represents the response from transaction validation.
type ValidationResponse struct {
	ValidationID      string             `json:"validationId"`
	RequestID         string             `json:"requestId"`
	Decision          string             `json:"decision"`
	Reason            string             `json:"reason"`
	MatchedRuleIDs    []string           `json:"matchedRuleIds"`
	EvaluatedRuleIDs  []string           `json:"evaluatedRuleIds"`
	LimitUsageDetails []LimitUsageDetail `json:"limitUsageDetails"`
	ProcessingTimeMs  float64            `json:"processingTimeMs"`
	EvaluatedAt       string             `json:"evaluatedAt,omitempty"`
}

// doRequest executes an HTTP request with common setup and returns response and body.
// This helper centralizes request construction, header application, and response reading.
func doRequest(t *testing.T, method, url string, body io.Reader, headers map[string]string) (*http.Response, []byte) {
	t.Helper()

	req, err := http.NewRequest(method, url, body)
	require.NoError(t, err)

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close() // Intentionally ignored in test helper

	require.NoError(t, err)

	return resp, respBody
}

// CreateValidation sends a validation request and returns the response and body.
func CreateValidation(t *testing.T, validationReq *ValidationRequest) (*http.Response, []byte) {
	t.Helper()

	body, err := json.Marshal(validationReq)
	require.NoError(t, err)

	return doRequest(t, http.MethodPost, GetBaseURL()+"/v1/validations", bytes.NewReader(body), map[string]string{
		"X-API-Key":    GetAPIKey(),
		"Content-Type": "application/json",
	})
}

// CreateValidationWithoutAuth sends a validation request without authentication.
func CreateValidationWithoutAuth(t *testing.T, validationReq *ValidationRequest) (*http.Response, []byte) {
	t.Helper()

	body, err := json.Marshal(validationReq)
	require.NoError(t, err)

	return doRequest(t, http.MethodPost, GetBaseURL()+"/v1/validations", bytes.NewReader(body), map[string]string{
		"Content-Type": "application/json",
	})
}

// CreateValidationWithAPIKey sends a validation request with a specific API key.
func CreateValidationWithAPIKey(t *testing.T, validationReq *ValidationRequest, apiKey string) (*http.Response, []byte) {
	t.Helper()

	body, err := json.Marshal(validationReq)
	require.NoError(t, err)

	return doRequest(t, http.MethodPost, GetBaseURL()+"/v1/validations", bytes.NewReader(body), map[string]string{
		"X-API-Key":    apiKey,
		"Content-Type": "application/json",
	})
}

// CreateValidationRaw sends a raw JSON validation request (for testing decimal amounts, etc.).
func CreateValidationRaw(t *testing.T, jsonPayload []byte) (*http.Response, []byte) {
	t.Helper()

	return doRequest(t, http.MethodPost, GetBaseURL()+"/v1/validations", bytes.NewReader(jsonPayload), map[string]string{
		"X-API-Key":    GetAPIKey(),
		"Content-Type": "application/json",
	})
}

// FaultInjectionHeader re-exports the constant from pkg/constant for convenience.
const FaultInjectionHeader = constant.FaultInjectionHeader

// Fault injection types re-exported from pkg/constant for convenience.
const (
	FaultTimeout     = constant.FaultTimeout     // Simulates 504 Gateway Timeout
	FaultUnavailable = constant.FaultUnavailable // Simulates 503 Service Unavailable
)

// CreateValidationWithFaultInjection sends a validation request with fault injection header.
// faultType should be one of FaultTimeout or FaultUnavailable.
func CreateValidationWithFaultInjection(t *testing.T, validationReq *ValidationRequest, faultType string) (*http.Response, []byte) {
	t.Helper()

	body, err := json.Marshal(validationReq)
	require.NoError(t, err)

	return doRequest(t, http.MethodPost, GetBaseURL()+"/v1/validations", bytes.NewReader(body), map[string]string{
		"X-API-Key":          GetAPIKey(),
		"Content-Type":       "application/json",
		FaultInjectionHeader: faultType,
	})
}

// ListValidationsWithFaultInjection sends a list validations request with fault injection header.
func ListValidationsWithFaultInjection(t *testing.T, queryParams string, faultType string) (*http.Response, []byte) {
	t.Helper()

	url := GetBaseURL() + "/v1/validations"
	if queryParams != "" {
		url += "?" + queryParams
	}

	return doRequest(t, http.MethodGet, url, nil, map[string]string{
		"X-API-Key":          GetAPIKey(),
		FaultInjectionHeader: faultType,
	})
}

// CreateTestRuleWithExpression creates a rule with a custom CEL expression and action.
// Returns the rule ID.
func CreateTestRuleWithExpression(t *testing.T, name, expression, action string) string {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	reqBody := RuleRequest{
		Name:       name,
		Expression: expression,
		Action:     action,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }() // Intentionally ignored in test helper

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "failed to read response body")
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Failed to create rule: %s", string(respBody))

	var createdRule RuleResponse

	err = json.Unmarshal(respBody, &createdRule)
	require.NoError(t, err)

	return createdRule.ID
}

// ActivateRule activates a rule by ID.
func ActivateRule(t *testing.T, ruleID string) {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }() // Intentionally ignored in test helper

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to activate rule: %s", string(respBody))
}

// DeactivateRule deactivates a rule by ID.
// Unlike CleanupRule, this function asserts on errors and is intended for test scenarios
// where deactivation is a critical step (e.g., testing rule state transitions).
// Note: If the rule is in DRAFT status, it will be activated first since DRAFT → INACTIVE
// is not a valid transition (only DRAFT → ACTIVE or DRAFT → DELETED are allowed).
func DeactivateRule(t *testing.T, ruleID string) {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	// First, check if rule is in DRAFT status - if so, activate it first
	// because DRAFT → INACTIVE is not allowed (only DRAFT → ACTIVE or DRAFT → DELETED)
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := HTTPClient.Do(getReq)
	require.NoError(t, err)
	getBody, err := io.ReadAll(getResp.Body)
	_ = getResp.Body.Close() // Intentionally ignored in test helper

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode, "Failed to get rule status")

	var rule map[string]any

	err = json.Unmarshal(getBody, &rule)
	require.NoError(t, err)

	status, ok := rule["status"].(string)
	require.True(t, ok, "expected status to be a string in rule response")

	if status == "DRAFT" {
		// Activate first (DRAFT → ACTIVE), then deactivate (ACTIVE → INACTIVE)
		ActivateRule(t, ruleID)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/deactivate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }() // Intentionally ignored in test helper

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to deactivate rule: %s", string(respBody))
}

// DraftRule transitions a rule back to DRAFT status by ID.
// Only INACTIVE rules can transition to DRAFT per state machine.
func DraftRule(t *testing.T, ruleID string) {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }() // Intentionally ignored in test helper

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to draft rule: %s", string(respBody))
}

// ValidationDetailResponse represents the response from GET /v1/validations/{id}.
// Fields match model.TransactionValidation for explicit traceability and queryability.
type ValidationDetailResponse struct {
	ID                   string             `json:"validationId"`
	RequestID            string             `json:"requestId"`
	TransactionType      string             `json:"transactionType"`
	SubType              *string            `json:"subType,omitempty"`
	Amount               decimal.Decimal    `json:"amount"`
	Currency             string             `json:"currency"`
	TransactionTimestamp string             `json:"transactionTimestamp"`
	Account              map[string]any     `json:"account"`
	Segment              map[string]any     `json:"segment,omitempty"`
	Portfolio            map[string]any     `json:"portfolio,omitempty"`
	Merchant             map[string]any     `json:"merchant,omitempty"`
	Metadata             map[string]any     `json:"metadata,omitempty"`
	Decision             string             `json:"decision"`
	Reason               string             `json:"reason"`
	MatchedRuleIDs       []string           `json:"matchedRuleIds"`
	EvaluatedRuleIDs     []string           `json:"evaluatedRuleIds"`
	LimitUsageDetails    []LimitUsageDetail `json:"limitUsageDetails"`
	ProcessingTimeMs     float64            `json:"processingTimeMs"`
	CreatedAt            string             `json:"createdAt"`
	RequestSnapshot      map[string]any     `json:"requestSnapshot,omitempty"`
	ResponseSnapshot     map[string]any     `json:"responseSnapshot,omitempty"`
}

// GetValidation retrieves a validation by ID and returns the response and body.
func GetValidation(t *testing.T, validationID string) (*http.Response, []byte) {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations/"+validationID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close() // Intentionally ignored in test helper

	require.NoError(t, err)

	return resp, respBody
}

// GetValidationWithoutAuth retrieves a validation by ID without authentication.
func GetValidationWithoutAuth(t *testing.T, validationID string) (*http.Response, []byte) {
	t.Helper()

	baseURL := GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/validations/"+validationID, nil)
	require.NoError(t, err)

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close() // Intentionally ignored in test helper

	require.NoError(t, err)

	return resp, respBody
}

// ValidationSummary represents a summary of a validation record in list responses.
type ValidationSummary struct {
	ID               string          `json:"validationId"`
	Decision         string          `json:"decision"`
	Reason           string          `json:"reason"`
	Amount           decimal.Decimal `json:"amount"`
	Currency         string          `json:"currency"`
	TransactionType  string          `json:"transactionType"`
	AccountID        string          `json:"accountId"`
	SegmentID        string          `json:"segmentId,omitempty"`
	PortfolioID      string          `json:"portfolioId,omitempty"`
	MatchedRuleIDs   []string        `json:"matchedRuleIds"`
	ExceededLimitIDs []string        `json:"exceededLimitIds"`
	ProcessingTimeMs float64         `json:"processingTimeMs"`
	CreatedAt        string          `json:"createdAt"`
}

// ListValidationsResponse represents the response from GET /v1/validations.
type ListValidationsResponse struct {
	TransactionValidations []ValidationSummary `json:"transactionValidations"`
	NextCursor             string              `json:"nextCursor,omitempty"`
	HasMore                bool                `json:"hasMore"`
}

// ListValidations retrieves validations with optional query parameters.
func ListValidations(t *testing.T, queryParams string) (*http.Response, []byte) {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	url := baseURL + "/v1/validations"
	if queryParams != "" {
		url += "?" + queryParams
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close() // Intentionally ignored in test helper

	require.NoError(t, err)

	return resp, respBody
}

// ListValidationsWithoutAuth retrieves validations without authentication.
func ListValidationsWithoutAuth(t *testing.T, queryParams string) (*http.Response, []byte) {
	t.Helper()

	baseURL := GetBaseURL()

	url := baseURL + "/v1/validations"
	if queryParams != "" {
		url += "?" + queryParams
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close() // Intentionally ignored in test helper

	require.NoError(t, err)

	return resp, respBody
}

// ErrorResponse represents the structured error response from the API.
// Used for parsing 4xx error responses in integration tests.
type ErrorResponse struct {
	Code    string `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// ParseErrorResponse parses a JSON error response body into ErrorResponse.
// Returns the parsed response or fails the test if parsing fails.
func ParseErrorResponse(t *testing.T, body []byte) ErrorResponse {
	t.Helper()

	var errResp ErrorResponse

	err := json.Unmarshal(body, &errResp)

	require.NoError(t, err, "Failed to parse error response: %s", string(body))

	return errResp
}

// limitScopeInputAccount represents a scope with accountId for limit creation.
type limitScopeInputAccount struct {
	AccountID *string `json:"accountId,omitempty"`
}

// limitScopeInputTransactionType represents a scope with transactionType for limit creation.
type limitScopeInputTransactionType struct {
	TransactionType *string `json:"transactionType,omitempty"`
}

// createLimitRequestAccount is the request body for creating a limit with account scope.
type createLimitRequestAccount struct {
	Name      string                   `json:"name"`
	LimitType string                   `json:"limitType"`
	MaxAmount decimal.Decimal          `json:"maxAmount"`
	Currency  string                   `json:"currency"`
	Scopes    []limitScopeInputAccount `json:"scopes"`
}

// createLimitRequestTransactionType is the request body for creating a limit with transaction type scope.
type createLimitRequestTransactionType struct {
	Name      string                           `json:"name"`
	LimitType string                           `json:"limitType"`
	MaxAmount decimal.Decimal                  `json:"maxAmount"`
	Currency  string                           `json:"currency"`
	Scopes    []limitScopeInputTransactionType `json:"scopes"`
}

// limitResponse represents the response from limit creation.
type limitResponse struct {
	ID string `json:"limitId"`
}

// CreateLimitWithAccountScope creates a DAILY limit with the specified account scope and max amount.
// Returns the limit ID.
func CreateLimitWithAccountScope(t *testing.T, accountID string, maxAmount string) string {
	t.Helper()

	return CreateLimitWithAccountScopeAndType(t, accountID, maxAmount, "DAILY")
}

// CreateLimitWithAccountScopeAndType creates a limit with the specified account scope, max amount, and limit type.
// Returns the limit ID.
func CreateLimitWithAccountScopeAndType(t *testing.T, accountID string, maxAmount string, limitType string) string {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	// Safe prefix to avoid slice bounds panic on short IDs
	safePrefix := accountID
	if len(accountID) >= 8 {
		safePrefix = accountID[:8]
	}

	uniqueName := "Test Limit " + safePrefix + " " + RandomSuffix()
	reqBody := createLimitRequestAccount{
		Name:      uniqueName,
		LimitType: limitType,
		MaxAmount: decimal.RequireFromString(maxAmount),
		Currency:  "BRL",
		Scopes: []limitScopeInputAccount{
			{AccountID: &accountID},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }() // Intentionally ignored in test helper

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Failed to create test limit: %s", string(respBody))

	var limit limitResponse

	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	return limit.ID
}

// CreateLimitWithTransactionTypeScope creates a PER_TRANSACTION limit with the specified transaction type scope.
// Returns the limit ID.
func CreateLimitWithTransactionTypeScope(t *testing.T, transactionType string, maxAmount string) string {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	uniqueName := "Test Per-Txn Limit " + transactionType + " " + RandomSuffix()
	reqBody := createLimitRequestTransactionType{
		Name:      uniqueName,
		LimitType: "PER_TRANSACTION",
		MaxAmount: decimal.RequireFromString(maxAmount),
		Currency:  "BRL",
		Scopes: []limitScopeInputTransactionType{
			{TransactionType: &transactionType},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }() // Intentionally ignored in test helper

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Failed to create test limit: %s", string(respBody))

	var limit limitResponse

	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	return limit.ID
}

// ActivateLimit activates a limit by ID.
// Limits are created in DRAFT status and must be activated to be enforced.
func ActivateLimit(t *testing.T, limitID string) {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits/"+limitID+"/activate", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }() // Intentionally ignored in test helper

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to activate limit: %s", string(respBody))
}

// DraftLimit transitions a limit back to DRAFT status by ID.
// Only INACTIVE limits can transition to DRAFT per state machine.
func DraftLimit(t *testing.T, limitID string) {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits/"+limitID+"/draft", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }() // Intentionally ignored in test helper

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to draft limit: %s", string(respBody))
}

// CleanupLimit deletes a limit. Called in t.Cleanup() to clean up test data.
func CleanupLimit(t *testing.T, limitID string) {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	// Step 1: Deactivate limit first (ACTIVE → INACTIVE)
	// Required because ACTIVE limits cannot be deleted directly
	deactivateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	if err != nil {
		t.Logf("Cleanup: failed to create deactivate request for limit %s: %v", limitID, err)

		return
	}

	deactivateReq.Header.Set("X-API-Key", apiKey)

	deactivateResp, err := HTTPClient.Do(deactivateReq)
	if err != nil {
		t.Logf("Cleanup: failed to deactivate limit %s: %v", limitID, err)

		return
	}

	_ = deactivateResp.Body.Close() // Intentionally ignored in test helper

	// Step 2: Delete limit (INACTIVE → DELETED)
	deleteReq, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/limits/"+limitID, nil)
	if err != nil {
		t.Logf("Cleanup: failed to create delete request for limit %s: %v", limitID, err)

		return
	}

	deleteReq.Header.Set("X-API-Key", apiKey)

	deleteResp, err := HTTPClient.Do(deleteReq)
	if err != nil {
		t.Logf("Cleanup: failed to delete limit %s: %v", limitID, err)

		return
	}

	defer deleteResp.Body.Close()

	// Log status for debugging
	if deleteResp.StatusCode != http.StatusOK && deleteResp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(deleteResp.Body)
		t.Logf("Cleanup: DELETE limit %s returned status %d: %s", limitID, deleteResp.StatusCode, string(bodyBytes))
	}
}

// CreateLimitWithScope creates a DAILY limit with arbitrary scopes for integration testing.
// Returns the limit ID. The limit is created in DRAFT status.
func CreateLimitWithScope(t *testing.T, name string, maxAmount string, scopes []ScopeInput) string {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	type createLimitReq struct {
		Name      string       `json:"name"`
		LimitType string       `json:"limitType"`
		MaxAmount string       `json:"maxAmount"`
		Currency  string       `json:"currency"`
		Scopes    []ScopeInput `json:"scopes"`
	}

	_, err := decimal.NewFromString(maxAmount)
	require.NoError(t, err, "maxAmount must be a valid decimal: %q", maxAmount)

	if scopes == nil {
		scopes = []ScopeInput{}
	}

	reqBody := createLimitReq{
		Name:      name,
		LimitType: "DAILY",
		MaxAmount: maxAmount,
		Currency:  "BRL",
		Scopes:    scopes,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Failed to create test limit: %s", string(respBody))

	var limit limitResponse

	err = json.Unmarshal(respBody, &limit)
	require.NoError(t, err)

	return limit.ID
}

// CreateRuleWithScope creates a rule with a scope and returns the rule ID.
func CreateRuleWithScope(t *testing.T, name, expression, action string, scopes []ScopeInput) string {
	t.Helper()

	apiKey := GetAPIKey()
	baseURL := GetBaseURL()

	reqBody := RuleRequest{
		Name:       name,
		Expression: expression,
		Action:     action,
		Scopes:     scopes,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }() // Intentionally ignored in test helper

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Failed to create rule: %s", string(respBody))

	var createdRule RuleResponse

	err = json.Unmarshal(respBody, &createdRule)
	require.NoError(t, err)

	return createdRule.ID
}

// basicPayloadCounter is used to generate deterministic UUIDs for CreateBasicValidationPayload.
// It starts from a high base (90000) to avoid collision with other test data.
var basicPayloadCounter int64 = 90000

// CreateBasicValidationPayload returns a basic valid validation request payload
// with all required fields (requestId, transactionType, amount, currency, timestamp, account).
// Helper for tests that need a minimal valid payload to customize.
// Uses deterministic UUIDs based on an incrementing counter for reproducible tests.
func CreateBasicValidationPayload() map[string]any {
	// Increment counter by 2 since we need 2 UUIDs per call (thread-safe)
	currentBase := atomic.AddInt64(&basicPayloadCounter, 2) - 2

	return map[string]any{
		"requestId":            MustDeterministicUUID(currentBase).String(),
		"transactionType":      "CARD",
		"amount":               "100.00",
		"currency":             "BRL",
		"transactionTimestamp": FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": MustDeterministicUUID(currentBase + 1).String(),
			"type":      "checking",
			"status":    "active",
		},
	}
}

// ExecuteValidationRequest executes a validation request with the given payload
// and returns the parsed result map and HTTP status code.
// This is a higher-level helper that combines CreateValidation with response parsing.
func ExecuteValidationRequest(t *testing.T, payload map[string]any) (map[string]any, int) {
	t.Helper()

	baseURL := GetBaseURL()
	apiKey := GetAPIKey()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }() // Intentionally ignored in test helper

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result map[string]any
	// Always attempt to parse response body for better error diagnostics
	if len(respBody) > 0 {
		err = json.Unmarshal(respBody, &result)
		if err != nil && resp.StatusCode == http.StatusOK {
			// Only fail on parse error for successful responses
			require.NoError(t, err, "Response: %s", string(respBody))
		}
		// For non-200 responses, ignore parse errors and let caller inspect status
	}

	return result, resp.StatusCode
}

// AssertRuleMatched validates that a rule appears in both matchedRuleIds and evaluatedRuleIds.
// Use this when a rule should have been evaluated and matched.
func AssertRuleMatched(t *testing.T, result map[string]any, ruleID string) {
	t.Helper()

	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be present and be an array, got: %v", result["matchedRuleIds"])
	assert.Contains(t, matchedRuleIDs, ruleID, "Rule should be matched")

	evaluatedRuleIDs, ok := result["evaluatedRuleIds"].([]any)
	require.True(t, ok, "evaluatedRuleIds should be present and be an array, got: %v", result["evaluatedRuleIds"])
	assert.Contains(t, evaluatedRuleIDs, ruleID, "Rule should be evaluated")
}

// AssertRuleEvaluatedButNotMatched validates that a rule was evaluated but did not match.
// Use this when a rule's expression evaluated to false.
func AssertRuleEvaluatedButNotMatched(t *testing.T, result map[string]any, ruleID string) {
	t.Helper()

	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be present and be an array, got: %v", result["matchedRuleIds"])
	assert.NotContains(t, matchedRuleIDs, ruleID, "Rule should NOT be matched")

	evaluatedRuleIDs, ok := result["evaluatedRuleIds"].([]any)
	require.True(t, ok, "evaluatedRuleIds should be present and be an array, got: %v", result["evaluatedRuleIds"])
	assert.Contains(t, evaluatedRuleIDs, ruleID, "Rule should be evaluated")
}

// AssertRuleNotEvaluated validates that a rule was filtered out (not evaluated).
// Use this when a rule should have been filtered by scope or status.
func AssertRuleNotEvaluated(t *testing.T, result map[string]any, ruleID string) {
	t.Helper()

	evaluatedRuleIDs, ok := result["evaluatedRuleIds"].([]any)
	require.True(t, ok, "evaluatedRuleIds should be present and be an array, got: %v", result["evaluatedRuleIds"])
	assert.NotContains(t, evaluatedRuleIDs, ruleID, "Rule should be filtered (not evaluated)")

	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be present and be an array, got: %v", result["matchedRuleIds"])
	assert.NotContains(t, matchedRuleIDs, ruleID, "Rule should NOT be matched")
}
