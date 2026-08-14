// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Shared audit fault-injection helpers for the integration suite.
//
// These helpers install / inspect PostgreSQL triggers that deliberately fail
// INSERTs on audit_events for a specific (resource_id, event_type) pair. They
// exist to exercise the SOX/GLBA atomicity contract for lifecycle commands:
// when the audit-event insert inside a state-transition transaction fails, the
// repository mutation AND any post-commit side effect (e.g. in-memory cache
// update) must be rolled back / never observed.
//
// Fault-injection approach:
//
//	A BEFORE INSERT trigger on audit_events raises an exception whenever a row
//	for the target resource_id with the target event_type is inserted. The
//	trigger is installed in test setup and dropped via t.Cleanup (runs on both
//	pass and fail paths, so a crashing test cannot leak it into subsequent
//	tests). This localized schema override is the correct hook point because
//	the integration suite boots the full production binary via
//	bootstrap.InitServers() — there is no in-process seam to wrap the audit
//	writer with a fault-injection proxy, but there is privileged access to the
//	backing PostgreSQL container.
// =============================================================================

// validAuditEventTypes is the closed set of event types accepted by the
// installFailOnAuditEvent and installFailOnAuditEventByType helpers.
// Restricting the allowed values here keeps the interpolation path inside the
// trigger body watertight: even if a future refactor widens the trust
// boundary of the caller, the helpers will still reject anything outside
// this set.
var validAuditEventTypes = map[string]struct{}{
	"RULE_CREATED":      {},
	"RULE_UPDATED":      {},
	"RULE_ACTIVATED":    {},
	"RULE_DEACTIVATED":  {},
	"RULE_DRAFTED":      {},
	"RULE_DELETED":      {},
	"LIMIT_CREATED":     {},
	"LIMIT_UPDATED":     {},
	"LIMIT_ACTIVATED":   {},
	"LIMIT_DEACTIVATED": {},
	"LIMIT_DRAFTED":     {},
	"LIMIT_DELETED":     {},
}

