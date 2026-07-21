// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// =============================================================================
// Context Objects Validation Tests - POST /v1/validations
// =============================================================================
//
// These tests verify that context objects (Account, Segment, Portfolio, Merchant)
// are correctly parsed and accessible in CEL expressions.
//
// Tests from roteiro section 4.7:
//   - 4.7.1: AccountContext with all fields
//   - 4.7.2: AccountContext with only accountId (minimal)
//   - 4.7.2b: AccountContext missing accountId (validation fails)
//   - 4.7.3: Missing account field
//   - 4.7.4-5: SegmentContext structure and CEL access
//   - 4.7.5b: SegmentContext minimal (only segmentId)
//   - 4.7.6-7: PortfolioContext structure and CEL access
//   - 4.7.7b: PortfolioContext minimal (only portfolioId)
//   - 4.7.8: MerchantContext complete structure
//   - 4.7.8b: MerchantContext minimal (only merchantId)
//
// References:
//   - API Design 6.11 AccountContext
//   - API Design 6.12 SegmentContext
//   - API Design 6.13 PortfolioContext
//   - API Design 6.15 MerchantContext
// =============================================================================

// TestValidation_AccountContext_AllFields verifies AccountContext with all fields.
// Test 4.7.1 from roteiro
// Reference: API Design 6.11 AccountContext
func TestValidation_AccountContext_AllFields(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create rule that accesses all account fields
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Account Full Context Rule",
		"account.type == 'checking' && account.status == 'active'",
		"ALLOW")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Create validation with complete AccountContext
	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4101).String(),
		"transactionType":      "CARD",
		"amount":               "100.00",
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4102).String(),
			"type":      "checking",
			"status":    "active",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	// Validate rule matched (CEL can access all account fields)
	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.NotEmpty(t, matchedRuleIDs, "Rule should match when account fields are accessible")
	assert.Equal(t, "ALLOW", result["decision"], "Decision should be ALLOW")
}

// TestValidation_AccountContext_MinimalFields verifies accountId only is accepted.
// Test 4.7.2 from roteiro
// Reference: API Design 6.11 AccountContext (accountId required, others optional)
func TestValidation_AccountContext_MinimalFields(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	accountID := testutil.MustDeterministicUUID(4103).String()

	// Create rule that only checks accountId
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Account ID Rule",
		fmt.Sprintf(`account["accountId"] == "%s"`, accountID),
		"ALLOW")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Create validation with minimal AccountContext (only accountId)
	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4104).String(),
		"transactionType":      "CARD",
		"amount":               "100.00",
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": accountID,
			// type and status omitted (optional fields)
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"Request should be accepted with minimal AccountContext (only accountId). Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	// Validate rule matched (CEL can access accountId)
	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.NotEmpty(t, matchedRuleIDs, "Rule should match when accountId is accessible")
	assert.Equal(t, "ALLOW", result["decision"])
}

// TestValidation_ContextObjects_MissingAccount_ReturnsError verifies missing account field returns error.
// Test 4.7.3 from roteiro
// Reference: API Design 4.1.1 Validation Rules (account is required)
func TestValidation_ContextObjects_MissingAccount_ReturnsError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create validation request WITHOUT account field
	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4105).String(),
		"transactionType":      "CARD",
		"amount":               "100.00",
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		// account field intentionally omitted
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0420", errResp.Code, "Expected 0420 for missing account")
	assert.Equal(t, "Validation Account Required", errResp.Title, "Error title should match the account error")
	assert.Equal(t, "Account is required.", tracerProblemDetail(t, respBody), "Error detail should match exactly")
}

// TestValidation_SegmentContext_Structure verifies SegmentContext parsing.
// Test 4.7.4 from roteiro
// Reference: API Design 6.12 SegmentContext
func TestValidation_SegmentContext_Structure(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create rule that accesses segment fields
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Segment Context Rule",
		"segment.name == 'premium'",
		"ALLOW")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Create validation with SegmentContext
	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4106).String(),
		"transactionType":      "CARD",
		"amount":               "100.00",
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4107).String(),
			"type":      "checking",
			"status":    "active",
		},
		"segment": map[string]any{
			"segmentId": testutil.MustDeterministicUUID(4108).String(),
			"name":      "premium",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	// Validate rule matched (CEL can access segment.name)
	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.NotEmpty(t, matchedRuleIDs, "Rule should match when segment context is accessible")
	assert.Equal(t, "ALLOW", result["decision"])
}

