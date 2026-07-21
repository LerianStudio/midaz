// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// resetAuditEvents clears all audit events for hash chain integrity tests.
// Temporarily disables the SOX compliance rule that prevents deletion,
// deletes all events, and immediately re-enables the rule via defer.
// This minimizes the vulnerability window where the rule is disabled.
func resetAuditEvents(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.ExecContext(context.Background(), "ALTER TABLE audit_events DISABLE RULE prevent_audit_event_delete")
	require.NoError(t, err, "Failed to disable delete rule")

	// Re-enable rule immediately after this function completes (before returning to caller)
	// This ensures minimal window where the rule is disabled
	defer func() {
		if _, enableErr := db.ExecContext(context.Background(), "ALTER TABLE audit_events ENABLE RULE prevent_audit_event_delete"); enableErr != nil {
			t.Errorf("Failed to re-enable audit event delete rule: %v", enableErr)
		}
	}()

	_, err = db.ExecContext(context.Background(), "DELETE FROM audit_events")
	require.NoError(t, err, "Failed to clean audit_events for hash chain test")
}

// ============================================================================
// 11.1 GET /v1/audit-events/{id} - Get Audit Event
// ============================================================================

// TestAuditEvents_11_1_1_RetrievesAuditEventByID tests retrieving an audit event by its ID.
func TestAuditEvents_11_1_1_RetrievesAuditEventByID(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Precondition: Create a rule to generate RULE_CREATED audit event
	ruleName := "audit-test-rule-" + testutil.MustDeterministicUUID(7001).String()[:8]
	ruleReq := testutil.RuleRequest{
		Name:       ruleName,
		Expression: "amount > 10",
		Action:     "DENY",
	}
	ruleBody, err := json.Marshal(ruleReq)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(ruleBody))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var rule testutil.RuleResponse
	err = json.NewDecoder(resp.Body).Decode(&rule)
	require.NoError(t, err)
	require.NotEmpty(t, rule.ID)
	defer testutil.CleanupRule(t, rule.ID)

	// Wait briefly for audit event to be created
	time.Sleep(100 * time.Millisecond)

	// Query audit events to find the RULE_CREATED event
	listReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_type=rule&action=CREATE&resource_id="+rule.ID, nil)
	require.NoError(t, err)
	listReq.Header.Set("X-API-Key", apiKey)

	listResp, err := testutil.HTTPClient.Do(listReq)
	require.NoError(t, err)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var listResult struct {
		AuditEvents []struct {
			EventID string `json:"eventId"`
		} `json:"auditEvents"`
	}
	err = json.NewDecoder(listResp.Body).Decode(&listResult)
	require.NoError(t, err)
	require.NotEmpty(t, listResult.AuditEvents, "Should find audit event for rule creation")

	eventID := listResult.AuditEvents[0].EventID

	// Test: Get audit event by ID
	getReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/"+eventID, nil)
	require.NoError(t, err)
	getReq.Header.Set("X-API-Key", apiKey)

	getResp, err := testutil.HTTPClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	getBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)

	var event map[string]any
	err = json.Unmarshal(getBody, &event)
	require.NoError(t, err)

	// Validations
	assert.Equal(t, eventID, event["eventId"])
	assert.Equal(t, "RULE_CREATED", event["eventType"])
	assert.Equal(t, "CREATE", event["action"])
	assert.Equal(t, "SUCCESS", event["result"])
	assert.Equal(t, "rule", event["resourceType"])
	assert.NotEmpty(t, event["resourceId"])
	assert.NotNil(t, event["actor"])
	assert.NotEmpty(t, event["hash"])
	assert.NotEmpty(t, event["createdAt"])
}

// TestAuditEvents_11_1_2_Returns404ForNonExistentEvent tests 404 response for non-existent event ID.
func TestAuditEvents_11_1_2_Returns404ForNonExistentEvent(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	nonExistentID := "00000000-0000-0000-0000-000000000000"

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/"+nonExistentID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	errResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0381", errResp.Code, "Error code should be TRC-0140 for audit event not found")
	assert.Equal(t, "Not Found", errResp.Title, "Error title should be Not Found")
	assert.Equal(t, "Audit event not found", errResp.Message, "Error message should indicate audit event not found")
}

// TestAuditEvents_11_1_3_Returns400ForInvalidUUID tests 400 response for invalid UUID format.
func TestAuditEvents_11_1_3_Returns400ForInvalidUUID(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/invalid-uuid", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Verify structured error response
	errResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "0065", errResp.Code, "Error response should have invalid path parameter code")
	assert.Equal(t, "Invalid Path Parameter", errResp.Title, "Error response should have invalid path parameter title")
	assert.Equal(t, "Invalid event ID format", errResp.Message, "Error message should indicate invalid UUID format")
}

// TestAuditEvents_11_1_4_Returns401WithoutAPIKey tests 401 response without authentication.
func TestAuditEvents_11_1_4_Returns401WithoutAPIKey(t *testing.T) {
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/"+testutil.MustDeterministicUUID(7002).String(), nil)
	require.NoError(t, err)
	// No API Key header

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// ============================================================================
// 11.2 GET /v1/audit-events - List Audit Events
// ============================================================================

// TestAuditEvents_11_2_1_ListsAllWithDefaultParams tests listing audit events with default parameters.
func TestAuditEvents_11_2_1_ListsAllWithDefaultParams(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
		HasMore     bool             `json:"hasMore"`
		NextCursor  string           `json:"nextCursor,omitempty"`
	}
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.AuditEvents)
	assert.GreaterOrEqual(t, len(result.AuditEvents), 1, "Should have at least some audit events")
	assert.NotNil(t, result.HasMore)

	// Verify events are sorted by createdAt DESC (default)
	if len(result.AuditEvents) >= 2 {
		first := result.AuditEvents[0]["createdAt"].(string)
		second := result.AuditEvents[1]["createdAt"].(string)
		assert.GreaterOrEqual(t, first, second, "Events should be sorted by createdAt DESC")
	}

	// Verify each event has required fields
	for _, event := range result.AuditEvents {
		assert.NotEmpty(t, event["eventId"])
		assert.NotEmpty(t, event["eventType"])
		assert.NotEmpty(t, event["action"])
		assert.NotEmpty(t, event["result"])
	}
}

// TestAuditEvents_11_2_2_FiltersByEventType tests filtering by eventType parameter.
func TestAuditEvents_11_2_2_FiltersByEventType(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a rule (generates RULE_CREATED event)
	ruleName := "filter-test-" + testutil.MustDeterministicUUID(7003).String()[:8]
	ruleReq := testutil.RuleRequest{
		Name:       ruleName,
		Expression: "true",
		Action:     "ALLOW",
	}
	ruleBody, err := json.Marshal(ruleReq)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(ruleBody))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var rule testutil.RuleResponse
	err = json.NewDecoder(createResp.Body).Decode(&rule)
	require.NoError(t, err)
	require.NotEmpty(t, rule.ID)
	createResp.Body.Close()
	defer testutil.CleanupRule(t, rule.ID)

	time.Sleep(100 * time.Millisecond)

	// Filter by RULE_CREATED
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?event_type=RULE_CREATED", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	// All events should have eventType == RULE_CREATED
	for _, event := range result.AuditEvents {
		assert.Equal(t, "RULE_CREATED", event["eventType"])
	}
}

// TestAuditEvents_11_2_3_FiltersByAction tests filtering by action parameter.
func TestAuditEvents_11_2_3_FiltersByAction(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?action=CREATE", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	// All events should have action == CREATE
	for _, event := range result.AuditEvents {
		assert.Equal(t, "CREATE", event["action"])
	}
}

// TestAuditEvents_11_2_4_FiltersByResult tests filtering by result parameter.
func TestAuditEvents_11_2_4_FiltersByResult(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?result=SUCCESS", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	for _, event := range result.AuditEvents {
		assert.Equal(t, "SUCCESS", event["result"])
	}
}

// TestAuditEvents_11_2_5_FiltersByResourceType tests filtering by resourceType.
func TestAuditEvents_11_2_5_FiltersByResourceType(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_type=rule", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	for _, event := range result.AuditEvents {
		assert.Equal(t, "rule", event["resourceType"])
	}
}

// TestAuditEvents_11_2_6_FiltersByDateRange tests date range filtering.
func TestAuditEvents_11_2_6_FiltersByDateRange(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	now := time.Now().UTC()
	startDate := now.Add(-24 * time.Hour).Format(time.RFC3339)
	endDate := now.Add(1 * time.Hour).Format(time.RFC3339)

	url := fmt.Sprintf("%s/v1/audit-events?start_date=%s&end_date=%s", baseURL, startDate, endDate)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	// Verify all events are within range
	from, _ := time.Parse(time.RFC3339, startDate)
	to, _ := time.Parse(time.RFC3339, endDate)

	for _, event := range result.AuditEvents {
		eventTime, _ := time.Parse(time.RFC3339, event["createdAt"].(string))
		assert.True(t, eventTime.After(from) || eventTime.Equal(from))
		assert.True(t, eventTime.Before(to) || eventTime.Equal(to))
	}
}

// TestAuditEvents_11_2_7_FiltersByAccountId tests filtering by account_id (JSONB query).
func TestAuditEvents_11_2_7_FiltersByAccountId(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a validation with known accountId
	accountID := testutil.MustDeterministicUUID(7004).String()
	validationReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(7005).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("500"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID:     accountID,
			Type:   "checking",
			Status: "active",
		},
	}

	resp, body := testutil.CreateValidation(t, validationReq)
	if resp.StatusCode != http.StatusCreated {
		t.Logf("Validation failed with status %d: %s", resp.StatusCode, string(body))
	}
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	time.Sleep(100 * time.Millisecond)

	// Filter by accountId
	url := fmt.Sprintf("%s/v1/audit-events?account_id=%s&event_type=TRANSACTION_VALIDATED", baseURL, accountID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer auditResp.Body.Close()
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(auditResp.Body).Decode(&result)
	require.NoError(t, err)

	// Should find at least one event with this accountId
	require.NotEmpty(t, result.AuditEvents, "Should find audit event for account_id")

	// Verify JSONB filtering worked
	for _, event := range result.AuditEvents {
		context := event["context"].(map[string]any)
		request := context["request"].(map[string]any)
		account := request["account"].(map[string]any)
		assert.Equal(t, accountID, account["id"], "All events should match account_id filter")
	}
}