// pgQuoteLiteral returns a SQL string literal with the input embedded verbatim
// after single-quote doubling. Inputs inside this test package come from
// deterministic UUIDs / whitelisted constants, not user input, but we still
// quote defensively so the helper cannot become a SQL-injection footgun if
// the inputs grow.
//
// Assumes PostgreSQL's default standard_conforming_strings = on (modern
// default). Does not handle the legacy E-string (backslash-escape) semantics.
func pgQuoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// installFailOnAuditEvent installs a one-shot BEFORE INSERT trigger on
// audit_events that raises an exception whenever a row for the given
// resourceID with the given eventType is inserted. Returns a cleanup function
// that drops the trigger and its backing function; register it with
// t.Cleanup so a test crash does not leak the trigger into later tests.
//
// Defense-in-depth:
//   - resourceID must be a canonical UUID — identifier derivation below
//     interpolates it verbatim via strings.ReplaceAll, which is NOT
//     SQL-escapeable, so we reject anything that is not a valid UUID.
//   - eventType must be a known audit event type from validAuditEventTypes.
//     The value is also quoted via pgQuoteLiteral before being embedded in
//     the trigger body, which makes the trigger safe even if the whitelist
//     is later relaxed.
func installFailOnAuditEvent(t *testing.T, db *sql.DB, resourceID string, eventType string) func() {
	t.Helper()

	parsed, perr := uuid.Parse(resourceID)
	if perr != nil {
		t.Fatalf("installFailOnAuditEvent: invalid resourceID %q: %v", resourceID, perr)
	}

	// Force the canonical 36-char hyphenated form. uuid.Parse also accepts
	// URN ("urn:uuid:..."), braced ("{...}"), and 32-hex-digit variants; the
	// downstream ReplaceAll(..., "-", "") would otherwise leave characters
	// like ':' or '{' in the SQL identifier derivation below.
	resourceID = parsed.String()

	if _, ok := validAuditEventTypes[eventType]; !ok {
		t.Fatalf("installFailOnAuditEvent: unsupported eventType %q (extend validAuditEventTypes)", eventType)
	}

	// Use deterministic identifiers tied to the (resourceID, eventType) pair
	// so concurrent runs (should they ever occur) would not collide on the
	// trigger namespace.
	safeResource := strings.ReplaceAll(resourceID, "-", "")
	safeEventType := strings.ToLower(eventType)
	fn := fmt.Sprintf("test_fail_audit_%s_%s", safeEventType, safeResource)
	trig := fmt.Sprintf("test_trg_fail_audit_%s_%s", safeEventType, safeResource)

	createFn := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s() RETURNS TRIGGER AS $$
		BEGIN
		    IF NEW.resource_id = %s AND NEW.event_type = %s THEN
		        RAISE EXCEPTION 'test-injected audit failure for resource %% event_type %%', NEW.resource_id, NEW.event_type;
		    END IF;
		    RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`, fn, pgQuoteLiteral(resourceID), pgQuoteLiteral(eventType))

	createTrig := fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE INSERT ON audit_events
		FOR EACH ROW EXECUTE FUNCTION %s();
	`, trig, fn)

	_, err := db.ExecContext(context.Background(), createFn)
	require.NoError(t, err, "install fault-injection function")

	_, err = db.ExecContext(context.Background(), createTrig)
	require.NoError(t, err, "install fault-injection trigger")

	return func() {
		// Drop trigger first, then function. Tolerant of partial install failures.
		_, _ = db.ExecContext(context.Background(), fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON audit_events", trig))
		_, _ = db.ExecContext(context.Background(), fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", fn))
	}
}

// fetchRuleStatusDirect reads the current status column of a rule row using a
// direct DB query (bypassing the API) so the assertion is independent of the
// HTTP error the caller is trying to validate.
func fetchRuleStatusDirect(t *testing.T, db *sql.DB, ruleID string) string {
	t.Helper()

	var status string
	err := db.QueryRowContext(context.Background(), `SELECT status FROM rules WHERE id = $1`, ruleID).Scan(&status)
	require.NoError(t, err, "read rule status directly from DB")

	return status
}

// fetchLimitStatusDirect reads the current status column of a limit row using
// a direct DB query (bypassing the API) so the assertion is independent of
// the HTTP error the caller is trying to validate.
func fetchLimitStatusDirect(t *testing.T, db *sql.DB, limitID string) string {
	t.Helper()

	var status string
	err := db.QueryRowContext(context.Background(), `SELECT status FROM limits WHERE id = $1`, limitID).Scan(&status)
	require.NoError(t, err, "read limit status directly from DB")

	return status
}

// countAuditEvents counts audit_events rows for the given resource_id and
// event_type. Used to assert that NO audit event landed when a transaction
// rolled back, or exactly one landed on a successful state transition.
func countAuditEvents(t *testing.T, db *sql.DB, resourceID string, eventType string) int {
	t.Helper()

	var count int
	err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM audit_events WHERE resource_id = $1 AND event_type = $2`,
		resourceID, eventType,
	).Scan(&count)
	require.NoError(t, err, "count audit events")

	return count
}

// installFailOnAuditEventByType installs a one-shot BEFORE INSERT trigger on
// audit_events that raises an exception whenever a row with the given
// eventType is inserted, REGARDLESS of resource_id. Returns a cleanup
// function that drops the trigger and its backing function; register it with
// t.Cleanup so a test crash does not leak the trigger into later tests.
//
// Use this variant when the resource_id is not knowable up front — namely
// the CREATE flows, where the server generates the rule/limit ID inside the
// transaction we are about to fail. For lifecycle transitions on existing
// resources (ACTIVATE, UPDATE, DELETE, DRAFT) prefer the resource-id-scoped
// installFailOnAuditEvent so the trigger does not affect concurrent inserts
// for unrelated resources.
//
// This helper is safe with the integration suite's -p=1 sequencing: only one
// test runs at a time, so the trigger cannot collide with audit inserts from
// a sibling test. The trigger body is filtered by event_type only, NOT by
// resource_id, so callers should ensure that the test's setup does not
// itself depend on inserting an event of the same type while the trigger is
// active.
func installFailOnAuditEventByType(t *testing.T, db *sql.DB, eventType string) func() {
	t.Helper()

	if _, ok := validAuditEventTypes[eventType]; !ok {
		t.Fatalf("installFailOnAuditEventByType: unsupported eventType %q (extend validAuditEventTypes)", eventType)
	}

	// Identifier is keyed on (eventType, deterministic per-test suffix) so two
	// cleanup calls do not collide on the trigger namespace if a future change
	// ever runs tests in parallel. Suffix is derived from (test name, eventType)
	// so reruns produce stable names that ease debugging.
	deterministicSuffix := uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name()+"|"+eventType)).String()
	uniqueSuffix := strings.ReplaceAll(deterministicSuffix, "-", "")
	safeEventType := strings.ToLower(eventType)
	fn := fmt.Sprintf("test_fail_audit_bytype_%s_%s", safeEventType, uniqueSuffix)
	trig := fmt.Sprintf("test_trg_fail_audit_bytype_%s_%s", safeEventType, uniqueSuffix)

	createFn := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s() RETURNS TRIGGER AS $$
		BEGIN
		    IF NEW.event_type = %s THEN
		        RAISE EXCEPTION 'test-injected audit failure for event_type %%', NEW.event_type;
		    END IF;
		    RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`, fn, pgQuoteLiteral(eventType))

	createTrig := fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE INSERT ON audit_events
		FOR EACH ROW EXECUTE FUNCTION %s();
	`, trig, fn)

	_, err := db.ExecContext(context.Background(), createFn)
	require.NoError(t, err, "install fault-injection function")

	_, err = db.ExecContext(context.Background(), createTrig)
	require.NoError(t, err, "install fault-injection trigger")

	return func() {
		// Drop trigger first, then function. Tolerant of partial install failures.
		_, _ = db.ExecContext(context.Background(), fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON audit_events", trig))
		_, _ = db.ExecContext(context.Background(), fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", fn))
	}
}

// countAuditEventsByName returns the number of audit events whose context.after.name JSONB
// field equals the given name. Depends on AuditEvent.context being JSONB with shape
// {before, after, reason} and after.name being set by RuleToMap / LimitToMap. Breaks silently
// if either contract changes — the happy-path control tests in this package mitigate that risk.
//
// Used by CREATE rollback assertions where the resource_id is not observable (the rule/limit
// was never persisted, so there is nothing to match resource_id against).
func countAuditEventsByName(t *testing.T, db *sql.DB, eventType string, resourceName string) int {
	t.Helper()

	var count int
	err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM audit_events
		 WHERE event_type = $1 AND context->'after'->>'name' = $2`,
		eventType, resourceName,
	).Scan(&count)
	require.NoError(t, err, "count audit events by name")

	return count
}

