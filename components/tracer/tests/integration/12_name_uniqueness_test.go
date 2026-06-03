// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// =============================================================================
// Rules & Limits Name Uniqueness Tests
//
// These tests verify:
// 1. Creating a rule with a duplicate name in the same context returns HTTP 409 + TRC-0303
// 2. Creating a rule with the same name in a DIFFERENT context returns HTTP 201
// 3. Creating a rule with a name that was previously soft-deleted returns HTTP 201
// 4. Creating a limit with a duplicate name (globally unique) returns HTTP 409 + TRC-0304
// 5. Creating a limit with a name that was previously soft-deleted returns HTTP 201
//
// Implementation complete:
// - Unique indexes on rules(context_id, name) and limits(name) with NULLS NOT DISTINCT
// - TRC-0303 and TRC-0304 error codes for duplicate names
// - Context-scoped uniqueness for rules (via context_id derived from first scope's segmentId)
// - Global uniqueness for limits (name must be unique across all non-deleted limits)
// TODO: update limit tests to include rule_id when limits become rule-scoped
// =============================================================================

// nameUniquenessRuleRequest is a helper struct for rule creation
type nameUniquenessRuleRequest struct {
	Name       string                `json:"name"`
	Expression string                `json:"expression"`
	Action     string                `json:"action"`
	Scopes     []testutil.ScopeInput `json:"scopes,omitempty"`
}

// nameUniquenessLimitRequest is a helper struct for limit creation
type nameUniquenessLimitRequest struct {
	Name      string                `json:"name"`
	LimitType string                `json:"limitType"`
	MaxAmount decimal.Decimal       `json:"maxAmount"`
	Currency  string                `json:"currency"`
	Scopes    []testutil.ScopeInput `json:"scopes"`
	RuleID    *string               `json:"ruleId,omitempty"` // Future field for rule association
}