// TestAuditEvents_11_2_8_FiltersByTransactionType tests filtering by transactionType (JSONB query).
func TestAuditEvents_11_2_8_FiltersByTransactionType(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create PIX validation
	pixReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(7006).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("10"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID:     testutil.MustDeterministicUUID(7007).String(),
			Type:   "checking",
			Status: "active",
		},
	}
	respPIX, _ := testutil.CreateValidation(t, pixReq)
	require.Equal(t, http.StatusCreated, respPIX.StatusCode)

	// Create CARD validation
	cardReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(7008).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("20"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID:     testutil.MustDeterministicUUID(7009).String(),
			Type:   "checking",
			Status: "active",
		},
	}
	respCard, _ := testutil.CreateValidation(t, cardReq)
	require.Equal(t, http.StatusCreated, respCard.StatusCode)

	time.Sleep(100 * time.Millisecond)

	// Filter by transactionType=PIX
	url := fmt.Sprintf("%s/v1/audit-events?transaction_type=PIX&event_type=TRANSACTION_VALIDATED", baseURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer auditResp.Body.Close()
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(auditResp.Body).Decode(&result)
	require.NoError(t, err)

	require.NotEmpty(t, result.AuditEvents, "Should find audit events for transaction_type")

	// Verify all events are PIX (no CARD)
	for _, event := range result.AuditEvents {
		context := event["context"].(map[string]any)
		request := context["request"].(map[string]any)
		assert.Equal(t, "PIX", request["transactionType"], "Should only return PIX transactions")
	}
}

// TestAuditEvents_11_2_9_FiltersByMatchedRuleId tests filtering by matchedRuleId (JSONB array contains).
func TestAuditEvents_11_2_9_FiltersByMatchedRuleId(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create and activate a rule with ALLOW action
	// (ALLOW rules are included in matchedRuleIds even when other rules might DENY)
	ruleName := "matched-rule-" + testutil.MustDeterministicUUID(7010).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "amount > 5", "ALLOW")
	defer testutil.CleanupRule(t, ruleID)

	testutil.ActivateRule(t, ruleID)

	time.Sleep(100 * time.Millisecond)

	// Validate a transaction that matches the rule
	validationReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(7011).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("10"), // 10 > 5, will match rule
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID:     testutil.MustDeterministicUUID(7012).String(),
			Type:   "checking",
			Status: "active",
		},
	}

	resp, body := testutil.CreateValidation(t, validationReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var validation testutil.ValidationResponse
	json.Unmarshal(body, &validation)

	// Verify rule was matched
	require.Contains(t, validation.MatchedRuleIDs, ruleID, "Rule must be in matched rules for this test to be valid")

	time.Sleep(100 * time.Millisecond)

	// Filter by matchedRuleId
	url := fmt.Sprintf("%s/v1/audit-events?matched_rule_id=%s&event_type=TRANSACTION_VALIDATED", baseURL, ruleID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer auditResp.Body.Close()
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(auditResp.Body).Decode(&result)
	require.NoError(t, err)

	require.NotEmpty(t, result.AuditEvents, "Should find audit events with matchedRuleId filter")

	// Verify JSONB array contains query worked
	for _, event := range result.AuditEvents {
		context := event["context"].(map[string]any)
		response := context["response"].(map[string]any)
		matchedRuleIDs := response["matchedRuleIds"].([]any)

		found := false
		for _, id := range matchedRuleIDs {
			if id.(string) == ruleID {
				found = true
				break
			}
		}
		assert.True(t, found, "Event should contain ruleId in matchedRuleIds array")
	}
}

// TestAuditEvents_11_2_10_A_ReturnsAllResultsWithoutPagination tests scenario where all results fit in one page.
func TestAuditEvents_11_2_10_A_ReturnsAllResultsWithoutPagination(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create exactly 2 rules to generate exactly 2 audit events (predictable scenario)
	var ruleIDs []string
	for i := 0; i < 2; i++ {
		ruleName := fmt.Sprintf("no-pagination-test-%d-%s", i, testutil.MustDeterministicUUID(int64(7100 + i)).String()[:8])
		ruleReq := testutil.RuleRequest{
			Name:       ruleName,
			Expression: "true",
			Action:     "ALLOW",
		}
		body, err := json.Marshal(ruleReq)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := testutil.HTTPClient.Do(req)
		require.NoError(t, err)
		var rule testutil.RuleResponse
		err = json.NewDecoder(resp.Body).Decode(&rule)
		require.NoError(t, err)
		resp.Body.Close()

		ruleIDs = append(ruleIDs, rule.ID)
		defer testutil.CleanupRule(t, rule.ID)
	}

	time.Sleep(100 * time.Millisecond)

	// Request with limit=100 (much larger than the 2 events we created)
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?limit=100&resource_type=rule&action=CREATE", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
		HasMore     bool             `json:"hasMore"`
		NextCursor  string           `json:"nextCursor,omitempty"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	// Find our events (may have other events in DB)
	ourEvents := 0
	for _, event := range result.AuditEvents {
		resourceID := event["resourceId"].(string)
		for _, ruleID := range ruleIDs {
			if resourceID == ruleID {
				ourEvents++
			}
		}
	}

	// Validations per spec
	assert.GreaterOrEqual(t, ourEvents, 2, "should find at least our 2 created events")
	assert.LessOrEqual(t, len(result.AuditEvents), 100, "should respect limit")

	// When limit is large enough to contain all results, hasMore should be false or nextCursor empty
	// Note: This depends on total events in DB, so we check if either condition is met
	if !result.HasMore {
		assert.Empty(t, result.NextCursor, "nextCursor should be empty when hasMore=false")
		t.Log("✓ All results returned in single page (hasMore=false, nextCursor empty)")
	}
}

// TestAuditEvents_11_2_10_B_IteratesThroughMultiplePages tests multi-page pagination with cursor iteration.
func TestAuditEvents_11_2_10_B_IteratesThroughMultiplePages(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create at least 7 rules to ensure we have at least 3 pages (7 events / 2 per page = 3.5 pages)
	var ruleIDs []string
	for i := 0; i < 7; i++ {
		ruleName := fmt.Sprintf("pagination-test-%d-%s", i, testutil.MustDeterministicUUID(int64(7200 + i)).String()[:8])
		ruleReq := testutil.RuleRequest{
			Name:       ruleName,
			Expression: "true",
			Action:     "ALLOW",
		}
		body, err := json.Marshal(ruleReq)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := testutil.HTTPClient.Do(req)
		require.NoError(t, err)
		var rule testutil.RuleResponse
		err = json.NewDecoder(resp.Body).Decode(&rule)
		require.NoError(t, err)
		resp.Body.Close()

		ruleIDs = append(ruleIDs, rule.ID)
		defer testutil.CleanupRule(t, rule.ID)
	}

	time.Sleep(100 * time.Millisecond)

	// Track all event IDs to verify no duplicates
	allEventIDs := make(map[string]bool)
	pageCount := 0
	nextCursor := ""

	// Page 1: Initial request
	req1, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?limit=2&resource_type=rule&action=CREATE", nil)
	require.NoError(t, err)
	req1.Header.Set("X-API-Key", apiKey)

	resp1, err := testutil.HTTPClient.Do(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusOK, resp1.StatusCode)

	var page1 struct {
		AuditEvents []map[string]any `json:"auditEvents"`
		HasMore     bool             `json:"hasMore"`
		NextCursor  string           `json:"nextCursor,omitempty"`
	}
	err = json.NewDecoder(resp1.Body).Decode(&page1)
	require.NoError(t, err)

	// Validations for page 1
	assert.LessOrEqual(t, len(page1.AuditEvents), 2, "page 1 should respect limit of 2")
	pageCount++

	// Track page 1 events
	for _, event := range page1.AuditEvents {
		eventID := event["eventId"].(string)
		assert.False(t, allEventIDs[eventID], "event %s should not be duplicated", eventID)
		allEventIDs[eventID] = true
	}

	if !page1.HasMore {
		t.Log("⚠️  Only 1 page of results (less events in DB than expected)")
		return
	}

	assert.NotEmpty(t, page1.NextCursor, "nextCursor should be present when hasMore=true")
	nextCursor = page1.NextCursor

	// Page 2: Request with cursor from page 1
	nextURL, err := url.Parse(baseURL + "/v1/audit-events")
	require.NoError(t, err)
	q := nextURL.Query()
	q.Set("limit", "2")
	q.Set("resource_type", "rule")
	q.Set("action", "CREATE")
	q.Set("cursor", nextCursor)
	nextURL.RawQuery = q.Encode()

	req2, err := http.NewRequest(http.MethodGet, nextURL.String(), nil)
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", apiKey)

	resp2, err := testutil.HTTPClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var page2 struct {
		AuditEvents []map[string]any `json:"auditEvents"`
		HasMore     bool             `json:"hasMore"`
		NextCursor  string           `json:"nextCursor,omitempty"`
	}
	err = json.NewDecoder(resp2.Body).Decode(&page2)
	require.NoError(t, err)

	// Validations for page 2
	assert.LessOrEqual(t, len(page2.AuditEvents), 2, "page 2 should respect limit of 2")
	pageCount++

	// Verify no duplicates from page 1
	for _, event := range page2.AuditEvents {
		eventID := event["eventId"].(string)
		assert.False(t, allEventIDs[eventID], "event %s should not be duplicated from page 1", eventID)
		allEventIDs[eventID] = true
	}

	if !page2.HasMore {
		t.Logf("✓ Completed pagination with %d pages and %d unique events", pageCount, len(allEventIDs))
		return
	}

	assert.NotEmpty(t, page2.NextCursor, "nextCursor should be present when hasMore=true")
	nextCursor = page2.NextCursor

	// Page 3: Request with cursor from page 2
	q.Set("cursor", nextCursor)
	nextURL.RawQuery = q.Encode()

	req3, err := http.NewRequest(http.MethodGet, nextURL.String(), nil)
	require.NoError(t, err)
	req3.Header.Set("X-API-Key", apiKey)

	resp3, err := testutil.HTTPClient.Do(req3)
	require.NoError(t, err)
	defer resp3.Body.Close()
	require.Equal(t, http.StatusOK, resp3.StatusCode)

	var page3 struct {
		AuditEvents []map[string]any `json:"auditEvents"`
		HasMore     bool             `json:"hasMore"`
		NextCursor  string           `json:"nextCursor,omitempty"`
	}
	err = json.NewDecoder(resp3.Body).Decode(&page3)
	require.NoError(t, err)

	// Validations for page 3
	assert.LessOrEqual(t, len(page3.AuditEvents), 2, "page 3 should respect limit of 2")
	pageCount++

	// Verify no duplicates from pages 1 and 2
	for _, event := range page3.AuditEvents {
		eventID := event["eventId"].(string)
		assert.False(t, allEventIDs[eventID], "event %s should not be duplicated from previous pages", eventID)
		allEventIDs[eventID] = true
	}

	// Final validations
	assert.GreaterOrEqual(t, pageCount, 3, "should have iterated through at least 3 pages")
	assert.GreaterOrEqual(t, len(allEventIDs), 6, "should have collected at least 6 unique events (2 per page × 3 pages)")

	t.Logf("✓ Successfully iterated through %d pages with %d unique events", pageCount, len(allEventIDs))
	t.Log("✓ No duplicate events detected across pages")
	t.Logf("✓ Page 3 hasMore=%v, nextCursor=%q", page3.HasMore, page3.NextCursor)
}

// TestAuditEvents_11_2_11_SupportsCustomSorting tests custom sorting.
func TestAuditEvents_11_2_11_SupportsCustomSorting(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?sort_by=event_type&sort_order=ASC&limit=10", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	// Verify ascending order
	if len(result.AuditEvents) >= 2 {
		for i := 0; i < len(result.AuditEvents)-1; i++ {
			curr := result.AuditEvents[i]["eventType"].(string)
			next := result.AuditEvents[i+1]["eventType"].(string)
			assert.LessOrEqual(t, curr, next, "Events should be sorted by event_type ASC")
		}
	}
}

// TestAuditEvents_11_2_12_Returns400ForInvalidFilters tests validation of filter parameters.
func TestAuditEvents_11_2_12_Returns400ForInvalidFilters(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?event_type=INVALID_TYPE", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Verify structured error response
	errResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "0053", errResp.Code, "Error response should have validation error code")
	assert.Equal(t, "Validation Error", errResp.Title, "Error response should have validation error title")
	assert.Equal(t, "EventType validation failed: auditeventtype", errResp.Message, "Error message should indicate invalid event type")
}

// TestAuditEvents_11_2_13_ReturnsEmptyArrayWhenNoMatches tests empty results.
func TestAuditEvents_11_2_13_ReturnsEmptyArrayWhenNoMatches(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	nonExistentID := "00000000-0000-0000-0000-000000000000"
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?account_id="+nonExistentID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
		HasMore     bool             `json:"hasMore"`
		NextCursor  string           `json:"nextCursor,omitempty"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Empty(t, result.AuditEvents)
	assert.False(t, result.HasMore)
	assert.Empty(t, result.NextCursor)
}

// ============================================================================
// 11.3 GET /v1/audit-events/{id}/verify - Verify Hash Chain
// ============================================================================

// TestAuditEvents_11_3_1_VerifiesValidHashChain tests hash chain verification.
// Clears audit_events first to ensure hash chain integrity (other tests may have
// created events with cleanup operations that corrupt the chain).
func TestAuditEvents_11_3_1_VerifiesValidHashChain(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()
	db := testutil.SetupIntegrationDB(t)

	// Clean slate: remove all audit events to ensure hash chain integrity.
	resetAuditEvents(t, db)

	// Create a rule to generate a fresh audit event with valid hash chain
	ruleName := "verify-hash-" + testutil.MustDeterministicUUID(7013).String()[:8]
	ruleReq := testutil.RuleRequest{
		Name:       ruleName,
		Expression: "true",
		Action:     "ALLOW",
	}
	body, err := json.Marshal(ruleReq)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var rule testutil.RuleResponse
	err = json.NewDecoder(createResp.Body).Decode(&rule)
	require.NoError(t, err)
	defer testutil.CleanupRule(t, rule.ID)

	time.Sleep(100 * time.Millisecond)

	// Get the event ID for verification (filter by rule ID to avoid interference from other tests)
	var eventID string
	err = db.QueryRowContext(context.Background(),
		`SELECT event_id FROM audit_events WHERE resource_id = $1 ORDER BY id DESC LIMIT 1`,
		rule.ID,
	).Scan(&eventID)
	require.NoError(t, err, "Should find audit event for rule %s", rule.ID)

	// Verify hash chain (should be valid with fresh data)
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/"+eventID+"/verify", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var verifyResult struct {
		IsValid      bool   `json:"isValid"`
		TotalChecked int64  `json:"totalChecked"`
		Message      string `json:"message"`
	}
	err = json.Unmarshal(respBody, &verifyResult)
	require.NoError(t, err)

	assert.True(t, verifyResult.IsValid, "Hash chain should be valid")
	assert.Greater(t, verifyResult.TotalChecked, int64(0), "Should check at least one event")
	assert.Equal(t, "Hash chain integrity verified successfully", verifyResult.Message, "Message should confirm successful verification")
}

