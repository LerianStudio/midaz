// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/readyz"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManagerHealthHandler_NilState_DefaultsHealthy verifies that when the
// SelfProbeState is nil (legacy callers / partial-bootstrap tests), /health
// returns 200. This preserves the pre-Gate-7 contract for callers that have
// not yet wired the self-probe.
func TestManagerHealthHandler_NilState_DefaultsHealthy(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/health", NewManagerHealthHandler(nil))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/health", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "alive", body["status"])
}

// TestManagerHealthHandler_StateUnhealthy_Returns503 verifies the Gate 7
// gating: until SelfProbeState.MarkHealthy() is called, /health returns 503
// with status="unhealthy" and a reason pointing at the self-probe.
func TestManagerHealthHandler_StateUnhealthy_Returns503(t *testing.T) {
	t.Parallel()

	state := &readyz.SelfProbeState{}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/health", NewManagerHealthHandler(state))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/health", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var body map[string]string

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "unhealthy", body["status"])
	assert.Contains(t, body["reason"], "self-probe")
}

// TestManagerHealthHandler_StateHealthy_Returns200 verifies that after
// MarkHealthy() flips the flag, /health returns 200 with status="alive".
func TestManagerHealthHandler_StateHealthy_Returns200(t *testing.T) {
	t.Parallel()

	state := &readyz.SelfProbeState{}
	state.MarkHealthy()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/health", NewManagerHealthHandler(state))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/health", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "alive", body["status"])
}

// TestManagerReadyzHandler_NilDeps_DoesNotPanic verifies that mounting the
// handler with nil deps produces a 503 unhealthy response listing every
// dependency as down/skipped/n/a — never a panic.
func TestManagerReadyzHandler_NilDeps_DoesNotPanic(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewManagerReadyzHandler(nil))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	// nil deps means every dep with a static connection (mongo, rabbit,
	// redis, storage) reports down → 503.
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var parsed readyz.Response

	require.NoError(t, json.Unmarshal(body, &parsed))
	assert.Equal(t, "unhealthy", parsed.Status)
	// We expect 6 dependency entries: mongo, rabbitmq, redis, storage, fetcher, tenant_manager.
	assert.Len(t, parsed.Checks, 6)
}

// TestManagerReadyzHandler_VersionAndDeploymentMode verifies that the
// version and deployment mode passed via deps are echoed verbatim.
func TestManagerReadyzHandler_VersionAndDeploymentMode(t *testing.T) {
	t.Parallel()

	deps := &ManagerReadyzDeps{
		Version:        "9.9.9-test",
		DeploymentMode: "byoc",
		DrainState:     &readyz.DrainState{},
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewManagerReadyzHandler(deps))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	var parsed readyz.Response

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&parsed))
	assert.Equal(t, "9.9.9-test", parsed.Version)
	assert.Equal(t, "byoc", parsed.DeploymentMode)
}

// TestManagerReadyzHandler_Draining_ShortCircuit verifies the drain path.
func TestManagerReadyzHandler_Draining_ShortCircuit(t *testing.T) {
	t.Parallel()

	drain := &readyz.DrainState{}
	drain.StartDraining()

	deps := &ManagerReadyzDeps{
		Version:        "1.0",
		DeploymentMode: "local",
		DrainState:     drain,
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewManagerReadyzHandler(deps))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var parsed readyz.Response

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&parsed))
	assert.Equal(t, "draining", parsed.Status)
	assert.Empty(t, parsed.Checks)
}

// TestManagerReadyzHandler_TenantManagerSkippedWhenMultiTenantDisabled verifies
// the contract decision that the tenant_manager dependency is skipped (not
// down) when MULTI_TENANT_ENABLED=false.
func TestManagerReadyzHandler_TenantManagerSkippedWhenMultiTenantDisabled(t *testing.T) {
	t.Parallel()

	deps := &ManagerReadyzDeps{
		MultiTenantEnabled: false,
		Version:            "1.0",
		DeploymentMode:     "local",
		DrainState:         &readyz.DrainState{},
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewManagerReadyzHandler(deps))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	var parsed readyz.Response

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&parsed))
	tm, ok := parsed.Checks["tenant_manager"]
	require.True(t, ok, "tenant_manager check must always be present")
	assert.Equal(t, readyz.StatusSkipped, tm.Status)
	assert.Contains(t, tm.Reason, "MULTI_TENANT_ENABLED=false")
}

// TestManagerReadyzHandler_FetcherSkippedWhenDirectProvider verifies that the
// fetcher dependency is reported as skipped when the DataSourceProvider is
// not a *FetcherProvider (DirectProvider mode).
func TestManagerReadyzHandler_FetcherSkippedWhenDirectProvider(t *testing.T) {
	t.Parallel()

	// Use nil DataSourceProvider — same behavior as DirectProvider for
	// the type-assertion path.
	deps := &ManagerReadyzDeps{
		MultiTenantEnabled: false,
		Version:            "1.0",
		DeploymentMode:     "local",
		DrainState:         &readyz.DrainState{},
		DataSourceProvider: nil,
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewManagerReadyzHandler(deps))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	var parsed readyz.Response

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&parsed))
	fetcher, ok := parsed.Checks["fetcher"]
	require.True(t, ok, "fetcher check must always be present")
	assert.Equal(t, readyz.StatusSkipped, fetcher.Status)
	assert.Contains(t, fetcher.Reason, "FETCHER_ENABLED=false")
}

// TestManagerReadyzHandler_MongoNAInMultiTenantMode verifies that mongo
// reports n/a (not down) in multi-tenant mode even though the connection is
// not provided.
func TestManagerReadyzHandler_MongoNAInMultiTenantMode(t *testing.T) {
	t.Parallel()

	deps := &ManagerReadyzDeps{
		MultiTenantEnabled: true,
		Version:            "1.0",
		DeploymentMode:     "saas",
		DrainState:         &readyz.DrainState{},
		// Provide a non-nil tenant manager client so tenant_manager is up.
		// (The struct is opaque; nil predicate returns false → checker is down.)
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewManagerReadyzHandler(deps))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	var parsed readyz.Response

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&parsed))

	mongo, ok := parsed.Checks["mongo"]
	require.True(t, ok)
	assert.Equal(t, readyz.StatusNA, mongo.Status)

	rabbit, ok := parsed.Checks["rabbitmq"]
	require.True(t, ok)
	assert.Equal(t, readyz.StatusNA, rabbit.Status)
}
