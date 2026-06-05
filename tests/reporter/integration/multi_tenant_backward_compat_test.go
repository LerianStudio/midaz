//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package integration holds integration tests for the reporter service.
package integration

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	h "github.com/LerianStudio/midaz/v4/tests/reporter/utils"
)

// TestMultiTenant_BackwardCompatibility is the canonical regression guard for
// single-tenant deployments. It verifies that all infrastructure endpoints are
// reachable without any tenant context when MULTI_TENANT_ENABLED is false (the
// default), and that no multi-tenant env vars are required to start the service.
//
// This test is part of the mandatory Gate 7 pre-merge checklist and must pass
// before any multi-tenant PR can be merged.
func TestMultiTenant_BackwardCompatibility(t *testing.T) {
	// Ensure no multi-tenant env vars pollute the test environment.
	multiTenantVars := []string{
		"MULTI_TENANT_ENABLED",
		"MULTI_TENANT_URL",
	}

	for _, v := range multiTenantVars {
		original, wasSet := os.LookupEnv(v)
		os.Unsetenv(v)

		if wasSet {
			t.Cleanup(func() { os.Setenv(v, original) })
		}
	}

	// Verify env is clean for the test.
	assert.Empty(t, os.Getenv("MULTI_TENANT_ENABLED"),
		"MULTI_TENANT_ENABLED must not be set for backward compatibility test")
	assert.Empty(t, os.Getenv("MULTI_TENANT_URL"),
		"MULTI_TENANT_URL must not be set for backward compatibility test")

	if managerAddr == "" {
		t.Skip("manager service not available (testcontainers not started)")
	}

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)

	t.Run("health_endpoint_accessible_without_tenant_context", func(t *testing.T) {
		// GET /health must return 200 without any tenant JWT or context.
		// This validates that tenantBypassPaths in initTenantMiddleware includes
		// "/health" and that the middleware returns c.Next() immediately for it.
		code, body, err := cli.Request(ctx, "GET", "/health", nil, nil)
		require.NoError(t, err, "health request must not error")
		assert.Equal(t, 200, code,
			"GET /health must return 200 without tenant context; body: %s", string(body))
	})

	t.Run("ready_endpoint_accessible_without_tenant_context", func(t *testing.T) {
		// GET /readyz must not be blocked by auth or tenant middleware.
		// The response may be 200 or 503 (dependencies not ready), but must
		// never be a 401 or 403 gated by tenant resolution.
		code, body, err := cli.Request(ctx, "GET", "/readyz", nil, nil)
		require.NoError(t, err, "ready request must not error")
		assert.NotEqual(t, 401, code,
			"GET /readyz must not be blocked by auth middleware; body: %s", string(body))
		assert.NotEqual(t, 403, code,
			"GET /readyz must not be blocked by tenant middleware; body: %s", string(body))
	})

	t.Run("version_endpoint_accessible_without_tenant_context", func(t *testing.T) {
		// GET /version must return 200 without tenant context.
		// Validates that "/version" is included in tenantBypassPaths.
		code, body, err := cli.Request(ctx, "GET", "/version", nil, nil)
		require.NoError(t, err, "version request must not error")
		assert.Equal(t, 200, code,
			"GET /version must return 200 without tenant context; body: %s", string(body))
	})

	t.Run("api_endpoint_reachable_in_single_tenant_mode", func(t *testing.T) {
		// Business endpoints must remain reachable in single-tenant mode.
		// A 401 (auth) or 404 (not found) is acceptable; a 403 from tenant
		// middleware would indicate a bypass failure.
		headers := h.AuthHeaders()
		code, body, err := cli.Request(ctx, "GET", "/v1/templates?limit=1", headers, nil)
		require.NoError(t, err, "list templates request must not error")
		assert.NotEqual(t, 403, code,
			"GET /v1/templates must not be blocked by tenant middleware in single-tenant mode; body: %s", string(body))
		// 200 OK or 401 Unauthorized are both valid single-tenant responses.
		assert.True(t, code == 200 || code == 401 || code == 404,
			"GET /v1/templates must return 200, 401, or 404 in single-tenant mode; got %d body: %s", code, string(body))
	})
}