// TestAuditEvents_11_3_3_VerifiesSingleEvent tests verification of single (first) event.
func TestAuditEvents_11_3_3_VerifiesSingleEvent(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()
	db := testutil.SetupIntegrationDB(t)

	// Setup: Create a rule to generate an audit event
	ruleName := "Verify Single Event Test " + testutil.MustDeterministicUUID(7102).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "amount > 50", "DENY")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Get the audit event that was just created
	var eventID string
	err := db.QueryRowContext(context.Background(),
		`SELECT event_id FROM audit_events WHERE resource_type = 'rule' AND resource_id = $1 ORDER BY id DESC LIMIT 1`,
		ruleID,
	).Scan(&eventID)
	require.NoError(t, err, "Audit event should be created for rule creation")

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/"+eventID+"/verify", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		IsValid      bool  `json:"isValid"`
		TotalChecked int64 `json:"totalChecked"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.True(t, result.IsValid, "Hash chain should be valid")
	assert.GreaterOrEqual(t, result.TotalChecked, int64(1), "Should verify at least 1 event")
}

// TestAuditEvents_11_3_4_Returns404ForNonExistentEvent tests 404 for verify endpoint.
func TestAuditEvents_11_3_4_Returns404ForNonExistentEvent(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/00000000-0000-0000-0000-000000000000/verify", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Expected 404, got %d: %s", resp.StatusCode, string(body))
	}

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestAuditEvents_11_3_5_Returns400ForInvalidUUID tests 400 for verify endpoint.
func TestAuditEvents_11_3_5_Returns400ForInvalidUUID(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/invalid/verify", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Verify structured error response
	errResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "0065", errResp.Code, "Error response should have invalid path parameter code")
	assert.Equal(t, "Invalid Path Parameter", errResp.Title, "Error response should have invalid path parameter title")
	assert.Equal(t, "Invalid event ID format", errResp.Message, "Error message should indicate invalid UUID format")
}

// ============================================================================
// 11.4 Audit Event Generation Tests
// ============================================================================

// TestAuditEvents_11_4_1_GeneratesAuditForRuleCreation tests RULE_CREATED event generation.
func TestAuditEvents_11_4_1_GeneratesAuditForRuleCreation(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	ruleName := "audit-gen-test-" + testutil.MustDeterministicUUID(7014).String()[:8]
	ruleReq := testutil.RuleRequest{
		Name:       ruleName,
		Expression: "amount > 10",
		Action:     "DENY",
	}
	body, err := json.Marshal(ruleReq)
	require.NoError(t, err)

	// Create rule
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var rule testutil.RuleResponse
	err = json.NewDecoder(resp.Body).Decode(&rule)
	require.NoError(t, err)
	require.NotEmpty(t, rule.ID)
	defer testutil.CleanupRule(t, rule.ID)

	time.Sleep(100 * time.Millisecond)

	// Query audit events
	auditReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_type=rule&action=CREATE&resource_id="+rule.ID, nil)
	require.NoError(t, err)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var auditResult struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(auditResp.Body).Decode(&auditResult)
	require.NoError(t, err)

	require.NotEmpty(t, auditResult.AuditEvents, "Should find RULE_CREATED audit event")

	event := auditResult.AuditEvents[0]
	assert.Equal(t, "RULE_CREATED", event["eventType"])
	assert.Equal(t, "CREATE", event["action"])
	assert.Equal(t, "SUCCESS", event["result"])
	assert.Equal(t, rule.ID, event["resourceId"])

	// Verify context contains rule data
	context := event["context"].(map[string]any)
	after := context["after"].(map[string]any)
	assert.Equal(t, ruleName, after["name"])
	assert.Equal(t, "amount > 10", after["expression"])

	// Verify before is null for CREATE
	assert.Nil(t, context["before"])
}

// TestAuditEvents_11_4_2_GeneratesAuditForRuleActivation tests RULE_ACTIVATED event.
func TestAuditEvents_11_4_2_GeneratesAuditForRuleActivation(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create rule in DRAFT
	ruleName := "activate-audit-" + testutil.MustDeterministicUUID(7015).String()[:8]
	ruleReq := testutil.RuleRequest{
		Name:       ruleName,
		Expression: "true",
		Action:     "ALLOW",
	}
	body, err := json.Marshal(ruleReq)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var rule testutil.RuleResponse
	err = json.NewDecoder(createResp.Body).Decode(&rule)
	require.NoError(t, err)
	require.NotEmpty(t, rule.ID)
	defer testutil.CleanupRule(t, rule.ID)

	// Activate rule
	activateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+rule.ID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()

	time.Sleep(100 * time.Millisecond)

	// Query audit for RULE_ACTIVATED
	// Note: action is UPDATE (activation is a status update operation)
	auditReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_id="+rule.ID+"&event_type=RULE_ACTIVATED", nil)
	require.NoError(t, err)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(auditResp.Body).Decode(&result)
	require.NoError(t, err)

	require.NotEmpty(t, result.AuditEvents, "Should find RULE_ACTIVATED event")

	event := result.AuditEvents[0]
	assert.Equal(t, "RULE_ACTIVATED", event["eventType"])

	// Verify before/after states
	context := event["context"].(map[string]any)
	before := context["before"].(map[string]any)
	after := context["after"].(map[string]any)

	assert.Equal(t, "DRAFT", before["status"])
	assert.Equal(t, "ACTIVE", after["status"])
}

// TestAuditEvents_11_4_3_GeneratesAuditForRuleUpdate tests RULE_UPDATED event generation.
func TestAuditEvents_11_4_3_GeneratesAuditForRuleUpdate(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create rule
	ruleName := "update-audit-" + testutil.MustDeterministicUUID(7016).String()[:8]
	ruleReq := testutil.RuleRequest{
		Name:       ruleName,
		Expression: "amount > 10",
		Action:     "DENY",
	}
	body, err := json.Marshal(ruleReq)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var rule testutil.RuleResponse
	err = json.NewDecoder(createResp.Body).Decode(&rule)
	require.NoError(t, err)
	require.NotEmpty(t, rule.ID)
	defer testutil.CleanupRule(t, rule.ID)

	time.Sleep(100 * time.Millisecond)

	// Update rule (note: names are normalized to lowercase)
	newName := "Updated Name"
	updateReq := map[string]any{
		"name": newName,
	}
	updateBody, err := json.Marshal(updateReq)
	require.NoError(t, err)

	patchReq, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+rule.ID, bytes.NewReader(updateBody))
	require.NoError(t, err)
	patchReq.Header.Set("X-API-Key", apiKey)
	patchReq.Header.Set("Content-Type", "application/json")

	patchResp, err := testutil.HTTPClient.Do(patchReq)
	require.NoError(t, err)
	defer patchResp.Body.Close()

	require.Equal(t, http.StatusOK, patchResp.StatusCode)

	time.Sleep(100 * time.Millisecond)

	// Query audit events
	auditReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_id="+rule.ID+"&event_type=RULE_UPDATED", nil)
	require.NoError(t, err)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var auditResult struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(auditResp.Body).Decode(&auditResult)
	require.NoError(t, err)

	require.NotEmpty(t, auditResult.AuditEvents, "Should find RULE_UPDATED audit event")

	event := auditResult.AuditEvents[0]
	assert.Equal(t, "RULE_UPDATED", event["eventType"])
	assert.Equal(t, "UPDATE", event["action"])
	assert.Equal(t, "SUCCESS", event["result"])

	// Verify before/after states
	context := event["context"].(map[string]any)
	before := context["before"].(map[string]any)
	after := context["after"].(map[string]any)

	assert.Equal(t, ruleName, before["name"], "Before should contain old name")
	// Names are normalized to lowercase in the system
	assert.Equal(t, strings.ToLower(newName), after["name"], "After should contain new name (normalized to lowercase)")
}

// TestAuditEvents_11_4_4_GeneratesAuditForRuleDelete tests RULE_DELETED event generation.
func TestAuditEvents_11_4_4_GeneratesAuditForRuleDelete(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create and deactivate rule
	ruleName := "delete-audit-" + testutil.MustDeterministicUUID(7017).String()[:8]
	ruleReq := testutil.RuleRequest{
		Name:       ruleName,
		Expression: "true",
		Action:     "ALLOW",
	}
	body, err := json.Marshal(ruleReq)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var rule testutil.RuleResponse
	err = json.NewDecoder(createResp.Body).Decode(&rule)
	require.NoError(t, err)
	require.NotEmpty(t, rule.ID)

	// Deactivate rule (required before delete)
	deactivateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+rule.ID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)
	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	deactivateResp.Body.Close()

	time.Sleep(100 * time.Millisecond)

	// Delete rule
	deleteReq, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/rules/"+rule.ID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("X-API-Key", apiKey)

	deleteResp, err := testutil.HTTPClient.Do(deleteReq)
	require.NoError(t, err)
	defer deleteResp.Body.Close()

	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	time.Sleep(100 * time.Millisecond)

	// Query audit events
	auditReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_id="+rule.ID+"&event_type=RULE_DELETED", nil)
	require.NoError(t, err)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var auditResult struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(auditResp.Body).Decode(&auditResult)
	require.NoError(t, err)

	require.NotEmpty(t, auditResult.AuditEvents, "Should find RULE_DELETED audit event")

	event := auditResult.AuditEvents[0]
	assert.Equal(t, "RULE_DELETED", event["eventType"])
	assert.Equal(t, "DELETE", event["action"])
	assert.Equal(t, "SUCCESS", event["result"])

	// Verify context has before state and null after
	context := event["context"].(map[string]any)
	before := context["before"].(map[string]any)
	after := context["after"]

	assert.NotNil(t, before, "Before should contain rule data")
	assert.Equal(t, ruleName, before["name"])
	assert.Nil(t, after, "After should be null for DELETE")
}

// TestAuditEvents_11_4_5_GeneratesAuditForTransactionValidation tests TRANSACTION_VALIDATED event generation.
func TestAuditEvents_11_4_5_GeneratesAuditForTransactionValidation(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create validation request
	accountID := testutil.MustDeterministicUUID(7018).String()
	requestID := testutil.MustDeterministicUUID(7019).String()

	validationReq := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("500"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID:     accountID,
			Type:   "checking",
			Status: "active",
		},
	}

	resp, body := testutil.CreateValidation(t, validationReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var validation testutil.ValidationResponse
	json.Unmarshal(body, &validation)

	time.Sleep(100 * time.Millisecond)

	// Query audit events for this validation
	url := fmt.Sprintf("%s/v1/audit-events?event_type=TRANSACTION_VALIDATED&account_id=%s", baseURL, accountID)
	auditReq, _ := http.NewRequest(http.MethodGet, url, nil)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()

	var auditResult struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	json.NewDecoder(auditResp.Body).Decode(&auditResult)

	require.NotEmpty(t, auditResult.AuditEvents, "Should find TRANSACTION_VALIDATED audit event")

	event := auditResult.AuditEvents[0]
	assert.Equal(t, "TRANSACTION_VALIDATED", event["eventType"])
	assert.Equal(t, validation.Decision, event["result"])

	// Verify context contains full transaction data
	context := event["context"].(map[string]any)
	request := context["request"].(map[string]any)
	response := context["response"].(map[string]any)

	// Validate request data
	assert.Equal(t, "PIX", request["transactionType"])
	assert.Equal(t, "500", request["amount"])
	assert.Equal(t, "BRL", request["currency"])

	account := request["account"].(map[string]any)
	assert.Equal(t, accountID, account["id"])

	// Validate response data
	assert.Equal(t, validation.Decision, response["decision"])
	assert.NotNil(t, response["processingTimeMs"])
	assert.NotNil(t, response["matchedRuleIds"])
	assert.NotNil(t, response["evaluatedRuleIds"])
}

// TestAuditEvents_11_4_6_CapturesClientIPInAuditEvents tests client IP capture.
func TestAuditEvents_11_4_6_CapturesClientIPInAuditEvents(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	ruleName := "ip-test-" + testutil.MustDeterministicUUID(7020).String()[:8]
	ruleReq := testutil.RuleRequest{
		Name:       ruleName,
		Expression: "true",
		Action:     "ALLOW",
	}
	body, err := json.Marshal(ruleReq)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "203.0.113.1")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var rule testutil.RuleResponse
	err = json.NewDecoder(resp.Body).Decode(&rule)
	require.NoError(t, err)
	require.NotEmpty(t, rule.ID)
	defer testutil.CleanupRule(t, rule.ID)

	time.Sleep(100 * time.Millisecond)

	// Query audit event
	auditReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_id="+rule.ID+"&action=CREATE", nil)
	require.NoError(t, err)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(auditResp.Body).Decode(&result)
	require.NoError(t, err)

	require.NotEmpty(t, result.AuditEvents)

	event := result.AuditEvents[0]
	actor := event["actor"].(map[string]any)

	// Verify client IP captured (should be real IP, not 0.0.0.0)
	ipAddress := actor["ipAddress"].(string)
	assert.NotEqual(t, "0.0.0.0", ipAddress, "Should capture real client IP via middleware")
	// Note: In test environment, actual IP may vary, but should not be default
}

// ============================================================================
// 11.4.7-11.4.11: LIMIT Lifecycle Audit Event Tests
// ============================================================================
// These tests verify that LIMIT operations generate correct audit events with
// proper before/after snapshots. Pattern mirrors RULE tests (11.4.1-11.4.6).
// Event types: LIMIT_CREATED, LIMIT_UPDATED, LIMIT_DELETED, LIMIT_ACTIVATED, LIMIT_DEACTIVATED
// ============================================================================

// TestAuditEvents_11_4_7_GeneratesAuditForLimitCreation tests LIMIT_CREATED event generation.
func TestAuditEvents_11_4_7_GeneratesAuditForLimitCreation(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Precondition: Create a limit to generate LIMIT_CREATED audit event
	accountID := testutil.MustDeterministicUUID(7021).String()
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	defer testutil.CleanupLimit(t, limitID)

	// Wait briefly for audit event to be created
	time.Sleep(100 * time.Millisecond)

	// Query audit events to find the LIMIT_CREATED event
	listReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_type=limit&action=CREATE&resource_id="+limitID, nil)
	require.NoError(t, err)
	listReq.Header.Set("X-API-Key", apiKey)

	listResp, err := testutil.HTTPClient.Do(listReq)
	require.NoError(t, err)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var listResult struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(listResp.Body).Decode(&listResult)
	require.NoError(t, err)
	require.NotEmpty(t, listResult.AuditEvents, "Should find audit event for limit creation")

	// Validations
	event := listResult.AuditEvents[0]
	assert.Equal(t, "LIMIT_CREATED", event["eventType"])
	assert.Equal(t, "CREATE", event["action"])
	assert.Equal(t, "SUCCESS", event["result"])
	assert.Equal(t, "limit", event["resourceType"])
	assert.NotEmpty(t, event["resourceId"])
	assert.NotNil(t, event["actor"])
	assert.NotEmpty(t, event["hash"])
	assert.NotEmpty(t, event["createdAt"])

	// Verify context contains limit data
	context := event["context"].(map[string]any)
	after := context["after"].(map[string]any)
	assert.Equal(t, "DAILY", after["limitType"])
	assert.Equal(t, "1000", after["maxAmount"])
	assert.Equal(t, "BRL", after["currency"])
	assert.Equal(t, "DRAFT", after["status"])

	// Verify before is null for CREATE
	assert.Nil(t, context["before"])
}

// TestAuditEvents_11_4_8_GeneratesAuditForLimitActivation tests LIMIT_ACTIVATED event.
func TestAuditEvents_11_4_8_GeneratesAuditForLimitActivation(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create limit in DRAFT
	accountID := testutil.MustDeterministicUUID(7022).String()
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	defer testutil.CleanupLimit(t, limitID)

	// Activate limit
	testutil.ActivateLimit(t, limitID)

	time.Sleep(100 * time.Millisecond)

	// Query audit for LIMIT_ACTIVATED
	auditReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_id="+limitID+"&event_type=LIMIT_ACTIVATED", nil)
	require.NoError(t, err)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(auditResp.Body).Decode(&result)
	require.NoError(t, err)

	require.NotEmpty(t, result.AuditEvents, "Should find LIMIT_ACTIVATED event")

	event := result.AuditEvents[0]
	assert.Equal(t, "LIMIT_ACTIVATED", event["eventType"])
	assert.Equal(t, "ACTIVATE", event["action"])
	assert.Equal(t, "SUCCESS", event["result"])

	// Verify before/after states
	context := event["context"].(map[string]any)
	before := context["before"].(map[string]any)
	after := context["after"].(map[string]any)

	assert.Equal(t, "DRAFT", before["status"])
	assert.Equal(t, "ACTIVE", after["status"])
}

// TestAuditEvents_11_4_9_GeneratesAuditForLimitUpdate tests LIMIT_UPDATED event generation.
func TestAuditEvents_11_4_9_GeneratesAuditForLimitUpdate(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create limit
	accountID := testutil.MustDeterministicUUID(7023).String()
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	defer testutil.CleanupLimit(t, limitID)

	time.Sleep(100 * time.Millisecond)

	// Update limit name
	newName := "Updated Limit Name " + testutil.RandomSuffix()
	updateReq := map[string]any{
		"name": newName,
	}
	updateBody, err := json.Marshal(updateReq)
	require.NoError(t, err)

	patchReq, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/limits/"+limitID, bytes.NewReader(updateBody))
	require.NoError(t, err)
	patchReq.Header.Set("X-API-Key", apiKey)
	patchReq.Header.Set("Content-Type", "application/json")

	patchResp, err := testutil.HTTPClient.Do(patchReq)
	require.NoError(t, err)
	defer patchResp.Body.Close()

	require.Equal(t, http.StatusOK, patchResp.StatusCode)

	time.Sleep(100 * time.Millisecond)

	// Query audit events
	auditReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_id="+limitID+"&event_type=LIMIT_UPDATED", nil)
	require.NoError(t, err)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var auditResult struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(auditResp.Body).Decode(&auditResult)
	require.NoError(t, err)

	require.NotEmpty(t, auditResult.AuditEvents, "Should find LIMIT_UPDATED audit event")

	event := auditResult.AuditEvents[0]
	assert.Equal(t, "LIMIT_UPDATED", event["eventType"])
	assert.Equal(t, "UPDATE", event["action"])
	assert.Equal(t, "SUCCESS", event["result"])

	// Verify before/after states
	context := event["context"].(map[string]any)
	before := context["before"].(map[string]any)
	after := context["after"].(map[string]any)

	assert.NotEqual(t, before["name"], after["name"], "Name should have changed")
	assert.Equal(t, newName, after["name"], "After should contain new name")
}

// TestAuditEvents_11_4_10_GeneratesAuditForLimitDeactivation tests LIMIT_DEACTIVATED event generation.
func TestAuditEvents_11_4_10_GeneratesAuditForLimitDeactivation(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create and activate limit
	accountID := testutil.MustDeterministicUUID(7024).String()
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	defer testutil.CleanupLimit(t, limitID)

	testutil.ActivateLimit(t, limitID)

	time.Sleep(100 * time.Millisecond)

	// Deactivate limit
	deactivateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)

	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	defer deactivateResp.Body.Close()

	require.Equal(t, http.StatusOK, deactivateResp.StatusCode)

	time.Sleep(100 * time.Millisecond)

	// Query audit events
	auditReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_id="+limitID+"&event_type=LIMIT_DEACTIVATED", nil)
	require.NoError(t, err)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var auditResult struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(auditResp.Body).Decode(&auditResult)
	require.NoError(t, err)

	require.NotEmpty(t, auditResult.AuditEvents, "Should find LIMIT_DEACTIVATED audit event")

	event := auditResult.AuditEvents[0]
	assert.Equal(t, "LIMIT_DEACTIVATED", event["eventType"])
	assert.Equal(t, "DEACTIVATE", event["action"])
	assert.Equal(t, "SUCCESS", event["result"])

	// Verify before/after states
	context := event["context"].(map[string]any)
	before := context["before"].(map[string]any)
	after := context["after"].(map[string]any)

	assert.Equal(t, "ACTIVE", before["status"])
	assert.Equal(t, "INACTIVE", after["status"])
}

// TestAuditEvents_11_4_11_GeneratesAuditForLimitDelete tests LIMIT_DELETED event generation.
func TestAuditEvents_11_4_11_GeneratesAuditForLimitDelete(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create limit in DRAFT status
	accountID := testutil.MustDeterministicUUID(7025).String()
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")

	// Delete limit directly from DRAFT (DRAFT → DELETED is allowed)
	// Valid transitions: DRAFT → ACTIVE/DELETED, ACTIVE → INACTIVE, INACTIVE → ACTIVE/DRAFT/DELETED
	deleteReq, err := http.NewRequest(http.MethodDelete, baseURL+"/v1/limits/"+limitID, nil)
	require.NoError(t, err)
	deleteReq.Header.Set("X-API-Key", apiKey)

	deleteResp, err := testutil.HTTPClient.Do(deleteReq)
	require.NoError(t, err)
	defer deleteResp.Body.Close()

	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	time.Sleep(100 * time.Millisecond)

	// Query audit events
	auditReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_id="+limitID+"&event_type=LIMIT_DELETED", nil)
	require.NoError(t, err)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var auditResult struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(auditResp.Body).Decode(&auditResult)
	require.NoError(t, err)

	require.NotEmpty(t, auditResult.AuditEvents, "Should find LIMIT_DELETED audit event")

	event := auditResult.AuditEvents[0]
	assert.Equal(t, "LIMIT_DELETED", event["eventType"])
	assert.Equal(t, "DELETE", event["action"])
	assert.Equal(t, "SUCCESS", event["result"])

	// Verify context has before state and null after
	context := event["context"].(map[string]any)
	before := context["before"].(map[string]any)
	after := context["after"]

	assert.NotNil(t, before, "Before should contain limit data")
	assert.Equal(t, "DRAFT", before["status"])
	assert.Nil(t, after, "After should be null for DELETE")
}

// ============================================================================
// 11.7 Immutability Tests (already implemented in audit_events_immutability_test.go)
// ============================================================================
// See: tests/integration/audit_events_immutability_test.go
// - TestAuditEvents_Update_Blocked
// - TestAuditEvents_Delete_Blocked
// See: tests/integration/truncate_test.go
// - TestAuditEvents_TruncateProtection

// ============================================================================
// 11.9 Edge Cases
// ============================================================================

// TestAuditEvents_11_9_1_EmptyDateRangeReturnsNoResults tests empty date range.
func TestAuditEvents_11_9_1_EmptyDateRangeReturnsNoResults(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Date range in the past with no events
	startDate := "2020-01-01T00:00:00Z"
	endDate := "2020-01-02T00:00:00Z"

	url := fmt.Sprintf("%s/v1/audit-events?start_date=%s&end_date=%s", baseURL, startDate, endDate)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "should return 200 OK even with empty results")

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
		HasMore     bool             `json:"hasMore"`
		NextCursor  string           `json:"nextCursor,omitempty"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	// Validations per spec 11.9.1
	assert.Empty(t, result.AuditEvents, "auditEvents should be empty array for date range with no events")
	assert.False(t, result.HasMore, "hasMore should be false when no results")
	assert.Empty(t, result.NextCursor, "nextCursor should be empty when no results")
}

