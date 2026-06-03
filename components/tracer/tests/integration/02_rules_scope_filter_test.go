// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// =============================================================================
// GET /v1/rules - Scope-Based Filtering Tests (2.4.x)
// =============================================================================

// TestListRules_2_4_1_FilterByAccountId verifies filtering rules by accountId scope.
// Rules with matching accountId and global rules (empty scopes) should be returned.
func TestListRules_2_4_1_FilterByAccountId(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	accountID := "550e8400-e29b-41d4-a716-446655440a01"
	otherAccountID := "660e8400-e29b-41d4-a716-446655440a01"

	ruleWithScope := testutil.CreateRuleWithScope(t, "ScopeFilter 2_4_1 matching account", "true", "DENY",
		[]testutil.ScopeInput{{AccountID: &accountID}})
	ruleOtherScope := testutil.CreateRuleWithScope(t, "ScopeFilter 2_4_1 other account", "true", "DENY",
		[]testutil.ScopeInput{{AccountID: &otherAccountID}})
	ruleGlobal := testutil.CreateTestRuleWithExpression(t, "ScopeFilter 2_4_1 global rule", "true", "DENY")

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleWithScope)
		testutil.CleanupRule(t, ruleOtherScope)
		testutil.CleanupRule(t, ruleGlobal)
	})

	url := fmt.Sprintf("%s/v1/rules?account_id=%s", baseURL, accountID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	rules, ok := result["rules"].([]any)
	require.True(t, ok, "rules should be an array")

	ruleIDs := extractRuleIDsFromList(t, rules)
	assert.Contains(t, ruleIDs, ruleWithScope, "Should contain rule with matching account_id scope")
	assert.Contains(t, ruleIDs, ruleGlobal, "Should contain global rule (no scopes)")
	assert.NotContains(t, ruleIDs, ruleOtherScope, "Should NOT contain rule with different account_id scope")
}

// TestListRules_2_4_2_FilterByTransactionType verifies filtering rules by transactionType scope.
func TestListRules_2_4_2_FilterByTransactionType(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	txType := "CARD"
	otherTxType := "PIX"

	ruleCard := testutil.CreateRuleWithScope(t, "ScopeFilter 2_4_2 card rule", "true", "ALLOW",
		[]testutil.ScopeInput{{TransactionType: &txType}})
	rulePix := testutil.CreateRuleWithScope(t, "ScopeFilter 2_4_2 pix rule", "true", "ALLOW",
		[]testutil.ScopeInput{{TransactionType: &otherTxType}})
	ruleGlobal := testutil.CreateTestRuleWithExpression(t, "ScopeFilter 2_4_2 global", "true", "ALLOW")

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleCard)
		testutil.CleanupRule(t, rulePix)
		testutil.CleanupRule(t, ruleGlobal)
	})

	url := fmt.Sprintf("%s/v1/rules?transaction_type=%s", baseURL, txType)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	rules, ok := result["rules"].([]any)
	require.True(t, ok)

	ruleIDs := extractRuleIDsFromList(t, rules)
	assert.Contains(t, ruleIDs, ruleCard, "Should contain rule with CARD transaction_type")
	assert.Contains(t, ruleIDs, ruleGlobal, "Should contain global rule")
	assert.NotContains(t, ruleIDs, rulePix, "Should NOT contain rule with PIX transaction_type")
}

// TestListRules_2_4_3_FilterByMultipleScopeFields verifies filtering by combining multiple scope fields.
// Rules with wildcard fields should match when the non-null fields match.
func TestListRules_2_4_3_FilterByMultipleScopeFields(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	accountID := "550e8400-e29b-41d4-a716-446655440a03"
	txType := "WIRE"

	ruleBoth := testutil.CreateRuleWithScope(t, "ScopeFilter 2_4_3 both fields", "true", "REVIEW",
		[]testutil.ScopeInput{{AccountID: &accountID, TransactionType: &txType}})
	ruleAccountOnly := testutil.CreateRuleWithScope(t, "ScopeFilter 2_4_3 account only", "true", "REVIEW",
		[]testutil.ScopeInput{{AccountID: &accountID}})
	ruleTxTypeOnly := testutil.CreateRuleWithScope(t, "ScopeFilter 2_4_3 txtype only", "true", "REVIEW",
		[]testutil.ScopeInput{{TransactionType: &txType}})

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleBoth)
		testutil.CleanupRule(t, ruleAccountOnly)
		testutil.CleanupRule(t, ruleTxTypeOnly)
	})

	url := fmt.Sprintf("%s/v1/rules?account_id=%s&transaction_type=%s", baseURL, accountID, txType)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	rules, ok := result["rules"].([]any)
	require.True(t, ok)

	ruleIDs := extractRuleIDsFromList(t, rules)
	assert.Contains(t, ruleIDs, ruleBoth, "Should contain rule with both fields matching")
	assert.Contains(t, ruleIDs, ruleAccountOnly, "Should contain rule with wildcard transaction_type")
	assert.Contains(t, ruleIDs, ruleTxTypeOnly, "Should contain rule with wildcard account_id")
}

