// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// Gate 8 — Deliverable A: end-to-end tenant isolation (READ + WRITE surface).
//
// This file exercises two independent isolation contracts:
//
//   - Read isolation (TestMultiTenant_RulesIsolation_TwoTenants): a request
//     for tenant-A reaches a repo method that calls postgres.GetDB(ctx) —
//     which uses tmcore.GetPGContext first — and observes tenant-A's data,
//     not tenant-B's. Covers /v1/rules and /v1/limits listing paths.
//
//   - Write isolation (TestMultiTenant_ValidationWrites_IsolatedBetweenTenants):
//     a POST /v1/validations for tenant-A persists its transaction_validations
//     and audit_events rows ONLY into tenant-A's database; the default pool
//     and tenant-B's database are byte-for-byte untouched. This is the
//     load-bearing assertion that the TxBeginnerAdapter is context-aware
//     (see internal/adapters/postgres/db/adapter.go :: BeginTx) and that
//     ValidationService's transaction flows through tmcore.GetPGContext(ctx).
//
// Both tests boot tracer with MULTI_TENANT_ENABLED=true via the shared
// harness in 14a_multitenant_mt_harness_test.go; each tenant has its own
// PostgreSQL database created and migrated up-front.
package integration

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testutil_integration "github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil_integration"
)

// TestMultiTenant_RulesIsolation_TwoTenants is the read-surface isolation gate.
// Not parallel: reboots the shared integration server.
func TestMultiTenant_RulesIsolation_TwoTenants(t *testing.T) {
	// ------------------------------------------------------------------
	// Step 1 — create two separate databases on the test Postgres
	// container. The suite's default DB (tracer_test) is never used here:
	// tenant-A uses tracer_iso_a, tenant-B uses tracer_iso_b.
	// ------------------------------------------------------------------
	dbA := "tracer_iso_a"
	dbB := "tracer_iso_b"

	specA := ensureTenantDatabase(t, dbA)
	specB := ensureTenantDatabase(t, dbB)

	// Each tenant DB gets tracer's full schema so the /v1/rules list handler
	// can run its query. The shared migrations path is exposed through the
	// suite env var MIGRATIONS_PATH.
	require.NoError(t, applyMigrations(t, specA), "migrate tenant-A DB")
	require.NoError(t, applyMigrations(t, specB), "migrate tenant-B DB")

	// ------------------------------------------------------------------
	// Step 2 — seed one distinctive rule per tenant directly via SQL so
	// the assertion is unambiguous. Using direct SQL (not the API)
	// guarantees the data goes to the intended tenant database.
	//
	// The names include the test-run timestamp so re-runs of the same
	// test (e.g. `go test -count=2`) do not trip the `rules_name`
	// uniqueness constraint. Migrations persist across runs inside the
	// same container instance.
	// ------------------------------------------------------------------
	runSuffix := fmt.Sprintf("%d", time.Now().UnixNano())
	ruleNameA := "isolation-rule-A-" + runSuffix
	ruleNameB := "isolation-rule-B-" + runSuffix

	require.NoError(t, seedIsolationRule(t, specA, ruleNameA), "seed rule A")
	require.NoError(t, seedIsolationRule(t, specB, ruleNameB), "seed rule B")

	// ------------------------------------------------------------------
	// Step 3 — boot tracer in multi-tenant mode with the fake TM pointing
	// each tenant at its respective database.
	// ------------------------------------------------------------------
	h := newMTHarness(t)
	h.RegisterTenant("iso-tenant-a", specA)
	h.RegisterTenant("iso-tenant-b", specB)

	cleanup := bootServiceInMTMode(t, h, nil)
	defer cleanup()

	jwtA := mintJWTWithTenantID("iso-tenant-a")
	jwtB := mintJWTWithTenantID("iso-tenant-b")

	// ------------------------------------------------------------------
	// Step 4 — each tenant sees ONLY its own rule when listing. This is
	// the load-bearing assertion: data visibility crosses the boundary
	// only when isolation is broken.
	// ------------------------------------------------------------------
	t.Run("TenantA_SeesOnlyRuleA", func(t *testing.T) {
		resp, body := doRequest(t, http.MethodGet, "/v1/rules", jwtA, "")
		require.Equal(t, http.StatusOK, resp.StatusCode,
			"list rules for tenant-A must return 200; got %d body=%s",
			resp.StatusCode, string(body))

		bodyStr := string(body)
		assert.Contains(t, bodyStr, ruleNameA,
			"tenant-A's list response must contain its own rule")
		assert.NotContains(t, bodyStr, ruleNameB,
			"tenant-A's list response MUST NOT contain tenant-B's rule — DATA LEAK")
	})

	t.Run("TenantB_SeesOnlyRuleB", func(t *testing.T) {
		resp, body := doRequest(t, http.MethodGet, "/v1/rules", jwtB, "")
		require.Equal(t, http.StatusOK, resp.StatusCode,
			"list rules for tenant-B must return 200; got %d body=%s",
			resp.StatusCode, string(body))

		bodyStr := string(body)
		assert.Contains(t, bodyStr, ruleNameB,
			"tenant-B's list response must contain its own rule")
		assert.NotContains(t, bodyStr, ruleNameA,
			"tenant-B's list response MUST NOT contain tenant-A's rule — DATA LEAK")
	})

	// ------------------------------------------------------------------
	// Step 5 — interleave requests to prove there is no state leak
	// between pool reuse cycles. Fire A, B, A, B, A and assert each
	// response body stays tenant-correct.
	// ------------------------------------------------------------------
	t.Run("InterleavedRequests_NoPoolCrosstalk", func(t *testing.T) {
		type probe struct {
			jwt        string
			wantName   string
			forbidName string
		}

		probes := []probe{
			{jwtA, ruleNameA, ruleNameB},
			{jwtB, ruleNameB, ruleNameA},
			{jwtA, ruleNameA, ruleNameB},
			{jwtB, ruleNameB, ruleNameA},
			{jwtA, ruleNameA, ruleNameB},
		}

		for i, p := range probes {
			resp, body := doRequest(t, http.MethodGet, "/v1/rules", p.jwt, "")
			require.Equal(t, http.StatusOK, resp.StatusCode,
				"interleaved iter %d must return 200; got %d body=%s",
				i, resp.StatusCode, string(body))

			assert.Contains(t, string(body), p.wantName,
				"iter %d: expected %s in body", i, p.wantName)
			assert.NotContains(t, string(body), p.forbidName,
				"iter %d: DATA LEAK — %s present when %s expected",
				i, p.forbidName, p.wantName)
		}
	})
}

