// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// =============================================================================
// Limit audit rollback test
//
// Mirrors the rule-side contract exercised by
// TestAuditRollback_ActivateRule_CacheConsistency_Integration: when the
// audit-event insert inside the limit activation transaction fails, the
// limit's status mutation must be rolled back and no LIMIT_ACTIVATED audit
// event must be persisted. Limits do not have an in-memory cache today, so
// the DB status is the full source of truth for the invariant.
//
// Fault-injection helpers come from audit_fault_injection_helpers_test.go.
// =============================================================================

// TestAuditRollback_ActivateLimit_Integration verifies that a failure of the
// audit INSERT inside the limit activation transaction leaves the limit in
// its pre-transaction DRAFT status with no audit event persisted.
//
// Flow:
//  1. Create a DRAFT limit via the API (default status is DRAFT).
//  2. Register cleanup for the limit BEFORE the fault-injection trigger
//     cleanup. t.Cleanup runs in LIFO order, so the trigger is dropped
//     FIRST — the current trigger only filters LIMIT_ACTIVATED, but
//     dropping it before CleanupLimit runs is defense-in-depth against
//     future edits that widen the filter to other LIMIT_* event types.
//  3. Install a BEFORE INSERT trigger that fails audit inserts for this
//     limit's LIMIT_ACTIVATED event.
//  4. Call POST /v1/limits/{id}/activate. Expect a 5xx response.
//  5. Assert directly against the DB that the limit status is still DRAFT
//     and that no LIMIT_ACTIVATED audit event was persisted.
func TestAuditRollback_ActivateLimit_Integration(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// ----- Step 1: create a DRAFT limit via the API -----------------------
	accountID := testutil.MustDeterministicUUID(120100).String()
	limitID := testutil.CreateLimitWithScope(
		t,
		"audit-rollback-test-limit-"+testutil.RandomSuffix(),
		"1000",
		[]testutil.ScopeInput{{AccountID: &accountID}},
	)

	// Register limit cleanup BEFORE trigger cleanup. t.Cleanup runs in LIFO
	// order so the trigger is dropped first, and CleanupLimit then sees a
	// trigger-free audit_events table and can deactivate / delete normally.
	t.Cleanup(func() {
		testutil.CleanupLimit(t, limitID)
	})

	// Defensive LIFO ordering: register trigger drop AFTER CleanupLimit so LIFO
	// semantics drop the trigger first. The fault trigger is scoped to
	// LIMIT_ACTIVATED only, but dropping it first is defense-in-depth against
	// future edits that broaden the filter to other LIMIT_* event types.
	// ----- Step 2: install fault-injection trigger ------------------------
	dropTrigger := installFailOnAuditEvent(t, db, limitID, "LIMIT_ACTIVATED")
	t.Cleanup(dropTrigger)

	// Snapshot pre-activate state from the DB for later comparison.
	statusBefore := fetchLimitStatusDirect(t, db, limitID)
	require.Equal(t, "DRAFT", statusBefore, "precondition: limit starts DRAFT")
	countBefore := countAuditEvents(t, db, limitID, "LIMIT_ACTIVATED")
	require.Equal(t, 0, countBefore, "precondition: no LIMIT_ACTIVATED events yet")

	// ----- Step 3: activate via API, expect 5xx ---------------------------
	activateReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/limits/"+limitID+"/activate", nil)
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

	// ----- Step 4: assert rollback landed in DB ---------------------------
	statusAfter := fetchLimitStatusDirect(t, db, limitID)
	assert.Equal(t, "DRAFT", statusAfter,
		"limit status must remain DRAFT: the activation transaction rolled back on audit failure")

	countAfter := countAuditEvents(t, db, limitID, "LIMIT_ACTIVATED")
	assert.Equal(t, 0, countAfter,
		"no LIMIT_ACTIVATED audit event must have been persisted: the transaction rolled back")
}
