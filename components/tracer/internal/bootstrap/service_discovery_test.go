// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgsd "github.com/LerianStudio/midaz/v4/pkg/servicediscovery"
)

// clearServiceDiscoveryEnv wipes the discovery-toggle and advertise env vars so a
// test starts from a known-disabled, no-endpoint baseline regardless of the host
// environment. t.Setenv restores the originals at test end.
func clearServiceDiscoveryEnv(t *testing.T) {
	t.Helper()

	t.Setenv("SD_ENABLED", "")
	t.Setenv("SERVICE_DISCOVERY_ENABLED", "")
	t.Setenv("SD_EXTERNAL_ADDRESS", "")
	t.Setenv("SD_INTERNAL_ADDRESS", "")
	t.Setenv("SD_ADVERTISE_ADDRESS", "")
}

// TestWireServiceDiscovery_DisabledIgnoresMalformedServerAddress locks the boot
// parity invariant: with discovery disabled the advertised port is never parsed,
// so a malformed SERVER_ADDRESS must NOT abort boot. The descriptor stays
// zero-value.
func TestWireServiceDiscovery_DisabledIgnoresMalformedServerAddress(t *testing.T) {
	clearServiceDiscoveryEnv(t)

	cfg := &Config{ServerAddress: "not-a-valid-address", PluginAuthEnabled: false, PluginAuthAddress: "http://plugin-auth:4000"}

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop(), metrics.NewNopFactory())

	require.NoError(t, err, "malformed SERVER_ADDRESS must not fail boot when discovery is disabled")
	require.False(t, sd.enabled)
	require.NotNil(t, sd.manager)
	assert.Empty(t, sd.descriptor.ID, "descriptor must stay zero-value when discovery is disabled")
	assert.Equal(t, "http://plugin-auth:4000", sd.authHost,
		"auth disabled returns the static host without resolving")
}

// TestWireServiceDiscovery_AuthEnabledDiscoveryDisabledFallsBackToStaticHost
// covers the realistic production state: auth on, discovery not yet rolled out.
// With SD disabled the no-op Manager's Resolve returns the fallback immediately,
// so authHost degrades to the static PluginAuthAddress without hanging.
func TestWireServiceDiscovery_AuthEnabledDiscoveryDisabledFallsBackToStaticHost(t *testing.T) {
	clearServiceDiscoveryEnv(t)

	cfg := &Config{ServerAddress: ":4020", PluginAuthEnabled: true, PluginAuthAddress: "http://plugin-auth:4000"}

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop(), metrics.NewNopFactory())

	require.NoError(t, err)
	require.False(t, sd.enabled, "discovery must stay disabled when SD_ENABLED is unset")
	require.NotNil(t, sd.manager)
	assert.Equal(t, "http://plugin-auth:4000", sd.authHost,
		"auth enabled + discovery disabled must fall back to the static host")
}

// TestWireServiceDiscovery_DisabledUsesNopRecorder locks the SD metrics
// INVARIANT: when discovery is disabled, wireServiceDiscovery must forward a
// NopMetricsRecorder — even though a real MetricsFactory is available — so the
// resolve path (and every downstream SD metric) emits nothing with SD off.
func TestWireServiceDiscovery_DisabledUsesNopRecorder(t *testing.T) {
	clearServiceDiscoveryEnv(t)

	cfg := &Config{ServerAddress: ":4020", PluginAuthEnabled: true, PluginAuthAddress: "http://plugin-auth:4000"}

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop(), metrics.NewNopFactory())

	require.NoError(t, err)
	require.False(t, sd.enabled)

	_, isNop := sd.recorder.(pkgsd.NopMetricsRecorder)
	assert.True(t, isNop,
		"SD disabled must yield a NopMetricsRecorder so zero SD metrics are emitted")
}

// TestMetricsFactoryFromTelemetry locks the nil-safe telemetry read: a nil
// handle (ENABLE_TELEMETRY=false) yields a nil factory so the downstream
// recorder degrades to a no-op, and a non-nil handle returns its MetricsFactory
// unchanged. NewNopFactory gives a cheap real *MetricsFactory to prove the
// non-nil branch without booting real telemetry.
func TestMetricsFactoryFromTelemetry(t *testing.T) {
	t.Parallel()

	assert.Nil(t, metricsFactoryFromTelemetry(nil),
		"nil telemetry must yield a nil factory so the recorder degrades to a no-op")

	factory := metrics.NewNopFactory()
	telemetry := &libOtel.Telemetry{MetricsFactory: factory}

	assert.Same(t, factory, metricsFactoryFromTelemetry(telemetry),
		"non-nil telemetry must return its MetricsFactory handle unchanged")
}

// TestShouldRegisterServiceDiscovery asserts the pure launcher predicate: the
// Service Discovery Launcher app registers IFF discovery is enabled AND a Manager
// is present.
func TestShouldRegisterServiceDiscovery(t *testing.T) {
	t.Parallel()

	mgr := &pkgsd.Manager{}

	assert.True(t, shouldRegisterServiceDiscovery(true, mgr),
		"enabled + non-nil manager must register")
	assert.False(t, shouldRegisterServiceDiscovery(false, mgr),
		"disabled must not register even with a manager")
	assert.False(t, shouldRegisterServiceDiscovery(true, nil),
		"enabled + nil manager must not register")
	assert.False(t, shouldRegisterServiceDiscovery(false, nil),
		"disabled + nil manager must not register")
}