// TestAuditEvents_11_9_2_InvalidDateFormatReturns400 tests invalid date format.
func TestAuditEvents_11_9_2_InvalidDateFormatReturns400(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?start_date=invalid-date", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Validations per spec 11.9.2
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "should return 400 Bad Request for invalid date format")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Verify structured error response
	errResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "0077", errResp.Code, "Error response should have TRC-0020 (Invalid Date Format)")
	assert.Equal(t, "Validation Error", errResp.Title, "Error response should have validation error title")
	assert.Contains(t, errResp.Message, "start_date must be in RFC3339 format", "Error message should indicate invalid date format")
}

// TestAuditEvents_11_9_3_PageSizeExceedsMaximum tests limit validation.
func TestAuditEvents_11_9_3_PageSizeExceedsMaximum(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Request with limit=5000 (exceeds max of 1000)
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?limit=5000", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Validations per spec 11.9.3
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "should return 400 Bad Request when limit exceeds maximum")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Verify structured error response with specific TRC code for limit exceeded
	errResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "0080", errResp.Code, "Error response should have TRC-0040 (Limit Exceeds Maximum)")
	assert.Equal(t, "Validation Error", errResp.Title, "Error response should have validation error title")
	assert.Equal(t, "limit must not exceed 1000", errResp.Message, "Error message should indicate limit exceeded")
}

