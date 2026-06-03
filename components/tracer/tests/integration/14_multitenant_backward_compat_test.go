// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

// Package integration hosts the multi-tenant backward compatibility gate.
// The Lerian multi-tenant standard requires every service to prove that
// single-tenant deployments remain fully functional after multi-tenant
// support is added — this file is that proof for tracer.
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	testutil_integration "github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil_integration"
)

// TestMultiTenant_BackwardCompatibility is the single-tenant backward
// compatibility gate. It boots tracer with only the standard env vars the
// integration harness already sets — crucially NO MULTI_TENANT_* vars
// except an explicit MULTI_TENANT_ENABLED=false — and verifies that:
//
//  1. the service starts without Tenant Manager / Redis running;
//  2. core endpoints still work with API-key auth (no JWT / tenant header);
//  3. the Service struct has no multi-tenant components wired;
//  4. singleton workers (rule sync + usage cleanup) ARE wired.
//
// Every check mirrors the "Single-Tenant Backward Compatibility Validation
// (MANDATORY)" section of the Lerian multi-tenant standard.
//
// This test is intentionally NOT parallel — it relies on the global test
// suite's shared server.
func TestMultiTenant_BackwardCompatibility(t *testing.T) {
	// Sanity check: the integration harness does not set MULTI_TENANT_*,
	// so this test observes the service as single-tenant by default.
	// Explicitly setting MULTI_TENANT_ENABLED=false is therefore redundant
	// for the running server but doubles as documentation for future
	// maintainers who read this file looking for the env contract.
	require.Empty(t, os.Getenv("MULTI_TENANT_URL"),
		"MULTI_TENANT_URL must be unset for backward-compat test. Unset it in your .env or test env.")
	require.Empty(t, os.Getenv("MULTI_TENANT_SERVICE_API_KEY"),
		"MULTI_TENANT_SERVICE_API_KEY must be unset for backward-compat test. Unset it in your .env or test env.")
	require.Empty(t, os.Getenv("MULTI_TENANT_REDIS_HOST"),
		"MULTI_TENANT_REDIS_HOST must be unset for backward-compat test. Unset it in your .env or test env.")

	suite := testutil_integration.GetTestSuite()
	require.NotNil(t, suite, "integration test suite must be initialised")

	svc := suite.ServiceForTest()
	require.NotNil(t, svc, "bootstrap.Service must be reachable")

	// ------------------------------------------------------------------
	// Check 1: Service struct — no MT components, singleton workers present.
	// This is the structural contract of single-tenant mode.
	// ------------------------------------------------------------------
	t.Run("ServiceWiring_NoMultiTenantComponents", func(t *testing.T) {
		assert.False(t, svc.HasSupervisorForTest(),
			"supervisor must be nil in single-tenant mode")
		assert.False(t, svc.HasPGManagerForTest(),
			"pgManager must be nil in single-tenant mode")
		assert.False(t, svc.HasEventListenerForTest(),
			"eventListener must be nil in single-tenant mode")

		assert.True(t, svc.HasSingletonSyncWorkerForTest(),
			"singleton syncWorker must be wired in single-tenant mode")

		// cleanupWorker is only wired when CLEANUP_WORKER_ENABLED=true —
		// the integration harness leaves it unset, which means the worker
		// is disabled but the service boots fine. The gate here is only
		// that the MT supervisor did not shadow the singleton path.
		if os.Getenv("CLEANUP_WORKER_ENABLED") == "true" {
			assert.True(t, svc.HasSingletonCleanupWorkerForTest(),
				"cleanup worker should be wired when enabled")
		}
	})

	// ------------------------------------------------------------------
	// Check 2: Health and readiness endpoints — no auth required.
	// ------------------------------------------------------------------
	t.Run("HealthEndpoint_200_NoAuth", func(t *testing.T) {
		baseURL := testutil.GetBaseURL()

		for _, path := range []string{"/health", "/version"} {
			path := path

			t.Run(path, func(t *testing.T) {
				resp, err := testutil.HTTPClient.Get(baseURL + path)
				require.NoError(t, err, "GET %s must not error", path)

				defer resp.Body.Close()

				// /health and /version are unconditional liveness/version
				// probes — the backward-compat contract is that the endpoint
				// is reachable, unauthenticated, and returns 200.
				assert.Equal(t, http.StatusOK, resp.StatusCode,
					"%s must return 200, got %d", path, resp.StatusCode)
			})
		}
	})

	// ------------------------------------------------------------------
	// Check 3: Validation endpoint works with API-key auth and no tenant
	// context. The request has NO tenant header, NO JWT with tenantId —
	// exactly the pre-multi-tenant call contract.
	// ------------------------------------------------------------------
	t.Run("Validations_POST_WorksWithAPIKeyOnly", func(t *testing.T) {
		apiKey := testutil.GetAPIKey()
		baseURL := testutil.GetBaseURL()

		body := map[string]any{
			"requestId":            uuid.New().String(),
			"transactionType":      "CARD",
			"amount":               decimal.RequireFromString("10").String(),
			"currency":             "BRL",
			"transactionTimestamp": time.Now().UTC().Format(time.RFC3339),
			"account": map[string]any{
				"accountId": uuid.New().String(),
			},
		}

		payload, err := json.Marshal(body)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/validations", bytes.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", apiKey)

		resp, err := testutil.HTTPClient.Do(req)
		require.NoError(t, err)

		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		// 201 for new validations (and 200 for duplicates — idempotency).
		// Anything else means the single-tenant path broke.
		assert.Contains(t, []int{http.StatusCreated, http.StatusOK}, resp.StatusCode,
			"validation must succeed without tenant context; got %d: %s",
			resp.StatusCode, string(respBody))
	})

	// ------------------------------------------------------------------
	// Check 4: Rules list endpoint works with API-key auth.
	// ------------------------------------------------------------------
	t.Run("Rules_GET_WorksWithAPIKeyOnly", func(t *testing.T) {
		apiKey := testutil.GetAPIKey()
		baseURL := testutil.GetBaseURL()

		req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules", nil)
		require.NoError(t, err)
		req.Header.Set("X-API-Key", apiKey)

		resp, err := testutil.HTTPClient.Do(req)
		require.NoError(t, err)

		defer resp.Body.Close()

		// The payload shape is unchanged — the list handler is the same
		// one single-tenant clients have always hit.
		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"GET /v1/rules must return 200 with API key in single-tenant mode")
	})

	// ------------------------------------------------------------------
	// Check 5: an unauthenticated request to a protected route still
	// returns 401. Guards against an accidental bypass introduced by the
	// multi-tenant middleware.
	// ------------------------------------------------------------------
	t.Run("UnauthenticatedRequest_401", func(t *testing.T) {
		baseURL := testutil.GetBaseURL()

		req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/rules", nil)
		require.NoError(t, err)
		// intentionally omit X-API-Key

		resp, err := testutil.HTTPClient.Do(req)
		require.NoError(t, err)

		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"auth contract must be unchanged: missing X-API-Key → 401")
	})
}

// TestMultiTenant_BackwardCompatibility_NoRedisConfigNeeded is a focused
// check that the boot path does not even read MULTI_TENANT_REDIS_HOST when
// MULTI_TENANT_ENABLED is false. If the env-var loading code ever starts
// validating Redis config unconditionally, this test will fail loudly.
func TestMultiTenant_BackwardCompatibility_NoRedisConfigNeeded(t *testing.T) {
	// The running server booted without MULTI_TENANT_REDIS_HOST set. If
	// that were a required var, SetupTestSuite would have failed before
	// this test ever ran. Asserting it is still unset here documents the
	// contract and gives a useful failure site for future regressions.
	assert.Empty(t, os.Getenv("MULTI_TENANT_REDIS_HOST"),
		"single-tenant boot must not require MULTI_TENANT_REDIS_HOST")

	suite := testutil_integration.GetTestSuite()
	require.NotNil(t, suite)
	require.NotNil(t, suite.ServiceForTest(), "service must be running")

	// If we got here, the service booted without Tenant Manager or Redis —
	// that IS the proof. The assertion above just nails the invariant.
	t.Log(fmt.Sprintf("service running at %s without MT infrastructure",
		suite.ServerURL))
}
