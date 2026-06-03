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

	"tracer/internal/testutil"
)

// =============================================================================
// CREATE / UPDATE audit rollback tests (T-001 atomic-audit fix)
//
// Companion to 09_audit_rollback_rule_cache_test.go (rule activation) and
// 12_audit_rollback_limit_test.go (limit activation). Those tests cover the
// state-transition lifecycle commands (ACTIVATE). This file covers the
// remaining mutating commands inside the same SOX/GLBA atomicity contract:
//
//   - POST   /v1/rules    → CreateRule    → AuditEventRuleCreated
//   - PATCH  /v1/rules/id → UpdateRule    → AuditEventRuleUpdated
//   - POST   /v1/limits   → CreateLimit   → AuditEventLimitCreated
//
// Contract under test (per command): when the audit-event insert inside the
// command transaction fails, the repository mutation must roll back AND no
// audit row must be persisted. The DB is the source of truth.
//
// CREATE-specific challenge: the resource_id is generated server-side inside
// the same transaction we're about to fail. We cannot install a fault
// trigger keyed on resource_id because the resource never gets that far.
// We use installFailOnAuditEventByType (event_type-only filter) instead and
// verify rollback by querying for a row with the unique test name in the
// rules / limits table.
//
// The integration suite runs with -p=1 (Makefile mk/tests.mk:test-integration),
// so a global event_type trigger does not collide with sibling tests.
// =============================================================================