// TestAuditEvents_11_9_4_NegativePageSizeReturnsError tests negative limit.
func TestAuditEvents_11_9_4_NegativePageSizeReturnsError(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?limit=-10", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Validations per spec 11.9.4
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "should return 400 Bad Request for negative limit")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Verify structured error response with specific TRC code for limit below minimum
	errResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "0331", errResp.Code, "Error response should have TRC-0041 (Limit Below Minimum)")
	assert.Equal(t, "Validation Error", errResp.Title, "Error response should have validation error title")
	assert.Equal(t, "limit must be at least 1", errResp.Message, "Error message should indicate limit validation failed")
}

// TestAuditEvents_11_9_5_InvalidSortByFieldReturns400 tests invalid sortBy field.
func TestAuditEvents_11_9_5_InvalidSortByFieldReturns400(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?sort_by=invalidField", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "should return 400 Bad Request for invalid sort_by field")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Verify structured error response
	errResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "0332", errResp.Code, "Error response should have TRC-0043 (Invalid SortBy)")
	assert.Equal(t, "Validation Error", errResp.Title, "Error response should have validation error title")
	assert.Contains(t, errResp.Message, "sort_by must be one of", "Error message should indicate invalid sort_by field")
}

// TestAuditEvents_11_9_6_EndDateBeforeStartDateReturns400 tests invalid date range.
func TestAuditEvents_11_9_6_EndDateBeforeStartDateReturns400(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// EndDate before StartDate (invalid)
	startDate := "2024-12-31T00:00:00Z"
	endDate := "2024-01-01T00:00:00Z"

	url := fmt.Sprintf("%s/v1/audit-events?start_date=%s&end_date=%s", baseURL, startDate, endDate)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "should return 400 Bad Request when end_date is before start_date")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Verify structured error response
	errResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "0083", errResp.Code, "Error response should have TRC-0023 (Invalid date range)")
	assert.Equal(t, "Validation Error", errResp.Title, "Error response should have validation error title")
	assert.Equal(t, "end_date must be on or after start_date", errResp.Message, "Error message should indicate invalid date range")
}

// TestAuditEvents_11_9_7_InvalidCursorReturns400 tests malformed cursor.
func TestAuditEvents_11_9_7_InvalidCursorReturns400(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Use an obviously invalid cursor (not base64 encoded JSON)
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?cursor=invalid-cursor-123", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"invalid cursor should return 400 Bad Request: %s", string(body))

	errResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "0333", errResp.Code, "Error response should have TRC-0044 for invalid cursor")
	assert.Equal(t, "Bad Request", errResp.Title, "Error title should be Bad Request")
	assert.Equal(t, "Invalid pagination cursor", errResp.Message, "Error message should indicate invalid cursor")
}

// TestAuditEvents_11_9_8_ZeroLimitReturnsError tests limit=0 behavior.
// Per current implementation, limit=0 is treated as invalid (must be at least 1).
func TestAuditEvents_11_9_8_ZeroLimitReturnsError(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?limit=0", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// limit=0 is invalid - must be at least 1
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "limit=0 should return 400 Bad Request")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Verify structured error response with specific TRC code
	errResp := testutil.ParseErrorResponse(t, body)
	assert.Equal(t, "0331", errResp.Code, "Error response should have TRC-0041 (Limit Below Minimum)")
	assert.Equal(t, "Validation Error", errResp.Title, "Error response should have validation error title")
	assert.Equal(t, "limit must be at least 1", errResp.Message, "Error message should indicate limit must be at least 1")
}

