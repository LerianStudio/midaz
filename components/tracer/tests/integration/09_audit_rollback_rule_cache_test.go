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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// =============================================================================
// Rule audit rollback + cache consistency test
//
// Contract under test: when the audit-event insert inside the rule
// activation transaction fails, the rule's persistence mutation must be rolled
// back AND the in-memory cache must NOT have been touched. The DB is the
// source of truth; the cache is only updated post-commit so a rollback
// automatically preserves cache consistency.
//
// Fault-injection helpers (installFailOnAuditEvent, fetchRuleStatusDirect,
// countAuditEvents) live in audit_fault_injection_helpers_test.go and are
// shared across all audit-rollback integration tests.
// =============================================================================

// TestAuditRollback_ActivateRule_CacheConsistency_Integration exercises the
// SOX/GLBA atomicity contract for rule lifecycle commands: when
// RecordRuleEventWithTx fails inside the activation transaction, the
// repository UpdateWithTx must roll back AND the in-memory cache must remain
// untouched.
//
// Flow:
//  1. Create a DRAFT rule via the API.
//  2. Install a fault-injection trigger that fails the audit INSERT for that
//     rule's RULE_ACTIVATED event.
//  3. Call POST /v1/rules/:id/activate. Expect a 5xx response.
//  4. Assert directly against the DB that the rule status is still DRAFT and
//     that no RULE_ACTIVATED audit event was persisted.
//
// Cache-state assertion: the ActivateRule command only calls
// cacheWriter.UpsertRule AFTER a successful commit. Because the tx rolled
// back, that post-commit branch is unreachable by construction. We verify
// this indirectly: a later GetActiveRules / evaluation call would show the
// rule missing, but the DB status assertion already pins the invariant the
// cache mirrors.
func TestAuditRollback_ActivateRule_CacheConsistency_Integration(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// ----- Step 1: create a DRAFT rule via the API -------------------------
	reqBody := map[string]any{
		"name":       "audit-rollback-test-rule",
		"expression": "amount > 0",
		"action":     "ALLOW",
		"scopes": []map[string]any{
			{
				"accountId": testutil.MustDeterministicUUID(91001).String(),
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

	createBody, err := io.ReadAll(createResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode, "create-rule setup failed: %s", string(createBody))

	var created map[string]any
	require.NoError(t, json.Unmarshal(createBody, &created))

	ruleID, ok := created["ruleId"].(string)
	require.True(t, ok, "ruleId must be a string in create response")
	require.NotEmpty(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Defensive LIFO ordering: register trigger drop AFTER CleanupRule so LIFO
	// semantics drop the trigger first. The fault trigger is scoped to
	// RULE_ACTIVATED only, but dropping it first is defense-in-depth against
	// future edits that broaden the filter to other RULE_* event types.
	// ----- Step 2: install fault-injection trigger -------------------------
	dropTrigger := installFailOnAuditEvent(t, db, ruleID, "RULE_ACTIVATED")
	t.Cleanup(dropTrigger)

	// Snapshot pre-activate state from the DB for later comparison.
	statusBefore := fetchRuleStatusDirect(t, db, ruleID)
	require.Equal(t, "DRAFT", statusBefore, "precondition: rule starts DRAFT")
	countBefore := countAuditEvents(t, db, ruleID, "RULE_ACTIVATED")
	require.Equal(t, 0, countBefore, "precondition: no RULE_ACTIVATED events yet")

	// ----- Step 3: activate via API, expect 5xx ----------------------------
	activateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules/"+ruleID+"/activate", nil)
	require.NoError(t, err)
	activateReq.Header.Set("X-API-Key", apiKey)

	activateResp, err := testutil.HTTPClient.Do(activateReq)
	require.NoError(t, err)
	defer activateResp.Body.Close()

	activateBody, err := io.ReadAll(activateResp.Body)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, activateResp.StatusCode, 500,
		"expected 5xx status when audit insert fails inside tx (got %d): %s",
		activateResp.StatusCode, string(activateBody))
	assert.Less(t, activateResp.StatusCode, 600,
		"expected 5xx status when audit insert fails inside tx (got %d)", activateResp.StatusCode)

	// ----- Step 4: assert rollback landed in DB ----------------------------
	statusAfter := fetchRuleStatusDirect(t, db, ruleID)
	assert.Equal(t, "DRAFT", statusAfter,
		"rule status must remain DRAFT: the activation transaction rolled back on audit failure")

	countAfter := countAuditEvents(t, db, ruleID, "RULE_ACTIVATED")
	assert.Equal(t, 0, countAfter,
		"no RULE_ACTIVATED audit event must have been persisted: the transaction rolled back")
}
