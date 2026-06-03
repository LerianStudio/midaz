// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	testutil_integration "github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil_integration"
)

// =============================================================================
// Decision Precedence Logic Tests - POST /v1/validations
// =============================================================================
//
// These tests verify the decision precedence rules:
//   - DENY takes precedence over ALLOW
//   - DENY takes precedence over REVIEW
//   - REVIEW takes precedence over ALLOW (when no DENY matches)
//   - Default decision when no rules match (configurable)
//
// Tests from roteiro section 4.6:
//   - 4.6.1: DENY over ALLOW
//   - 4.6.2: DENY over REVIEW
//   - 4.6.3: REVIEW when no DENY
//   - 4.6.4: Default decision ALLOW mode
//   - 4.6.5: Default decision DENY mode
//
// Reference: API Design 5.1 Validation Flow Integration
// =============================================================================

// TestValidation_DenyPrecedence_OverAllow verifies DENY takes precedence over ALLOW.
// Test 4.6.1 from roteiro
// Reference: API Design 5.1 Validation Flow Integration
func TestValidation_DenyPrecedence_OverAllow(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(4701).String()

	// Create 5 ALLOW rules that match
	allowRules := make([]string, 5)

	for i := 0; i < 5; i++ {
		ruleName := fmt.Sprintf("ALLOW Rule %c", 'A'+i)
		ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "amount > 0", "ALLOW")
		testutil.ActivateRule(t, ruleID)
		allowRules[i] = ruleID

		ruleIDCopy := ruleID
		t.Cleanup(func() {
			testutil.CleanupRule(t, ruleIDCopy)
		})
	}

	// Create 1 DENY rule that matches
	denyRule := testutil.CreateTestRuleWithExpression(t, "DENY Rule", "amount > 0", "DENY")
	testutil.ActivateRule(t, denyRule)

	t.Cleanup(func() {
		testutil.CleanupRule(t, denyRule)
	})

	// EXECUTION: Send validation request using testutil helpers
	payload := testutil.CreateBasicValidationPayload()
	payload["amount"] = 100000
	payload["account"] = map[string]any{
		"accountId": accountID,
		"type":      "checking",
		"status":    "active",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// Validate decision is DENY (takes precedence over ALLOW)
	decision, ok := result["decision"].(string)
	require.True(t, ok, "decision must be string type")
	assert.Equal(t, "DENY", decision, "DENY must take precedence over ALLOW")

	// Validate matchedRuleIds contains exactly the expected rule IDs
	matchedRuleIDs, exists := result["matchedRuleIds"]
	require.True(t, exists, "Response must contain matchedRuleIds")

	matchedArray, ok := matchedRuleIDs.([]any)
	require.True(t, ok, "matchedRuleIds must be array")

	// Verify all 5 ALLOW rules are present
	for i, allowRuleID := range allowRules {
		assert.Contains(t, matchedArray, allowRuleID,
			"matchedRuleIds must contain ALLOW rule %d: %s", i, allowRuleID)
	}

	// Verify the DENY rule is present
	assert.Contains(t, matchedArray, denyRule,
		"matchedRuleIds must contain DENY rule: %s", denyRule)

	// Validate evaluatedRuleIds contains exactly the expected rule IDs
	evaluatedRuleIDs, exists := result["evaluatedRuleIds"]
	require.True(t, exists, "Response must contain evaluatedRuleIds")

	evaluatedArray, ok := evaluatedRuleIDs.([]any)
	require.True(t, ok, "evaluatedRuleIds must be array")

	// Verify all 5 ALLOW rules were evaluated
	for i, allowRuleID := range allowRules {
		assert.Contains(t, evaluatedArray, allowRuleID,
			"evaluatedRuleIds must contain ALLOW rule %d: %s", i, allowRuleID)
	}

	// Verify the DENY rule was evaluated
	assert.Contains(t, evaluatedArray, denyRule,
		"evaluatedRuleIds must contain DENY rule: %s", denyRule)

	// Validate reason mentions DENY
	reason, ok := result["reason"].(string)
	require.True(t, ok, "reason must be string")
	reasonLower := strings.ToLower(reason)
	assert.True(t,
		strings.Contains(reasonLower, "deny") ||
			strings.Contains(reasonLower, "block") ||
			strings.Contains(reasonLower, "reject"),
		"reason should mention DENY/block/reject, got: %s", reason)
}