// ============================================================================
// 11.10 Cross-Operation Audit Trail
// ============================================================================

// TestAuditEvents_11_10_1_CompleteRuleLifecycleIsAudited tests full lifecycle audit trail.
func TestAuditEvents_11_10_1_CompleteRuleLifecycleIsAudited(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// 1. Create rule
	ruleName := "lifecycle-test-" + testutil.MustDeterministicUUID(7026).String()[:8]
	ruleReq := testutil.RuleRequest{
		Name:       ruleName,
		Expression: "true",
		Action:     "ALLOW",
	}
	body, _ := json.Marshal(ruleReq)

	createReq, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	var rule testutil.RuleResponse
	json.NewDecoder(createResp.Body).Decode(&rule)

	// 2. Activate rule
	activateReq, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+rule.ID+"/activate", nil)
	activateReq.Header.Set("X-API-Key", apiKey)
	activateResp, _ := testutil.HTTPClient.Do(activateReq)
	activateResp.Body.Close()

	// 3. Update rule
	updateReq := map[string]any{"description": "Updated description"}
	updateBody, _ := json.Marshal(updateReq)
	patchReq, _ := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+rule.ID, bytes.NewReader(updateBody))
	patchReq.Header.Set("X-API-Key", apiKey)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, _ := testutil.HTTPClient.Do(patchReq)
	patchResp.Body.Close()

	// 4. Deactivate rule
	deactivateReq, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+rule.ID+"/deactivate", nil)
	deactivateReq.Header.Set("X-API-Key", apiKey)
	deactivateResp, _ := testutil.HTTPClient.Do(deactivateReq)
	deactivateResp.Body.Close()

	// 5. Delete rule
	deleteReq, _ := http.NewRequest(http.MethodDelete, baseURL+"/v1/rules/"+rule.ID, nil)
	deleteReq.Header.Set("X-API-Key", apiKey)
	deleteResp, _ := testutil.HTTPClient.Do(deleteReq)
	deleteResp.Body.Close()

	time.Sleep(200 * time.Millisecond)

	// Query all audit events for this rule
	auditReq, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_id="+rule.ID+"&resource_type=rule", nil)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	json.NewDecoder(auditResp.Body).Decode(&result)

	// Should have 5 events: CREATE, ACTIVATE, UPDATE, DEACTIVATE, DELETE
	assert.GreaterOrEqual(t, len(result.AuditEvents), 5, "Should have at least 5 lifecycle events")

	// Verify event types exist
	eventTypes := make(map[string]bool)
	for _, event := range result.AuditEvents {
		eventTypes[event["eventType"].(string)] = true
	}

	assert.True(t, eventTypes["RULE_CREATED"], "Should have RULE_CREATED event")
	assert.True(t, eventTypes["RULE_ACTIVATED"], "Should have RULE_ACTIVATED event")
	assert.True(t, eventTypes["RULE_UPDATED"], "Should have RULE_UPDATED event")
	assert.True(t, eventTypes["RULE_DEACTIVATED"], "Should have RULE_DEACTIVATED event")
	assert.True(t, eventTypes["RULE_DELETED"], "Should have RULE_DELETED event")
}

// TestAuditEvents_11_10_2_ValidationTriggersAuditEvent tests that validation creates audit event synchronously.
func TestAuditEvents_11_10_2_ValidationTriggersAuditEvent(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Step 1: POST /v1/validations
	accountID := testutil.MustDeterministicUUID(7027).String()
	requestID := testutil.MustDeterministicUUID(7028).String()

	validationReq := &testutil.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("100"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID:     accountID,
			Type:   "checking",
			Status: "active",
		},
	}

	resp, body := testutil.CreateValidation(t, validationReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var validation testutil.ValidationResponse
	json.Unmarshal(body, &validation)

	// Step 2: GET /v1/validations/{validationId}
	getResp, getBody := testutil.GetValidation(t, validation.ValidationID)
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var validationDetail testutil.ValidationDetailResponse
	json.Unmarshal(getBody, &validationDetail)

	assert.Equal(t, validation.ValidationID, validationDetail.ID)
	assert.Equal(t, requestID, validationDetail.RequestID)

	// Step 3: GET /v1/audit-events - audit should be available IMMEDIATELY (synchronous creation)
	// Query by eventType first to check if any validation events exist
	auditURL := fmt.Sprintf("%s/v1/audit-events?event_type=TRANSACTION_VALIDATED&resource_type=transaction",
		baseURL)
	auditReq, _ := http.NewRequest(http.MethodGet, auditURL, nil)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()

	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var auditResult struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	json.NewDecoder(auditResp.Body).Decode(&auditResult)

	require.NotEmpty(t, auditResult.AuditEvents, "Audit events for validations should be created")

	// Find the event for our specific validation
	var event map[string]any
	for _, e := range auditResult.AuditEvents {
		if e["resourceId"] == validation.ValidationID {
			event = e
			break
		}
	}

	require.NotNil(t, event, "Audit event should exist immediately after validation")

	// Validations per spec
	assert.Equal(t, "TRANSACTION_VALIDATED", event["eventType"])
	assert.Equal(t, validation.ValidationID, event["resourceId"])

	// Verify context matches validation request/response
	context, ok := event["context"].(map[string]any)
	require.True(t, ok, "Audit event must have context")

	request, ok := context["request"].(map[string]any)
	require.True(t, ok, "Audit event context must have request")

	response, ok := context["response"].(map[string]any)
	require.True(t, ok, "Audit event context must have response")

	// Request data should match
	assert.Equal(t, requestID, request["requestId"], "requestId should match")
	assert.Equal(t, "PIX", request["transactionType"], "transactionType should match")
	assert.Equal(t, "100", request["amount"], "amount should match")

	account, ok := request["account"].(map[string]any)
	require.True(t, ok, "request must have account")
	assert.Equal(t, accountID, account["id"], "account ID should match")

	// Response data should match
	assert.Equal(t, validation.Decision, response["decision"], "decision should match")
	assert.NotNil(t, response["processingTimeMs"], "processingTimeMs must be present")
	assert.NotNil(t, response["matchedRuleIds"], "matchedRuleIds must be present")
	assert.NotNil(t, response["evaluatedRuleIds"], "evaluatedRuleIds must be present")

	// Confirm audit was created synchronously (available immediately)
	t.Log("✓ Audit event available immediately after validation (synchronous creation)")
}

// TestAuditEvents_11_10_3_CompleteLimitLifecycleIsAudited tests full lifecycle audit trail.
func TestAuditEvents_11_10_3_CompleteLimitLifecycleIsAudited(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// 1. Create limit
	accountID := testutil.MustDeterministicUUID(7029).String()
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")

	// 2. Activate limit
	testutil.ActivateLimit(t, limitID)

	// 3. Update limit
	updateReq := map[string]any{"description": "Updated description"}
	updateBody, _ := json.Marshal(updateReq)
	patchReq, _ := http.NewRequest(http.MethodPatch, baseURL+"/v1/limits/"+limitID, bytes.NewReader(updateBody))
	patchReq.Header.Set("X-API-Key", apiKey)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, _ := testutil.HTTPClient.Do(patchReq)
	patchResp.Body.Close()

	// 4. Deactivate limit
	deactivateReq, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	deactivateReq.Header.Set("X-API-Key", apiKey)
	deactivateResp, _ := testutil.HTTPClient.Do(deactivateReq)
	deactivateResp.Body.Close()

	// 5. Delete limit
	deleteReq, _ := http.NewRequest(http.MethodDelete, baseURL+"/v1/limits/"+limitID, nil)
	deleteReq.Header.Set("X-API-Key", apiKey)
	deleteResp, _ := testutil.HTTPClient.Do(deleteReq)
	deleteResp.Body.Close()

	time.Sleep(200 * time.Millisecond)

	// Query all audit events for this limit
	auditReq, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_id="+limitID+"&resource_type=limit", nil)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	json.NewDecoder(auditResp.Body).Decode(&result)

	// Should have 5 events: CREATE, ACTIVATE, UPDATE, DEACTIVATE, DELETE
	assert.GreaterOrEqual(t, len(result.AuditEvents), 5, "Should have at least 5 lifecycle events")

	// Verify event types exist
	eventTypes := make(map[string]bool)
	for _, event := range result.AuditEvents {
		eventTypes[event["eventType"].(string)] = true
	}

	assert.True(t, eventTypes["LIMIT_CREATED"], "Should have LIMIT_CREATED event")
	assert.True(t, eventTypes["LIMIT_ACTIVATED"], "Should have LIMIT_ACTIVATED event")
	assert.True(t, eventTypes["LIMIT_UPDATED"], "Should have LIMIT_UPDATED event")
	assert.True(t, eventTypes["LIMIT_DEACTIVATED"], "Should have LIMIT_DEACTIVATED event")
	assert.True(t, eventTypes["LIMIT_DELETED"], "Should have LIMIT_DELETED event")
}

// ============================================================================
// 11.11 Compliance Tests
// ============================================================================

// TestAuditEvents_11_11_1_AuditEventsRetainedPerSOXGLBA validates retention policy for compliance.
// Note: This is a policy test, not a runtime test.
func TestAuditEvents_11_11_1_AuditEventsRetainedPerSOXGLBA(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)

	// Validation 1: Verify retention policy is documented
	// (This would typically check documentation files, but we validate protection instead)

	// Validation 2: No automatic deletion before 7 years
	// We verify that audit_events table has no DELETE triggers or scheduled jobs
	// (only protection triggers like prevent_* are allowed)
	var nonProtectionTriggerCount int
	err := db.QueryRowContext(context.Background(), `
		SELECT COUNT(*)
		FROM pg_trigger t
		JOIN pg_class c ON t.tgrelid = c.oid
		WHERE c.relname = 'audit_events'
		AND t.tgname NOT LIKE 'prevent_%'
		AND t.tgname NOT LIKE 'audit_events_hash_chain'
	`).Scan(&nonProtectionTriggerCount)
	require.NoError(t, err)

	assert.Equal(t, 0, nonProtectionTriggerCount, "Should not have automatic deletion triggers")

	// Validation 3: TRUNCATE protection is active (per spec in 11.7.3)
	// Verify trigger exists to prevent TRUNCATE
	var truncateTriggerExists bool
	err = db.QueryRowContext(context.Background(), `
		SELECT EXISTS(
			SELECT 1
			FROM pg_trigger t
			JOIN pg_class c ON t.tgrelid = c.oid
			WHERE c.relname = 'audit_events'
			AND t.tgname = 'prevent_audit_event_truncate_trigger'
		)
	`).Scan(&truncateTriggerExists)
	require.NoError(t, err)

	assert.True(t, truncateTriggerExists, "TRUNCATE protection trigger must exist for SOX/GLBA compliance")

	// Validation 4: UPDATE/DELETE rules exist (immutability)
	var updateRuleExists bool
	err = db.QueryRowContext(context.Background(), `
		SELECT EXISTS(
			SELECT 1
			FROM pg_rules
			WHERE tablename = 'audit_events'
			AND rulename = 'prevent_audit_event_update'
		)
	`).Scan(&updateRuleExists)
	require.NoError(t, err)

	assert.True(t, updateRuleExists, "UPDATE prevention rule must exist for immutability")

	var deleteRuleExists bool
	err = db.QueryRowContext(context.Background(), `
		SELECT EXISTS(
			SELECT 1
			FROM pg_rules
			WHERE tablename = 'audit_events'
			AND rulename = 'prevent_audit_event_delete'
		)
	`).Scan(&deleteRuleExists)
	require.NoError(t, err)

	assert.True(t, deleteRuleExists, "DELETE prevention rule must exist for immutability")

	// Log compliance summary
	t.Log("✓ SOX/GLBA Compliance Validation:")
	t.Log("  - No automatic deletion triggers")
	t.Log("  - TRUNCATE protection active")
	t.Log("  - UPDATE/DELETE rules enforced")
	t.Log("  - Audit events are immutable and retained per regulatory requirements")
}

