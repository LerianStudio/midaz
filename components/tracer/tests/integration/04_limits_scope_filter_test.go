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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// =============================================================================
// GET /v1/limits - Name and Scope-Based Filtering Tests (4.x)
// =============================================================================

// TestListLimits_4_1_FilterByName verifies filtering limits by name (case-insensitive partial match).
func TestListLimits_4_1_FilterByName(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	accountID := "550e8400-e29b-41d4-a716-446655440b01"

	limitMatch := testutil.CreateLimitWithScope(t, "ScopeFilter 4_1 Daily Spending", "1000",
		[]testutil.ScopeInput{{AccountID: &accountID}})
	limitNoMatch := testutil.CreateLimitWithScope(t, "ScopeFilter 4_1 Monthly Budget", "5000",
		[]testutil.ScopeInput{{AccountID: &accountID}})

	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitMatch)
		testutil.CleanupLimit(t, limitNoMatch)
	})

	// Filter by "ScopeFilter 4_1 Daily" (case-insensitive, specific prefix avoids false positives)
	url := fmt.Sprintf("%s/v1/limits?name=ScopeFilter+4_1+Daily", baseURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result listLimitsResponse
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	limitIDs := extractLimitIDsFromList(t, result.Limits)
	assert.Contains(t, limitIDs, limitMatch, "Should contain limit with 'Daily Spending' in name")
	assert.NotContains(t, limitIDs, limitNoMatch, "Should NOT contain 'Monthly Budget' limit")
}

// TestListLimits_4_2_FilterByAccountId verifies filtering limits by accountId scope.
func TestListLimits_4_2_FilterByAccountId(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	accountID := "550e8400-e29b-41d4-a716-446655440b02"
	otherAccountID := "660e8400-e29b-41d4-a716-446655440b02"

	limitMatch := testutil.CreateLimitWithScope(t, "ScopeFilter 4_2 matching account", "1000",
		[]testutil.ScopeInput{{AccountID: &accountID}})
	limitNoMatch := testutil.CreateLimitWithScope(t, "ScopeFilter 4_2 other account", "1000",
		[]testutil.ScopeInput{{AccountID: &otherAccountID}})

	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitMatch)
		testutil.CleanupLimit(t, limitNoMatch)
	})

	url := fmt.Sprintf("%s/v1/limits?account_id=%s", baseURL, accountID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result listLimitsResponse
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	limitIDs := extractLimitIDsFromList(t, result.Limits)
	assert.Contains(t, limitIDs, limitMatch, "Should contain limit with matching account_id scope")
	assert.NotContains(t, limitIDs, limitNoMatch, "Should NOT contain limit with different account_id scope")
}

// TestListLimits_4_3_FilterByTransactionType verifies filtering limits by transactionType scope.
func TestListLimits_4_3_FilterByTransactionType(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	txType := "CARD"
	otherTxType := "PIX"

	limitCard := testutil.CreateLimitWithScope(t, "ScopeFilter 4_3 card limit", "1000",
		[]testutil.ScopeInput{{TransactionType: &txType}})
	limitPix := testutil.CreateLimitWithScope(t, "ScopeFilter 4_3 pix limit", "2000",
		[]testutil.ScopeInput{{TransactionType: &otherTxType}})

	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitCard)
		testutil.CleanupLimit(t, limitPix)
	})

	url := fmt.Sprintf("%s/v1/limits?transaction_type=%s", baseURL, txType)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result listLimitsResponse
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	limitIDs := extractLimitIDsFromList(t, result.Limits)
	assert.Contains(t, limitIDs, limitCard, "Should contain limit with CARD transaction_type")
	assert.NotContains(t, limitIDs, limitPix, "Should NOT contain limit with PIX transaction_type")
}

