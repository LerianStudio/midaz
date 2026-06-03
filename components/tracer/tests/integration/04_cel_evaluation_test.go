// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/internal/testutil"
)

// =============================================================================
// CEL Expression Evaluation Tests - POST /v1/validations
// =============================================================================
//
// These tests verify the CEL engine can evaluate expressions against validation requests.
//
// Tests from roteiro section 4.1:
//   - 4.1.1: TransactionType comparison
//   - 4.1.2 + 4.1.2b: SubType (match + no-match scenarios)
//   - 4.1.3: Amount (table-driven com 4 boundary cases)
//   - 4.1.4: Currency comparison
//   - 4.1.5 + 4.1.5b: Segment context (dot + bracket notation)
//   - 4.1.6: Merchant context opcional (size() function)
//   - 4.1.7: Account context fields
//   - 4.1.8: Metadata bracket notation
//   - 4.1.9: Complex combined expression
//
// Reference: API Design 5.1 Validation Flow Integration
// =============================================================================

// TestValidation_CEL_TransactionType verifies CEL can evaluate transactionType field.
// Test 4.1.1 from roteiro 04-rules-evaluation.md
// Reference: API Design 6.2 TransactionType
func TestValidation_CEL_TransactionType(t *testing.T) {
	// PRECONDITIONS: Create and activate rule
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"TransactionType Check Rule",
		"transactionType == 'CARD'",
		"ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation request
	payload := map[string]any{
		"requestId":            testutil.MustDeterministicUUID(4201).String(),
		"transactionType":      "CARD",
		"amount":               100,
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": testutil.MustDeterministicUUID(4202).String(),
			"type":      "checking",
			"status":    "active",
		},
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)

	// VALIDATIONS
	require.Equal(t, http.StatusCreated, status)

	// Validate decision
	assert.Equal(t, "ALLOW", result["decision"])

	// Validate matchedRuleIds
	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds must be array")
	assert.NotEmpty(t, matchedRuleIDs, "At least one rule should match")
	assert.Contains(t, matchedRuleIDs, ruleID)

	// Validate evaluatedRuleIds
	evaluatedRuleIDs, ok := result["evaluatedRuleIds"].([]any)
	require.True(t, ok, "evaluatedRuleIds must be array")
	assert.Contains(t, evaluatedRuleIDs, ruleID)

	// Validate reason and processingTimeMs
	assert.NotEmpty(t, result["reason"])
	assert.Greater(t, result["processingTimeMs"], float64(0))
}

// TestValidation_CEL_SubType_Match verifies CEL can evaluate subType field (match scenario).
// Test 4.1.2 from roteiro 04-rules-evaluation.md
func TestValidation_CEL_SubType_Match(t *testing.T) {
	// PRECONDITIONS: Create and activate rule
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"SubType Check Rule",
		"subType == 'credit'",
		"ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation request with matching subType
	payload := testutil.CreateBasicValidationPayload()
	payload["subType"] = "credit"

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS
	assert.Equal(t, "ALLOW", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)
	assert.NotEmpty(t, result["reason"])
	assert.Greater(t, result["processingTimeMs"], float64(0))
}

// TestValidation_CEL_SubType_NoMatch verifies CEL evaluates but doesn't match when subType differs.
// Test 4.1.2b from roteiro 04-rules-evaluation.md
func TestValidation_CEL_SubType_NoMatch(t *testing.T) {
	// PRECONDITIONS: Create and activate rule
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"SubType Check Rule",
		"subType == 'credit'",
		"DENY")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation request with non-matching subType
	payload := testutil.CreateBasicValidationPayload()
	payload["subType"] = "debit"

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Rule evaluated but NOT matched
	testutil.AssertRuleEvaluatedButNotMatched(t, result, ruleID)
}