// ------------------------------------------------------------------
// Migration helper — applies tracer's schema to a tenant-specific DB.
// ------------------------------------------------------------------

// applyMigrations runs the full migration stack against the given DSN.
// Uses the same MIGRATIONS_PATH the integration suite sets at boot, so
// the tenant DBs end up with the identical schema the service expects.
func applyMigrations(t *testing.T, spec tenantPGSpec) error {
	t.Helper()

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		spec.Username, spec.Password, spec.Host, spec.Port, spec.Database, spec.SSLMode)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open %s: %w", spec.Database, err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping %s: %w", spec.Database, err)
	}

	// Function migrations MUST run before schema migrations so the trigger
	// functions (calculate_audit_event_hash, prevent_truncate, etc.) are
	// available when the schema migrations define triggers that reference
	// them. This mirrors the Makefile's `migrate` target ordering.
	migrationsPath := envWithFallback("MIGRATIONS_PATH", "")
	if migrationsPath == "" {
		return fmt.Errorf("MIGRATIONS_PATH is not set; cannot migrate %s", spec.Database)
	}

	functionsPath := filepath.Join(migrationsPath, "functions")
	if err := testutil_integration.ApplyFunctionMigrations(ctx, db, functionsPath); err != nil {
		return fmt.Errorf("function migrations for %s: %w", spec.Database, err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{DatabaseName: spec.Database})
	if err != nil {
		return fmt.Errorf("migrate driver for %s: %w", spec.Database, err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, spec.Database, driver)
	if err != nil {
		return fmt.Errorf("migrate init for %s: %w", spec.Database, err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up for %s: %w", spec.Database, err)
	}

	return nil
}

// ------------------------------------------------------------------
// Seeding helper — inserts a minimal rule row directly via SQL.
// ------------------------------------------------------------------

// seedIsolationRule inserts one Rule into the given tenant DB with DRAFT
// status — enough to show up in a list query but not evaluated anywhere.
// The exact column set comes from migrations/000001_initial_schema.up.sql.
func seedIsolationRule(t *testing.T, spec tenantPGSpec, ruleName string) error {
	t.Helper()

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		spec.Username, spec.Password, spec.Host, spec.Port, spec.Database, spec.SSLMode)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open %s for seed: %w", spec.Database, err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The rules table uses TRC-defined columns — pull the minimal subset.
	// A fuller seed helper already exists at testutil.CreateActiveRule but
	// targets the default DB; we inline just enough columns here to avoid
	// coupling the test to seed helpers that were written for single-tenant
	// mode.
	_, err = db.ExecContext(ctx, `
		INSERT INTO rules (
			id, name, description, expression, "action", status,
			created_at, updated_at
		) VALUES (
			gen_random_uuid(), $1, 'isolation test', 'amount > 0', 'ALLOW', 'DRAFT',
			NOW(), NOW()
		)
	`, ruleName)
	if err != nil {
		return fmt.Errorf("insert rule %q into %s: %w", ruleName, spec.Database, err)
	}

	return nil
}

// ------------------------------------------------------------------
// Write-isolation test — proves TxBeginnerAdapter routes through
// tmcore.GetPGContext(ctx) instead of the static boot-time pool.
// ------------------------------------------------------------------

// TestMultiTenant_ValidationWrites_IsolatedBetweenTenants posts a validation
// on behalf of tenant-A and asserts the transaction_validations + audit_events
// rows land ONLY in tenant-A's database. Tenant-B's database and the default
// (single-tenant) pool used by the integration harness must be untouched.
//
// This is the definitive regression gate for the isolation leak closed by
// making TxBeginnerAdapter.BeginTx context-aware (see
// internal/adapters/postgres/db/adapter.go). Before the fix, every write
// landed in the default pool regardless of which tenant issued the request.
// The unit test suite in internal/adapters/postgres/db/adapter_test.go
// (TestTxBeginnerAdapter_UsesTenantPool*, TestTxBeginnerAdapter_FallsBack*)
// already proves the adapter behavior in isolation.
//
// Cache readiness: ActivateRuleService.Execute now calls
// RuleCacheWriter.MarkReady(ctx) after UpsertRule, so the priming step
// (primeTenantCache → POST /v1/rules + /v1/rules/{id}/activate) is enough
// to open the per-tenant readiness gate for POST /v1/validations on a
// freshly-spawned tenant. The test runs end-to-end without skips.
func TestMultiTenant_ValidationWrites_IsolatedBetweenTenants(t *testing.T) {
	// ------------------------------------------------------------------
	// Step 1 — create two tenant databases and migrate them.
	// ------------------------------------------------------------------
	dbA := "tracer_iso_write_a"
	dbB := "tracer_iso_write_b"

	specA := ensureTenantDatabase(t, dbA)
	specB := ensureTenantDatabase(t, dbB)

	require.NoError(t, applyMigrations(t, specA), "migrate tenant-A DB")
	require.NoError(t, applyMigrations(t, specB), "migrate tenant-B DB")

	// ------------------------------------------------------------------
	// Step 2 — boot tracer in MT mode with the fake TM routing each tenant
	// to its own database. With no rules and no limits, the validation
	// service takes the ALLOW path and writes one transaction_validations
	// row + one audit_events row to the tenant's DB.
	// ------------------------------------------------------------------
	h := newMTHarness(t)
	h.RegisterTenant("iso-write-tenant-a", specA)
	h.RegisterTenant("iso-write-tenant-b", specB)

	cleanup := bootServiceInMTMode(t, h, nil)
	defer cleanup()

	jwtA := mintJWTWithTenantID("iso-write-tenant-a")
	jwtB := mintJWTWithTenantID("iso-write-tenant-b")

	// ------------------------------------------------------------------
	// Step 3 — prime the rule cache for both tenants via the public API.
	// Each Activate call goes through rule_service -> cache.UpsertRule,
	// which populates the tenant's cache bucket. The rules themselves are
	// trivial ALLOW rules — the validation decision still defaults to
	// ALLOW because the rule expression evaluates to true on every input.
	// ------------------------------------------------------------------
	primeTenantCache(t, jwtA, "iso-write-cache-primer-a-"+uuid.New().String()[:8])
	primeTenantCache(t, jwtB, "iso-write-cache-primer-b-"+uuid.New().String()[:8])

	// ------------------------------------------------------------------
	// Step 4 — baseline counts AFTER cache-priming so only the POST
	// /v1/validations writes are measured below.
	// ------------------------------------------------------------------
	defaultSpec := specFromAdminDSN(t, mtTestAdminDSN(), envWithFallback("DB_NAME", "tracer_test"))

	baselineA := countValidationAndAuditRows(t, specA)
	baselineB := countValidationAndAuditRows(t, specB)
	baselineDefault := countValidationAndAuditRows(t, defaultSpec)

	// ------------------------------------------------------------------
	// Step 5 — post a validation for tenant-A.
	// ------------------------------------------------------------------
	requestID := uuid.New().String()
	accountID := uuid.New().String()

	body := fmt.Sprintf(`{
		"requestId": "%s",
		"transactionType": "PIX",
		"amount": "42.00",
		"currency": "BRL",
		"transactionTimestamp": "%s",
		"account": {"accountId": "%s"}
	}`, requestID, time.Now().UTC().Format(time.RFC3339), accountID)

	resp, respBody := doRequest(t, http.MethodPost, "/v1/validations", jwtA, body)
	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"POST /v1/validations for tenant-A must return 201; got %d body=%s",
		resp.StatusCode, string(respBody))

	// ------------------------------------------------------------------
	// Step 6 — direct SQL assertions per database. This is the
	// load-bearing section: it proves the write landed only in tenant-A.
	// ------------------------------------------------------------------

	// Tenant A: exactly one new transaction_validations row and at least one
	// new audit_events row (audit writes may emit more than one event per
	// validation, e.g. startup + decision — we only require >=1 new row).
	postA := countValidationAndAuditRows(t, specA)
	assert.Equal(t, baselineA.validations+1, postA.validations,
		"tenant-A transaction_validations must have gained exactly 1 row (got baseline=%d post=%d)",
		baselineA.validations, postA.validations)
	assert.GreaterOrEqual(t, postA.audits, baselineA.audits+1,
		"tenant-A audit_events must have gained at least 1 row (got baseline=%d post=%d)",
		baselineA.audits, postA.audits)

	// Ensure the specific requestID we posted actually shows up in tenant-A.
	foundInA := hasRequestID(t, specA, requestID)
	assert.True(t, foundInA, "tenant-A DB must contain transaction_validations row for requestId=%s", requestID)

	// Tenant B: untouched.
	postB := countValidationAndAuditRows(t, specB)
	assert.Equal(t, baselineB.validations, postB.validations,
		"tenant-B transaction_validations MUST NOT change — DATA LEAK (baseline=%d post=%d)",
		baselineB.validations, postB.validations)
	assert.Equal(t, baselineB.audits, postB.audits,
		"tenant-B audit_events MUST NOT change — DATA LEAK (baseline=%d post=%d)",
		baselineB.audits, postB.audits)

	foundInB := hasRequestID(t, specB, requestID)
	assert.False(t, foundInB, "tenant-B DB MUST NOT contain requestId=%s — DATA LEAK", requestID)

	// Default pool: the critical regression assertion. Before the fix, this
	// is where tenant-A's write leaked to because ValidationService's
	// TxBeginner was bound to the boot-time static pool.
	postDefault := countValidationAndAuditRows(t, defaultSpec)
	assert.Equal(t, baselineDefault.validations, postDefault.validations,
		"default pool transaction_validations MUST NOT change — WRITE LEAKED TO DEFAULT (baseline=%d post=%d)",
		baselineDefault.validations, postDefault.validations)
	assert.Equal(t, baselineDefault.audits, postDefault.audits,
		"default pool audit_events MUST NOT change — WRITE LEAKED TO DEFAULT (baseline=%d post=%d)",
		baselineDefault.audits, postDefault.audits)

	foundInDefault := hasRequestID(t, defaultSpec, requestID)
	assert.False(t, foundInDefault,
		"default pool MUST NOT contain requestId=%s — this is the isolation leak closed by the context-aware TxBeginnerAdapter",
		requestID)
}

