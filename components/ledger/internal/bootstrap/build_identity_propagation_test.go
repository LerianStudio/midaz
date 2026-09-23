// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-commons/v7/commons/buildinfo"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Sentinels no real build can produce, so an assertion that sees them proves
// the value travelled from the link-time injection point to the wire.
const (
	sentinelVersion   = "9.9.9-rvi"
	sentinelRevision  = "0123456789abcdef0123456789abcdef01234567"
	sentinelBuildTime = "2026-09-23T00:00:00Z"
)

// TestBuildIdentityPropagatesToEveryRuntimeSurface is the only test that
// injects a build identity instead of reading the one the test binary already
// has. Every other identity assertion in this package compares a handler's
// answer to buildinfo.Get() evaluated in the same process, so it would still
// pass if a handler returned a hardcoded constant — under `make test-unit`
// (GOFLAGS=-buildvcs=false) both sides are the unstamped fallback
// {dev, unknown, unknown}. This one calls buildinfo.Set, so hardcoding any of
// the three surfaces fails it.
//
// Sequential on purpose: buildinfo.Set mutates process-global state. Go runs
// every top-level sequential test to completion (cleanups included) before
// releasing the package's t.Parallel() tests, so the parallel readers here
// never observe the sentinels.
func TestBuildIdentityPropagatesToEveryRuntimeSurface(t *testing.T) {
	buildinfo.Set(buildinfo.Build{
		Version:   sentinelVersion,
		Revision:  sentinelRevision,
		BuildTime: sentinelBuildTime,
	})

	t.Cleanup(func() {
		// An empty Build clears the injection: buildinfo only overrides a
		// field when the injected value is non-empty.
		buildinfo.Set(buildinfo.Build{})
	})

	t.Run("version_endpoint", func(t *testing.T) {
		server := NewUnifiedServer(":0", "ledger", newTestLogger(), &libOpentelemetry.Telemetry{}, nil, nil, nil)
		require.NotNil(t, server)

		req, err := http.NewRequest(http.MethodGet, "/version", nil)
		require.NoError(t, err)

		resp, err := server.app.Test(req)
		require.NoError(t, err)

		defer func() {
			_ = resp.Body.Close()
		}()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var got map[string]any
		require.NoError(t, json.Unmarshal(body, &got), "body=%s", body)

		assert.Equal(t, sentinelVersion, got["version"], "/version must serve the injected version")
		assert.Equal(t, sentinelRevision, got["revision"], "/version must serve the injected revision")
		assert.Equal(t, sentinelBuildTime, got["buildTime"], "/version must serve the injected build time")
	})

	// Every /readyz branch reports the identity: the 200 path and both 503
	// lifecycle branches (startup not finished, graceful drain).
	readyzCases := []struct {
		name       string
		prepare    func(*ReadyzHandler)
		wantStatus int
	}{
		{name: "readyz_endpoint", prepare: (*ReadyzHandler).SetServerReady, wantStatus: http.StatusOK},
		{name: "readyz_not_ready", prepare: func(*ReadyzHandler) {}, wantStatus: http.StatusServiceUnavailable},
		{name: "readyz_draining", prepare: func(h *ReadyzHandler) {
			h.SetServerReady()
			h.StartDrain()
		}, wantStatus: http.StatusServiceUnavailable},
	}

	for _, tc := range readyzCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewReadyzHandler(ReadyzHandlerConfig{
				Logger:         libLog.NewNop(),
				Checkers:       []DependencyChecker{},
				DeploymentMode: "local",
			})
			tc.prepare(handler)

			app := fiber.New()
			app.Get("/readyz", handler.HandleReadyz)

			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil), fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			defer func() {
				_ = resp.Body.Close()
			}()

			require.Equal(t, tc.wantStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var response ReadyzResponse
			require.NoError(t, json.Unmarshal(body, &response), "body=%s", body)

			assert.Equal(t, sentinelVersion, response.Version, "/readyz must report the injected version")
			assert.Equal(t, sentinelRevision, response.Revision, "/readyz must report the injected revision")
			assert.Equal(t, sentinelBuildTime, response.BuildTime, "/readyz must report the injected build time")
		})
	}

	t.Run("openapi_info_version", func(t *testing.T) {
		t.Setenv("OPENAPI_DOCS_ENABLED", "true")

		doc := fetchOpenAPISpec(t, newSingleDocServer(t).app, "/openapi.json")

		info, _ := doc["info"].(map[string]any)
		require.NotNil(t, info, "the spec should carry an info object")
		assert.Equal(t, sentinelVersion, info["version"], "OpenAPI info.version must carry the injected version")
	})

	t.Run("otel_resource", func(t *testing.T) {
		tc := telemetryConfig(&Config{OtelServiceName: "ledger", OtelLibraryName: "ledger"}, libLog.NewNop())

		assert.Equal(t, "ledger", tc.ServiceName, "service.name is the configured service name")
		assert.Equal(t, sentinelVersion, tc.ServiceVersion, "service.version must carry the injected version")
		assert.Equal(t, sentinelRevision, tc.ServiceRevision, "vcs.ref.head.revision must carry the injected revision")
	})
}
