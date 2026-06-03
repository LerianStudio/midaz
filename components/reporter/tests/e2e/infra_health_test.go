// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	shared "github.com/LerianStudio/midaz/v3/components/reporter/tests/e2e/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TC-HEALTH-001: GET /health returns 200.
func TestHealth_ManagerHealthEndpoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	status, err := apiClient.Health(ctx)
	require.NoError(t, err, "health endpoint should be reachable")
	assert.Equal(t, http.StatusOK, status, "health endpoint should return 200")
}

// TC-HEALTH-002: GET /readyz returns 200 or 503 with valid JSON body.
func TestHealth_ManagerReadyEndpoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	status, _, err := apiClient.Ready(ctx)
	require.NoError(t, err, "readyz endpoint should be reachable")
	assert.True(t, status == http.StatusOK || status == http.StatusServiceUnavailable,
		"readyz endpoint should return 200 or 503, got %d", status)

	// Verify the raw response is valid JSON regardless of status code
	readyURL := env.ManagerBaseURL + "/readyz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "readyz response should be valid JSON")
	assert.Contains(t, body, "status", "readyz response should have 'status' field")
	assert.Contains(t, body, "checks", "readyz response should have 'checks' field")
}

// TC-HEALTH-003: GET /readyz returns dependency checks with expected structure.
func TestHealth_ManagerReadyDependencies(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Parse the raw response to inspect dependency checks regardless of status code.
	readyURL := env.ManagerBaseURL + "/readyz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "readyz response should be valid JSON")

	checks, ok := body["checks"].(map[string]any)
	require.True(t, ok, "checks should be a map")

	// Verify expected dependency checks are present.
	// Canonical names match pkg/readyz/checks.go: nameMongo="mongo",
	// nameRabbitMQ="rabbitmq", nameRedis="redis", nameStorage="storage".
	expectedDeps := []string{"mongo", "rabbitmq", "redis", "storage"}
	for _, name := range expectedDeps {
		dep, exists := checks[name]
		assert.True(t, exists, "dependency check %q should be present", name)
		if depMap, ok := dep.(map[string]any); ok {
			_, hasStatus := depMap["status"]
			assert.True(t, hasStatus, "dependency check %q should have a 'status' field", name)
			t.Logf("dependency check %q: %v", name, depMap)
		}
	}
}

// TC-HEALTH-004: GET /version returns 200.
func TestHealth_ManagerVersionEndpoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	status, versionInfo, err := apiClient.Version(ctx)
	require.NoError(t, err, "version endpoint should be reachable")
	assert.Equal(t, http.StatusOK, status, "version endpoint should return 200")
	require.NotNil(t, versionInfo, "version response should not be nil")
}

// TC-HEALTH-005: Worker /health returns 200.
func TestHealth_WorkerHealthEndpoint(t *testing.T) {
	t.Parallel()

	require.NotNil(t, env.WorkerApp, "worker app must be running")

	ctx := context.Background()

	status, err := shared.WorkerHealth(ctx, env.WorkerApp.BaseURL)
	require.NoError(t, err, "worker health endpoint should be reachable")
	assert.Equal(t, http.StatusOK, status, "worker health endpoint should return 200")
}

// TC-HEALTH-006: Worker legacy /ready alias is permanently removed (regression guard).
func TestHealth_WorkerReadyEndpoint(t *testing.T) {
	t.Parallel()

	require.NotNil(t, env.WorkerApp, "worker app must be running")

	ctx := context.Background()
	readyURL := env.WorkerApp.BaseURL + "/ready"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL, nil)
	require.NoError(t, err, "should create request for worker /ready")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "worker ready endpoint should be reachable")
	defer resp.Body.Close()

	// /ready is the legacy alias path. Under the ring:dev-readyz contract (Gate 2)
	// it is permanently unregistered on both Worker and Manager — the canonical
	// path is /readyz. This test is a regression guard: any future re-introduction
	// of the legacy alias would silently pass health probes while bypassing the
	// canonical readiness contract, so we lock 404 in here.
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"legacy /ready alias must NOT be registered; the canonical path is /readyz")
}

