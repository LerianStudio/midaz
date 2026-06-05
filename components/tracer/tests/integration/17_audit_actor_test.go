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
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// =============================================================================
// Audit Actor Identity Integration Tests (Taura security fix port from flowker)
//
// These tests verify the end-to-end behavior introduced by migration 000017
// and the resolveActor changes in RecordAuditEventCommand:
//
//   1. Requests authenticated via API key must produce audit rows with
//      actor_type='api_key' and actor_id=<configured label> — replacing the
//      pre-fix hardcoded {actor_type:system, actor_id:svc_tracer} fallback.
//
//   2. The migrated hash chain — which now covers actor_type / actor_id /
//      actor_name / actor_ip_address — must remain verifiable end-to-end.
//      verify_audit_hash_chain() returning is_valid=TRUE post-migration is
//      the canonical check that the trigger and verifier formulas stayed in
//      lockstep.
//
//   3. The actor_type_enum must accept 'api_key' values. This is the schema
//      half of the migration; the application-layer enum (model.ActorTypeAPIKey)
//      is unit-tested. This integration test confirms ALTER TYPE … ADD VALUE
//      applied successfully.
//
// The JWT/plugin-auth path is exercised at unit level by the auth_guard tests
// (TestPrincipal_FromJWT_* in internal/adapters/http/in/middleware), where the
// lib-auth Access Manager round-trip can be mocked without spinning a full
// plugin-auth server in-test.
// =============================================================================

// validationPayloadForActor builds a small ALLOW-eligible validation payload
// suitable for exercising the audit writer through the /v1/validations
// endpoint.
func validationPayloadForActor(t *testing.T) []byte {
	t.Helper()

	payload := map[string]any{
		"requestId":            uuid.New().String(),
		"transactionType":      "PIX",
		"amount":               "10.00",
		"currency":             "BRL",
		"transactionTimestamp": testutil.FixedTime().Format(time.RFC3339),
		"account": map[string]any{
			"accountId": "550e8400-e29b-41d4-a716-446655440001",
			"type":      "checking",
			"status":    "active",
		},
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err, "failed to marshal validation payload")

	return data
}

// fetchLatestAuditActor returns the actor columns of the most recently written
// audit_events row, or empty strings if no rows exist.
func fetchLatestAuditActor(t *testing.T, db *sql.DB) (actorType, actorID, actorName, actorIP string) {
	t.Helper()

	query := `
		SELECT
			COALESCE(actor_type::text, ''),
			COALESCE(actor_id, ''),
			COALESCE(actor_name, ''),
			COALESCE(actor_ip_address, '')
		FROM audit_events
		ORDER BY id DESC
		LIMIT 1
	`

	err := db.QueryRowContext(context.Background(), query).Scan(&actorType, &actorID, &actorName, &actorIP)
	if err == sql.ErrNoRows {
		return "", "", "", ""
	}

	require.NoError(t, err, "failed to read latest audit_events row")

	return actorType, actorID, actorName, actorIP
}

// callValidationEndpoint sends a POST to /v1/validations with the test API key
// and returns the parsed response body. Fails the test on transport errors.
func callValidationEndpoint(t *testing.T) map[string]any {
	t.Helper()

	req, err := http.NewRequest(
		http.MethodPost,
		testutil.GetBaseURL()+"/v1/validations",
		bytes.NewReader(validationPayloadForActor(t)),
	)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", testutil.GetAPIKey())

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"expected 201 Created from /v1/validations, body: %s", string(body))

	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result), "failed to parse response body")

	return result
}

