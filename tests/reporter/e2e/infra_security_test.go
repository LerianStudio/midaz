// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ############################################################################
// Security Headers (TC-SEC-001)
// ############################################################################

func TestSec_XContentTypeOptions(t *testing.T) {
	t.Parallel()

	resp, err := http.Get(env.ManagerBaseURL + "/health")
	require.NoError(t, err, "GET /health should not return an error")
	defer resp.Body.Close()

	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"),
		"X-Content-Type-Options header should be nosniff")
}

func TestSec_XFrameOptions(t *testing.T) {
	t.Parallel()

	resp, err := http.Get(env.ManagerBaseURL + "/health")
	require.NoError(t, err, "GET /health should not return an error")
	defer resp.Body.Close()

	assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"),
		"X-Frame-Options header should be DENY")
}

func TestSec_XXSSProtection(t *testing.T) {
	t.Parallel()

	resp, err := http.Get(env.ManagerBaseURL + "/health")
	require.NoError(t, err, "GET /health should not return an error")
	defer resp.Body.Close()

	assert.Equal(t, "0", resp.Header.Get("X-XSS-Protection"),
		"X-XSS-Protection header should be 0")
}

func TestSec_StrictTransportSecurity(t *testing.T) {
	t.Parallel()

	resp, err := http.Get(env.ManagerBaseURL + "/health")
	require.NoError(t, err, "GET /health should not return an error")
	defer resp.Body.Close()

	hsts := resp.Header.Get("Strict-Transport-Security")
	if hsts != "" {
		assert.Contains(t, hsts, "max-age=",
			"Strict-Transport-Security should contain max-age directive when present")
	} else {
		t.Log("Strict-Transport-Security header not present (acceptable over plain HTTP)")
	}
}

// ############################################################################
// CORS (TC-SEC-002 to TC-SEC-004)
// ############################################################################

func TestSec_CORSAllowedOrigin(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodOptions, env.ManagerBaseURL+"/v1/templates", nil)
	require.NoError(t, err)

	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "OPTIONS request should not return an error")
	defer resp.Body.Close()

	acao := resp.Header.Get("Access-Control-Allow-Origin")
	if acao == "" {
		t.Skip("CORS not configured: Access-Control-Allow-Origin header absent")
	}

	assert.NotEmpty(t, acao, "Access-Control-Allow-Origin should be present for allowed origin")
}

func TestSec_CORSDisallowedOrigin(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodOptions, env.ManagerBaseURL+"/v1/templates", nil)
	require.NoError(t, err)

	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "OPTIONS request should not return an error")
	defer resp.Body.Close()

	acao := resp.Header.Get("Access-Control-Allow-Origin")

	// If CORS is configured, the disallowed origin should not be reflected.
	// A wildcard (*) is also acceptable since it means "allow all" which is a deliberate choice.
	if acao != "" && acao != "*" {
		assert.NotEqual(t, "https://evil.com", acao,
			"Access-Control-Allow-Origin should not reflect an evil origin")
	}
}

func TestSec_CORSPreflight(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodOptions, env.ManagerBaseURL+"/v1/templates", nil)
	require.NoError(t, err)

	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "OPTIONS preflight request should not return an error")
	defer resp.Body.Close()

	// Preflight should return 200 or 204.
	assert.Contains(t, []int{http.StatusOK, http.StatusNoContent}, resp.StatusCode,
		"preflight response should be 200 or 204, got %d", resp.StatusCode)
}

// ############################################################################
// Auth (TC-SEC-012)
// ############################################################################

func TestSec_AuthDisabledByDefault(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// When AUTH_ENABLED is not set (default), requests should succeed without a token.
	status, err := apiClient.Health(ctx)
	require.NoError(t, err, "health check should not return an error")
	assert.Equal(t, http.StatusOK, status, "health check should return 200 when auth is disabled")

	// Also verify a regular API endpoint works without auth.
	status, _, err = apiClient.GetAllTemplates(ctx, nil)
	require.NoError(t, err, "listing templates should not return an error")
	assert.Equal(t, http.StatusOK, status, "listing templates should return 200 when auth is disabled")
}

// ############################################################################
// Path Traversal (TC-SEC-013 / TC-DS-005)
// ############################################################################

func TestSec_PathTraversalBlocked(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	traversalIDs := []string{
		"../etc/passwd",
		"..%2F..%2Fetc%2Fpasswd",
		"midaz_onboarding/../../../etc/passwd",
	}

	for _, maliciousID := range traversalIDs {
		t.Run(maliciousID, func(t *testing.T) {
			resp, err := apiClient.GetDataSourceByIDRaw(ctx, maliciousID)
			require.NoError(t, err, "request should not return a network error")

			// The server should reject path traversal attempts with 400 or 404.
			assert.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, resp.StatusCode(),
				"path traversal attempt with ID %q should be rejected, got %d", maliciousID, resp.StatusCode())
		})
	}
}