// TestListRules_2_4_4_ScopeFiltersWithExistingFilters verifies scope filters work alongside action filter.
func TestListRules_2_4_4_ScopeFiltersWithExistingFilters(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	accountID := "550e8400-e29b-41d4-a716-446655440a04"

	ruleDeny := testutil.CreateRuleWithScope(t, "ScopeFilter 2_4_4 deny", "true", "DENY",
		[]testutil.ScopeInput{{AccountID: &accountID}})
	ruleAllow := testutil.CreateRuleWithScope(t, "ScopeFilter 2_4_4 allow", "true", "ALLOW",
		[]testutil.ScopeInput{{AccountID: &accountID}})

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleDeny)
		testutil.CleanupRule(t, ruleAllow)
	})

	url := fmt.Sprintf("%s/v1/rules?account_id=%s&action=DENY", baseURL, accountID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	rules, ok := result["rules"].([]any)
	require.True(t, ok)

	for _, r := range rules {
		rule, ok := r.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "DENY", rule["action"], "All returned rules should have action=DENY")
	}

	ruleIDs := extractRuleIDsFromList(t, rules)
	assert.Contains(t, ruleIDs, ruleDeny, "Should contain DENY rule with matching scope")
	assert.NotContains(t, ruleIDs, ruleAllow, "Should NOT contain ALLOW rule")
}

// TestListRules_2_4_5_ScopeFilterWithPagination verifies scope filters work with cursor-based pagination.
func TestListRules_2_4_5_ScopeFilterWithPagination(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	segmentID := "550e8400-e29b-41d4-a716-446655440a05"

	createdIDs := make([]string, 4)
	for i := 0; i < 4; i++ {
		createdIDs[i] = testutil.CreateRuleWithScope(t,
			fmt.Sprintf("ScopeFilter 2_4_5 pagination %d", i+1), "true", "REVIEW",
			[]testutil.ScopeInput{{SegmentID: &segmentID}})
	}

	t.Cleanup(func() {
		for _, id := range createdIDs {
			testutil.CleanupRule(t, id)
		}
	})

	// First page: limit=2
	url := fmt.Sprintf("%s/v1/rules?segment_id=%s&limit=2", baseURL, segmentID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result1 map[string]any
	err = json.Unmarshal(respBody, &result1)
	require.NoError(t, err)

	rules1, ok := result1["rules"].([]any)
	require.True(t, ok)
	assert.Len(t, rules1, 2, "First page should have 2 rules")
	hasMore, ok := result1["hasMore"].(bool)
	require.True(t, ok, "hasMore should be a boolean")
	assert.True(t, hasMore, "Should have more results")

	cursor, ok := result1["nextCursor"].(string)
	require.True(t, ok, "nextCursor should be a string")
	require.NotEmpty(t, cursor, "Should have nextCursor")

	// Second page using cursor
	url2 := fmt.Sprintf("%s/v1/rules?segment_id=%s&limit=2&cursor=%s", baseURL, segmentID, cursor)
	req2, err := http.NewRequest(http.MethodGet, url2, nil)
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", apiKey)

	resp2, err := testutil.HTTPClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode, "Response: %s", string(respBody2))

	var result2 map[string]any
	err = json.Unmarshal(respBody2, &result2)
	require.NoError(t, err)

	rules2, ok := result2["rules"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, rules2, "Second page should have results")

	// Verify no duplicates between pages
	page1IDs := extractRuleIDsFromList(t, rules1)
	page2IDs := extractRuleIDsFromList(t, rules2)
	for _, id := range page1IDs {
		assert.NotContains(t, page2IDs, id, "No duplicate rules between pages")
	}
}