// TestAuditEvents_11_11_2_ActorInformationCaptured tests that actor information is captured.
func TestAuditEvents_11_11_2_ActorInformationCaptured(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Perform any operation
	ruleName := "actor-test-" + testutil.MustDeterministicUUID(7030).String()[:8]
	ruleReq := testutil.RuleRequest{
		Name:       ruleName,
		Expression: "true",
		Action:     "ALLOW",
	}
	body, _ := json.Marshal(ruleReq)

	req, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var rule testutil.RuleResponse
	json.NewDecoder(resp.Body).Decode(&rule)
	defer testutil.CleanupRule(t, rule.ID)

	time.Sleep(100 * time.Millisecond)

	// Query audit event
	auditReq, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?resource_id="+rule.ID, nil)
	auditReq.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(auditReq)
	require.NoError(t, err)
	defer auditResp.Body.Close()

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	json.NewDecoder(auditResp.Body).Decode(&result)

	require.NotEmpty(t, result.AuditEvents)

	event := result.AuditEvents[0]
	actor := event["actor"].(map[string]any)

	// Validate actor information per SOX/GLBA compliance
	assert.NotEmpty(t, actor["actorType"], "actorType must be present")
	assert.Contains(t, []string{"user", "api_key", "system"}, actor["actorType"],
		"actorType must be valid (user from JWT, api_key from API-Key deployment label, "+
			"or system for background events)")
	assert.NotEmpty(t, actor["id"], "actor.id must be present")
	// actor.name is required for human principals (actorType=user). API-key principals
	// legitimately carry an empty name — the label encodes the deployment, not a
	// person — and system actors set a subsystem label. Only assert name presence on
	// the user path, where its absence would mean we lost the human identity.
	if actor["actorType"] == "user" {
		assert.NotEmpty(t, actor["name"], "actor.name must be present for user principals")
	}

	assert.NotEmpty(t, actor["ipAddress"], "actor.ipAddress must be present for traceability")
	// actor.role is optional
}

// ============================================================================
// Hash Chain Integrity Tests (11.5)
// ============================================================================

// TestAuditEvents_11_5_1_HashChainIntactAfterMultipleOperations tests hash chain integrity.
// Clears audit_events first to ensure hash chain integrity (other tests may have
// created events with cleanup operations that corrupt the chain).
func TestAuditEvents_11_5_1_HashChainIntactAfterMultipleOperations(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()
	db := testutil.SetupIntegrationDB(t)

	// Clean slate: remove all audit events to ensure hash chain integrity.
	resetAuditEvents(t, db)

	// Perform multiple operations to generate events
	for i := 0; i < 5; i++ {
		ruleName := fmt.Sprintf("chain-test-%d-%s", i, testutil.MustDeterministicUUID(int64(7300 + i)).String()[:8])
		ruleReq := testutil.RuleRequest{
			Name:       ruleName,
			Expression: "true",
			Action:     "ALLOW",
		}
		body, _ := json.Marshal(ruleReq)

		req, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, _ := testutil.HTTPClient.Do(req)
		var rule testutil.RuleResponse
		json.NewDecoder(resp.Body).Decode(&rule)
		resp.Body.Close()

		defer testutil.CleanupRule(t, rule.ID)
	}

	time.Sleep(200 * time.Millisecond)

	// Get last event
	var lastEventID string
	err := db.QueryRowContext(context.Background(),
		`SELECT event_id FROM audit_events ORDER BY id DESC LIMIT 1`,
	).Scan(&lastEventID)
	require.NoError(t, err)

	// Verify chain
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/"+lastEventID+"/verify", nil)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var verifyResult struct {
		IsValid      bool  `json:"isValid"`
		TotalChecked int64 `json:"totalChecked"`
	}
	json.NewDecoder(resp.Body).Decode(&verifyResult)

	assert.True(t, verifyResult.IsValid, "Hash chain must be valid")
	assert.GreaterOrEqual(t, verifyResult.TotalChecked, int64(5), "Should check at least 5 events")
}

// TestAuditEvents_11_5_2_FirstEventHasGenesisHash tests first event in chain.
func TestAuditEvents_11_5_2_FirstEventHasGenesisHash(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()
	db := testutil.SetupIntegrationDB(t)

	// Setup: Create a rule to generate an audit event (ensures we have at least one event)
	ruleName := "Genesis Hash Test " + testutil.MustDeterministicUUID(7103).String()[:8]
	ruleID := testutil.CreateTestRuleWithExpression(t, ruleName, "amount > 90", "DENY")
	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Get the first event from database (by sequence, not by our test)
	var eventID string
	err := db.QueryRowContext(context.Background(),
		`SELECT event_id FROM audit_events ORDER BY id ASC LIMIT 1`,
	).Scan(&eventID)
	require.NoError(t, err, "Should have at least one audit event after rule creation")

	// Get event details via API
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events/"+eventID, nil)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var firstEvent map[string]any
	json.NewDecoder(resp.Body).Decode(&firstEvent)

	// First event should have null or empty previousHash (genesis)
	previousHash := firstEvent["previousHash"]
	assert.True(t, previousHash == nil || previousHash == "" || previousHash == "GENESIS",
		"First event should have null, empty, or GENESIS previous hash")
	assert.NotEmpty(t, firstEvent["hash"], "First event should have hash calculated")
}

// TestAuditEvents_11_5_3_SubsequentEventsChainCorrectly tests chain linking.
func TestAuditEvents_11_5_3_SubsequentEventsChainCorrectly(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Setup: Create 2 rules to generate at least 2 audit events
	rule1Name := "Chain Test Rule 1 " + testutil.MustDeterministicUUID(7104).String()[:8]
	rule1ID := testutil.CreateTestRuleWithExpression(t, rule1Name, "amount > 100", "DENY")
	t.Cleanup(func() {
		testutil.CleanupRule(t, rule1ID)
	})

	rule2Name := "Chain Test Rule 2 " + testutil.MustDeterministicUUID(7105).String()[:8]
	rule2ID := testutil.CreateTestRuleWithExpression(t, rule2Name, "amount < 1", "ALLOW")
	t.Cleanup(func() {
		testutil.CleanupRule(t, rule2ID)
	})

	// Get first 10 events in order
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?sort_by=created_at&sort_order=ASC&limit=10", nil)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	require.GreaterOrEqual(t, len(result.AuditEvents), 2, "Should have at least 2 audit events after creating 2 rules")

	// Verify each event's previousHash matches previous event's hash
	for i := 1; i < len(result.AuditEvents); i++ {
		prevEvent := result.AuditEvents[i-1]
		currEvent := result.AuditEvents[i]

		prevHash := prevEvent["hash"].(string)
		currPrevHash := currEvent["previousHash"]

		assert.NotNil(t, currPrevHash, "Event %d should have previousHash", i)
		assert.Equal(t, prevHash, currPrevHash,
			"Event %d previousHash should match event %d hash", i, i-1)
	}
}

// ============================================================================
// 11.6 JSONB Filtering Tests
// ============================================================================

// TestAuditEvents_11_6_1_FiltersBySegmentId tests filtering validation events by segmentId.
func TestAuditEvents_11_6_1_FiltersBySegmentId(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create validation with segmentId
	segmentID := testutil.MustDeterministicUUID(7031).String()
	validationReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(7032).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("10"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID:     testutil.MustDeterministicUUID(7033).String(),
			Type:   "checking",
			Status: "active",
		},
		Segment: &testutil.SegmentContext{
			ID:   segmentID,
			Name: "Test Segment",
		},
	}

	resp, _ := testutil.CreateValidation(t, validationReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	time.Sleep(100 * time.Millisecond)

	// Filter by segmentId
	url := fmt.Sprintf("%s/v1/audit-events?segment_id=%s&event_type=TRANSACTION_VALIDATED", baseURL, segmentID)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer auditResp.Body.Close()

	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	json.NewDecoder(auditResp.Body).Decode(&result)

	require.NotEmpty(t, result.AuditEvents, "Should find audit event for segmentId")

	// Verify JSONB filtering
	for _, event := range result.AuditEvents {
		context := event["context"].(map[string]any)
		request := context["request"].(map[string]any)

		// SegmentId can be in account.segmentId or segment.segmentId
		segmentFound := false
		if account, ok := request["account"].(map[string]any); ok {
			if accSegID, exists := account["segmentId"]; exists && accSegID == segmentID {
				segmentFound = true
			}
		}
		if segment, ok := request["segment"].(map[string]any); ok {
			if segID, exists := segment["segmentId"]; exists && segID == segmentID {
				segmentFound = true
			}
		}

		assert.True(t, segmentFound, "Event should contain segmentId in account or segment")
	}
}