// TestValidation_CEL_Amount_Comparison verifies CEL can evaluate amount with comparison operators.
// Test 4.1.3 from roteiro 04-rules-evaluation.md - Table-driven test with 4 boundary scenarios
func TestValidation_CEL_Amount_Comparison(t *testing.T) {
	// PRECONDITIONS: Create rule: amount > 100000
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Amount Threshold",
		"amount > 100000",
		"DENY")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	testCases := []struct {
		name        string
		amount      int64
		shouldMatch bool
		description string
	}{
		{"above_threshold", 150000, true, "amount > 100000 is true"},
		{"boundary_above", 100001, true, "just above threshold"},
		{"boundary_equal", 100000, false, "amount > 100000 is false (not inclusive)"},
		{"below_threshold", 50000, false, "amount > 100000 is false"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := testutil.CreateBasicValidationPayload()
			payload["amount"] = tc.amount

			result, status := testutil.ExecuteValidationRequest(t, payload)
			require.Equal(t, http.StatusCreated, status)

			// Rule always evaluated
			evaluatedRuleIDs, ok := result["evaluatedRuleIds"].([]any)
			require.True(t, ok, "evaluatedRuleIds should be an array")
			assert.Contains(t, evaluatedRuleIDs, ruleID, "Rule always evaluated")

			matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
			require.True(t, ok, "matchedRuleIds should be an array")
			if tc.shouldMatch {
				assert.Contains(t, matchedRuleIDs, ruleID, tc.description)
				assert.Equal(t, "DENY", result["decision"])
			} else {
				assert.NotContains(t, matchedRuleIDs, ruleID, tc.description)
			}
		})
	}
}

// TestValidation_CEL_Currency verifies CEL can evaluate currency field.
// Test 4.1.4 from roteiro 04-rules-evaluation.md
func TestValidation_CEL_Currency(t *testing.T) {
	// PRECONDITIONS: Create and activate rule
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Currency Check Rule",
		"currency == 'BRL'",
		"ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation request
	payload := testutil.CreateBasicValidationPayload()
	payload["currency"] = "BRL"

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS
	assert.Equal(t, "ALLOW", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)
	assert.NotEmpty(t, result["reason"])
	assert.Greater(t, result["processingTimeMs"], float64(0))
}