// TestAuditRollback_CreateRule_Integration verifies that when
// RecordRuleEventWithTx fails inside the rule creation transaction, the
// CreateWithTx insert is rolled back. Asserts directly against the DB that
// no rule row with the unique test name exists and that no RULE_CREATED
// audit event for that name was persisted.
//
// Flow:
//  1. Install a global fault-injection trigger for RULE_CREATED.
//  2. Call POST /v1/rules with a unique rule name. Expect a 5xx response.
//  3. Assert directly against the DB that:
//     - rules table has NO row with that name (the insert rolled back), and
//     - audit_events table has NO RULE_CREATED row referring to that name
//     (the audit insert that triggered the rollback did not commit).
//  4. Drop the trigger via t.Cleanup so the post-cleanup pass (re-create +
//     delete to keep the suite tidy) is not blocked.
func TestAuditRollback_CreateRule_Integration(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	uniqueRuleName := "audit-rollback-create-rule-" + testutil.RandomSuffix()

	// ----- Step 1: install fault-injection trigger ------------------------
	dropTrigger := installFailOnAuditEventByType(t, db, "RULE_CREATED")
	t.Cleanup(dropTrigger)

	// Pre-condition: the rule name is unused.
	require.Empty(t, fetchRuleIDByName(t, db, uniqueRuleName),
		"precondition: no rule with this unique test name exists")
	require.Equal(t, 0, countAuditEventsByName(t, db, "RULE_CREATED", uniqueRuleName),
		"precondition: no RULE_CREATED audit event for this name exists")

	// ----- Step 2: POST /v1/rules, expect 5xx -----------------------------
	accountID := testutil.MustDeterministicUUID(140001).String()
	reqBody := map[string]any{
		"name":       uniqueRuleName,
		"expression": "amount > 0",
		"action":     "ALLOW",
		"scopes": []map[string]any{
			{"accountId": accountID},
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

	assert.GreaterOrEqual(t, createResp.StatusCode, 500,
		"expected 5xx status when audit insert fails inside tx (got %d): %s",
		createResp.StatusCode, string(createBody))
	assert.Less(t, createResp.StatusCode, 600,
		"expected 5xx status when audit insert fails inside tx (got %d)", createResp.StatusCode)

	// ----- Step 3: assert rollback landed in DB ---------------------------
	assert.Empty(t, fetchRuleIDByName(t, db, uniqueRuleName),
		"no rule row must exist with the unique test name: the create transaction rolled back")
	assert.Equal(t, 0, countAuditEventsByName(t, db, "RULE_CREATED", uniqueRuleName),
		"no RULE_CREATED audit event must have been persisted: the transaction rolled back")
}

// TestAuditRollback_UpdateRule_Integration verifies that when
// RecordRuleEventWithTx fails inside the rule update transaction, the
// UpdateWithTx mutation is rolled back. Pre-creates a DRAFT rule, snapshots
// its (name, description, expression, action), installs a fault trigger
// scoped to the rule's UUID + RULE_UPDATED event, issues PATCH, and asserts
// that the snapshot is unchanged.
//
// Unlike the CREATE test, UPDATE has a knowable resource_id, so we use the
// resource-id-scoped installFailOnAuditEvent helper for tighter isolation.
func TestAuditRollback_UpdateRule_Integration(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// ----- Step 1: pre-create a DRAFT rule via the API --------------------
	originalName := "audit-rollback-update-rule-" + testutil.RandomSuffix()
	originalExpression := "amount > 0"
	originalAction := "ALLOW"
	accountID := testutil.MustDeterministicUUID(140101).String()

	createBody, err := json.Marshal(map[string]any{
		"name":       originalName,
		"expression": originalExpression,
		"action":     originalAction,
		"scopes": []map[string]any{
			{"accountId": accountID},
		},
	})
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(createBody))
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

	var created map[string]any
	require.NoError(t, json.Unmarshal(createRespBody, &created))

	ruleID, ok := created["ruleId"].(string)
	require.True(t, ok, "ruleId must be a string in create response")
	require.NotEmpty(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Snapshot pre-update DB state — assertions use this as the baseline.
	nameBefore, descBefore, exprBefore, actionBefore := fetchRuleSnapshot(t, db, ruleID)
	require.Equal(t, originalName, nameBefore)
	require.Equal(t, originalExpression, exprBefore)
	require.Equal(t, originalAction, actionBefore)

	// Defensive LIFO ordering: register trigger drop AFTER CleanupRule so
	// LIFO semantics drop the trigger first. The fault trigger is scoped to
	// RULE_UPDATED only, but dropping it first is defense-in-depth in case
	// a future cleanup edit ever updates the rule (which would re-fire the
	// trigger) before deleting it.
	// ----- Step 2: install fault-injection trigger ------------------------
	dropTrigger := installFailOnAuditEvent(t, db, ruleID, "RULE_UPDATED")
	t.Cleanup(dropTrigger)

	// ----- Step 3: PATCH /v1/rules/:id with new fields, expect 5xx --------
	updatedExpression := "amount > 999"
	updatedDescription := "this update must roll back"
	patchBody, err := json.Marshal(map[string]any{
		"expression":  updatedExpression,
		"description": updatedDescription,
	})
	require.NoError(t, err)

	patchReq, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(patchBody))
	require.NoError(t, err)
	patchReq.Header.Set("X-API-Key", apiKey)
	patchReq.Header.Set("Content-Type", "application/json")

	patchResp, err := testutil.HTTPClient.Do(patchReq)
	require.NoError(t, err)
	defer patchResp.Body.Close()

	patchRespBody, err := io.ReadAll(patchResp.Body)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, patchResp.StatusCode, 500,
		"expected 5xx status when audit insert fails inside tx (got %d): %s",
		patchResp.StatusCode, string(patchRespBody))
	assert.Less(t, patchResp.StatusCode, 600,
		"expected 5xx status when audit insert fails inside tx (got %d)", patchResp.StatusCode)

	// ----- Step 4: assert rollback landed in DB ---------------------------
	nameAfter, descAfter, exprAfter, actionAfter := fetchRuleSnapshot(t, db, ruleID)
	assert.Equal(t, nameBefore, nameAfter, "rule name must remain unchanged: update tx rolled back")
	assert.Equal(t, descBefore, descAfter, "rule description must remain unchanged: update tx rolled back")
	assert.Equal(t, exprBefore, exprAfter, "rule expression must remain unchanged: update tx rolled back")
	assert.Equal(t, actionBefore, actionAfter, "rule action must remain unchanged: update tx rolled back")

	assert.Equal(t, 0, countAuditEvents(t, db, ruleID, "RULE_UPDATED"),
		"no RULE_UPDATED audit event must have been persisted: the transaction rolled back")
}

