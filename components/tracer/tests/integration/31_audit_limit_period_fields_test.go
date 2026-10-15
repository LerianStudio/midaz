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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// periodFieldKeys are the limit fields that decide whether a transaction
// counts in a period. A limit audit snapshot always carries them, with nil
// when the limit does not set one, so before/after can be diffed key by key.
var periodFieldKeys = []string{
	"activeTimeStart",
	"activeTimeEnd",
	"resetTime",
	"customStartDate",
	"customEndDate",
}

// TestAuditLimitPeriodFields_WindowUpdate verifies that the audit trail of a
// DAILY limit with a time window and a reset time records the window, the
// reset time and a resetAt on the reset-time boundary, both on creation and
// on a window update.
func TestAuditLimitPeriodFields_WindowUpdate(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(7106).String()
	limitID := createLimitWithTimeWindowAndResetTime(t, accountID, "09:00", "17:00", "03:30", "1000.00")
	defer testutil.CleanupLimit(t, limitID)

	patchLimitFields(t, limitID, map[string]any{
		"activeTimeStart": "10:00",
		"activeTimeEnd":   "18:00",
	})

	createdCtx := fetchLimitAuditContext(t, limitID, "CREATE", "LIMIT_CREATED")
	assert.Nil(t, createdCtx["before"], "CREATE has no before snapshot")

	created := requireSnapshot(t, createdCtx, "after")
	for _, key := range periodFieldKeys {
		_, ok := created[key]
		require.True(t, ok, "LIMIT_CREATED after must carry %q", key)
	}

	assert.Equal(t, "09:00", created["activeTimeStart"])
	assert.Equal(t, "17:00", created["activeTimeEnd"])
	assert.Equal(t, "03:30", created["resetTime"])
	requireNilKey(t, created, "customStartDate")
	requireNilKey(t, created, "customEndDate")
	requireResetAtOnBoundary(t, created, 3, 30)

	updatedCtx := fetchLimitAuditContext(t, limitID, "UPDATE", "LIMIT_UPDATED")
	before := requireSnapshot(t, updatedCtx, "before")
	after := requireSnapshot(t, updatedCtx, "after")

	assert.Equal(t, "09:00", before["activeTimeStart"])
	assert.Equal(t, "17:00", before["activeTimeEnd"])
	assert.Equal(t, "10:00", after["activeTimeStart"])
	assert.Equal(t, "18:00", after["activeTimeEnd"])

	// resetTime is immutable on update, so it is identical on both sides.
	assert.Equal(t, "03:30", before["resetTime"])
	assert.Equal(t, "03:30", after["resetTime"])

	for _, snapshot := range []map[string]any{before, after} {
		requireNilKey(t, snapshot, "customStartDate")
		requireNilKey(t, snapshot, "customEndDate")
	}

	requireResetAtOnBoundary(t, before, 3, 30)
	requireResetAtOnBoundary(t, after, 3, 30)
}

// TestAuditLimitPeriodFields_CustomPeriodUpdate verifies that moving the end
// of a CUSTOM period is recorded in the audit trail with both dates and a
// resetAt on the day after each end date.
func TestAuditLimitPeriodFields_CustomPeriodUpdate(t *testing.T) {
	const (
		startDate      = "2030-01-01T00:00:00Z"
		endDate        = "2030-01-31T23:59:59Z"
		updatedEndDate = "2030-02-15T23:59:59Z"
	)

	accountID := testutil.MustDeterministicUUID(7107).String()
	limitID := createLimitWithCustomPeriod(t, accountID, startDate, endDate, "10000.00")
	defer testutil.CleanupLimit(t, limitID)

	// The custom dates are validated as a pair, so the unchanged start date
	// is sent together with the new end date.
	patchLimitFields(t, limitID, map[string]any{
		"customStartDate": startDate,
		"customEndDate":   updatedEndDate,
	})

	updatedCtx := fetchLimitAuditContext(t, limitID, "UPDATE", "LIMIT_UPDATED")
	before := requireSnapshot(t, updatedCtx, "before")
	after := requireSnapshot(t, updatedCtx, "after")

	assertSameInstant(t, startDate, requireTimeKey(t, before, "customStartDate"))
	assertSameInstant(t, startDate, requireTimeKey(t, after, "customStartDate"))
	assertSameInstant(t, endDate, requireTimeKey(t, before, "customEndDate"))
	assertSameInstant(t, updatedEndDate, requireTimeKey(t, after, "customEndDate"))

	assertSameInstant(t, "2030-02-01T00:00:00Z", requireTimeKey(t, before, "resetAt"))
	assertSameInstant(t, "2030-02-16T00:00:00Z", requireTimeKey(t, after, "resetAt"))

	for _, snapshot := range []map[string]any{before, after} {
		requireNilKey(t, snapshot, "resetTime")
		requireNilKey(t, snapshot, "activeTimeStart")
		requireNilKey(t, snapshot, "activeTimeEnd")
	}
}