// TC-HEALTH-006b: Worker /readyz returns 200 or 503 with canonical body shape.
func TestHealth_WorkerReadyzEndpoint(t *testing.T) {
	t.Parallel()

	require.NotNil(t, env.WorkerApp, "worker app must be running")

	ctx := context.Background()
	readyURL := env.WorkerApp.BaseURL + "/readyz"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL, nil)
	require.NoError(t, err, "should create request for worker /readyz")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "worker readyz endpoint should be reachable")
	defer resp.Body.Close()

	assert.True(t,
		resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusServiceUnavailable,
		"worker /readyz must return 200 or 503, got %d", resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "readyz response should be valid JSON")
	assert.Contains(t, body, "status")
	assert.Contains(t, body, "version")
	assert.Contains(t, body, "deployment_mode")

	// Top-level status uses the canonical vocabulary.
	statusStr, _ := body["status"].(string)
	assert.Contains(t, []string{"healthy", "unhealthy", "draining"}, statusStr)
}

// TC-HEALTH-006c: Manager /readyz returns 200 or 503 with canonical body shape.
func TestHealth_ManagerReadyzEndpoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	readyURL := env.ManagerBaseURL + "/readyz"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "manager /readyz must be reachable")
	defer resp.Body.Close()

	assert.True(t,
		resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusServiceUnavailable,
		"manager /readyz must return 200 or 503, got %d", resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Contains(t, body, "status")
	assert.Contains(t, body, "version")
	assert.Contains(t, body, "deployment_mode")

	statusStr, _ := body["status"].(string)
	assert.Contains(t, []string{"healthy", "unhealthy", "draining"}, statusStr)
}

// TC-HEALTH-007: Verify health response returns 200 with a body.
// Note: /health uses commonsHttp.Ping which may return plain text "healthy" or JSON.
func TestHealth_ManagerHealthResponseFormat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	healthURL := env.ManagerBaseURL + "/health"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	require.NoError(t, err, "should create request for /health")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "health endpoint should be reachable")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "health endpoint should return 200")

	// Response may be plain text or JSON — just verify it has content
	var buf [1024]byte
	n, _ := resp.Body.Read(buf[:])
	assert.Greater(t, n, 0, "health response should have a non-empty body")
}

// TC-HEALTH-008: Verify readyz response has expected fields (status + checks).
func TestHealth_ManagerReadyResponseBody(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Parse raw response to verify structure regardless of HTTP status.
	readyURL := env.ManagerBaseURL + "/readyz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err, "readyz response should be valid JSON")

	status, ok := body["status"].(string)
	require.True(t, ok, "readyz response should have a 'status' string field")
	assert.NotEmpty(t, status, "status field should not be empty")
	assert.Contains(t, []string{"healthy", "unhealthy", "draining"}, status,
		"status should use the canonical readyz vocabulary")

	checks, ok := body["checks"]
	assert.True(t, ok, "readyz response should have a 'checks' field")
	assert.NotNil(t, checks, "checks field should not be nil")
}

// TC-HEALTH-009: Multiple sequential /health calls all return 200 (stability).
func TestHealth_MultipleHealthChecks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	iterations := 5

	for i := range iterations {
		status, err := apiClient.Health(ctx)
		require.NoError(t, err, "health check iteration %d should not error", i)
		assert.Equal(t, http.StatusOK, status, "health check iteration %d should return 200", i)
	}
}

// TC-HEALTH-010: Concurrent /health calls all return 200.
func TestHealth_ConcurrentHealthChecks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	concurrency := 10

	var wg sync.WaitGroup

	results := make([]int, concurrency)
	errs := make([]error, concurrency)

	wg.Add(concurrency)

	for i := range concurrency {
		go func(idx int) {
			defer wg.Done()

			status, err := apiClient.Health(ctx)
			results[idx] = status
			errs[idx] = err
		}(i)
	}

	wg.Wait()

	for i := range concurrency {
		require.NoError(t, errs[i], "concurrent health check %d should not error", i)
		assert.Equal(t, http.StatusOK, results[i], "concurrent health check %d should return 200", i)
	}
}

// TC-HEALTH-011: GET /swagger/index.html returns 200.
func TestHealth_SwaggerEndpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.ManagerBaseURL+"/swagger/index.html", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "swagger endpoint should return 200")
}