// TestListLimits_4_4_FilterByMultipleScopeFields verifies combining multiple scope fields.
func TestListLimits_4_4_FilterByMultipleScopeFields(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	accountID := "550e8400-e29b-41d4-a716-446655440b04"
	txType := "WIRE"

	limitBoth := testutil.CreateLimitWithScope(t, "ScopeFilter 4_4 both fields", "1000",
		[]testutil.ScopeInput{{AccountID: &accountID, TransactionType: &txType}})
	limitAccountOnly := testutil.CreateLimitWithScope(t, "ScopeFilter 4_4 account only", "2000",
		[]testutil.ScopeInput{{AccountID: &accountID}})
	limitTxTypeOnly := testutil.CreateLimitWithScope(t, "ScopeFilter 4_4 txtype only", "3000",
		[]testutil.ScopeInput{{TransactionType: &txType}})

	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitBoth)
		testutil.CleanupLimit(t, limitAccountOnly)
		testutil.CleanupLimit(t, limitTxTypeOnly)
	})

	url := fmt.Sprintf("%s/v1/limits?account_id=%s&transaction_type=%s", baseURL, accountID, txType)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result listLimitsResponse
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	limitIDs := extractLimitIDsFromList(t, result.Limits)
	assert.Contains(t, limitIDs, limitBoth, "Should contain limit with both fields matching")
	assert.Contains(t, limitIDs, limitAccountOnly, "Should contain limit with wildcard transaction_type")
	assert.Contains(t, limitIDs, limitTxTypeOnly, "Should contain limit with wildcard account_id")
}

// TestListLimits_4_5_ScopeFiltersWithExistingFilters verifies scope filters work alongside status filter.
func TestListLimits_4_5_ScopeFiltersWithExistingFilters(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	accountID := "550e8400-e29b-41d4-a716-446655440b05"

	// Create two limits - both DRAFT by default
	limitDraft := testutil.CreateLimitWithScope(t, "ScopeFilter 4_5 draft", "1000",
		[]testutil.ScopeInput{{AccountID: &accountID}})
	limitActive := testutil.CreateLimitWithScope(t, "ScopeFilter 4_5 active", "2000",
		[]testutil.ScopeInput{{AccountID: &accountID}})
	// Activate one
	testutil.ActivateLimit(t, limitActive)

	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitDraft)
		testutil.CleanupLimit(t, limitActive)
	})

	// Filter by scope AND status=ACTIVE
	url := fmt.Sprintf("%s/v1/limits?account_id=%s&status=ACTIVE", baseURL, accountID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result listLimitsResponse
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	limitIDs := extractLimitIDsFromList(t, result.Limits)
	assert.Contains(t, limitIDs, limitActive, "Should contain ACTIVE limit with matching scope")
	assert.NotContains(t, limitIDs, limitDraft, "Should NOT contain DRAFT limit")
}

// TestListLimits_4_6_InvalidScopeFiltersReturnError verifies validation errors for invalid scope params.
func TestListLimits_4_6_InvalidScopeFiltersReturnError(t *testing.T) {
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
		{
			name:        "sub_type exceeds max length",
			queryParams: "sub_type=" + strings.Repeat("x", 51),
			expectCode:  "TRC-0006",
			expectMsg:   "sub_type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := fmt.Sprintf("%s/v1/limits?%s", baseURL, tc.queryParams)
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

// TestListLimits_4_7_NoScopeFiltersReturnsAllLimits verifies backward compatibility.
func TestListLimits_4_7_NoScopeFiltersReturnsAllLimits(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	accountID := "550e8400-e29b-41d4-a716-446655440b07"

	limitScoped := testutil.CreateLimitWithScope(t, "ScopeFilter 4_7 scoped", "1000",
		[]testutil.ScopeInput{{AccountID: &accountID}})
	limitOther := testutil.CreateLimitWithScope(t, "ScopeFilter 4_7 other", "2000",
		[]testutil.ScopeInput{{AccountID: &accountID}})

	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitScoped)
		testutil.CleanupLimit(t, limitOther)
	})

	// No scope filters - should return both
	url := fmt.Sprintf("%s/v1/limits", baseURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result listLimitsResponse
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	limitIDs := extractLimitIDsFromList(t, result.Limits)
	assert.Contains(t, limitIDs, limitScoped, "Should contain scoped limit when no scope filter")
	assert.Contains(t, limitIDs, limitOther, "Should contain other limit when no scope filter")
}

