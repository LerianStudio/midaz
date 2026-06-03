// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// =============================================================================
// Complete Evaluation + Response Structure Tests - POST /v1/validations
// =============================================================================
//
// These tests verify complete rule evaluation (no short-circuit) and response structure.
//
// Tests from roteiro sections 4.3 and 4.5:
//   - 4.3.1: All ACTIVE rules evaluated (no short-circuit)
//   - 4.3.2: Collects matching + DENY precedence
//   - 4.3.3: Collects evaluated rules (scope filtering)
//   - 4.3.4: DRAFT rules not evaluated
//   - 4.3.5: INACTIVE rules not evaluated
//   - 4.3.6: DELETED rules not evaluated
//   - 4.5.1: ValidationId is server-generated UUID
//   - 4.5.2: RequestId echoed from input
//   - 4.5.3: ProcessingTimeMs always present
//   - 4.5.4: Reason field (3 scenarios: ALLOW/DENY/REVIEW)
//   - 4.5.5: LimitUsageDetails empty array
//   - 4.5.6: LimitUsageDetails structure populated
//
// Note: Performance tests (4.3.7, 4.3.8) will be implemented in a separate phase
//
// Reference: API Design 5.1 Validation Flow Integration
// =============================================================================

// TestValidation_CompleteEvaluation_AllActiveRules verifies all ACTIVE rules are evaluated without short-circuit.
// Test 4.3.1 from roteiro 04-rules-evaluation.md
// Reference: API Design 5.1 Validation Flow Integration
func TestValidation_CompleteEvaluation_AllActiveRules(t *testing.T) {
	// Create unique scope for test isolation
	testAccountID := testutil.MustDeterministicUUID(4001).String()
	accountScope := testutil.ScopeInput{AccountID: &testAccountID}

	// PRECONDITIONS: Create and activate 5 rules that all match
	rules := []struct {
		name       string
		expression string
	}{
		{"Rule 1", `transactionType == "CARD"`},
		{"Rule 2", "amount > 10"},
		{"Rule 3", `currency == "BRL"`},
		{"Rule 4", `account["status"] == "active"`},
		{"Rule 5", "amount < 10000"},
	}

	ruleIDs := make([]string, len(rules))
	for i, r := range rules {
		ruleID := testutil.CreateRuleWithScope(t, r.name, r.expression, "ALLOW", []testutil.ScopeInput{accountScope})
		testutil.ActivateRule(t, ruleID)
		ruleIDs[i] = ruleID
		ruleIDCopy := ruleID
		t.Cleanup(func() { testutil.CleanupRule(t, ruleIDCopy) })
	}

	// EXECUTION: Send validation request that matches all rules with scoped account
	payload := testutil.CreateBasicValidationPayload()
	payload["transactionType"] = "CARD"
	payload["amount"] = "100.00"
	payload["currency"] = "BRL"
	payload["account"] = map[string]any{
		"accountId": testAccountID,
		"type":      "checking",
		"status":    "active",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: All test rules evaluated and matched (no short-circuit)
	evaluatedRuleIDs, ok := result["evaluatedRuleIds"].([]any)
	require.True(t, ok, "evaluatedRuleIds should be an array")
	assert.GreaterOrEqual(t, len(evaluatedRuleIDs), len(ruleIDs),
		"At least all %d test rules should be evaluated", len(ruleIDs))

	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.GreaterOrEqual(t, len(matchedRuleIDs), len(ruleIDs),
		"At least all %d test rules should match", len(ruleIDs))

	// Verify all rule IDs present
	for i, ruleID := range ruleIDs {
		assert.Contains(t, evaluatedRuleIDs, ruleID, "Rule %d should be evaluated", i+1)
		assert.Contains(t, matchedRuleIDs, ruleID, "Rule %d should match", i+1)
	}

	assert.Equal(t, "ALLOW", result["decision"])
}

// TestValidation_CompleteEvaluation_CollectsMatchingWithDenyPrecedence verifies system collects all matching rules and applies DENY precedence.
// Test 4.3.2 from roteiro 04-rules-evaluation.md
// Reference: API Design 5.1 Validation Flow Integration
func TestValidation_CompleteEvaluation_CollectsMatchingWithDenyPrecedence(t *testing.T) {
	// Create unique scope for test isolation
	testAccountID := testutil.MustDeterministicUUID(4002).String()
	accountScope := testutil.ScopeInput{AccountID: &testAccountID}

	// PRECONDITIONS: Create 3 ALLOW rules that match
	allowRules := []string{
		testutil.CreateRuleWithScope(t, "ALLOW Rule 1", `transactionType == "CARD"`, "ALLOW", []testutil.ScopeInput{accountScope}),
		testutil.CreateRuleWithScope(t, "ALLOW Rule 2", "amount > 50", "ALLOW", []testutil.ScopeInput{accountScope}),
		testutil.CreateRuleWithScope(t, "ALLOW Rule 3", `currency == "BRL"`, "ALLOW", []testutil.ScopeInput{accountScope}),
	}
	for _, ruleID := range allowRules {
		testutil.ActivateRule(t, ruleID)
		ruleIDCopy := ruleID
		t.Cleanup(func() { testutil.CleanupRule(t, ruleIDCopy) })
	}

	// Create 1 DENY rule that matches
	denyRule := testutil.CreateRuleWithScope(t, "DENY Rule", "amount > 500", "DENY", []testutil.ScopeInput{accountScope})
	testutil.ActivateRule(t, denyRule)
	t.Cleanup(func() { testutil.CleanupRule(t, denyRule) })

	// Create 1 ALLOW rule that does NOT match
	noMatchRule := testutil.CreateRuleWithScope(t, "No Match Rule", "amount > 10000", "ALLOW", []testutil.ScopeInput{accountScope})
	testutil.ActivateRule(t, noMatchRule)
	t.Cleanup(func() { testutil.CleanupRule(t, noMatchRule) })

	// EXECUTION: Send validation request with scoped account
	payload := testutil.CreateBasicValidationPayload()
	payload["amount"] = "1000.00" // Matches 3 ALLOW + 1 DENY, but not the "amount > 10000" rule
	payload["account"] = map[string]any{
		"accountId": testAccountID,
		"type":      "checking",
		"status":    "active",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Verify matching rules (at least the test rules)
	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	// Should have at least 4 matches (3 ALLOW + 1 DENY)
	assert.GreaterOrEqual(t, len(matchedRuleIDs), 4, "Should match at least 3 ALLOW + 1 DENY rules")

	// Verify DENY precedence
	assert.Equal(t, "DENY", result["decision"], "DENY should take precedence over ALLOW")

	// Verify reason mentions DENY
	reason, ok := result["reason"].(string)
	require.True(t, ok, "reason should be a string")
	reasonLower := strings.ToLower(reason)
	assert.True(t,
		strings.Contains(reasonLower, "deny") ||
			strings.Contains(reasonLower, "block") ||
			strings.Contains(reasonLower, "reject"),
		"reason should mention DENY/block/reject, got: %s", reason)

	// Verify all test rules evaluated (including non-matching one)
	evaluatedRuleIDs, ok := result["evaluatedRuleIds"].([]any)
	require.True(t, ok, "evaluatedRuleIds should be an array")
	// All 5 test rules should be evaluated
	allTestRules := append(allowRules, denyRule, noMatchRule)
	for _, ruleID := range allTestRules {
		assert.Contains(t, evaluatedRuleIDs, ruleID, "Test rule %s should be evaluated", ruleID)
	}
}

// TestValidation_CompleteEvaluation_CollectsEvaluatedRules verifies evaluatedRuleIds reflects scope filtering.
// Test 4.3.3 from roteiro 04-rules-evaluation.md
func TestValidation_CompleteEvaluation_CollectsEvaluatedRules(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(4003).String()

	// PRECONDITIONS: Create 3 rules scoped to accountID
	accountIDValue := accountID
	rule1 := testutil.CreateRuleWithScope(t, "Scoped Rule 1", "amount > 0", "ALLOW",
		[]testutil.ScopeInput{{AccountID: &accountIDValue}})
	testutil.ActivateRule(t, rule1)
	t.Cleanup(func() { testutil.CleanupRule(t, rule1) })

	rule2 := testutil.CreateRuleWithScope(t, "Scoped Rule 2", "amount > 0", "ALLOW",
		[]testutil.ScopeInput{{AccountID: &accountIDValue}})
	testutil.ActivateRule(t, rule2)
	t.Cleanup(func() { testutil.CleanupRule(t, rule2) })

	rule3 := testutil.CreateRuleWithScope(t, "Scoped Rule 3", "amount > 0", "ALLOW",
		[]testutil.ScopeInput{{AccountID: &accountIDValue}})
	testutil.ActivateRule(t, rule3)
	t.Cleanup(func() { testutil.CleanupRule(t, rule3) })

	// Create 1 rule scoped to different accountId (should be filtered)
	differentAccountID := testutil.MustDeterministicUUID(4004).String()
	differentAccountIDValue := differentAccountID
	rule4 := testutil.CreateRuleWithScope(t, "Different Account Rule", "amount > 0", "ALLOW",
		[]testutil.ScopeInput{{AccountID: &differentAccountIDValue}})
	testutil.ActivateRule(t, rule4)
	t.Cleanup(func() { testutil.CleanupRule(t, rule4) })

	// Create 1 rule scoped to PIX transaction type (should be filtered)
	pixType := "PIX"
	rule5 := testutil.CreateRuleWithScope(t, "PIX Rule", "amount > 0", "ALLOW",
		[]testutil.ScopeInput{{TransactionType: &pixType}})
	testutil.ActivateRule(t, rule5)
	t.Cleanup(func() { testutil.CleanupRule(t, rule5) })

	// EXECUTION: Send validation with accountID (matches rules 1-3, not 4-5)
	payload := testutil.CreateBasicValidationPayload()
	payload["account"] = map[string]any{
		"accountId": accountID,
		"type":      "checking",
		"status":    "active",
	}
	payload["transactionType"] = "CARD"
	payload["amount"] = "100.00"

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Scoped rules evaluated, others filtered
	evaluatedRuleIDs, ok := result["evaluatedRuleIds"].([]any)
	require.True(t, ok, "evaluatedRuleIds should be an array")
	// Verify at least our 3 scoped rules are evaluated (allow for global rules)
	assert.GreaterOrEqual(t, len(evaluatedRuleIDs), 3,
		"At least 3 scoped rules should be evaluated")
	assert.Contains(t, evaluatedRuleIDs, rule1, "Rule 1 should be evaluated")
	assert.Contains(t, evaluatedRuleIDs, rule2, "Rule 2 should be evaluated")
	assert.Contains(t, evaluatedRuleIDs, rule3, "Rule 3 should be evaluated")
	assert.NotContains(t, evaluatedRuleIDs, rule4, "Rule 4 filtered by accountId scope")
	assert.NotContains(t, evaluatedRuleIDs, rule5, "Rule 5 filtered by transactionType scope")
}

// TestValidation_CompleteEvaluation_DraftRulesNotEvaluated verifies DRAFT rules are not evaluated.
// Test 4.3.4 from roteiro 04-rules-evaluation.md
// Reference: API Design 6.4 RuleStatus
func TestValidation_CompleteEvaluation_DraftRulesNotEvaluated(t *testing.T) {
	// PRECONDITIONS: Create 2 ACTIVE rules
	activeRule1 := testutil.CreateTestRuleWithExpression(t, "Active Rule 1", `transactionType == "CARD"`, "ALLOW")
	testutil.ActivateRule(t, activeRule1)
	t.Cleanup(func() { testutil.CleanupRule(t, activeRule1) })

	activeRule2 := testutil.CreateTestRuleWithExpression(t, "Active Rule 2", "amount > 10", "ALLOW")
	testutil.ActivateRule(t, activeRule2)
	t.Cleanup(func() { testutil.CleanupRule(t, activeRule2) })

	// Create 1 DRAFT rule (should NOT be evaluated)
	draftRule := testutil.CreateTestRuleWithExpression(t, "Draft Rule", `currency == "BRL"`, "ALLOW")
	// Do NOT activate - keep in DRAFT status
	t.Cleanup(func() { testutil.CleanupRule(t, draftRule) })

	// EXECUTION: Send validation request
	payload := testutil.CreateBasicValidationPayload()
	payload["transactionType"] = "CARD"
	payload["amount"] = "100.00"
	payload["currency"] = "BRL" // Would match DRAFT rule if it were active

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Only ACTIVE rules evaluated
	evaluatedRuleIDs, ok := result["evaluatedRuleIds"].([]any)
	require.True(t, ok, "evaluatedRuleIds should be an array")
	assert.GreaterOrEqual(t, len(evaluatedRuleIDs), 2, "At least 2 ACTIVE rules should be evaluated")
	assert.Contains(t, evaluatedRuleIDs, activeRule1, "Active Rule 1 should be evaluated")
	assert.Contains(t, evaluatedRuleIDs, activeRule2, "Active Rule 2 should be evaluated")
	assert.NotContains(t, evaluatedRuleIDs, draftRule, "DRAFT rule should NOT be evaluated")

	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.NotContains(t, matchedRuleIDs, draftRule, "DRAFT rule should NOT be matched")
}

// TestValidation_CompleteEvaluation_InactiveRulesNotEvaluated verifies INACTIVE rules are not evaluated.
// Test 4.3.5 from roteiro 04-rules-evaluation.md
// Reference: API Design 6.4 RuleStatus
func TestValidation_CompleteEvaluation_InactiveRulesNotEvaluated(t *testing.T) {
	// PRECONDITIONS: Create 2 ACTIVE rules
	activeRule1 := testutil.CreateTestRuleWithExpression(t, "Active Rule 1", `transactionType == "CARD"`, "ALLOW")
	testutil.ActivateRule(t, activeRule1)
	t.Cleanup(func() { testutil.CleanupRule(t, activeRule1) })

	activeRule2 := testutil.CreateTestRuleWithExpression(t, "Active Rule 2", "amount > 10", "ALLOW")
	testutil.ActivateRule(t, activeRule2)
	t.Cleanup(func() { testutil.CleanupRule(t, activeRule2) })

	// Create and then DEACTIVATE 1 rule
	inactiveRule := testutil.CreateTestRuleWithExpression(t, "Inactive Rule", `currency == "BRL"`, "ALLOW")
	testutil.ActivateRule(t, inactiveRule)
	testutil.DeactivateRule(t, inactiveRule) // Deactivate it
	t.Cleanup(func() { testutil.CleanupRule(t, inactiveRule) })

	// EXECUTION: Send validation request
	payload := testutil.CreateBasicValidationPayload()
	payload["transactionType"] = "CARD"
	payload["amount"] = "100.00"
	payload["currency"] = "BRL" // Would match INACTIVE rule if it were active

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Only ACTIVE rules evaluated
	evaluatedRuleIDs, ok := result["evaluatedRuleIds"].([]any)
	require.True(t, ok, "evaluatedRuleIds should be an array")
	assert.GreaterOrEqual(t, len(evaluatedRuleIDs), 2, "At least 2 ACTIVE rules should be evaluated")
	assert.Contains(t, evaluatedRuleIDs, activeRule1, "Active Rule 1 should be evaluated")
	assert.Contains(t, evaluatedRuleIDs, activeRule2, "Active Rule 2 should be evaluated")
	assert.NotContains(t, evaluatedRuleIDs, inactiveRule, "INACTIVE rule should NOT be evaluated")

	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.NotContains(t, matchedRuleIDs, inactiveRule, "INACTIVE rule should NOT be matched")
}

// TestValidation_CompleteEvaluation_DeletedRulesNotEvaluated verifies DELETED rules are not evaluated.
// Test 4.3.6 from roteiro 04-rules-evaluation.md
// Reference: API Design 6.4 RuleStatus
func TestValidation_CompleteEvaluation_DeletedRulesNotEvaluated(t *testing.T) {
	// PRECONDITIONS: Create 2 ACTIVE rules
	activeRule1 := testutil.CreateTestRuleWithExpression(t, "Active Rule 1", `transactionType == "CARD"`, "ALLOW")
	testutil.ActivateRule(t, activeRule1)
	t.Cleanup(func() { testutil.CleanupRule(t, activeRule1) })

	activeRule2 := testutil.CreateTestRuleWithExpression(t, "Active Rule 2", "amount > 10", "ALLOW")
	testutil.ActivateRule(t, activeRule2)
	t.Cleanup(func() { testutil.CleanupRule(t, activeRule2) })

	// Create, activate, deactivate, and DELETE 1 rule
	deletedRule := testutil.CreateTestRuleWithExpression(t, "Deleted Rule", `currency == "BRL"`, "ALLOW")
	testutil.ActivateRule(t, deletedRule)
	testutil.DeleteRuleViaAPI(t, deletedRule) // This deactivates and deletes

	// EXECUTION: Send validation request
	payload := testutil.CreateBasicValidationPayload()
	payload["transactionType"] = "CARD"
	payload["amount"] = "100.00"
	payload["currency"] = "BRL" // Would match DELETED rule if it existed

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Only ACTIVE rules evaluated
	evaluatedRuleIDs, ok := result["evaluatedRuleIds"].([]any)
	require.True(t, ok, "evaluatedRuleIds should be an array")
	assert.GreaterOrEqual(t, len(evaluatedRuleIDs), 2, "At least 2 ACTIVE rules should be evaluated")
	assert.Contains(t, evaluatedRuleIDs, activeRule1, "Active Rule 1 should be evaluated")
	assert.Contains(t, evaluatedRuleIDs, activeRule2, "Active Rule 2 should be evaluated")
	assert.NotContains(t, evaluatedRuleIDs, deletedRule, "DELETED rule should NOT be evaluated")

	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.NotContains(t, matchedRuleIDs, deletedRule, "DELETED rule should NOT be matched")
}

// TestValidation_ResponseStructure_ValidationIdIsServerGenerated verifies validationId is server-generated UUID.
// Test 4.5.1 from roteiro 04-rules-evaluation.md
// Reference: API Design 4.1.1 ValidationResponse, Change Log 1.3.2
func TestValidation_ResponseStructure_ValidationIdIsServerGenerated(t *testing.T) {
	requestID := testutil.MustDeterministicUUID(4005).String()

	// EXECUTION 1: First validation
	payload1 := testutil.CreateBasicValidationPayload()
	payload1["requestId"] = requestID

	result1, status1 := testutil.ExecuteValidationRequest(t, payload1)
	require.Equal(t, http.StatusCreated, status1)

	// VALIDATIONS for first call
	validationID1, ok := result1["validationId"].(string)
	require.True(t, ok, "validationId must be string")

	// Verify validationId is valid UUID format
	_, err := uuid.Parse(validationID1)
	require.NoError(t, err, "validationId must be valid UUID format")

	// Verify validationId is DIFFERENT from requestId
	assert.NotEqual(t, requestID, validationID1,
		"validationId should be server-generated, not equal to requestId")

	// EXECUTION 2: Second validation with different requestId
	payload2 := testutil.CreateBasicValidationPayload()
	payload2["requestId"] = testutil.MustDeterministicUUID(4006).String()

	result2, status2 := testutil.ExecuteValidationRequest(t, payload2)
	require.Equal(t, http.StatusCreated, status2)

	validationID2, ok := result2["validationId"].(string)
	require.True(t, ok, "validationId must be string")

	// Verify each validation gets unique validationId
	assert.NotEqual(t, validationID1, validationID2,
		"Each validation should get unique server-generated validationId")
}

// TestValidation_ResponseStructure_RequestIdEchoed verifies requestId is echoed from input.
// Test 4.5.2 from roteiro 04-rules-evaluation.md
// Reference: API Design 4.1.1 ValidationResponse, Change Log 1.3.2
func TestValidation_ResponseStructure_RequestIdEchoed(t *testing.T) {
	requestID := "550e8400-e29b-41d4-a716-446655440010"

	// EXECUTION: Send validation with specific requestId
	payload := testutil.CreateBasicValidationPayload()
	payload["requestId"] = requestID

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: requestId echoed exactly
	responseRequestID, ok := result["requestId"].(string)
	require.True(t, ok, "requestId must be string")
	assert.Equal(t, requestID, responseRequestID,
		"requestId in response must exactly match input (not modified)")
}

// TestValidation_ResponseStructure_ProcessingTimeMsAlwaysPresent verifies processingTimeMs is always present and positive.
// Test 4.5.3 from roteiro 04-rules-evaluation.md
// Reference: API Design 4.1.1 ValidationResponse
func TestValidation_ResponseStructure_ProcessingTimeMsAlwaysPresent(t *testing.T) {
	// EXECUTION: Send validation request
	payload := testutil.CreateBasicValidationPayload()

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS
	processingTimeMs, ok := result["processingTimeMs"].(float64)
	require.True(t, ok, "processingTimeMs must be numeric type")

	assert.Greater(t, processingTimeMs, float64(0), "processingTimeMs must be positive")
	assert.Less(t, processingTimeMs, float64(1000),
		"processingTimeMs should be reasonable (< 1000ms for normal operation)")
}

// TestValidation_ResponseStructure_ReasonField_AllDecisions verifies reason field for all decision types.
// Test 4.5.4 from roteiro 04-rules-evaluation.md
// Reference: API Design 4.1.1 ValidationResponse
func TestValidation_ResponseStructure_ReasonField_AllDecisions(t *testing.T) {
	// PRECONDITIONS: Create rules for different decisions
	allowRuleID := testutil.CreateTestRuleWithExpression(t, "Allow Rule", "amount < 1000", "ALLOW")
	testutil.ActivateRule(t, allowRuleID)
	t.Cleanup(func() { testutil.CleanupRule(t, allowRuleID) })

	denyRuleID := testutil.CreateTestRuleWithExpression(t, "Deny Rule", "amount > 5000", "DENY")
	testutil.ActivateRule(t, denyRuleID)
	t.Cleanup(func() { testutil.CleanupRule(t, denyRuleID) })

	reviewRuleID := testutil.CreateTestRuleWithExpression(t, "Review Rule",
		"amount > 1000 && amount <= 5000", "REVIEW")
	testutil.ActivateRule(t, reviewRuleID)
	t.Cleanup(func() { testutil.CleanupRule(t, reviewRuleID) })

	testCases := []struct {
		name             string
		amount           string
		expectedDecision string
		expectedKeywords []string
	}{
		{
			name:             "allow_decision",
			amount:           "500.00",
			expectedDecision: "ALLOW",
			expectedKeywords: []string{"allow", "approved", "permitted", "rule matched"},
		},
		{
			name:             "deny_decision",
			amount:           "6000.00",
			expectedDecision: "DENY",
			expectedKeywords: []string{"deny", "blocked", "denied", "exceeded", "rule matched"},
		},
		{
			name:             "review_decision",
			amount:           "3000.00",
			expectedDecision: "REVIEW",
			expectedKeywords: []string{"review", "manual", "flagged"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := testutil.CreateBasicValidationPayload()
			payload["amount"] = tc.amount

			result, status := testutil.ExecuteValidationRequest(t, payload)
			require.Equal(t, http.StatusCreated, status)

			// Validate decision
			assert.Equal(t, tc.expectedDecision, result["decision"])

			// Validate reason
			reason, ok := result["reason"].(string)
			require.True(t, ok, "reason must be string")
			assert.NotEmpty(t, reason)
			assert.GreaterOrEqual(t, len(reason), 10, "reason should be descriptive")

			// Check keywords (case-insensitive)
			reasonLower := strings.ToLower(reason)
			foundKeyword := false
			for _, keyword := range tc.expectedKeywords {
				if strings.Contains(reasonLower, keyword) {
					foundKeyword = true
					break
				}
			}
			assert.True(t, foundKeyword,
				"reason should contain one of %v, got: %s", tc.expectedKeywords, reason)
		})
	}
}

// TestValidation_ResponseStructure_LimitUsageDetails_EmptyArray verifies limitUsageDetails is empty array when no limits.
// Test 4.5.5 from roteiro 04-rules-evaluation.md
// Reference: API Design 4.1.1 LimitUsage Structure
func TestValidation_ResponseStructure_LimitUsageDetails_EmptyArray(t *testing.T) {
	// PRECONDITIONS: No active limits (or ensure account has no matching limits)

	// EXECUTION: Send validation request
	payload := testutil.CreateBasicValidationPayload()
	// Use unique account to avoid any existing limits
	payload["account"] = map[string]any{
		"accountId": testutil.MustDeterministicUUID(4007).String(),
		"type":      "checking",
		"status":    "active",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS
	limitUsageDetails, ok := result["limitUsageDetails"].([]any)
	require.True(t, ok, "limitUsageDetails must be array")
	assert.Empty(t, limitUsageDetails, "limitUsageDetails should be empty array when no limits active")
}

// TestValidation_ResponseStructure_LimitUsageDetails_Populated verifies limitUsageDetails structure when limits are active.
// Test 4.5.6 from roteiro 04-rules-evaluation.md
// Reference: API Design 4.1.1 LimitUsage Structure
func TestValidation_ResponseStructure_LimitUsageDetails_Populated(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(4008).String()

	// PRECONDITIONS: Create and activate 2 limits
	dailyLimitID := testutil.CreateLimitWithAccountScopeAndType(t, accountID, "5000", "DAILY")
	testutil.ActivateLimit(t, dailyLimitID)
	t.Cleanup(func() { testutil.CleanupLimit(t, dailyLimitID) })

	perTxnLimitID := testutil.CreateLimitWithAccountScopeAndType(t, accountID, "1000", "PER_TRANSACTION")
	testutil.ActivateLimit(t, perTxnLimitID)
	t.Cleanup(func() { testutil.CleanupLimit(t, perTxnLimitID) })

	// EXECUTION: Send validation request
	payload := testutil.CreateBasicValidationPayload()
	payload["account"] = map[string]any{
		"accountId": accountID,
		"type":      "checking",
		"status":    "active",
	}
	payload["amount"] = "500.00"

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS
	limitUsageDetails, ok := result["limitUsageDetails"].([]any)
	require.True(t, ok, "limitUsageDetails must be array")
	assert.GreaterOrEqual(t, len(limitUsageDetails), 2,
		"limitUsageDetails should contain at least 2 elements (both limits)")

	// Validate each limit usage detail has required fields
	for i, detail := range limitUsageDetails {
		detailMap, ok := detail.(map[string]any)
		require.True(t, ok, "Each limit usage detail must be object")

		// Required fields
		limitID, ok := detailMap["limitId"].(string)
		require.True(t, ok, "limitId must be string")
		_, err := uuid.Parse(limitID)
		assert.NoError(t, err, "limitId must be valid UUID")

		scope, ok := detailMap["scope"].(string)
		require.True(t, ok, "scope must be string")
		assert.NotEmpty(t, scope, "scope must be non-empty")

		period, ok := detailMap["period"].(string)
		require.True(t, ok, "period must be string")
		assert.Contains(t, []string{"DAILY", "MONTHLY", "PER_TRANSACTION"}, period,
			"period must be valid enum value")

		// limitAmount is a decimal.Decimal serialized as JSON string
		limitAmountStr, ok := detailMap["limitAmount"].(string)
		require.True(t, ok, "limitAmount must be string (decimal serialization)")
		limitAmount, err := decimal.NewFromString(limitAmountStr)
		require.NoError(t, err, "limitAmount must be a valid decimal string")
		assert.True(t, limitAmount.GreaterThan(decimal.Zero), "limitAmount must be positive")

		currentUsageStr, ok := detailMap["currentUsage"].(string)
		require.True(t, ok, "currentUsage must be string (decimal serialization)")
		currentUsage, err := decimal.NewFromString(currentUsageStr)
		require.NoError(t, err, "currentUsage must be a valid decimal string")
		assert.True(t, currentUsage.GreaterThanOrEqual(decimal.Zero), "currentUsage must be >= 0")

		attemptedAmountStr, ok := detailMap["attemptedAmount"].(string)
		require.True(t, ok, "attemptedAmount must be string (decimal serialization)")
		attemptedAmount, err := decimal.NewFromString(attemptedAmountStr)
		require.NoError(t, err, "attemptedAmount must be a valid decimal string")
		assert.True(t, attemptedAmount.Equal(decimal.RequireFromString("500")),
			"attemptedAmount should equal request amount")

		exceeded, ok := detailMap["exceeded"].(bool)
		require.True(t, ok, "exceeded must be boolean")

		t.Logf("Limit %d: limitId=%s, period=%s, limitAmount=%s, currentUsage=%s, exceeded=%v",
			i, limitID, period, limitAmount, currentUsage, exceeded)
	}
}
