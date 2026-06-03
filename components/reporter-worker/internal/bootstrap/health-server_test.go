// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/readyz"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestHealthServer builds a HealthServer with a no-op logger and the
// provided config overrides. Used by tests below to drive endpoint assertions.
func newTestHealthServer(cfg HealthServerConfig) *HealthServer {
	if cfg.Logger == nil {
		cfg.Logger = &log.NopLogger{}
	}

	if cfg.Port == "" {
		cfg.Port = "0"
	}

	if cfg.Version == "" {
		cfg.Version = "test-version"
	}

	if cfg.DeploymentMode == "" {
		cfg.DeploymentMode = "local"
	}

	if cfg.DrainState == nil {
		cfg.DrainState = &readyz.DrainState{}
	}

	return NewHealthServer(cfg)
}

// TestHealthServer_HealthEndpoint_NilState_DefaultsHealthy verifies that when
// SelfProbeState is nil (legacy / partial-bootstrap test wiring), /health
// returns 200. Preserves the pre-Gate-7 contract for callers that have not
// yet wired the self-probe.
func TestHealthServer_HealthEndpoint_NilState_DefaultsHealthy(t *testing.T) {
	t.Parallel()

	hs := newTestHealthServer(HealthServerConfig{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	hs.server.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]string

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "alive", body["status"])
}

// TestHealthServer_HealthEndpoint_StateUnhealthy_Returns503 verifies the
// Gate 7 gating: until SelfProbeState.MarkHealthy() is called, /health
// returns 503. K8s livenessProbe interprets the 503 as "restart this pod".
func TestHealthServer_HealthEndpoint_StateUnhealthy_Returns503(t *testing.T) {
	t.Parallel()

	state := &readyz.SelfProbeState{}

	hs := newTestHealthServer(HealthServerConfig{
		SelfProbeState: state,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	hs.server.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body map[string]string

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "unhealthy", body["status"])
	assert.Contains(t, body["reason"], "self-probe")
}

// TestHealthServer_HealthEndpoint_StateHealthy_Returns200 verifies that
// after MarkHealthy() flips the flag, /health returns 200 with
// status="alive".
func TestHealthServer_HealthEndpoint_StateHealthy_Returns200(t *testing.T) {
	t.Parallel()

	state := &readyz.SelfProbeState{}
	state.MarkHealthy()

	hs := newTestHealthServer(HealthServerConfig{
		SelfProbeState: state,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	hs.server.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "alive", body["status"])
}

// TestHealthServer_ReadyzEndpoint_NoDeps_Unhealthy verifies that a worker
// running with no static connections (multi-tenant style) but
// MultiTenantEnabled=false reports unhealthy because mongo/rabbit/redis
// are missing.
func TestHealthServer_ReadyzEndpoint_NoDeps_Unhealthy(t *testing.T) {
	t.Parallel()

	hs := newTestHealthServer(HealthServerConfig{
		MultiTenantEnabled: false,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	hs.server.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body readyz.Response

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "unhealthy", body.Status)
	assert.Equal(t, "test-version", body.Version)
	assert.Equal(t, "local", body.DeploymentMode)
}

// TestHealthServer_ReadyzEndpoint_MultiTenant_Mongo_RabbitNA verifies that
// in multi-tenant mode the mongo and rabbitmq dependencies report n/a with
// a non-empty reason explaining why per-tenant probing is deferred.
//
// Mirrors the Manager-side TestManagerReadyzHandler_MongoNAInMultiTenantMode
// for the Worker transport (net/http). Test-reviewer M5: closes the
// asymmetry between Manager and Worker /readyz n/a coverage.
func TestHealthServer_ReadyzEndpoint_MultiTenant_Mongo_RabbitNA(t *testing.T) {
	t.Parallel()

	hs := newTestHealthServer(HealthServerConfig{
		MultiTenantEnabled: true,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	hs.server.Handler.ServeHTTP(rec, req)

	var body readyz.Response

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	mongo, ok := body.Checks["mongo"]
	require.True(t, ok, "mongo check must always be present in /readyz response")
	assert.Equal(t, readyz.StatusNA, mongo.Status)
	assert.NotEmpty(t, mongo.Reason,
		"n/a status must carry a human-readable reason for operators")
	assert.Contains(t, mongo.Reason, "multi-tenant",
		"reason must explain why per-tenant probing is deferred: %q", mongo.Reason)
	assert.Empty(t, mongo.Error,
		"n/a is not an error condition — Error must be empty")

	rabbit, ok := body.Checks["rabbitmq"]
	require.True(t, ok, "rabbitmq check must always be present in /readyz response")
	assert.Equal(t, readyz.StatusNA, rabbit.Status)
	assert.NotEmpty(t, rabbit.Reason)
	assert.Contains(t, rabbit.Reason, "multi-tenant")
	assert.Empty(t, rabbit.Error)
}

// TestHealthServer_ReadyzEndpoint_Draining_503 verifies the drain path.
func TestHealthServer_ReadyzEndpoint_Draining_503(t *testing.T) {
	t.Parallel()

	drain := &readyz.DrainState{}
	drain.StartDraining()

	hs := newTestHealthServer(HealthServerConfig{
		DrainState: drain,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	hs.server.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body readyz.Response

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "draining", body.Status)
	assert.Empty(t, body.Checks)
}

// TestHealthServer_LegacyReadyPath_NotRegistered verifies that the legacy
// /ready path is NOT registered. The contract path is exactly /readyz.
func TestHealthServer_LegacyReadyPath_NotRegistered(t *testing.T) {
	t.Parallel()

	hs := newTestHealthServer(HealthServerConfig{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	hs.server.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code,
		"/ready must NOT be registered as an alias; the contract path is /readyz")
}

// TestHealthServer_StartAndShutdown_NoPanic verifies the lifecycle.
func TestHealthServer_StartAndShutdown_NoPanic(t *testing.T) {
	t.Parallel()

	hs := newTestHealthServer(HealthServerConfig{})

	// Override the goroutine launcher so Start does not actually bind a port.
	hs.goNamedFn = func(_ log.Logger, _ string, fn func()) {
		go fn()
	}

	hs.Start()
	hs.Shutdown()
}