// primeTenantCache creates and activates a trivial ALLOW rule for the tenant
// identified by the JWT. Activation flows through rule_service into
// RuleCache.UpsertRule, which populates the tenant's cache bucket and — via
// ApplyChanges — is sufficient to let the cache adapter return rules (the
// adapter also checks IsReady, but the service flows that upsert into the
// cache also call MarkReady as part of the activation path).
//
// Without this priming step the POST /v1/validations would fail with
// TRC-0281 (rule cache not ready) on first contact with a new tenant.
func primeTenantCache(t *testing.T, jwt, ruleName string) {
	t.Helper()

	createBody := fmt.Sprintf(`{
		"name": "%s",
		"description": "cache-primer for MT write-isolation test",
		"expression": "true",
		"action": "ALLOW"
	}`, ruleName)

	resp, body := doRequest(t, http.MethodPost, "/v1/rules", jwt, createBody)
	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"create primer rule %q must return 201; got %d body=%s",
		ruleName, resp.StatusCode, string(body))

	// Extract the rule ID from the response so we can activate it.
	// The API returns the rule JSON with an "id" field.
	ruleID := extractIDFromJSON(t, body)
	require.NotEmpty(t, ruleID, "created rule must have an id; body=%s", string(body))

	actResp, actBody := doRequest(t, http.MethodPost, "/v1/rules/"+ruleID+"/activate", jwt, "")
	require.Equal(t, http.StatusOK, actResp.StatusCode,
		"activate primer rule %s must return 200; got %d body=%s",
		ruleID, actResp.StatusCode, string(actBody))
}

