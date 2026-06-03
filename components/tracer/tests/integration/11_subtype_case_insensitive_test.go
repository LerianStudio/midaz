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
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// =============================================================================
// SubType case-insensitive matching (T-2)
//
// Contract under test:
//
//  1. On write, SubType is normalized to its canonical form: trimmed + lowercased.
//     Persisted value and GET response therefore both show the lowercase form.
//  2. On read / evaluation, matching is case-insensitive (ptrMatchesFold), so a
//     rule or limit stored with subType="sell" matches a validation request
//     whose SubType is "SELL", "Sell", or "sell".
//
// Note: storing lowercased/trimmed values is a documented breaking change
// from the pre-T-2 behavior where casing/whitespace were preserved verbatim.
//
// Both tests cover the full lifecycle: create -> GET (assert canonical form)
// -> activate -> /v1/validations with mixed casing (assert match / no-match).
// =============================================================================

// TestRuleSubTypeCaseInsensitive_Integration verifies that rules created with
// uppercase subType input are stored in canonical lowercase form and match
// validation requests regardless of casing.
func TestRuleSubTypeCaseInsensitive_Integration(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	accountID := testutil.MustDeterministicUUID(110200).String()

	// ----- Step 1: POST /v1/rules with uppercase subType ------------------
	uppercaseSubType := "SELL"
	reqBody := map[string]any{
		"name":       "rule-subtype-case-" + testutil.MustDeterministicUUID(110210).String()[:8],
		"expression": "amount > 0",
		"action":     "ALLOW",
		"scopes": []map[string]any{
			{
				"accountId": accountID,
				"subType":   uppercaseSubType,
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createRespBody, err := io.ReadAll(createResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode,
		"create-rule setup failed: %s", string(createRespBody))

	var createResult map[string]any
	require.NoError(t, json.Unmarshal(createRespBody, &createResult))

	ruleID, ok := createResult["ruleId"].(string)
	require.True(t, ok, "ruleId must be a string")
	require.NotEmpty(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// ----- Step 2: GET /v1/rules/{id} and assert canonical (lowercase) form.
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules/"+ruleID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode, "GET rule failed: %s", string(getBody))

	var fetched map[string]any
	require.NoError(t, json.Unmarshal(getBody, &fetched))

	scopes, ok := fetched["scopes"].([]any)
	require.True(t, ok, "scopes should be an array")
	require.Len(t, scopes, 1, "exactly one scope expected")

	scope0, ok := scopes[0].(map[string]any)
	require.True(t, ok, "scope[0] should be an object")

	storedSubType, ok := scope0["subType"].(string)
	require.True(t, ok, "scope[0].subType should be a string")
	assert.Equal(t, "sell", storedSubType,
		"subType must be persisted in canonical (lowercase) form; input %q must be normalized to %q",
		uppercaseSubType, "sell")

	// ----- Step 3: activate the rule so it participates in evaluation -----
	testutil.ActivateRule(t, ruleID)

	// ----- Step 4: validate with mixed-case subType, expect rule to match.
	matchPayload := testutil.CreateBasicValidationPayload()
	matchPayload["subType"] = "Sell"
	matchPayload["account"] = map[string]any{
		"accountId": accountID,
		"type":      "checking",
		"status":    "active",
	}

	matchResult, matchStatus := testutil.ExecuteValidationRequest(t, matchPayload)
	require.Equal(t, http.StatusCreated, matchStatus,
		"validation with matching (mixed-case) subType must succeed: %v", matchResult)
	testutil.AssertRuleMatched(t, matchResult, ruleID)

	// ----- Step 5: validate with a different subType, expect rule filtered.
	noMatchPayload := testutil.CreateBasicValidationPayload()
	noMatchPayload["subType"] = "BUY"
	noMatchPayload["account"] = map[string]any{
		"accountId": accountID,
		"type":      "checking",
		"status":    "active",
	}

	noMatchResult, noMatchStatus := testutil.ExecuteValidationRequest(t, noMatchPayload)
	require.Equal(t, http.StatusCreated, noMatchStatus,
		"validation with non-matching subType must still return 201: %v", noMatchResult)
	testutil.AssertRuleNotEvaluated(t, noMatchResult, ruleID)
}

// TestLimitSubTypeCaseInsensitive_Integration verifies that limits created with
// whitespace-wrapped, mixed-case subType input are stored trimmed + lowercased
// and enforced against validation requests regardless of casing.
func TestLimitSubTypeCaseInsensitive_Integration(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	accountID := testutil.MustDeterministicUUID(110300).String()

	// ----- Step 1: POST /v1/limits with "  Buy  " as subType --------------
	rawSubType := "  Buy  "
	limitName := "limit-subtype-case-" + testutil.MustDeterministicUUID(110310).String()[:8]

	limitID := testutil.CreateLimitWithScope(
		t,
		limitName,
		"1000",
		[]testutil.ScopeInput{{AccountID: &accountID, SubType: &rawSubType}},
	)
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// ----- Step 2: GET /v1/limits/{id} and assert canonical form ---------
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	getBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode, "GET limit failed: %s", string(getBody))

	// Reuse limitResponse defined in 03_limits_management_test.go (same package).
	var fetched limitResponse
	require.NoError(t, json.Unmarshal(getBody, &fetched))
	require.Len(t, fetched.Scopes, 1, "exactly one scope expected")
	require.NotNil(t, fetched.Scopes[0].SubType, "scope[0].subType must be present")
	assert.Equal(t, "buy", *fetched.Scopes[0].SubType,
		"subType must be persisted in canonical (trimmed + lowercase) form; input %q must be normalized to %q",
		rawSubType, "buy")

	// ----- Step 3: activate the limit so it is enforced -------------------
	testutil.ActivateLimit(t, limitID)

	// ----- Step 4: validate with uppercase subType, expect limit enforced.
	matchingSubType := "BUY"
	matchingReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(110310).String(),
		TransactionType:      "CARD",
		SubType:              matchingSubType,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	matchingResp, matchingBody := testutil.CreateValidation(t, matchingReq)
	defer matchingResp.Body.Close()
	require.Equal(t, http.StatusCreated, matchingResp.StatusCode,
		"validation with matching (uppercase) subType must succeed: %s", string(matchingBody))

	var matchingResult testutil.ValidationResponse
	require.NoError(t, json.Unmarshal(matchingBody, &matchingResult))

	foundForMatching := false

	for _, detail := range matchingResult.LimitUsageDetails {
		if detail.LimitID == limitID {
			foundForMatching = true

			break
		}
	}

	assert.True(t, foundForMatching,
		"limit with subType=%q must match validation with subType=%q (case-insensitive)",
		"buy", matchingSubType)

	// ----- Step 5: validate with "sell", expect limit NOT enforced --------
	otherSubType := "sell"
	otherReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(110311).String(),
		TransactionType:      "CARD",
		SubType:              otherSubType,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID: accountID,
		},
	}

	otherResp, otherBody := testutil.CreateValidation(t, otherReq)
	defer otherResp.Body.Close()
	require.Equal(t, http.StatusCreated, otherResp.StatusCode,
		"validation with non-matching subType must still return 201: %s", string(otherBody))

	var otherResult testutil.ValidationResponse
	require.NoError(t, json.Unmarshal(otherBody, &otherResult))

	for _, detail := range otherResult.LimitUsageDetails {
		assert.NotEqual(t, limitID, detail.LimitID,
			"limit scoped to subType=%q must NOT be enforced on subType=%q",
			"buy", otherSubType)
	}
}