// fetchRuleIDByName returns the UUID of a rule with the given name, or an
// empty string if no such row exists. Used by CREATE rollback assertions
// where the test does not learn the rule ID via the API (because the API
// call returned a 5xx).
func fetchRuleIDByName(t *testing.T, db *sql.DB, name string) string {
	t.Helper()

	var id string
	err := db.QueryRowContext(context.Background(),
		`SELECT id FROM rules WHERE name = $1`, name,
	).Scan(&id)

	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}

	require.NoError(t, err, "fetch rule id by name")

	return id
}

// fetchLimitIDByName returns the UUID of a limit with the given name, or an
// empty string if no such row exists. Mirror of fetchRuleIDByName for the
// limit-side rollback assertions.
func fetchLimitIDByName(t *testing.T, db *sql.DB, name string) string {
	t.Helper()

	var id string
	err := db.QueryRowContext(context.Background(),
		`SELECT id FROM limits WHERE name = $1`, name,
	).Scan(&id)

	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}

	require.NoError(t, err, "fetch limit id by name")

	return id
}

// fetchRuleSnapshot reads (name, description, expression, action) for the
// given ruleID. Used by UPDATE rollback assertions to verify that no
// mutation landed when the audit insert failed.
func fetchRuleSnapshot(t *testing.T, db *sql.DB, ruleID string) (name, description, expression, action string) {
	t.Helper()

	var nullDescription sql.NullString

	err := db.QueryRowContext(context.Background(),
		`SELECT name, description, expression, action FROM rules WHERE id = $1`, ruleID,
	).Scan(&name, &nullDescription, &expression, &action)
	require.NoError(t, err, "fetch rule snapshot")

	if nullDescription.Valid {
		description = nullDescription.String
	}

	return name, description, expression, action
}