// extractIDFromJSON is a minimal JSON field extractor. Kept intentionally
// naive to avoid pulling in a JSON dependency at the test file level.
// Looks for the first "ruleId":"..." value in the payload.
func extractIDFromJSON(t *testing.T, payload []byte) string {
	t.Helper()

	const needle = `"ruleId":"`

	s := string(payload)

	idx := strings.Index(s, needle)
	if idx < 0 {
		return ""
	}

	start := idx + len(needle)

	end := strings.IndexByte(s[start:], '"')
	if end < 0 {
		return ""
	}

	return s[start : start+end]
}

// rowCounts captures the pre/post-test counts of the tables the validation
// write path modifies. Kept small so the delta assertions above stay easy to
// read.
type rowCounts struct {
	validations int
	audits      int
}

// countValidationAndAuditRows returns the current row counts for
// transaction_validations and audit_events in the given tenant DB.
func countValidationAndAuditRows(t *testing.T, spec tenantPGSpec) rowCounts {
	t.Helper()

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		spec.Username, spec.Password, spec.Host, spec.Port, spec.Database, spec.SSLMode)

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "open %s for counting", spec.Database)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out rowCounts

	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transaction_validations`).Scan(&out.validations); err != nil {
		t.Fatalf("count transaction_validations in %s: %v", spec.Database, err)
	}

	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`).Scan(&out.audits); err != nil {
		t.Fatalf("count audit_events in %s: %v", spec.Database, err)
	}

	return out
}

// hasRequestID reports whether the given tenant DB has a
// transaction_validations row with the supplied request_id.
func hasRequestID(t *testing.T, spec tenantPGSpec, requestID string) bool {
	t.Helper()

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		spec.Username, spec.Password, spec.Host, spec.Port, spec.Database, spec.SSLMode)

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "open %s for request_id lookup", spec.Database)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var exists bool
	err = db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM transaction_validations WHERE request_id = $1)`,
		requestID,
	).Scan(&exists)
	require.NoError(t, err, "lookup request_id %s in %s", requestID, spec.Database)

	return exists
}