// TestListRules_2_4_6_InvalidScopeFiltersReturnError verifies validation errors for invalid scope params.
func TestListRules_2_4_6_InvalidScopeFiltersReturnError(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	testCases := []struct {
		name        string
		queryParams string
		expectCode  string
		expectMsg   string
	}{
		{
			name:        "invalid account_id UUID",
			queryParams: "account_id=not-a-uuid",
			expectCode:  "TRC-0006",
			expectMsg:   "account_id",
		},
		{
			name:        "invalid segment_id UUID",
			queryParams: "segment_id=invalid",
			expectCode:  "TRC-0006",
			expectMsg:   "segment_id",
		},
		{
			name:        "invalid portfolio_id UUID",
			queryParams: "portfolio_id=bad-uuid",
			expectCode:  "TRC-0006",
			expectMsg:   "portfolio_id",
		},
		{
			name:        "invalid merchant_id UUID",
			queryParams: "merchant_id=xyz",
			expectCode:  "TRC-0006",
			expectMsg:   "merchant_id",
		},
		{
			name:        "invalid transaction_type enum",
			queryParams: "transaction_type=INVALID_TYPE",
			expectCode:  "TRC-0006",
			expectMsg:   "transaction_type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := fmt.Sprintf("%s/v1/rules?%s", baseURL, tc.queryParams)
			req, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := testutil.HTTPClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response: %s", string(respBody))

			errResp := testutil.ParseErrorResponse(t, respBody)
			assert.Equal(t, tc.expectCode, errResp.Code, "Error code mismatch")
			assert.Contains(t, errResp.Message, tc.expectMsg, "Error message should mention the invalid field")
		})
	}
}

// TestListRules_2_4_7_NoScopeFiltersReturnsAllRules verifies backward compatibility.
// When no scope filters are provided, all rules are returned regardless of their scopes.
func TestListRules_2_4_7_NoScopeFiltersReturnsAllRules(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	accountID := "550e8400-e29b-41d4-a716-446655440a07"

	ruleScopedDeny := testutil.CreateRuleWithScope(t, "ScopeFilter 2_4_7 scoped", "true", "DENY",
		[]testutil.ScopeInput{{AccountID: &accountID}})
	ruleGlobalDeny := testutil.CreateTestRuleWithExpression(t, "ScopeFilter 2_4_7 global", "true", "DENY")

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleScopedDeny)
		testutil.CleanupRule(t, ruleGlobalDeny)
	})

	// No scope filters - should return both
	url := fmt.Sprintf("%s/v1/rules", baseURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	rules, ok := result["rules"].([]any)
	require.True(t, ok)

	ruleIDs := extractRuleIDsFromList(t, rules)
	assert.Contains(t, ruleIDs, ruleScopedDeny, "Should contain scoped rule when no scope filter")
	assert.Contains(t, ruleIDs, ruleGlobalDeny, "Should contain global rule when no scope filter")
}

// TestListRules_2_4_8_FilterBySegmentId verifies filtering by segmentId scope.
func TestListRules_2_4_8_FilterBySegmentId(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	segmentID := "550e8400-e29b-41d4-a716-446655440a08"
	otherSegmentID := "660e8400-e29b-41d4-a716-446655440a08"

	ruleMatch := testutil.CreateRuleWithScope(t, "ScopeFilter 2_4_8 segment match", "true", "REVIEW",
		[]testutil.ScopeInput{{SegmentID: &segmentID}})
	ruleNoMatch := testutil.CreateRuleWithScope(t, "ScopeFilter 2_4_8 segment no match", "true", "REVIEW",
		[]testutil.ScopeInput{{SegmentID: &otherSegmentID}})

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleMatch)
		testutil.CleanupRule(t, ruleNoMatch)
	})

	url := fmt.Sprintf("%s/v1/rules?segment_id=%s", baseURL, segmentID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result map[string]any
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	rules, ok := result["rules"].([]any)
	require.True(t, ok)

	ruleIDs := extractRuleIDsFromList(t, rules)
	assert.Contains(t, ruleIDs, ruleMatch, "Should contain rule with matching segment_id")
	assert.NotContains(t, ruleIDs, ruleNoMatch, "Should NOT contain rule with different segment_id")
}

// extractRuleIDsFromList extracts rule IDs from a list of rule maps.
func extractRuleIDsFromList(t *testing.T, rules []any) []string {
	t.Helper()

	var ids []string

	for _, r := range rules {
		rule, ok := r.(map[string]any)
		require.True(t, ok, "rule should be a map")
		id, ok := rule["ruleId"].(string)
		require.True(t, ok, "ruleId should be a string")
		ids = append(ids, id)
	}

	return ids
}