// TestAuditActor_APIKey_StampsAPIKeyActor verifies the central contract of the
// Taura security fix: a request authenticated via API key produces an audit
// row whose actor identifies the deployment (api_key principal) instead of
// falling back to the generic system actor.
//
// API_KEY_LABEL is not configured in the standard integration harness, so
// the middleware applies its default label "tracer-default" — that is the
// expected actor.id.
func TestAuditActor_APIKey_StampsAPIKeyActor(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)

	resp := callValidationEndpoint(t)

	validationIDStr, ok := resp["validationId"].(string)
	require.True(t, ok, "response must include validationId")

	// Best-effort wait for the persistAuditEvent goroutine to land its row.
	// validation_service uses context.WithoutCancel + timeout in a
	// goroutine-style detached persistence — a short poll is enough for the
	// row to appear under normal load.
	var actorType, actorID string

	require.Eventually(t, func() bool {
		row := db.QueryRowContext(context.Background(),
			`SELECT actor_type::text, actor_id
			 FROM audit_events
			 WHERE resource_id = $1
			   AND event_type = 'TRANSACTION_VALIDATED'
			 ORDER BY id DESC LIMIT 1`, validationIDStr)
		err := row.Scan(&actorType, &actorID)

		return err == nil
	}, 5*time.Second, 50*time.Millisecond, "audit row for validation must appear within 5s")

	assert.Equal(t, "api_key", actorType,
		"API-key authenticated requests MUST produce api_key actor (Taura finding) — not system fallback")
	assert.Equal(t, "tracer-default", actorID,
		"actor_id MUST equal the configured API_KEY_LABEL (default 'tracer-default' here)")
}

// TestAuditActor_HashChainStaysVerifiableAfterMigration confirms that the
// new canonical hash formula (which includes the four actor fields) keeps
// the chain internally consistent: trigger output and verifier output match
// for every row inserted post-migration 000017.
//
// This is the core integration assertion that the migration applied cleanly
// — both calculate_audit_event_hash() and verify_audit_hash_chain() updated
// to the new formula in lockstep, with no drift.
func TestAuditActor_HashChainStaysVerifiableAfterMigration(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)

	// Drive one fresh audit row so the verifier has something to check that
	// was definitely written under the new trigger.
	_ = callValidationEndpoint(t)

	var (
		isValid        bool
		firstInvalidID sql.NullInt64
		totalChecked   int64
		errorDetail    sql.NullString
	)

	require.Eventually(t, func() bool {
		row := db.QueryRowContext(context.Background(), `SELECT is_valid, first_invalid_id, total_checked, error_detail FROM verify_audit_hash_chain(1, NULL)`)

		return row.Scan(&isValid, &firstInvalidID, &totalChecked, &errorDetail) == nil
	}, 5*time.Second, 50*time.Millisecond, "verify_audit_hash_chain must be callable")

	assert.True(t, isValid,
		"hash chain MUST verify clean post-migration — failure here means calculate_audit_event_hash() and verify_audit_hash_chain() drifted out of lockstep. Detail: %s",
		errorDetail.String)
	assert.False(t, firstInvalidID.Valid,
		"first_invalid_id MUST be NULL when chain is valid; got id=%d (detail=%s)",
		firstInvalidID.Int64, errorDetail.String)
	assert.Greater(t, totalChecked, int64(0),
		"verifier should have inspected at least one row — the validation call above produces one")
}

// TestAuditActor_EnumAcceptsAPIKeyValue is the schema-level sanity check that
// migration 000017's `ALTER TYPE actor_type_enum ADD VALUE 'api_key'` applied.
// Inserting a row directly with actor_type='api_key' must succeed.
//
// This protects against a silent migration skip where the application-layer
// model accepts ActorTypeAPIKey but the database still rejects it — an
// expensive failure mode to debug at runtime.
func TestAuditActor_EnumAcceptsAPIKeyValue(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)

	eventID := uuid.New()

	// Direct INSERT without setting hash/previous_hash — the trigger fills
	// them in with the post-000017 formula. The point of this test is to
	// confirm the enum accepts the new value; the hash assertion is covered
	// by TestAuditActor_HashChainStaysVerifiableAfterMigration.
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO audit_events (
			event_id, event_type, created_at, action, result,
			resource_id, resource_type,
			actor_type, actor_id, actor_name, actor_ip_address,
			context, metadata
		) VALUES (
			$1, 'RULE_CREATED', NOW(), 'CREATE', 'SUCCESS',
			$2, 'rule',
			'api_key', 'tracer-test', '', '203.0.113.1',
			'{}'::jsonb, '{}'::jsonb
		)
	`, eventID, uuid.New().String())

	require.NoError(t, err,
		"actor_type_enum MUST accept 'api_key' — migration 000017 ALTER TYPE step is mandatory")

	// Cleanup is not necessary — testcontainers tears down between suite runs.
}