// TestAuditLimitPeriodFields_PerTransactionCreate verifies that a limit with
// no period records every period field, and resetAt, as present and nil.
func TestAuditLimitPeriodFields_PerTransactionCreate(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(7108).String()
	limitID := testutil.CreateLimitWithAccountScopeAndType(t, accountID, "1000", "PER_TRANSACTION")
	defer testutil.CleanupLimit(t, limitID)

	createdCtx := fetchLimitAuditContext(t, limitID, "CREATE", "LIMIT_CREATED")
	after := requireSnapshot(t, createdCtx, "after")

	assert.Equal(t, "PER_TRANSACTION", after["limitType"])

	for _, key := range periodFieldKeys {
		requireNilKey(t, after, key)
	}

	requireNilKey(t, after, "resetAt")
}

// patchLimitFields sends PATCH /v1/limits/{id} and requires a 200, so an
// audit assertion never runs against an update the API refused.
func patchLimitFields(t *testing.T, limitID string, fields map[string]any) {
	t.Helper()

	body, err := json.Marshal(fields)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, testutil.GetBaseURL()+"/v1/limits/"+limitID, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", testutil.GetAPIKey())
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "PATCH limit should return 200: %s", string(respBody))
}

// fetchLimitAuditContext returns the context of the single audit event of the
// given action recorded for the limit, after checking its event type.
func fetchLimitAuditContext(t *testing.T, limitID, action, eventType string) map[string]any {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet,
		testutil.GetBaseURL()+"/v1/audit-events?resource_type=limit&action="+action+"&resource_id="+limitID, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", testutil.GetAPIKey())

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AuditEvents []map[string]any `json:"auditEvents"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.Len(t, result.AuditEvents, 1, "expected exactly one %s audit event for limit %s", action, limitID)

	event := result.AuditEvents[0]
	require.Equal(t, eventType, event["eventType"])

	auditContext, ok := event["context"].(map[string]any)
	require.True(t, ok, "audit event must carry a context object")

	return auditContext
}

// requireSnapshot returns context[side] as a map, failing when it is absent.
func requireSnapshot(t *testing.T, auditContext map[string]any, side string) map[string]any {
	t.Helper()

	snapshot, ok := auditContext[side].(map[string]any)
	require.True(t, ok, "audit context must carry a %q snapshot", side)

	return snapshot
}

// requireNilKey requires the key to be present with a nil value: absence and
// nil are different contracts for a consumer diffing before and after.
func requireNilKey(t *testing.T, snapshot map[string]any, key string) {
	t.Helper()

	v, ok := snapshot[key]
	require.True(t, ok, "snapshot must carry %q", key)
	assert.Nil(t, v, "snapshot %q must be nil", key)
}

// requireTimeKey parses snapshot[key] as RFC3339. The snapshot layout drops
// trailing zero fractions, so values are compared as instants, not strings.
func requireTimeKey(t *testing.T, snapshot map[string]any, key string) time.Time {
	t.Helper()

	raw, ok := snapshot[key].(string)
	require.True(t, ok, "snapshot %q must be a timestamp string, got %v", key, snapshot[key])

	return mustParseRFC3339(t, raw)
}

// requireResetAtOnBoundary checks a periodic limit's resetAt structurally: the
// suite runs on a real clock, so the exact instant is unknown, but resetAt
// must fall on the reset-time boundary and close a period that starts at or
// after the snapshot's updatedAt. An update's before snapshot carries the
// creation updatedAt but a resetAt taken at the update, so the bound allows
// the minute a create and its update may straddle a boundary.
func requireResetAtOnBoundary(t *testing.T, snapshot map[string]any, hour, minute int) {
	t.Helper()

	resetAt := requireTimeKey(t, snapshot, "resetAt").UTC()
	updatedAt := requireTimeKey(t, snapshot, "updatedAt")

	assert.Equal(t, hour, resetAt.Hour(), "resetAt hour")
	assert.Equal(t, minute, resetAt.Minute(), "resetAt minute")
	assert.Zero(t, resetAt.Second(), "resetAt second")
	assert.True(t, resetAt.After(updatedAt), "resetAt %s must be after updatedAt %s", resetAt, updatedAt)
	assert.LessOrEqual(t, resetAt.Sub(updatedAt), 24*time.Hour+time.Minute, "a DAILY period ends within a day")
}

func mustParseRFC3339(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err, "parse %q as RFC3339", value)

	return parsed
}

// assertSameInstant compares as instants so a location or monotonic-clock
// difference between two parses cannot fail an otherwise equal timestamp.
func assertSameInstant(t *testing.T, want string, got time.Time) {
	t.Helper()

	expected := mustParseRFC3339(t, want)
	assert.True(t, expected.Equal(got), "want %s, got %s", expected, got)
}