// nameUniquenessErrorResponse represents the error response structure
type nameUniquenessErrorResponse struct {
	Code    string `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// =============================================================================
// Test 1: CreateRule_DuplicateName_Returns409
// Same name + same context -> 409 + TRC-0303
// =============================================================================

func TestCreateRule_DuplicateName_Returns409(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Use deterministic UUIDs for test reproducibility (12000+ range to avoid conflicts with other test suites)
	contextID := testutil.MustDeterministicUUID(12001).String()
	ruleName := "duplicate rule name test " + testutil.RandomSuffix()

	// Create the first rule (should succeed)
	reqBody1 := nameUniquenessRuleRequest{
		Name:       ruleName,
		Expression: "amount > 100",
		Action:     "DENY",
		Scopes: []testutil.ScopeInput{
			{SegmentID: testutil.StringPtr(contextID)}, // Using segmentId as context
		},
	}

	body1, err := json.Marshal(reqBody1)
	require.NoError(t, err)

	req1, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body1))
	require.NoError(t, err)
	req1.Header.Set("X-API-Key", apiKey)
	req1.Header.Set("Content-Type", "application/json")

	resp1, err := testutil.HTTPClient.Do(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()

	respBody1, err := io.ReadAll(resp1.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First rule creation should succeed: %s", string(respBody1))

	var result1 map[string]any
	err = json.Unmarshal(respBody1, &result1)
	require.NoError(t, err)

	firstRuleID, ok := result1["ruleId"].(string)
	require.True(t, ok, "expected ruleId to be a string")
	t.Cleanup(func() {
		testutil.CleanupRule(t, firstRuleID)
	})

	// Create the second rule with the SAME name and SAME context (should fail with 409)
	reqBody2 := nameUniquenessRuleRequest{
		Name:       ruleName, // Same name as first rule
		Expression: "amount > 200",
		Action:     "ALLOW",
		Scopes: []testutil.ScopeInput{
			{SegmentID: testutil.StringPtr(contextID)}, // Same context as first rule
		},
	}

	body2, err := json.Marshal(reqBody2)
	require.NoError(t, err)

	req2, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body2))
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", apiKey)
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := testutil.HTTPClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	// Verify duplicate rule name in same context returns 409 + TRC-0303
	assert.Equal(t, http.StatusConflict, resp2.StatusCode, "Duplicate rule name in same context should return 409: %s", string(respBody2))

	if resp2.StatusCode == http.StatusConflict {
		var errResp nameUniquenessErrorResponse
		err = json.Unmarshal(respBody2, &errResp)
		require.NoError(t, err)

		assert.Equal(t, "TRC-0303", errResp.Code, "Error code should be TRC-0303 for duplicate rule name in same context")
		assert.Equal(t, "Conflict", errResp.Title, "Error title should be Conflict")
		assert.Contains(t, errResp.Message, "name", "Error message should mention name")
	}
}

// =============================================================================
// Test 2: CreateRule_DuplicateName_DifferentContext_Returns201
// Same name + different context -> 201
// =============================================================================

func TestCreateRule_DuplicateName_DifferentContext_Returns201(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Use deterministic UUIDs for test reproducibility
	contextID1 := testutil.MustDeterministicUUID(12003).String()
	contextID2 := testutil.MustDeterministicUUID(12004).String()
	ruleName := "same name different context " + testutil.RandomSuffix()

	// Create the first rule in context 1
	reqBody1 := nameUniquenessRuleRequest{
		Name:       ruleName,
		Expression: "amount > 100",
		Action:     "DENY",
		Scopes: []testutil.ScopeInput{
			{SegmentID: testutil.StringPtr(contextID1)}, // Context 1
		},
	}

	body1, err := json.Marshal(reqBody1)
	require.NoError(t, err)

	req1, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body1))
	require.NoError(t, err)
	req1.Header.Set("X-API-Key", apiKey)
	req1.Header.Set("Content-Type", "application/json")

	resp1, err := testutil.HTTPClient.Do(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()

	respBody1, err := io.ReadAll(resp1.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First rule creation should succeed: %s", string(respBody1))

	var result1 map[string]any
	err = json.Unmarshal(respBody1, &result1)
	require.NoError(t, err)

	firstRuleID, ok := result1["ruleId"].(string)
	require.True(t, ok, "expected ruleId to be a string")
	t.Cleanup(func() {
		testutil.CleanupRule(t, firstRuleID)
	})

	// Create the second rule with the SAME name but DIFFERENT context (should succeed)
	reqBody2 := nameUniquenessRuleRequest{
		Name:       ruleName, // Same name as first rule
		Expression: "amount > 200",
		Action:     "ALLOW",
		Scopes: []testutil.ScopeInput{
			{SegmentID: testutil.StringPtr(contextID2)}, // Different context
		},
	}

	body2, err := json.Marshal(reqBody2)
	require.NoError(t, err)

	req2, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body2))
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", apiKey)
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := testutil.HTTPClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	// Verify same name in different context is allowed (context-scoped uniqueness)
	assert.Equal(t, http.StatusCreated, resp2.StatusCode, "Same name in different context should be allowed (201): %s", string(respBody2))

	if resp2.StatusCode == http.StatusCreated {
		var result2 map[string]any
		err = json.Unmarshal(respBody2, &result2)
		require.NoError(t, err)

		secondRuleID, ok := result2["ruleId"].(string)
		require.True(t, ok, "expected ruleId to be a string")
		t.Cleanup(func() {
			testutil.CleanupRule(t, secondRuleID)
		})

		// Verify both rules exist with the same name
		assert.NotEqual(t, firstRuleID, secondRuleID, "Rule IDs should be different")
	}
}

// =============================================================================
// Test 3: CreateRule_DeletedNameReuse_Returns201
// Create -> soft-delete -> create same name -> 201
// =============================================================================

func TestCreateRule_DeletedNameReuse_Returns201(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Use deterministic UUIDs for test reproducibility
	contextID := testutil.MustDeterministicUUID(12006).String()
	ruleName := "deleted name reuse " + testutil.RandomSuffix()

	// Create the first rule
	reqBody1 := nameUniquenessRuleRequest{
		Name:       ruleName,
		Expression: "amount > 100",
		Action:     "DENY",
		Scopes: []testutil.ScopeInput{
			{SegmentID: testutil.StringPtr(contextID)},
		},
	}

	body1, err := json.Marshal(reqBody1)
	require.NoError(t, err)

	req1, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body1))
	require.NoError(t, err)
	req1.Header.Set("X-API-Key", apiKey)
	req1.Header.Set("Content-Type", "application/json")

	resp1, err := testutil.HTTPClient.Do(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()

	respBody1, err := io.ReadAll(resp1.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First rule creation should succeed: %s", string(respBody1))

	var result1 map[string]any
	err = json.Unmarshal(respBody1, &result1)
	require.NoError(t, err)

	firstRuleID, ok := result1["ruleId"].(string)
	require.True(t, ok, "expected ruleId to be a string")

	// Soft-delete the first rule (deactivate + delete)
	testutil.DeactivateRule(t, firstRuleID)
	testutil.DeleteRuleViaAPI(t, firstRuleID)

	// Create a NEW rule with the SAME name (should succeed because old one is soft-deleted)
	reqBody2 := nameUniquenessRuleRequest{
		Name:       ruleName, // Same name as the deleted rule
		Expression: "amount > 500",
		Action:     "REVIEW",
		Scopes: []testutil.ScopeInput{
			{SegmentID: testutil.StringPtr(contextID)}, // Same context
		},
	}

	body2, err := json.Marshal(reqBody2)
	require.NoError(t, err)

	req2, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body2))
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", apiKey)
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := testutil.HTTPClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	// Verify soft-deleted names can be reused (partial index excludes DELETED)
	assert.Equal(t, http.StatusCreated, resp2.StatusCode, "Reusing soft-deleted rule name should succeed (201): %s", string(respBody2))

	if resp2.StatusCode == http.StatusCreated {
		var result2 map[string]any
		err = json.Unmarshal(respBody2, &result2)
		require.NoError(t, err)

		secondRuleID, ok := result2["ruleId"].(string)
		require.True(t, ok, "expected ruleId to be a string")
		t.Cleanup(func() {
			testutil.CleanupRule(t, secondRuleID)
		})

		// Verify the new rule was created with the reused name
		assert.NotEmpty(t, secondRuleID, "New rule ID should not be empty")
	}
}

// =============================================================================
// Test 4: CreateLimit_DuplicateName_Returns409
// Same name (globally unique) -> 409 + TRC-0304
// =============================================================================

func TestCreateLimit_DuplicateName_Returns409(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Use deterministic UUIDs for test reproducibility
	accountID := testutil.MustDeterministicUUID(12008).String()
	limitName := "duplicate limit name test " + testutil.RandomSuffix()

	// Create the first limit
	reqBody1 := nameUniquenessLimitRequest{
		Name:      limitName,
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "BRL",
		Scopes: []testutil.ScopeInput{
			{AccountID: testutil.StringPtr(accountID)},
		},
	}

	body1, err := json.Marshal(reqBody1)
	require.NoError(t, err)

	req1, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(body1))
	require.NoError(t, err)
	req1.Header.Set("X-API-Key", apiKey)
	req1.Header.Set("Content-Type", "application/json")

	resp1, err := testutil.HTTPClient.Do(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()

	respBody1, err := io.ReadAll(resp1.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First limit creation should succeed: %s", string(respBody1))

	var result1 map[string]any
	err = json.Unmarshal(respBody1, &result1)
	require.NoError(t, err)

	firstLimitID, ok := result1["limitId"].(string)
	require.True(t, ok, "expected limitId to be a string")
	t.Cleanup(func() {
		testutil.CleanupLimit(t, firstLimitID)
	})

	// Create the second limit with the SAME name (should fail with 409)
	// Note: Limit names are globally unique among non-deleted limits
	reqBody2 := nameUniquenessLimitRequest{
		Name:      limitName, // Same name as first limit
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("2000"),
		Currency:  "BRL",
		Scopes: []testutil.ScopeInput{
			{AccountID: testutil.StringPtr(accountID)},
		},
	}

	body2, err := json.Marshal(reqBody2)
	require.NoError(t, err)

	req2, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(body2))
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", apiKey)
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := testutil.HTTPClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	// Verify duplicate limit name returns 409 + TRC-0304
	assert.Equal(t, http.StatusConflict, resp2.StatusCode, "Duplicate limit name should return 409: %s", string(respBody2))

	if resp2.StatusCode == http.StatusConflict {
		var errResp nameUniquenessErrorResponse
		err = json.Unmarshal(respBody2, &errResp)
		require.NoError(t, err)

		assert.Equal(t, "TRC-0304", errResp.Code, "Error code should be TRC-0304 for duplicate limit name")
		assert.Equal(t, "Conflict", errResp.Title, "Error title should be Conflict")
		assert.Contains(t, errResp.Message, "name", "Error message should mention name")
	}
}

// =============================================================================
// Test 5: CreateLimit_DeletedNameReuse_Returns201
// Create -> soft-delete -> create same name -> 201
// =============================================================================

func TestCreateLimit_DeletedNameReuse_Returns201(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// Use deterministic UUIDs for test reproducibility
	accountID := testutil.MustDeterministicUUID(12010).String()
	limitName := "deleted limit name reuse " + testutil.RandomSuffix()

	// Create the first limit
	reqBody1 := nameUniquenessLimitRequest{
		Name:      limitName,
		LimitType: "DAILY",
		MaxAmount: decimal.RequireFromString("1000"),
		Currency:  "BRL",
		Scopes: []testutil.ScopeInput{
			{AccountID: testutil.StringPtr(accountID)},
		},
	}

	body1, err := json.Marshal(reqBody1)
	require.NoError(t, err)

	req1, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(body1))
	require.NoError(t, err)
	req1.Header.Set("X-API-Key", apiKey)
	req1.Header.Set("Content-Type", "application/json")

	resp1, err := testutil.HTTPClient.Do(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()

	respBody1, err := io.ReadAll(resp1.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp1.StatusCode, "First limit creation should succeed: %s", string(respBody1))

	var result1 map[string]any
	err = json.Unmarshal(respBody1, &result1)
	require.NoError(t, err)

	firstLimitID, ok := result1["limitId"].(string)
	require.True(t, ok, "expected limitId to be a string")

	// Activate and then soft-delete the first limit
	testutil.ActivateLimit(t, firstLimitID)
	testutil.CleanupLimit(t, firstLimitID) // This deactivates and deletes

	// Create a NEW limit with the SAME name (should succeed because old one is soft-deleted)
	reqBody2 := nameUniquenessLimitRequest{
		Name:      limitName, // Same name as the deleted limit
		LimitType: "MONTHLY",
		MaxAmount: decimal.RequireFromString("5000"),
		Currency:  "BRL",
		Scopes: []testutil.ScopeInput{
			{AccountID: testutil.StringPtr(accountID)},
		},
	}

	body2, err := json.Marshal(reqBody2)
	require.NoError(t, err)

	req2, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(body2))
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", apiKey)
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := testutil.HTTPClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	// Verify soft-deleted limit names can be reused (partial index excludes DELETED)
	assert.Equal(t, http.StatusCreated, resp2.StatusCode, "Reusing soft-deleted limit name should succeed (201): %s", string(respBody2))

	if resp2.StatusCode == http.StatusCreated {
		var result2 map[string]any
		err = json.Unmarshal(respBody2, &result2)
		require.NoError(t, err)

		secondLimitID, ok := result2["limitId"].(string)
		require.True(t, ok, "expected limitId to be a string")
		t.Cleanup(func() {
			testutil.CleanupLimit(t, secondLimitID)
		})

		// Verify the new limit was created with the reused name
		assert.NotEmpty(t, secondLimitID, "New limit ID should not be empty")
	}
}
