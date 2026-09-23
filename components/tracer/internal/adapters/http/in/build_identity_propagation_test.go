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

	"github.com/LerianStudio/lib-commons/v7/commons/buildinfo"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/api"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// Sentinels no real build can produce, so an assertion that sees them proves
// the value travelled from the link-time injection point to the wire.
const (
	sentinelVersion   = "9.9.9-rvi"
	sentinelRevision  = "0123456789abcdef0123456789abcdef01234567"
	sentinelBuildTime = "2026-09-23T00:00:00Z"
)

// TestBuildIdentityPropagatesToEveryRuntimeSurface is the only test in this
// package that injects a build identity instead of reading the one the test
// binary already has. Every other identity assertion compares a handler's
// answer to buildinfo.Get() evaluated in the same process, so it would still
// pass if a handler returned a hardcoded constant — under `make test-unit`
// (GOFLAGS=-buildvcs=false) both sides are the unstamped fallback
// {dev, unknown, unknown}. This one calls buildinfo.Set, so hardcoding any
// surface fails it.
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
		deps := newTestRouterDeps(t, middleware.AuthGuardConfig{})
		deps.serviceName = "tracer"
		app := deps.build()

		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/version", nil), fiber.TestConfig{Timeout: 0})
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

	t.Run("readyz_endpoint", func(t *testing.T) {
		testutil.SetupTestTracing(t)

		hc, mock, cleanup := newReadyzCheckerWithDB(t, "saas")
		defer cleanup()

		mock.ExpectPing()

		app := createReadyzTestApp(hc)

		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil), fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		defer func() {
			_ = resp.Body.Close()
		}()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var response api.ReadyzResponse
		require.NoError(t, json.Unmarshal(body, &response), "body=%s", body)

		assert.Equal(t, sentinelVersion, response.Version, "/readyz must report the injected version")
		assert.Equal(t, sentinelRevision, response.Revision, "/readyz must report the injected revision")
		assert.Equal(t, sentinelBuildTime, response.BuildTime, "/readyz must report the injected build time")
	})

	t.Run("openapi_info_version", func(t *testing.T) {
		spec := fetchTracerSpec(t)

		assert.Equal(t, sentinelVersion, spec.Info.Version, "OpenAPI info.version must carry the injected version")
	})
}