// TestValidation_DenyPrecedence_OverReview verifies DENY takes precedence over REVIEW.
// Test 4.6.2 from roteiro
// Reference: API Design 5.1 Validation Flow Integration
func TestValidation_DenyPrecedence_OverReview(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(4702).String()

	// Create 3 REVIEW rules that match
	reviewRules := make([]string, 3)

	for i := 0; i < 3; i++ {
		ruleName := fmt.Sprintf("REVIEW Rule %c", 'A'+i)
		ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "amount > 0", "REVIEW")
		testutil.ActivateRule(t, ruleID)
		reviewRules[i] = ruleID

		ruleIDCopy := ruleID
		t.Cleanup(func() {
			testutil.CleanupRule(t, ruleIDCopy)
		})
	}

	// Create 1 DENY rule that matches
	denyRule := testutil.CreateTestRuleWithExpression(t, "DENY Rule", "amount > 0", "DENY")
	testutil.ActivateRule(t, denyRule)

	t.Cleanup(func() {
		testutil.CleanupRule(t, denyRule)
	})

	// EXECUTION: Send validation request using testutil helpers
	payload := testutil.CreateBasicValidationPayload()
	payload["amount"] = 100000
	payload["account"] = map[string]any{
		"accountId": accountID,
		"type":      "checking",
		"status":    "active",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// Validate decision is DENY (takes precedence over REVIEW)
	decision, ok := result["decision"].(string)
	require.True(t, ok, "decision must be string type")
	assert.Equal(t, "DENY", decision, "DENY must take precedence over REVIEW")

	// Validate matchedRuleIds contains exactly the expected rule IDs
	matchedRuleIDs, exists := result["matchedRuleIds"]
	require.True(t, exists, "Response must contain matchedRuleIds")

	matchedArray, ok := matchedRuleIDs.([]any)
	require.True(t, ok, "matchedRuleIds must be array")

	// Verify all 3 REVIEW rules are present
	for i, reviewRuleID := range reviewRules {
		assert.Contains(t, matchedArray, reviewRuleID,
			"matchedRuleIds must contain REVIEW rule %d: %s", i, reviewRuleID)
	}

	// Verify the DENY rule is present
	assert.Contains(t, matchedArray, denyRule,
		"matchedRuleIds must contain DENY rule: %s", denyRule)

	// Validate evaluatedRuleIds contains exactly the expected rule IDs
	evaluatedRuleIDs, exists := result["evaluatedRuleIds"]
	require.True(t, exists, "Response must contain evaluatedRuleIds")

	evaluatedArray, ok := evaluatedRuleIDs.([]any)
	require.True(t, ok, "evaluatedRuleIds must be array")

	// Verify all 3 REVIEW rules were evaluated
	for i, reviewRuleID := range reviewRules {
		assert.Contains(t, evaluatedArray, reviewRuleID,
			"evaluatedRuleIds must contain REVIEW rule %d: %s", i, reviewRuleID)
	}

	// Verify the DENY rule was evaluated
	assert.Contains(t, evaluatedArray, denyRule,
		"evaluatedRuleIds must contain DENY rule: %s", denyRule)

	// Validate reason mentions DENY
	reason, ok := result["reason"].(string)
	require.True(t, ok, "reason must be string")
	reasonLower := strings.ToLower(reason)
	assert.True(t,
		strings.Contains(reasonLower, "deny") ||
			strings.Contains(reasonLower, "block"),
		"reason should mention DENY/block, got: %s", reason)
}