// TestValidation_CEL_SegmentContext_NameField verifies CEL can access segment name field with bracket notation.
// Test 4.1.5 from roteiro 04-rules-evaluation.md
// Reference: API Design 6.12 SegmentContext
// Note: CEL implementation uses bracket notation and size() for optional fields
func TestValidation_CEL_SegmentContext_NameField(t *testing.T) {
	segmentID := testutil.MustDeterministicUUID(4203).String()

	// PRECONDITIONS: Create rule with segment["name"] check
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Segment Name Check",
		`size(segment) > 0 && segment["name"] == "premium"`,
		"ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation request with segment context
	payload := testutil.CreateBasicValidationPayload()
	payload["segment"] = map[string]any{
		"segmentId": segmentID,
		"name":      "premium",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS
	assert.Equal(t, "ALLOW", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)
}

// TestValidation_CEL_SegmentContext_BracketNotation verifies CEL can access segment with bracket notation.
// Test 4.1.5b from roteiro 04-rules-evaluation.md
func TestValidation_CEL_SegmentContext_BracketNotation(t *testing.T) {
	segmentID := testutil.MustDeterministicUUID(4204).String()

	// PRECONDITIONS: Create rule with segment["segmentId"] check
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Segment ID Bracket Check",
		fmt.Sprintf(`size(segment) > 0 && segment["segmentId"] == "%s"`, segmentID),
		"ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation request with segment context
	payload := testutil.CreateBasicValidationPayload()
	payload["segment"] = map[string]any{
		"segmentId": segmentID,
		"name":      "premium",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS
	assert.Equal(t, "ALLOW", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)
}

// TestValidation_CEL_MerchantContext_Optional verifies CEL handles optional merchant context with size().
// Test 4.1.6 from roteiro 04-rules-evaluation.md
// Reference: API Design 6.13 MerchantContext
func TestValidation_CEL_MerchantContext_Optional(t *testing.T) {
	// PRECONDITIONS: Create rule that checks for merchant category
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Merchant Category Check",
		`size(merchant) > 0 && merchant["category"] == "5411"`,
		"REVIEW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation request WITH merchant context
	payload := testutil.CreateBasicValidationPayload()
	payload["merchant"] = map[string]any{
		"merchantId": testutil.MustDeterministicUUID(4205).String(),
		"category":   "5411",
		"country":    "BR",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Rule matches when merchant present
	assert.Equal(t, "REVIEW", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)

	// EXECUTION 2: Send validation request WITHOUT merchant context
	payload2 := testutil.CreateBasicValidationPayload()
	// No merchant field

	result2, status2 := testutil.ExecuteValidationRequest(t, payload2)
	require.Equal(t, http.StatusCreated, status2)

	// VALIDATIONS: Rule evaluated but NOT matched (size(merchant) == 0)
	testutil.AssertRuleEvaluatedButNotMatched(t, result2, ruleID)
}

// TestValidation_CEL_AccountContext verifies CEL can access account context fields.
// Test 4.1.7 from roteiro 04-rules-evaluation.md
// Reference: API Design 6.11 AccountContext
func TestValidation_CEL_AccountContext(t *testing.T) {
	// PRECONDITIONS: Create rule checking account["status"]
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Account Status Check",
		`account["status"] == "active" && account["type"] == "checking"`,
		"ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation request
	payload := testutil.CreateBasicValidationPayload()
	payload["account"] = map[string]any{
		"accountId": testutil.MustDeterministicUUID(4206).String(),
		"type":      "checking",
		"status":    "active",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS
	assert.Equal(t, "ALLOW", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)
}

// TestValidation_CEL_Metadata_BracketNotation verifies CEL can access metadata with bracket notation.
// Test 4.1.8 from roteiro 04-rules-evaluation.md
func TestValidation_CEL_Metadata_BracketNotation(t *testing.T) {
	// PRECONDITIONS: Create rule checking metadata["riskScore"]
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Metadata Risk Score Check",
		`size(metadata) > 0 && metadata["riskScore"] > 80`,
		"DENY")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation request with metadata
	payload := testutil.CreateBasicValidationPayload()
	payload["metadata"] = map[string]any{
		"riskScore":  85,
		"ipAddress":  "192.168.1.1",
		"deviceType": "mobile",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS
	assert.Equal(t, "DENY", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)
}

// TestValidation_CEL_ComplexCombinedExpression verifies CEL can evaluate complex combined expressions.
// Test 4.1.9 from roteiro 04-rules-evaluation.md
func TestValidation_CEL_ComplexCombinedExpression(t *testing.T) {
	// PRECONDITIONS: Create rule with complex expression
	complexExpr := `transactionType == "CARD" && amount > 50000 && ` +
		`currency == "BRL" && size(segment) > 0 && segment["name"] == "premium" && ` +
		`account["status"] == "active"`

	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Complex Combined Rule",
		complexExpr,
		"ALLOW")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION: Send validation request matching all conditions
	payload := testutil.CreateBasicValidationPayload()
	payload["transactionType"] = "CARD"
	payload["amount"] = 75000
	payload["currency"] = "BRL"
	payload["segment"] = map[string]any{
		"segmentId": testutil.MustDeterministicUUID(4207).String(),
		"name":      "premium",
	}
	payload["account"] = map[string]any{
		"accountId": testutil.MustDeterministicUUID(4208).String(),
		"type":      "checking",
		"status":    "active",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS
	assert.Equal(t, "ALLOW", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)

	// EXECUTION 2: Send request with one condition failing (amount too low)
	payload2 := testutil.CreateBasicValidationPayload()
	payload2["transactionType"] = "CARD"
	payload2["amount"] = 30000 // Below 50000 threshold
	payload2["currency"] = "BRL"
	payload2["segment"] = map[string]any{
		"segmentId": testutil.MustDeterministicUUID(4209).String(),
		"name":      "premium",
	}
	payload2["account"] = map[string]any{
		"accountId": testutil.MustDeterministicUUID(4210).String(),
		"type":      "checking",
		"status":    "active",
	}

	result2, status2 := testutil.ExecuteValidationRequest(t, payload2)
	require.Equal(t, http.StatusCreated, status2)

	// VALIDATIONS: Rule evaluated but NOT matched (only amount condition fails)
	testutil.AssertRuleEvaluatedButNotMatched(t, result2, ruleID)
}
