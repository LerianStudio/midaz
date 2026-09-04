// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUnifiedServer_RegistersTenantTelemetryVariant guards the one line that
// turns the per-tenant HTTP metrics on. The behaviour itself is proven in
// pkg/net/http's wiring test, which builds its own app; nothing there observes
// what the ledger registers, so a revert to WithTelemetry would drop every
// tenant-labelled series with a fully green suite.
//
// The two variants are mutually exclusive: WithAuthenticatedTenantHTTPMetrics
// already records the standard HTTP telemetry, so registering both would double
// every observation of http.server.request.duration.
func TestUnifiedServer_RegistersTenantTelemetryVariant(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("unified-server.go")
	require.NoError(t, err)

	body := string(source)

	require.Contains(t, body, "WithAuthenticatedTenantHTTPMetrics(",
		"the unified server must register the per-tenant telemetry variant")
	require.False(t, strings.Contains(body, "tlMid.WithTelemetry("),
		"WithTelemetry must not be registered alongside the per-tenant variant: both record http.server.request.duration")

	// T11: probe paths must reach the middleware as excluded routes. Behaviour
	// is covered in pkg/net/http; this pins that the ledger actually passes the
	// list, which that test cannot observe.
	require.Contains(t, body, "WithAuthenticatedTenantHTTPMetrics(telemetry, skipTelemetryPaths...)",
		"the probe paths must be passed to the telemetry middleware as excluded routes")

	for _, path := range []string{"/health", "/readyz", "/metrics"} {
		require.Containsf(t, body, `"`+path+`"`, "%s must be in skipTelemetryPaths", path)
	}
}