// TestValidation_ReviewDecision_WhenNoDeny verifies REVIEW when no DENY rules match.
// Test 4.6.3 from roteiro
// Reference: API Design 5.1 Validation Flow Integration
func TestValidation_ReviewDecision_WhenNoDeny(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(4703).String()

	// Create 3 ALLOW rules that match
	allowRules := make([]string, 3)

	for i := 0; i < 3; i++ {
		ruleName := fmt.Sprintf("ALLOW Rule %c", 'A'+i)
		ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "amount > 0", "ALLOW")
		testutil.ActivateRule(t, ruleID)
		allowRules[i] = ruleID

		ruleIDCopy := ruleID
		t.Cleanup(func() {
			testutil.CleanupRule(t, ruleIDCopy)
		})
	}

	// Create 1 REVIEW rule that matches
	reviewRule := testutil.CreateTestRuleWithExpression(t, "REVIEW Rule", "amount > 50000", "REVIEW")
	testutil.ActivateRule(t, reviewRule)

	t.Cleanup(func() {
		testutil.CleanupRule(t, reviewRule)
	})

	// EXECUTION: Send validation request using testutil helpers
	payload := testutil.CreateBasicValidationPayload()
	payload["amount"] = 80000
	payload["account"] = map[string]any{
		"accountId": accountID,
		"type":      "checking",
		"status":    "active",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// Validate decision is REVIEW (no DENY, but REVIEW rule matched)
	decision, ok := result["decision"].(string)
	require.True(t, ok, "decision must be string type")
	assert.Equal(t, "REVIEW", decision, "Decision should be REVIEW when no DENY rules match but REVIEW rule matches")

	// Validate matchedRuleIds contains exactly the expected rule IDs
	matchedRuleIDs, exists := result["matchedRuleIds"]
	require.True(t, exists, "Response must contain matchedRuleIds")

	matchedArray, ok := matchedRuleIDs.([]any)
	require.True(t, ok, "matchedRuleIds must be array")

	// Verify all 3 ALLOW rules are present
	for i, allowRuleID := range allowRules {
		assert.Contains(t, matchedArray, allowRuleID,
			"matchedRuleIds must contain ALLOW rule %d: %s", i, allowRuleID)
	}

	// Verify the REVIEW rule is present
	assert.Contains(t, matchedArray, reviewRule,
		"matchedRuleIds must contain REVIEW rule: %s", reviewRule)

	// Validate evaluatedRuleIds contains exactly the expected rule IDs
	evaluatedRuleIDs, exists := result["evaluatedRuleIds"]
	require.True(t, exists, "Response must contain evaluatedRuleIds")

	evaluatedArray, ok := evaluatedRuleIDs.([]any)
	require.True(t, ok, "evaluatedRuleIds must be array")

	// Verify all 3 ALLOW rules were evaluated
	for i, allowRuleID := range allowRules {
		assert.Contains(t, evaluatedArray, allowRuleID,
			"evaluatedRuleIds must contain ALLOW rule %d: %s", i, allowRuleID)
	}

	// Verify the REVIEW rule was evaluated
	assert.Contains(t, evaluatedArray, reviewRule,
		"evaluatedRuleIds must contain REVIEW rule: %s", reviewRule)

	// Validate reason mentions review
	reason, ok := result["reason"].(string)
	require.True(t, ok, "reason must be string")
	reasonLower := strings.ToLower(reason)
	assert.True(t,
		strings.Contains(reasonLower, "review") ||
			strings.Contains(reasonLower, "manual") ||
			strings.Contains(reasonLower, "flag"),
		"reason should mention review/manual/flag, got: %s", reason)
}

