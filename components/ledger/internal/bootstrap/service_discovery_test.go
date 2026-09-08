// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgsd "github.com/LerianStudio/midaz/v4/pkg/servicediscovery"
)

// launcherAppNames extracts the ordered display names from the service's
// assembled Launcher apps so a test can assert on the guard-driven set.
func launcherAppNames(s *Service) []string {
	apps := s.launcherApps()
	names := make([]string, len(apps))

	for i, a := range apps {
		names[i] = a.name
	}

	return names
}

// TestService_launcherApps_ServiceDiscoveryGuard asserts the observable effect
// of the s.ServiceDiscoveryEnabled guard: the "Service Discovery" launcher app is
// present IFF discovery is enabled. Inspects the assembled app list rather than
// starting the blocking Run().
func TestService_launcherApps_ServiceDiscoveryGuard(t *testing.T) {
	t.Parallel()

	disabled := &Service{ServiceDiscoveryEnabled: false}
	assert.NotContains(t, launcherAppNames(disabled), "Service Discovery",
		"disabled service must not register the Service Discovery app")

	enabled := &Service{
		ServiceDiscoveryEnabled: true,
		ServiceDescriptor:       pkgsd.BuildServiceDescriptor("midaz-ledger", 3002),
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

	cfg := &Config{ServerAddress: "not-a-valid-address", AuthEnabled: false, AuthHost: "http://plugin-auth:4000"}

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
// so authHost degrades to the static cfg.AuthHost without hanging.
func TestWireServiceDiscovery_AuthEnabledDiscoveryDisabledFallsBackToStaticHost(t *testing.T) {
	t.Setenv("SD_ENABLED", "")
	t.Setenv("SERVICE_DISCOVERY_ENABLED", "")

	cfg := &Config{ServerAddress: ":3002", AuthEnabled: true, AuthHost: "http://plugin-auth:4000"}

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

	cfg := &Config{ServerAddress: ":3002", AuthEnabled: true, AuthHost: "http://plugin-auth:4000"}

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop(), metrics.NewNopFactory())

	require.NoError(t, err)
	require.False(t, sd.enabled)

	_, isNop := sd.recorder.(pkgsd.NopMetricsRecorder)
	assert.True(t, isNop,
		"SD disabled must yield a NopMetricsRecorder so zero SD metrics are emitted")
}