// TestListLimits_4_8_FilterBySegmentId verifies filtering by segmentId scope.
func TestListLimits_4_8_FilterBySegmentId(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	segmentID := "550e8400-e29b-41d4-a716-446655440b08"
	otherSegmentID := "660e8400-e29b-41d4-a716-446655440b08"

	limitMatch := testutil.CreateLimitWithScope(t, "ScopeFilter 4_8 segment match", "1000",
		[]testutil.ScopeInput{{SegmentID: &segmentID}})
	limitNoMatch := testutil.CreateLimitWithScope(t, "ScopeFilter 4_8 segment no match", "2000",
		[]testutil.ScopeInput{{SegmentID: &otherSegmentID}})

	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitMatch)
		testutil.CleanupLimit(t, limitNoMatch)
	})

	url := fmt.Sprintf("%s/v1/limits?segment_id=%s", baseURL, segmentID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result listLimitsResponse
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	limitIDs := extractLimitIDsFromList(t, result.Limits)
	assert.Contains(t, limitIDs, limitMatch, "Should contain limit with matching segment_id")
	assert.NotContains(t, limitIDs, limitNoMatch, "Should NOT contain limit with different segment_id")
}

// TestListLimits_4_9_ScopeFilterWithPagination verifies scope filters work with cursor-based pagination.
func TestListLimits_4_9_ScopeFilterWithPagination(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	segmentID := "550e8400-e29b-41d4-a716-446655440b09"

	createdIDs := make([]string, 4)
	for i := 0; i < 4; i++ {
		createdIDs[i] = testutil.CreateLimitWithScope(t,
			fmt.Sprintf("ScopeFilter 4_9 pagination %d", i+1), "1000",
			[]testutil.ScopeInput{{SegmentID: &segmentID}})
	}

	t.Cleanup(func() {
		for _, id := range createdIDs {
			testutil.CleanupLimit(t, id)
		}
	})

	// First page: limit=2
	url := fmt.Sprintf("%s/v1/limits?segment_id=%s&limit=2", baseURL, segmentID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Response: %s", string(respBody))

	var result1 listLimitsResponse
	err = json.Unmarshal(respBody, &result1)
	require.NoError(t, err)

	assert.Len(t, result1.Limits, 2, "First page should have 2 limits")
	assert.True(t, result1.HasMore, "Should have more results")
	assert.NotEmpty(t, result1.NextCursor, "Should have nextCursor")

	// Second page using cursor
	url2 := fmt.Sprintf("%s/v1/limits?segment_id=%s&limit=2&cursor=%s", baseURL, segmentID, result1.NextCursor)
	req2, err := http.NewRequest(http.MethodGet, url2, nil)
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", apiKey)

	resp2, err := testutil.HTTPClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode, "Response: %s", string(respBody2))

	var result2 listLimitsResponse
	err = json.Unmarshal(respBody2, &result2)
	require.NoError(t, err)

	assert.Len(t, result2.Limits, 2, "Second page should have exactly 2 remaining limits")

	// Verify no duplicates between pages
	page1IDs := extractLimitIDsFromList(t, result1.Limits)
	page2IDs := extractLimitIDsFromList(t, result2.Limits)
	for _, id := range page1IDs {
		assert.NotContains(t, page2IDs, id, "No duplicate limits between pages")
	}
}

// extractLimitIDsFromList extracts limit IDs from a list of limit responses.
func extractLimitIDsFromList(t *testing.T, limits []limitResponse) []string {
	t.Helper()

	var ids []string
	for _, l := range limits {
		ids = append(ids, l.ID)
	}

	return ids
}