// TestValidation_SegmentContext_CELFieldAccess verifies CEL can access all segment fields.
// Test 4.7.5 from roteiro
// Reference: API Design 6.12 SegmentContext
func TestValidation_SegmentContext_CELFieldAccess(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	segmentID := testutil.MustDeterministicUUID(4109).String()

	// Create rule that accesses both segmentId and name
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Segment Full Access Rule",
		"segment.segmentId == '"+segmentID+"' && segment.name == 'premium'",
		"ALLOW")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Create validation with complete SegmentContext
	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4110).String(),
		"transactionType":      "CARD",
		"amount":               "100.00",
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4111).String(),
			"type":      "checking",
			"status":    "active",
		},
		"segment": map[string]any{
			"segmentId": segmentID,
			"name":      "premium",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	// Validate rule matched (CEL can access all segment fields)
	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.NotEmpty(t, matchedRuleIDs, "Rule should match - CEL can access segment.segmentId and segment.name")
	assert.Equal(t, "ALLOW", result["decision"])
}

// TestValidation_PortfolioContext_Structure verifies PortfolioContext parsing.
// Test 4.7.6 from roteiro
// Reference: API Design 6.13 PortfolioContext
func TestValidation_PortfolioContext_Structure(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create rule that accesses portfolio fields
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Portfolio Context Rule",
		"portfolio.name == 'corporate'",
		"ALLOW")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Create validation with PortfolioContext
	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4112).String(),
		"transactionType":      "CARD",
		"amount":               "100.00",
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4113).String(),
			"type":      "checking",
			"status":    "active",
		},
		"portfolio": map[string]any{
			"portfolioId": testutil.MustDeterministicUUID(4114).String(),
			"name":        "corporate",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	// Validate rule matched (CEL can access portfolio.name)
	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.NotEmpty(t, matchedRuleIDs, "Rule should match when portfolio context is accessible")
	assert.Equal(t, "ALLOW", result["decision"])
}

// TestValidation_PortfolioContext_CELFieldAccess verifies CEL can access all portfolio fields.
// Test 4.7.7 from roteiro
// Reference: API Design 6.13 PortfolioContext
func TestValidation_PortfolioContext_CELFieldAccess(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	portfolioID := testutil.MustDeterministicUUID(4115).String()

	// Create rule that accesses both portfolioId and name
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Portfolio Full Access Rule",
		"portfolio.portfolioId == '"+portfolioID+"' && portfolio.name == 'corporate'",
		"ALLOW")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Create validation with complete PortfolioContext
	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4116).String(),
		"transactionType":      "CARD",
		"amount":               "100.00",
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4117).String(),
			"type":      "checking",
			"status":    "active",
		},
		"portfolio": map[string]any{
			"portfolioId": portfolioID,
			"name":        "corporate",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	// Validate rule matched (CEL can access all portfolio fields)
	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.NotEmpty(t, matchedRuleIDs, "Rule should match - CEL can access portfolio.portfolioId and portfolio.name")
	assert.Equal(t, "ALLOW", result["decision"])
}

// TestValidation_MerchantContext_CompleteStructure verifies MerchantContext with all fields.
// Test 4.7.8 from roteiro
// Reference: API Design 6.15 MerchantContext
func TestValidation_MerchantContext_CompleteStructure(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Create rule that accesses multiple merchant fields
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Merchant Context Rule",
		"merchant.category == '5411' && merchant.country == 'BR'",
		"ALLOW")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Create validation with complete MerchantContext
	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4118).String(),
		"transactionType":      "CARD",
		"amount":               "100.00",
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4119).String(),
			"type":      "checking",
			"status":    "active",
		},
		"merchant": map[string]any{
			"merchantId": testutil.MustDeterministicUUID(4120).String(),
			"name":       "Grocery Store ABC",
			"category":   "5411",
			"country":    "BR",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	// Validate rule matched (CEL can access all merchant fields)
	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.NotEmpty(t, matchedRuleIDs,
		"Rule should match - CEL can access merchant.category and merchant.country")
	assert.Equal(t, "ALLOW", result["decision"])
}