// TestAuditRollback_CreateLimit_Integration mirrors
// TestAuditRollback_CreateRule_Integration on the limit side: when the audit
// insert for LIMIT_CREATED fails inside the limit creation transaction, no
// limit row must exist with the unique test name and no LIMIT_CREATED audit
// event for that name must be persisted.
func TestAuditRollback_CreateLimit_Integration(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	uniqueLimitName := "audit-rollback-create-limit-" + testutil.RandomSuffix()

	// ----- Step 1: install fault-injection trigger ------------------------
	dropTrigger := installFailOnAuditEventByType(t, db, "LIMIT_CREATED")
	t.Cleanup(dropTrigger)

	// Pre-condition.
	require.Empty(t, fetchLimitIDByName(t, db, uniqueLimitName),
		"precondition: no limit with this unique test name exists")
	require.Equal(t, 0, countAuditEventsByName(t, db, "LIMIT_CREATED", uniqueLimitName),
		"precondition: no LIMIT_CREATED audit event for this name exists")

	// ----- Step 2: POST /v1/limits, expect 5xx ----------------------------
	accountID := testutil.MustDeterministicUUID(140201).String()
	reqBody := map[string]any{
		"name":      uniqueLimitName,
		"limitType": "DAILY",
		"maxAmount": "1000",
		"currency":  "BRL",
		"scopes": []map[string]any{
			{"accountId": accountID},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createBody, err := io.ReadAll(createResp.Body)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, createResp.StatusCode, 500,
		"expected 5xx status when audit insert fails inside tx (got %d): %s",
		createResp.StatusCode, string(createBody))
	assert.Less(t, createResp.StatusCode, 600,
		"expected 5xx status when audit insert fails inside tx (got %d)", createResp.StatusCode)

	// ----- Step 3: assert rollback landed in DB ---------------------------
	assert.Empty(t, fetchLimitIDByName(t, db, uniqueLimitName),
		"no limit row must exist with the unique test name: the create transaction rolled back")
	assert.Equal(t, 0, countAuditEventsByName(t, db, "LIMIT_CREATED", uniqueLimitName),
		"no LIMIT_CREATED audit event must have been persisted: the transaction rolled back")
}

// TestAuditRollback_CreateRule_HappyPath_BothRowsPresent is the control test
// for TestAuditRollback_CreateRule_Integration: it verifies that without the
// fault trigger, a successful POST /v1/rules persists BOTH the rule row AND
// the RULE_CREATED audit event, proving that:
//
//  1. the assertions in the rollback test are not vacuously satisfied by a
//     misconfigured test environment (e.g. a stale fault trigger from a
//     prior crash, or audit events being silently disabled), and
//  2. the same atomic flow that fails-closed under fault injection also
//     succeeds-closed end-to-end under normal conditions.
//
// Without this control, a future regression that disables the audit insert
// entirely (e.g. by short-circuiting RecordRuleEventWithTx) would still
// pass TestAuditRollback_CreateRule_Integration — the rule wouldn't be
// inserted (because the createWithTx returns an error before reaching audit)
// or the audit count would still be 0 (because audit was never called),
// producing a green test for a broken atomicity contract. Asserting that the
// happy path persists exactly one rule and one audit row pins the lower
// bound of correctness.
func TestAuditRollback_CreateRule_HappyPath_BothRowsPresent(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	uniqueRuleName := "audit-happy-create-rule-" + testutil.RandomSuffix()
	accountID := testutil.MustDeterministicUUID(140301).String()

	reqBody := map[string]any{
		"name":       uniqueRuleName,
		"expression": "amount > 0",
		"action":     "ALLOW",
		"scopes": []map[string]any{
			{"accountId": accountID},
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
		"happy-path create must succeed: %s", string(createRespBody))

	var created map[string]any
	require.NoError(t, json.Unmarshal(createRespBody, &created))

	ruleID, ok := created["ruleId"].(string)
	require.True(t, ok, "ruleId must be a string in create response")
	require.NotEmpty(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// Both rows MUST exist post-commit.
	assert.Equal(t, ruleID, fetchRuleIDByName(t, db, uniqueRuleName),
		"rule row must exist with the API-returned id: the create transaction committed")
	assert.Equal(t, 1, countAuditEvents(t, db, ruleID, "RULE_CREATED"),
		"exactly one RULE_CREATED audit event must exist for the new rule: atomic commit")

	// And the rule's name on the audit row matches the rule's name on the
	// rules row — no impossible cross-name leak.
	assert.Equal(t, 1, countAuditEventsByName(t, db, "RULE_CREATED", uniqueRuleName),
		"the RULE_CREATED audit event must reference the same rule name as the rules row")
}

// TestAuditRollback_CreateLimit_HappyPath_BothRowsPresent is the control test
// for TestAuditRollback_CreateLimit_Integration. Without it, a regression that
// silently disables the LIMIT_CREATED audit insert (e.g. by short-circuiting
// RecordLimitEventWithTx) would still pass the rollback test — the limit
// wouldn't be inserted (because the createWithTx returns an error before
// reaching audit) or the audit count would still be 0 (because audit was
// never called) — producing a green test for a broken atomicity contract.
// Asserting that the happy path persists exactly one limit and one audit row
// pins the lower bound of correctness.
func TestAuditRollback_CreateLimit_HappyPath_BothRowsPresent(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	uniqueLimitName := "audit-happy-create-limit-" + testutil.RandomSuffix()
	accountID := testutil.MustDeterministicUUID(140401).String()

	reqBody := map[string]any{
		"name":      uniqueLimitName,
		"limitType": "DAILY",
		"maxAmount": "1000",
		"currency":  "BRL",
		"scopes": []map[string]any{
			{"accountId": accountID},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits", bytes.NewReader(body))
	require.NoError(t, err)
	createReq.Header.Set("X-API-Key", apiKey)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := testutil.HTTPClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()

	createRespBody, err := io.ReadAll(createResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode,
		"happy-path create must succeed: %s", string(createRespBody))

	var created map[string]any
	require.NoError(t, json.Unmarshal(createRespBody, &created))

	limitID, ok := created["limitId"].(string)
	require.True(t, ok, "limitId must be a string in create response")
	require.NotEmpty(t, limitID)

	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Both rows MUST exist post-commit.
	assert.Equal(t, limitID, fetchLimitIDByName(t, db, uniqueLimitName),
		"limit row must exist with the API-returned id: the create transaction committed")
	assert.Equal(t, 1, countAuditEvents(t, db, limitID, "LIMIT_CREATED"),
		"exactly one LIMIT_CREATED audit event must exist for the new limit: atomic commit")
	assert.Equal(t, 1, countAuditEventsByName(t, db, "LIMIT_CREATED", uniqueLimitName),
		"the LIMIT_CREATED audit event must reference the same limit name as the limits row")
}

// TestAuditRollback_UpdateRule_HappyPath_AuditEventPresent is the control test
// for TestAuditRollback_UpdateRule_Integration: it pre-creates a DRAFT rule,
// updates it normally (no fault injection), then asserts:
//
//  1. the rule row reflects the updated fields (commit landed), and
//  2. exactly one RULE_UPDATED audit event exists for the rule.
//
// Without this control, a regression that silently disables the RULE_UPDATED
// audit insert would still pass TestAuditRollback_UpdateRule_Integration: the
// rule wouldn't be updated (no audit error → no rollback → mutation lands)
// but a 0-count audit assertion would also be vacuously true.
func TestAuditRollback_UpdateRule_HappyPath_AuditEventPresent(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// ----- Step 1: pre-create a DRAFT rule via the API --------------------
	originalName := "audit-happy-update-rule-" + testutil.RandomSuffix()
	originalExpression := "amount > 0"
	originalAction := "ALLOW"
	accountID := testutil.MustDeterministicUUID(140501).String()

	createBody, err := json.Marshal(map[string]any{
		"name":       originalName,
		"expression": originalExpression,
		"action":     originalAction,
		"scopes": []map[string]any{
			{"accountId": accountID},
		},
	})
	require.NoError(t, err)

	createReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/rules", bytes.NewReader(createBody))
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

	var created map[string]any
	require.NoError(t, json.Unmarshal(createRespBody, &created))

	ruleID, ok := created["ruleId"].(string)
	require.True(t, ok, "ruleId must be a string in create response")
	require.NotEmpty(t, ruleID)

	t.Cleanup(func() {
		testutil.CleanupRule(t, ruleID)
	})

	// ----- Step 2: PATCH /v1/rules/:id with new fields, expect 200 --------
	updatedExpression := "amount > 999"
	updatedDescription := "happy-path update must commit"
	patchBody, err := json.Marshal(map[string]any{
		"expression":  updatedExpression,
		"description": updatedDescription,
	})
	require.NoError(t, err)

	patchReq, err := http.NewRequest(http.MethodPatch, baseURL+"/v1/rules/"+ruleID, bytes.NewReader(patchBody))
	require.NoError(t, err)
	patchReq.Header.Set("X-API-Key", apiKey)
	patchReq.Header.Set("Content-Type", "application/json")

	patchResp, err := testutil.HTTPClient.Do(patchReq)
	require.NoError(t, err)
	defer patchResp.Body.Close()

	patchRespBody, err := io.ReadAll(patchResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, patchResp.StatusCode,
		"happy-path update must succeed: %s", string(patchRespBody))

	// ----- Step 3: assert commit landed in DB -----------------------------
	_, descAfter, exprAfter, _ := fetchRuleSnapshot(t, db, ruleID)
	assert.Equal(t, updatedExpression, exprAfter,
		"rule expression must reflect the update: commit landed")
	assert.Equal(t, updatedDescription, descAfter,
		"rule description must reflect the update: commit landed")

	assert.Equal(t, 1, countAuditEvents(t, db, ruleID, "RULE_UPDATED"),
		"exactly one RULE_UPDATED audit event must exist for the updated rule: atomic commit")
}