// TestValidation_DefaultDecision_AllowMode verifies default ALLOW when no rules match.
// Test 4.6.4 from roteiro
// Reference: API Design 5.1 Error Handling - configurable default
//
// NOTE: This test restarts the server with DEFAULT_DECISION_WHEN_NO_MATCH=ALLOW.
// It cannot run in parallel with other tests that restart the server.
func TestValidation_DefaultDecision_AllowMode(t *testing.T) {
	// Restart server with ALLOW mode
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"DEFAULT_DECISION_WHEN_NO_MATCH": "ALLOW",
	})
	require.NoError(t, err, "Failed to restart server with ALLOW mode")
	defer func() {
		if err := cleanup(); err != nil {
			t.Errorf("Failed to cleanup server config: %v", err)
		}
	}()

	// Create rule that does NOT match the payload
	ruleID := testutil.CreateTestRuleWithExpression(t, "Non-matching Rule", "amount > 1000000", "DENY")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// EXECUTION: Send validation request with amount that doesn't match any rule
	payload := testutil.CreateBasicValidationPayload()
	payload["amount"] = 5000 // Less than 1000000, won't match rule

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// Validate decision is ALLOW (default when no rules match)
	decision, ok := result["decision"].(string)
	require.True(t, ok, "decision must be string type")
	assert.Equal(t, "ALLOW", decision, "Default decision should be ALLOW when no rules match")

	// Validate matchedRuleIds is empty
	matchedRuleIDs, exists := result["matchedRuleIds"]
	require.True(t, exists, "Response must contain matchedRuleIds")

	matchedArray, ok := matchedRuleIDs.([]any)
	require.True(t, ok, "matchedRuleIds must be array")
	assert.Empty(t, matchedArray, "matchedRuleIds should be empty when no rules match")

	// Validate evaluatedRuleIds contains the rule (it was evaluated but didn't match)
	evaluatedRuleIDs, exists := result["evaluatedRuleIds"]
	require.True(t, exists, "Response must contain evaluatedRuleIds")

	evaluatedArray, ok := evaluatedRuleIDs.([]any)
	require.True(t, ok, "evaluatedRuleIds must be array")
	assert.NotEmpty(t, evaluatedArray, "evaluatedRuleIds should contain rules that were evaluated")

	// Validate reason mentions default or no match
	reason, ok := result["reason"].(string)
	require.True(t, ok, "reason must be string")
	reasonLower := strings.ToLower(reason)
	assert.True(t,
		strings.Contains(reasonLower, "matching") ||
			strings.Contains(reasonLower, "no rules") ||
			strings.Contains(reasonLower, "default") ||
			strings.Contains(reasonLower, "allow"),
		"reason should mention 'no rules matched' or 'default allow', got: %s", reason)
}

// TestValidation_DefaultDecision_DenyMode verifies configurable default DENY.
// Test 4.6.5 from roteiro
// Reference: API Design 5.1 Error Handling - configurable default
//
// NOTE: This test restarts the server with DEFAULT_DECISION_WHEN_NO_MATCH=DENY.
// It cannot run in parallel with other tests that restart the server.
func TestValidation_DefaultDecision_DenyMode(t *testing.T) {
	// Restart server with DENY mode
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"DEFAULT_DECISION_WHEN_NO_MATCH": "DENY",
	})
	require.NoError(t, err, "Failed to restart server with DENY mode")
	defer func() {
		if err := cleanup(); err != nil {
			t.Errorf("Failed to cleanup server config: %v", err)
		}
	}()

	// Create rule that does NOT match the payload
	ruleID := testutil.CreateTestRuleWithExpression(t, "Non-matching Rule", "amount > 1000000", "ALLOW")
	testutil.ActivateRule(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// EXECUTION: Send validation request with amount that doesn't match any rule
	payload := testutil.CreateBasicValidationPayload()
	payload["amount"] = 5000 // Less than 1000000, won't match rule

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// Validate decision is DENY (configurable default in fail-closed mode)
	decision, ok := result["decision"].(string)
	require.True(t, ok, "decision must be string type")
	assert.Equal(t, "DENY", decision, "Default decision should be DENY when configured in fail-closed mode")

	// Validate matchedRuleIds is empty
	matchedRuleIDs, exists := result["matchedRuleIds"]
	require.True(t, exists, "Response must contain matchedRuleIds")

	matchedArray, ok := matchedRuleIDs.([]any)
	require.True(t, ok, "matchedRuleIds must be array")
	assert.Empty(t, matchedArray, "matchedRuleIds should be empty when no rules match")

	// Validate reason mentions no match or default deny
	reason, ok := result["reason"].(string)
	require.True(t, ok, "reason must be string")
	reasonLower := strings.ToLower(reason)
	assert.True(t,
		strings.Contains(reasonLower, "default") ||
			strings.Contains(reasonLower, "deny") ||
			strings.Contains(reasonLower, "no matching") ||
			strings.Contains(reasonLower, "no rules"),
		"reason should mention default/deny/no match, got: %s", reason)
}
