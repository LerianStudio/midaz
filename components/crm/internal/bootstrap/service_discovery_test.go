// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-observability/log"
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

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop())

	require.NoError(t, err, "malformed SERVER_ADDRESS must not fail boot when discovery is disabled")
	require.False(t, sd.enabled)
	require.NotNil(t, sd.manager)
	assert.Empty(t, sd.descriptor.ID, "descriptor must stay zero-value when discovery is disabled")
	// Fix #5: auth disabled returns the static host without resolving.
	assert.Equal(t, "http://plugin-auth:4000", sd.authHost)
}