// TestValidation_AccountContext_MissingAccountId verifies validation fails when accountId is missing.
// Test 4.7.2b from roteiro 04-rules-evaluation.md
// Reference: API Design 6.11 AccountContext ("accountId is required")
func TestValidation_AccountContext_MissingAccountId(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// EXECUTION: Send request with account missing accountId
	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4121).String(),
		"transactionType":      "CARD",
		"amount":               "100.00",
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			// "accountId" intentionally omitted
			"type":   "checking",
			"status": "active",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// VALIDATIONS
	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"Missing accountId should return 400 Bad Request")

	errorResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0420", errorResp.Code, "Error code should be 0420")
	assert.Equal(t, "Validation Account Required", errorResp.Title, "Error title should match the account error")
	assert.Equal(t, "Account is required.", tracerProblemDetail(t, respBody), "Error detail should match exactly")
}

// TestValidation_SegmentContext_Minimal verifies segment with only segmentId (name optional).
// Test 4.7.5b from roteiro 04-rules-evaluation.md
// Reference: API Design 6.12 SegmentContext ("name is optional")
func TestValidation_SegmentContext_Minimal(t *testing.T) {
	segmentID := testutil.MustDeterministicUUID(4122).String()

	// PRECONDITIONS: Create rule checking segment["segmentId"]
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Segment Minimal Rule",
		`size(segment) > 0 && segment["segmentId"] == "`+segmentID+`"`,
		"ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation with segment containing only segmentId (name omitted)
	payload := testutil.CreateBasicValidationPayload()
	payload["segment"] = map[string]any{
		"segmentId": segmentID,
		// "name" intentionally omitted (optional field)
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status, "Segment with only segmentId should be accepted (name is optional)")

	// VALIDATIONS: Rule should match
	assert.Equal(t, "ALLOW", result["decision"])
	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.Contains(t, matchedRuleIDs, ruleID, "Rule should match - CEL can access segmentId without name")
}

// TestValidation_PortfolioContext_Minimal verifies portfolio with only portfolioId (name optional).
// Test 4.7.7b from roteiro 04-rules-evaluation.md
// Reference: API Design 6.13 PortfolioContext ("name is optional")
func TestValidation_PortfolioContext_Minimal(t *testing.T) {
	portfolioID := testutil.MustDeterministicUUID(4123).String()

	// PRECONDITIONS: Create rule checking portfolio["portfolioId"]
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Portfolio Minimal Rule",
		`size(portfolio) > 0 && portfolio["portfolioId"] == "`+portfolioID+`"`,
		"ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation with portfolio containing only portfolioId (name omitted)
	payload := testutil.CreateBasicValidationPayload()
	payload["portfolio"] = map[string]any{
		"portfolioId": portfolioID,
		// "name" intentionally omitted (optional field)
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status, "Portfolio with only portfolioId should be accepted (name is optional)")

	// VALIDATIONS: Rule should match
	assert.Equal(t, "ALLOW", result["decision"])
	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.Contains(t, matchedRuleIDs, ruleID, "Rule should match - CEL can access portfolioId without name")
}

// TestValidation_MerchantContext_Minimal verifies merchant with only merchantId.
// Test 4.7.8b from roteiro 04-rules-evaluation.md
// Reference: API Design 6.15 MerchantContext ("All fields optional except merchantId")
func TestValidation_MerchantContext_Minimal(t *testing.T) {
	merchantID := testutil.MustDeterministicUUID(4124).String()

	// PRECONDITIONS: Create rule checking merchant["merchantId"]
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Merchant Minimal Rule",
		`size(merchant) > 0 && merchant["merchantId"] == "`+merchantID+`"`,
		"ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation with merchant containing only merchantId
	payload := testutil.CreateBasicValidationPayload()
	payload["merchant"] = map[string]any{
		"merchantId": merchantID,
		// "name", "category", "country", "mcc" intentionally omitted (all optional)
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status,
		"Merchant with only merchantId should be accepted (other fields optional)")

	// VALIDATIONS: Rule should match
	assert.Equal(t, "ALLOW", result["decision"])
	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.Contains(t, matchedRuleIDs, ruleID,
		"Rule should match - CEL can access merchantId without other fields")
}
