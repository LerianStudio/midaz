// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package support

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // registers pgx driver for database/sql

	"github.com/google/uuid"

	"tracer/internal/testutil"
)

// =============================================================================
// Audit fault-injection helpers for the E2E BDD suite.
//
// These helpers mirror the integration-suite helpers in
// tests/integration/audit_fault_injection_helpers_test.go, adapted for the
// BDD step layer: they return `error` instead of calling t.Fatalf so they
// compose cleanly with godog step definitions.
//
// Contract under test: when the audit-event insert inside a lifecycle
// transaction (rule activation, limit activation) fails, the repository
// mutation must be rolled back AND no audit event must be persisted. The
// helpers install a BEFORE INSERT trigger on audit_events that raises an
// exception for a specific (resource_id, event_type) pair. On teardown the
// scenario context drops every trigger it installed.
// =============================================================================

// validAuditEventTypes is the closed set of event types accepted by the
// InstallFailOnAuditEvent helper. Restricting the allowed values here keeps
// the interpolation path inside the trigger body watertight.
var validAuditEventTypes = map[string]struct{}{
	"RULE_ACTIVATED":    {},
	"RULE_DEACTIVATED":  {},
	"RULE_DRAFTED":      {},
	"RULE_DELETED":      {},
	"LIMIT_ACTIVATED":   {},
	"LIMIT_DEACTIVATED": {},
	"LIMIT_DRAFTED":     {},
	"LIMIT_DELETED":     {},
}

// pgQuoteLiteral returns a SQL string literal with the input embedded verbatim
// after single-quote doubling. Inputs in the BDD suite come from deterministic
// UUIDs / whitelisted event-type constants, but we quote defensively so the
// helper cannot become a SQL-injection footgun if the inputs grow.
//
// Assumes PostgreSQL's default standard_conforming_strings = on.
func pgQuoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// OpenTestDB opens a PostgreSQL connection using the DSN derived from the
// standard DB_* environment variables that drive both docker-compose and the
// integration/E2E suites. The caller owns the connection and must Close it.
func OpenTestDB() (*sql.DB, error) {
	dsn := testutil.GetTestDSN()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("pinging db: %w", err)
	}

	return db, nil
}

// InstallFailOnAuditEvent installs a one-shot BEFORE INSERT trigger on
// audit_events that raises an exception whenever a row for the given
// resourceID with the given eventType is inserted. Returns a cleanup function
// that drops the trigger and its backing function.
//
// Defense-in-depth:
//   - resourceID must be a canonical UUID — identifier derivation below
//     interpolates it verbatim via strings.ReplaceAll, which is NOT
//     SQL-escapeable, so we reject anything that is not a valid UUID.
//   - eventType must be a known audit event type from validAuditEventTypes.
//     The value is also quoted via pgQuoteLiteral before being embedded in
//     the trigger body.
func InstallFailOnAuditEvent(db *sql.DB, resourceID string, eventType string) (func(), error) {
	parsed, perr := uuid.Parse(resourceID)
	if perr != nil {
		return nil, fmt.Errorf("invalid resourceID %q: %w", resourceID, perr)
	}

	// Force the canonical 36-char hyphenated form. uuid.Parse also accepts
	// URN ("urn:uuid:..."), braced ("{...}"), and 32-hex-digit variants; the
	// downstream ReplaceAll(..., "-", "") would otherwise leave characters
	// like ':' or '{' in the SQL identifier derivation below.
	resourceID = parsed.String()

	if _, ok := validAuditEventTypes[eventType]; !ok {
		return nil, fmt.Errorf("unsupported eventType %q", eventType)
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

	ctx := context.Background()

	if _, err := db.ExecContext(ctx, createFn); err != nil {
		return nil, fmt.Errorf("install fault-injection function: %w", err)
	}

	if _, err := db.ExecContext(ctx, createTrig); err != nil {
		// Clean up the orphan function before returning the error.
		_, _ = db.ExecContext(ctx, fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", fn))

		return nil, fmt.Errorf("install fault-injection trigger: %w", err)
	}

	cleanup := func() {
		// Drop trigger first, then function. Tolerant of partial install failures.
		_, _ = db.ExecContext(ctx, fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON audit_events", trig))
		_, _ = db.ExecContext(ctx, fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", fn))
	}

	return cleanup, nil
}

// FetchRuleStatusDirect reads the current status column of a rule row using a
// direct DB query so the assertion is independent of any HTTP error the
// lifecycle call produced.
func FetchRuleStatusDirect(db *sql.DB, ruleID string) (string, error) {
	var status string
	if err := db.QueryRowContext(context.Background(),
		`SELECT status FROM rules WHERE id = $1`, ruleID,
	).Scan(&status); err != nil {
		return "", fmt.Errorf("reading rule status: %w", err)
	}

	return status, nil
}

// FetchLimitStatusDirect reads the current status column of a limit row
// directly from the DB.
func FetchLimitStatusDirect(db *sql.DB, limitID string) (string, error) {
	var status string
	if err := db.QueryRowContext(context.Background(),
		`SELECT status FROM limits WHERE id = $1`, limitID,
	).Scan(&status); err != nil {
		return "", fmt.Errorf("reading limit status: %w", err)
	}

	return status, nil
}

// CountAuditEventsDirect counts audit_events rows for the given resource_id
// and event_type. Used to assert that NO audit event landed when a
// transaction rolled back, or exactly one landed on a successful state
// transition.
func CountAuditEventsDirect(db *sql.DB, resourceID, eventType string) (int, error) {
	var count int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM audit_events WHERE resource_id = $1 AND event_type = $2`,
		resourceID, eventType,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting audit events: %w", err)
	}

	return count, nil
}

// ActivateRuleRawE performs POST /v1/rules/{id}/activate and returns the raw
// HTTP status + body without treating non-2xx as an error. Use this for
// negative-path tests that need to assert on 5xx responses (fault injection).
func ActivateRuleRawE(ruleID string) (int, []byte, error) {
	resp, body, err := doRequestE(http.MethodPost, GetBaseURL()+"/v1/rules/"+ruleID+"/activate", nil, authHeaders())
	if err != nil {
		return 0, nil, err
	}

	return resp.StatusCode, body, nil
}

// ActivateLimitRawE performs POST /v1/limits/{id}/activate and returns the
// raw HTTP status + body without treating non-2xx as an error.
func ActivateLimitRawE(limitID string) (int, []byte, error) {
	resp, body, err := doRequestE(http.MethodPost, GetBaseURL()+"/v1/limits/"+limitID+"/activate", nil, authHeaders())
	if err != nil {
		return 0, nil, err
	}

	return resp.StatusCode, body, nil
}
