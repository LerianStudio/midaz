// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/lib-observability/metrics"
	pkgsd "github.com/LerianStudio/midaz/v3/pkg/servicediscovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// launcherAppNames extracts the ordered display names from the service's
// assembled Launcher apps so a test can assert on the guard-driven set.
func launcherAppNames(app *Service) []string {
	apps := app.launcherApps()
	names := make([]string, len(apps))

	for i, a := range apps {
		names[i] = a.name
	}

	return names
}

// TestService_launcherApps_ServiceDiscoveryGuard asserts the observable effect
// of the app.ServiceDiscoveryEnabled guard: the "Service Discovery" launcher app
// is present IFF discovery is enabled. Inspects the assembled app list rather than
// starting the blocking Run().
func TestService_launcherApps_ServiceDiscoveryGuard(t *testing.T) {
	t.Parallel()

	disabled := &Service{ServiceDiscoveryEnabled: false}
	assert.NotContains(t, launcherAppNames(disabled), "Service Discovery",
		"disabled service must not register the Service Discovery app")

	enabled := &Service{
		ServiceDiscoveryEnabled: true,
		ServiceDescriptor:       pkgsd.BuildServiceDescriptor("midaz-crm", 4003),
		ServiceDiscoveryMetrics: pkgsd.NewMetricsFactoryRecorder(metrics.NewNopFactory(), libLog.NewNop()),
	}
	assert.Contains(t, launcherAppNames(enabled), "Service Discovery",
		"enabled service must register the Service Discovery app")
}

// TestWireServiceDiscovery_DisabledIgnoresMalformedServerAddress locks Fix #1:
// with discovery disabled, the advertised port is never parsed, so a malformed
// SERVER_ADDRESS must NOT abort boot. The descriptor is left zero-value.
func TestWireServiceDiscovery_DisabledIgnoresMalformedServerAddress(t *testing.T) {
	t.Setenv("SD_ENABLED", "")
	t.Setenv("SERVICE_DISCOVERY_ENABLED", "")

	cfg := &Config{ServerAddress: "not-a-valid-address", AuthEnabled: false, AuthAddress: "http://plugin-auth:4000"}

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop(), metrics.NewNopFactory())

	require.NoError(t, err, "malformed SERVER_ADDRESS must not fail boot when discovery is disabled")
	require.False(t, sd.enabled)
	require.NotNil(t, sd.manager)
	assert.Empty(t, sd.descriptor.ID, "descriptor must stay zero-value when discovery is disabled")
	// Fix #5: auth disabled returns the static host without resolving.
	assert.Equal(t, "http://plugin-auth:4000", sd.authHost)
}

// TestWireServiceDiscovery_AuthEnabledDiscoveryDisabledFallsBackToStaticHost
// covers the realistic production state: auth on, discovery not yet rolled out.
// With SD disabled the no-op Manager's Resolve returns the fallback immediately,
// so authHost degrades to the static cfg.AuthAddress without hanging.
func TestWireServiceDiscovery_AuthEnabledDiscoveryDisabledFallsBackToStaticHost(t *testing.T) {
	t.Setenv("SD_ENABLED", "")
	t.Setenv("SERVICE_DISCOVERY_ENABLED", "")

	cfg := &Config{ServerAddress: ":4003", AuthEnabled: true, AuthAddress: "http://plugin-auth:4000"}

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop(), metrics.NewNopFactory())

	require.NoError(t, err)
	require.False(t, sd.enabled, "discovery must stay disabled when SD_ENABLED is unset")
	require.NotNil(t, sd.manager)
	assert.Equal(t, "http://plugin-auth:4000", sd.authHost,
		"auth enabled + discovery disabled must fall back to the static host")
}

// TestWireServiceDiscovery_DisabledUsesNopRecorder locks the SD metrics INVARIANT:
// when discovery is disabled, wireServiceDiscovery must forward a
// NopMetricsRecorder — even though a real MetricsFactory is available — so that
// the resolve path (and every downstream SD metric) emits nothing with SD off.
func TestWireServiceDiscovery_DisabledUsesNopRecorder(t *testing.T) {
	t.Setenv("SD_ENABLED", "")
	t.Setenv("SERVICE_DISCOVERY_ENABLED", "")

	cfg := &Config{ServerAddress: ":4003", AuthEnabled: true, AuthAddress: "http://plugin-auth:4000"}

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop(), metrics.NewNopFactory())

	require.NoError(t, err)
	require.False(t, sd.enabled)

	_, isNop := sd.recorder.(pkgsd.NopMetricsRecorder)
	assert.True(t, isNop,
		"SD disabled must yield a NopMetricsRecorder so zero SD metrics are emitted")
}

// TestWireServiceDiscovery_EnabledUsesRealRecorder asserts that when discovery is
// enabled with a real MetricsFactory the wiring carries the OTel-backed recorder
// (not the no-op), so register/deregister/resolve metrics actually flow.
func TestWireServiceDiscovery_EnabledUsesRealRecorder(t *testing.T) {
	t.Setenv("SD_ENABLED", "true")
	t.Setenv("SD_ADVERTISE_ADDRESS", "midaz-crm")

	// AuthEnabled=false so ResolveAuthHost is skipped and the test never dials a
	// registry; the recorder posture is what is under test.
	cfg := &Config{ServerAddress: ":4003", AuthEnabled: false, AuthAddress: "http://plugin-auth:4000"}

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop(), metrics.NewNopFactory())

	require.NoError(t, err)
	require.True(t, sd.enabled)
	require.NotNil(t, sd.recorder)

	_, isNop := sd.recorder.(pkgsd.NopMetricsRecorder)
	assert.False(t, isNop,
		"SD enabled with a real factory must yield the OTel-backed recorder")
}

// TestWireServiceDiscovery_EnabledNilFactoryDegradesToNop asserts that when SD is
// enabled but telemetry is off (nil factory), the recorder degrades to a no-op via
// NewMetricsFactoryRecorder — safe, and never a nil deref at the call sites.
func TestWireServiceDiscovery_EnabledNilFactoryDegradesToNop(t *testing.T) {
	t.Setenv("SD_ENABLED", "true")
	t.Setenv("SD_ADVERTISE_ADDRESS", "midaz-crm")

	cfg := &Config{ServerAddress: ":4003", AuthEnabled: false, AuthAddress: "http://plugin-auth:4000"}

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop(), nil)

	require.NoError(t, err)
	require.True(t, sd.enabled)
	require.NotNil(t, sd.recorder)

	_, isNop := sd.recorder.(pkgsd.NopMetricsRecorder)
	assert.True(t, isNop, "nil factory must degrade to a NopMetricsRecorder")
}