// TestAuditEvents_11_6_2_FiltersByPortfolioId tests filtering validation events by portfolioId.
func TestAuditEvents_11_6_2_FiltersByPortfolioId(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create validation with portfolioId
	portfolioID := testutil.MustDeterministicUUID(7034).String()
	validationReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(7035).String(),
		TransactionType:      "CARD",
		Amount:               decimal.RequireFromString("20"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID:     testutil.MustDeterministicUUID(7036).String(),
			Type:   "checking",
			Status: "active",
		},
		Portfolio: &testutil.PortfolioContext{
			ID:   portfolioID,
			Name: "Test Portfolio",
		},
	}

	resp, _ := testutil.CreateValidation(t, validationReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	time.Sleep(100 * time.Millisecond)

	// Filter by portfolioId
	url := fmt.Sprintf("%s/v1/audit-events?portfolio_id=%s&event_type=TRANSACTION_VALIDATED", baseURL, portfolioID)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer auditResp.Body.Close()

	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	json.NewDecoder(auditResp.Body).Decode(&result)

	require.NotEmpty(t, result.AuditEvents, "Should find audit event for portfolioId")

	// Verify JSONB filtering
	for _, event := range result.AuditEvents {
		context := event["context"].(map[string]any)
		request := context["request"].(map[string]any)

		// PortfolioId can be in account.portfolioId or portfolio.portfolioId
		portfolioFound := false
		if account, ok := request["account"].(map[string]any); ok {
			if accPortID, exists := account["portfolioId"]; exists && accPortID == portfolioID {
				portfolioFound = true
			}
		}
		if portfolio, ok := request["portfolio"].(map[string]any); ok {
			if portID, exists := portfolio["portfolioId"]; exists && portID == portfolioID {
				portfolioFound = true
			}
		}

		assert.True(t, portfolioFound, "Event should contain portfolioId in account or portfolio")
	}
}

// TestAuditEvents_11_6_3_CombinesMultipleJSONBFilters tests combining multiple JSONB filters.
func TestAuditEvents_11_6_3_CombinesMultipleJSONBFilters(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create validation with multiple identifiers
	accountID := testutil.MustDeterministicUUID(7037).String()
	segmentID := testutil.MustDeterministicUUID(7038).String()
	portfolioID := testutil.MustDeterministicUUID(7039).String()

	validationReq := &testutil.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(7040).String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString("50"),
		Currency:             "BRL",
		TransactionTimestamp: testutil.FixedTime().Format(time.RFC3339),
		Account: &testutil.AccountContext{
			ID:     accountID,
			Type:   "checking",
			Status: "active",
		},
		Segment: &testutil.SegmentContext{
			ID:   segmentID,
			Name: "Test Segment",
		},
		Portfolio: &testutil.PortfolioContext{
			ID:   portfolioID,
			Name: "Test Portfolio",
		},
	}

	resp, _ := testutil.CreateValidation(t, validationReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	time.Sleep(100 * time.Millisecond)

	// Filter by multiple JSONB fields (AND logic)
	url := fmt.Sprintf("%s/v1/audit-events?account_id=%s&transaction_type=PIX&segment_id=%s&event_type=TRANSACTION_VALIDATED",
		baseURL, accountID, segmentID)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-API-Key", apiKey)

	auditResp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer auditResp.Body.Close()

	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	json.NewDecoder(auditResp.Body).Decode(&result)

	require.NotEmpty(t, result.AuditEvents, "Should find events matching all filters")

	// Verify ALL filters match (AND logic)
	for _, event := range result.AuditEvents {
		context := event["context"].(map[string]any)
		request := context["request"].(map[string]any)
		account := request["account"].(map[string]any)

		assert.Equal(t, accountID, account["id"], "Should match accountId")
		assert.Equal(t, "PIX", request["transactionType"], "Should match transactionType")

		// Verify segmentId in account or segment
		segmentFound := false
		if accSegID, exists := account["segmentId"]; exists && accSegID == segmentID {
			segmentFound = true
		}
		if segment, ok := request["segment"].(map[string]any); ok {
			if segID, exists := segment["segmentId"]; exists && segID == segmentID {
				segmentFound = true
			}
		}
		assert.True(t, segmentFound, "Should match segmentId in account or segment")
	}
}

// TestAuditEvents_11_2_14_FiltersByEventTypeRuleDrafted tests filtering by eventType=RULE_DRAFTED.
func TestAuditEvents_11_2_14_FiltersByEventTypeRuleDrafted(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a rule (starts in DRAFT)
	ruleName := "draft-filter-evt-" + testutil.MustDeterministicUUID(7041).String()[:8]
	ruleReq := testutil.RuleRequest{
		Name:       ruleName,
		Expression: "true",
		Action:     "ALLOW",
	}
	body, err := json.Marshal(ruleReq)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var rule testutil.RuleResponse
	err = json.NewDecoder(createResp.Body).Decode(&rule)
	require.NoError(t, err)
	require.NotEmpty(t, rule.ID)
	defer testutil.CleanupRule(t, rule.ID)

	// Activate rule (DRAFT → ACTIVE)
	activateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+rule.ID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)
	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()
	require.Equal(t, http.StatusOK, activateResp.StatusCode)

	// Deactivate rule (ACTIVE → INACTIVE)
	deactivateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+rule.ID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)
	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	deactivateResp.Body.Close()
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode)

	// Draft rule (INACTIVE → DRAFT) — generates RULE_DRAFTED event
	draftReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+rule.ID+"/draft", nil)
	require.NoError(t, err)
	draftReq.Header.Set("X-API-Key", apiKey)
	draftResp, err := testutil.HTTPClient.Do(draftReq)
	require.NoError(t, err)
	draftResp.Body.Close()
	require.Equal(t, http.StatusOK, draftResp.StatusCode)

	time.Sleep(100 * time.Millisecond)

	// Filter by eventType=RULE_DRAFTED scoped to this resource
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?event_type=RULE_DRAFTED&resource_id="+rule.ID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	require.NotEmpty(t, result.AuditEvents, "Should find RULE_DRAFTED event")

	for _, event := range result.AuditEvents {
		assert.Equal(t, "RULE_DRAFTED", event["eventType"])
	}

	// Verify before/after states in the first event
	require.GreaterOrEqual(t, len(result.AuditEvents), 1, "Need at least one event to inspect")

	event := result.AuditEvents[0]

	ctxVal, ok := event["context"].(map[string]any)
	require.True(t, ok, "context should be a map")

	before, ok := ctxVal["before"].(map[string]any)
	require.True(t, ok, "before should be a map")

	after, ok := ctxVal["after"].(map[string]any)
	require.True(t, ok, "after should be a map")

	assert.Equal(t, "INACTIVE", before["status"])
	assert.Equal(t, "DRAFT", after["status"])
}

// TestAuditEvents_11_2_15_FiltersByActionDraft tests filtering by action=DRAFT.
func TestAuditEvents_11_2_15_FiltersByActionDraft(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create a rule (starts in DRAFT)
	ruleName := "draft-filter-act-" + testutil.MustDeterministicUUID(7042).String()[:8]
	ruleReq := testutil.RuleRequest{
		Name:       ruleName,
		Expression: "true",
		Action:     "ALLOW",
	}
	body, err := json.Marshal(ruleReq)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var rule testutil.RuleResponse
	err = json.NewDecoder(createResp.Body).Decode(&rule)
	require.NoError(t, err)
	require.NotEmpty(t, rule.ID)
	defer testutil.CleanupRule(t, rule.ID)

	// Activate rule (DRAFT → ACTIVE)
	activateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+rule.ID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)
	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	activateResp.Body.Close()
	require.Equal(t, http.StatusOK, activateResp.StatusCode)

	// Deactivate rule (ACTIVE → INACTIVE)
	deactivateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+rule.ID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)
	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	deactivateResp.Body.Close()
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode)

	// Draft rule (INACTIVE → DRAFT) — generates event with action=DRAFT
	draftReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+rule.ID+"/draft", nil)
	require.NoError(t, err)
	draftReq.Header.Set("X-API-Key", apiKey)
	draftResp, err := testutil.HTTPClient.Do(draftReq)
	require.NoError(t, err)
	draftResp.Body.Close()
	require.Equal(t, http.StatusOK, draftResp.StatusCode)

	time.Sleep(100 * time.Millisecond)

	// Filter by action=DRAFT scoped to this resource
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?action=DRAFT&resource_id="+rule.ID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	require.NotEmpty(t, result.AuditEvents, "Should find event with action=DRAFT")

	for _, event := range result.AuditEvents {
		assert.Equal(t, "DRAFT", event["action"])
	}
}

// TestAuditEvents_11_2_16_FiltersByEventTypeLimitDrafted tests filtering by eventType=LIMIT_DRAFTED.
func TestAuditEvents_11_2_16_FiltersByEventTypeLimitDrafted(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create limit (starts in DRAFT)
	accountID := testutil.MustDeterministicUUID(7043).String()
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	defer testutil.CleanupLimit(t, limitID)

	// Activate limit (DRAFT → ACTIVE)
	testutil.ActivateLimit(t, limitID)

	// Deactivate limit (ACTIVE → INACTIVE)
	deactivateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)
	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	deactivateResp.Body.Close()
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode)

	// Draft limit (INACTIVE → DRAFT) — generates LIMIT_DRAFTED event
	testutil.DraftLimit(t, limitID)

	time.Sleep(100 * time.Millisecond)

	// Filter by eventType=LIMIT_DRAFTED scoped to this resource
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?event_type=LIMIT_DRAFTED&resource_id="+limitID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	require.NotEmpty(t, result.AuditEvents, "Should find LIMIT_DRAFTED event")

	for _, event := range result.AuditEvents {
		assert.Equal(t, "LIMIT_DRAFTED", event["eventType"])
	}

	// Verify before/after states in the first event
	event := result.AuditEvents[0]
	ctx := event["context"].(map[string]any)
	before := ctx["before"].(map[string]any)
	after := ctx["after"].(map[string]any)

	assert.Equal(t, "INACTIVE", before["status"])
	assert.Equal(t, "DRAFT", after["status"])
}

// TestAuditEvents_11_2_17_FiltersByActionDraftForLimit tests filtering by action=DRAFT for limits.
func TestAuditEvents_11_2_17_FiltersByActionDraftForLimit(t *testing.T) {
	apiKey := testutil.GetAPIKey()
	baseURL := testutil.GetBaseURL()

	// Create limit (starts in DRAFT)
	accountID := testutil.MustDeterministicUUID(7044).String()
	limitID := testutil.CreateLimitWithAccountScope(t, accountID, "1000")
	defer testutil.CleanupLimit(t, limitID)

	// Activate limit (DRAFT → ACTIVE)
	testutil.ActivateLimit(t, limitID)

	// Deactivate limit (ACTIVE → INACTIVE)
	deactivateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits/"+limitID+"/deactivate", nil)
	require.NoError(t, err)
	deactivateReq.Header.Set("X-API-Key", apiKey)
	deactivateResp, err := testutil.HTTPClient.Do(deactivateReq)
	require.NoError(t, err)
	deactivateResp.Body.Close()
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode)

	// Draft limit (INACTIVE → DRAFT) — generates event with action=DRAFT
	testutil.DraftLimit(t, limitID)

	time.Sleep(100 * time.Millisecond)

	// Filter by action=DRAFT scoped to this resource
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/audit-events?action=DRAFT&resource_id="+limitID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	require.NotEmpty(t, result.AuditEvents, "Should find event with action=DRAFT for limit")

	for _, event := range result.AuditEvents {
		assert.Equal(t, "DRAFT", event["action"])
	}
}
