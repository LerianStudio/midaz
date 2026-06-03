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

	"tracer/internal/testutil"
)

// =============================================================================
// CEL Missing Key Resilience Tests - POST /v1/validations
// =============================================================================
//
// Regression coverage for the production bug observed in STG:
//   An ACTIVE rule with `metadata["channel"] == "mobile"` (or similar map
//   lookup) caused HTTP 500 ("validation processing failed") whenever
//   the validation request did NOT carry the referenced key in its metadata
//   payload.
//
// Fix: missing-key runtime errors from cel-go are now treated as non-match for
// that single rule. Other rules continue to be evaluated and the configured
// default-when-no-match decision applies.
// =============================================================================

// TestValidation_CEL_MissingMetadataKey_DoesNotFail covers the four expected
// outcomes for a rule referencing a metadata key that may or may not be
// present in the request.
func TestValidation_CEL_MissingMetadataKey_DoesNotFail(t *testing.T) {
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Missing Metadata Key Rule",
		`metadata["channel"] == "mobile"`,
		"DENY")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	t.Run("metadata absent - rule does not match, no 500", func(t *testing.T) {
		payload := testutil.CreateBasicValidationPayload()
		delete(payload, "metadata")

		result, status := testutil.ExecuteValidationRequest(t, payload)

		require.Equal(t, http.StatusCreated, status,
			"missing metadata key must not produce HTTP 500 (regression for catch-all error path)")
		assert.NotEqual(t, "DENY", result["decision"],
			"rule must not be considered matched when its referenced key is absent")

		evaluatedRuleIDs, ok := result["evaluatedRuleIds"].([]any)
		require.True(t, ok, "evaluatedRuleIds should be an array")
		assert.Contains(t, evaluatedRuleIDs, ruleID,
			"rule must still appear in evaluatedRuleIds (loop completed)")

		matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
		require.True(t, ok, "matchedRuleIds should be an array")
		assert.NotContains(t, matchedRuleIDs, ruleID,
			"rule must NOT appear in matchedRuleIds when key missing")
	})

	t.Run("metadata present with no channel key - rule does not match", func(t *testing.T) {
		payload := testutil.CreateBasicValidationPayload()
		payload["metadata"] = map[string]any{
			"runId":  "integration-test",
			"caseId": "case-007",
			// channel intentionally absent
		}

		result, status := testutil.ExecuteValidationRequest(t, payload)

		require.Equal(t, http.StatusCreated, status)
		testutil.AssertRuleEvaluatedButNotMatched(t, result, ruleID)
	})

	t.Run("channel = mobile - rule matches and DENIES", func(t *testing.T) {
		payload := testutil.CreateBasicValidationPayload()
		payload["metadata"] = map[string]any{
			"channel": "mobile",
		}

		result, status := testutil.ExecuteValidationRequest(t, payload)

		require.Equal(t, http.StatusCreated, status)
		assert.Equal(t, "DENY", result["decision"])
		testutil.AssertRuleMatched(t, result, ruleID)
	})

	t.Run("channel = web - rule does not match", func(t *testing.T) {
		payload := testutil.CreateBasicValidationPayload()
		payload["metadata"] = map[string]any{
			"channel": "web",
		}

		result, status := testutil.ExecuteValidationRequest(t, payload)

		require.Equal(t, http.StatusCreated, status)
		testutil.AssertRuleEvaluatedButNotMatched(t, result, ruleID)
	})
}

// TestValidation_CEL_MissingKey_DoesNotAbortOtherRules verifies that a rule
// referencing a missing key does not short-circuit the rule evaluation loop —
// other coexisting rules must still be evaluated and applied.
func TestValidation_CEL_MissingKey_DoesNotAbortOtherRules(t *testing.T) {
	// Rule A: missing-key reference (would have aborted the loop pre-fix)
	missingKeyRuleID := testutil.CreateTestRuleWithExpression(t,
		"Missing Key Rule (Co-existing)",
		`metadata["channel"] == "mobile"`,
		"DENY")
	testutil.ActivateRule(t, missingKeyRuleID)
	t.Cleanup(func() { testutil.CleanupRule(t, missingKeyRuleID) })

	// Rule B: valid amount-based rule that should ALWAYS be evaluated
	amountRuleID := testutil.CreateTestRuleWithExpression(t,
		"Amount Threshold Co-existing",
		"amount > 50",
		"DENY")
	testutil.ActivateRule(t, amountRuleID)
	t.Cleanup(func() { testutil.CleanupRule(t, amountRuleID) })

	payload := testutil.CreateBasicValidationPayload()
	payload["amount"] = 1000 // above 50, will trigger amount rule
	delete(payload, "metadata")

	result, status := testutil.ExecuteValidationRequest(t, payload)

	require.Equal(t, http.StatusCreated, status,
		"missing-key rule must not abort the evaluation loop")

	evaluatedRuleIDs, ok := result["evaluatedRuleIds"].([]any)
	require.True(t, ok, "evaluatedRuleIds should be an array")
	assert.Contains(t, evaluatedRuleIDs, missingKeyRuleID,
		"missing-key rule must still be tracked as evaluated")
	assert.Contains(t, evaluatedRuleIDs, amountRuleID,
		"co-existing amount rule must still be evaluated despite missing-key sibling")

	matchedRuleIDs, ok := result["matchedRuleIds"].([]any)
	require.True(t, ok, "matchedRuleIds should be an array")
	assert.Contains(t, matchedRuleIDs, amountRuleID,
		"amount rule must match independently of missing-key sibling")
	assert.NotContains(t, matchedRuleIDs, missingKeyRuleID,
		"missing-key rule must NOT be in matchedRuleIds")

	assert.Equal(t, "DENY", result["decision"],
		"decision must follow the matched amount rule")
}

// TestValidation_CEL_MetadataKeyPresent_StillEvaluatesCorrectly is the happy-
// path counterpart: when the referenced key IS present, behaviour is
// unchanged (no regression on existing matching logic).
func TestValidation_CEL_MetadataKeyPresent_StillEvaluatesCorrectly(t *testing.T) {
	ruleID := testutil.CreateTestRuleWithExpression(t,
		"Present Metadata Key Rule",
		`metadata["caseId"] == "case-001"`,
		"DENY")
	testutil.ActivateRule(t, ruleID)
	t.Cleanup(func() { testutil.CleanupRule(t, ruleID) })

	payload := testutil.CreateBasicValidationPayload()
	payload["metadata"] = map[string]any{"caseId": "case-001"}

	result, status := testutil.ExecuteValidationRequest(t, payload)

	require.Equal(t, http.StatusCreated, status)
	assert.Equal(t, "DENY", result["decision"])
	testutil.AssertRuleMatched(t, result, ruleID)
}
