// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// =============================================================================
// Scope Matching Tests - POST /v1/validations
// =============================================================================
//
// These tests verify scope filtering logic ensures rules are only evaluated
// when their scope criteria match the validation request.
//
// Tests from roteiro section 4.2:
//   - 4.2.1: Match by accountId (match + no-match scenarios)
//   - 4.2.2: Match by segmentId
//   - 4.2.3: Match by portfolioId
//   - 4.2.4: Match by merchantId
//   - 4.2.5: Match by transactionType
//   - 4.2.6: Match by subType
//   - 4.2.7: Multiple scope fields (AND logic)
//   - 4.2.8: Multiple scopes in rule (OR logic)
//   - 4.2.9: Empty scopes (global rule)
//
// Reference: API Design 5.1 Validation Flow Integration
// =============================================================================

// TestValidation_Scope_AccountId verifies rules are filtered by accountId scope.
// Test 4.2.1 from roteiro 04-rules-evaluation.md
func TestValidation_Scope_AccountId(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(4501).String()

	// PRECONDITIONS: Create rule scoped to specific accountId
	accountIDValue := accountID
	ruleID := testutil.CreateRuleWithScope(t,
		"Scoped by AccountId",
		"amount > 0",
		"ALLOW",
		[]testutil.ScopeInput{{AccountID: &accountIDValue}})
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION 1: Send validation request with MATCHING accountId
	payload := testutil.CreateBasicValidationPayload()
	payload["account"] = map[string]any{
		"accountId": accountID,
		"type":      "checking",
		"status":    "active",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Rule evaluated and matched
	assert.Equal(t, "ALLOW", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)

	// EXECUTION 2: Send validation request with DIFFERENT accountId
	payload2 := testutil.CreateBasicValidationPayload()
	payload2["account"] = map[string]any{
		"accountId": testutil.MustDeterministicUUID(4502).String(), // Different account
		"type":      "checking",
		"status":    "active",
	}

	result2, status2 := testutil.ExecuteValidationRequest(t, payload2)
	require.Equal(t, http.StatusCreated, status2)

	// VALIDATIONS: Rule NOT evaluated (filtered by scope)
	testutil.AssertRuleNotEvaluated(t, result2, ruleID)
}

// TestValidation_Scope_SegmentId verifies rules are filtered by segmentId scope.
// Test 4.2.2 from roteiro 04-rules-evaluation.md
func TestValidation_Scope_SegmentId(t *testing.T) {
	segmentID := testutil.MustDeterministicUUID(4503).String()

	// PRECONDITIONS: Create rule scoped to specific segmentId
	segmentIDValue := segmentID
	ruleID := testutil.CreateRuleWithScope(t,
		"Scoped by SegmentId",
		"amount > 0",
		"ALLOW",
		[]testutil.ScopeInput{{SegmentID: &segmentIDValue}})
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION 1: Send validation request with MATCHING segmentId
	payload := testutil.CreateBasicValidationPayload()
	payload["segment"] = map[string]any{
		"segmentId": segmentID,
		"name":      "premium",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Rule evaluated and matched
	assert.Equal(t, "ALLOW", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)

	// EXECUTION 2: Send validation request with DIFFERENT segmentId
	payload2 := testutil.CreateBasicValidationPayload()
	payload2["segment"] = map[string]any{
		"segmentId": testutil.MustDeterministicUUID(4504).String(), // Different segment
		"name":      "standard",
	}

	result2, status2 := testutil.ExecuteValidationRequest(t, payload2)
	require.Equal(t, http.StatusCreated, status2)

	// VALIDATIONS: Rule NOT evaluated (filtered by scope)
	testutil.AssertRuleNotEvaluated(t, result2, ruleID)
}

// TestValidation_Scope_PortfolioId verifies rules are filtered by portfolioId scope.
// Test 4.2.3 from roteiro 04-rules-evaluation.md
func TestValidation_Scope_PortfolioId(t *testing.T) {

	portfolioID := testutil.MustDeterministicUUID(4505).String()

	// PRECONDITIONS: Create rule scoped to specific portfolioId
	portfolioIDValue := portfolioID
	ruleID := testutil.CreateRuleWithScope(t,
		"Scoped by PortfolioId",
		"amount > 0",
		"ALLOW",
		[]testutil.ScopeInput{{PortfolioID: &portfolioIDValue}})
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION 1: Send validation request with MATCHING portfolioId
	payload := testutil.CreateBasicValidationPayload()
	payload["portfolio"] = map[string]any{
		"portfolioId": portfolioID,
		"name":        "corporate",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Rule evaluated and matched
	assert.Equal(t, "ALLOW", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)

	// EXECUTION 2: Send validation request with DIFFERENT portfolioId
	payload2 := testutil.CreateBasicValidationPayload()
	payload2["portfolio"] = map[string]any{
		"portfolioId": testutil.MustDeterministicUUID(4506).String(), // Different portfolio
		"name":        "retail",
	}

	result2, status2 := testutil.ExecuteValidationRequest(t, payload2)
	require.Equal(t, http.StatusCreated, status2)

	// VALIDATIONS: Rule NOT evaluated (filtered by scope)
	testutil.AssertRuleNotEvaluated(t, result2, ruleID)
}

// TestValidation_Scope_MerchantId verifies rules are filtered by merchantId scope.
// Test 4.2.4 from roteiro 04-rules-evaluation.md
func TestValidation_Scope_MerchantId(t *testing.T) {

	merchantID := testutil.MustDeterministicUUID(4507).String()

	// PRECONDITIONS: Create rule scoped to specific merchantId
	merchantIDValue := merchantID
	ruleID := testutil.CreateRuleWithScope(t,
		"Scoped by MerchantId",
		"amount > 0",
		"ALLOW",
		[]testutil.ScopeInput{{MerchantID: &merchantIDValue}})
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION 1: Send validation request with MATCHING merchantId
	payload := testutil.CreateBasicValidationPayload()
	payload["merchant"] = map[string]any{
		"merchantId": merchantID,
		"category":   "5411",
	}

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Rule evaluated and matched
	assert.Equal(t, "ALLOW", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)

	// EXECUTION 2: Send validation request with DIFFERENT merchantId
	payload2 := testutil.CreateBasicValidationPayload()
	payload2["merchant"] = map[string]any{
		"merchantId": testutil.MustDeterministicUUID(4508).String(), // Different merchant
		"category":   "5812",
	}

	result2, status2 := testutil.ExecuteValidationRequest(t, payload2)
	require.Equal(t, http.StatusCreated, status2)

	// VALIDATIONS: Rule NOT evaluated (filtered by scope)
	testutil.AssertRuleNotEvaluated(t, result2, ruleID)
}

// TestValidation_Scope_TransactionType verifies rules are filtered by transactionType scope.
// Test 4.2.5 from roteiro 04-rules-evaluation.md
func TestValidation_Scope_TransactionType(t *testing.T) {
	transactionType := "CARD"

	// PRECONDITIONS: Create rule scoped to CARD transaction type
	transactionTypeValue := transactionType
	ruleID := testutil.CreateRuleWithScope(t,
		"Scoped by TransactionType",
		"amount > 0",
		"ALLOW",
		[]testutil.ScopeInput{{TransactionType: &transactionTypeValue}})
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION 1: Send validation request with MATCHING transactionType
	payload := testutil.CreateBasicValidationPayload()
	payload["transactionType"] = "CARD"

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Rule evaluated and matched
	assert.Equal(t, "ALLOW", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)

	// EXECUTION 2: Send validation request with DIFFERENT transactionType
	payload2 := testutil.CreateBasicValidationPayload()
	payload2["transactionType"] = "PIX"

	result2, status2 := testutil.ExecuteValidationRequest(t, payload2)
	require.Equal(t, http.StatusCreated, status2)

	// VALIDATIONS: Rule NOT evaluated (filtered by scope)
	testutil.AssertRuleNotEvaluated(t, result2, ruleID)
}

// TestValidation_Scope_SubType verifies rules are filtered by subType scope.
// Test 4.2.6 from roteiro 04-rules-evaluation.md
func TestValidation_Scope_SubType(t *testing.T) {

	subType := "credit"

	// PRECONDITIONS: Create rule scoped to credit subType
	subTypeValue := subType
	ruleID := testutil.CreateRuleWithScope(t,
		"Scoped by SubType",
		"amount > 0",
		"ALLOW",
		[]testutil.ScopeInput{{SubType: &subTypeValue}})
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	// EXECUTION 1: Send validation request with MATCHING subType
	payload := testutil.CreateBasicValidationPayload()
	payload["subType"] = "credit"

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Equal(t, http.StatusCreated, status)

	// VALIDATIONS: Rule evaluated and matched
	assert.Equal(t, "ALLOW", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)

	// EXECUTION 2: Send validation request with DIFFERENT subType
	payload2 := testutil.CreateBasicValidationPayload()
	payload2["subType"] = "debit"

	result2, status2 := testutil.ExecuteValidationRequest(t, payload2)
	require.Equal(t, http.StatusCreated, status2)

	// VALIDATIONS: Rule NOT evaluated (filtered by scope)
	testutil.AssertRuleNotEvaluated(t, result2, ruleID)
}

// TestValidation_Scope_MultipleScopeFields verifies AND logic for multiple fields in one scope.
// Test 4.2.7 from roteiro 04-rules-evaluation.md
func TestValidation_Scope_MultipleScopeFields(t *testing.T) {

	accountID := testutil.MustDeterministicUUID(4509).String()

	// PRECONDITIONS: Create rule with scope containing both accountId AND transactionType
	accountIDValue := accountID
	transactionTypeValue := "CARD"
	ruleID := testutil.CreateRuleWithScope(t,
		"Scoped by AccountId AND TransactionType",
		"amount > 0",
		"ALLOW",
		[]testutil.ScopeInput{{
			AccountID:       &accountIDValue,
			TransactionType: &transactionTypeValue,
		}})
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	testCases := []struct {
		name              string
		accountID         string
		transactionType   string
		shouldBeEvaluated bool
		description       string
	}{
		{
			name:              "both_match",
			accountID:         accountID,
			transactionType:   "CARD",
			shouldBeEvaluated: true,
			description:       "Both accountId and transactionType match scope",
		},
		{
			name:              "account_matches_type_differs",
			accountID:         accountID,
			transactionType:   "PIX",
			shouldBeEvaluated: false,
			description:       "accountId matches but transactionType differs (AND logic)",
		},
		{
			name:              "type_matches_account_differs",
			accountID:         testutil.MustDeterministicUUID(4510).String(),
			transactionType:   "CARD",
			shouldBeEvaluated: false,
			description:       "transactionType matches but accountId differs (AND logic)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := testutil.CreateBasicValidationPayload()
			payload["account"] = map[string]any{
				"accountId": tc.accountID,
				"type":      "checking",
				"status":    "active",
			}
			payload["transactionType"] = tc.transactionType

			result, status := testutil.ExecuteValidationRequest(t, payload)
			require.Equal(t, http.StatusCreated, status)

			if tc.shouldBeEvaluated {
				testutil.AssertRuleMatched(t, result, ruleID)
				assert.Equal(t, "ALLOW", result["decision"], tc.description)
			} else {
				testutil.AssertRuleNotEvaluated(t, result, ruleID)
			}
		})
	}
}

// TestValidation_Scope_MultipleScopes verifies OR logic for multiple scopes in one rule.
// Test 4.2.8 from roteiro 04-rules-evaluation.md
func TestValidation_Scope_MultipleScopes(t *testing.T) {

	accountID1 := testutil.MustDeterministicUUID(4511).String()
	accountID2 := testutil.MustDeterministicUUID(4512).String()

	// PRECONDITIONS: Create rule with 3 scopes (OR logic)
	accountID1Value := accountID1
	accountID2Value := accountID2
	transactionTypeValue := "PIX"
	ruleID := testutil.CreateRuleWithScope(t,
		"Multiple Scopes OR Logic",
		"amount > 0",
		"ALLOW",
		[]testutil.ScopeInput{
			{AccountID: &accountID1Value},
			{AccountID: &accountID2Value},
			{TransactionType: &transactionTypeValue},
		})
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	testCases := []struct {
		name              string
		accountID         string
		transactionType   string
		shouldBeEvaluated bool
		description       string
	}{
		{
			name:              "matches_first_scope",
			accountID:         accountID1,
			transactionType:   "CARD",
			shouldBeEvaluated: true,
			description:       "Matches first scope (accountId1)",
		},
		{
			name:              "matches_second_scope",
			accountID:         accountID2,
			transactionType:   "CARD",
			shouldBeEvaluated: true,
			description:       "Matches second scope (accountId2)",
		},
		{
			name:              "matches_third_scope",
			accountID:         testutil.MustDeterministicUUID(4513).String(),
			transactionType:   "PIX",
			shouldBeEvaluated: true,
			description:       "Matches third scope (transactionType PIX)",
		},
		{
			name:              "matches_none",
			accountID:         testutil.MustDeterministicUUID(4514).String(),
			transactionType:   "CARD",
			shouldBeEvaluated: false,
			description:       "Matches no scope",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := testutil.CreateBasicValidationPayload()
			payload["account"] = map[string]any{
				"accountId": tc.accountID,
				"type":      "checking",
				"status":    "active",
			}
			payload["transactionType"] = tc.transactionType

			result, status := testutil.ExecuteValidationRequest(t, payload)
			require.Equal(t, http.StatusCreated, status)

			if tc.shouldBeEvaluated {
				testutil.AssertRuleMatched(t, result, ruleID)
				assert.Equal(t, "ALLOW", result["decision"], tc.description)
			} else {
				testutil.AssertRuleNotEvaluated(t, result, ruleID)
			}
		})
	}
}

// TestValidation_Scope_EmptyScopes verifies rules with empty scopes are evaluated for all transactions.
// Test 4.2.9 from roteiro 04-rules-evaluation.md
func TestValidation_Scope_EmptyScopes(t *testing.T) {
	// PRECONDITIONS: Create rule with empty scopes (global rule)
	// Use amount >= 5000 to avoid interfering with other tests that use smaller amounts
	ruleID := testutil.CreateRuleWithScope(t,
		"Global Rule (Empty Scopes)",
		"amount >= 5000",
		"ALLOW",
		[]testutil.ScopeInput{})
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	testCases := []struct {
		name            string
		accountID       string
		transactionType string
		description     string
	}{
		{
			name:            "scenario_1_card_account1",
			accountID:       testutil.MustDeterministicUUID(4515).String(),
			transactionType: "CARD",
			description:     "CARD transaction with accountId 1",
		},
		{
			name:            "scenario_2_pix_account2",
			accountID:       testutil.MustDeterministicUUID(4516).String(),
			transactionType: "PIX",
			description:     "PIX transaction with accountId 2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := testutil.CreateBasicValidationPayload()
			payload["account"] = map[string]any{
				"accountId": tc.accountID,
				"type":      "checking",
				"status":    "active",
			}
			payload["transactionType"] = tc.transactionType
			payload["amount"] = "5000.00" // Always > 0 to match expression

			result, status := testutil.ExecuteValidationRequest(t, payload)
			require.Equal(t, http.StatusCreated, status)

			// VALIDATIONS: Global rule always evaluated and matched
			testutil.AssertRuleMatched(t, result, ruleID)
			assert.Equal(t, "ALLOW", result["decision"], tc.description)
		})
	}
}
