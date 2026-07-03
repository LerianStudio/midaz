// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	pkgsd "github.com/LerianStudio/midaz/v3/pkg/servicediscovery"
	"github.com/stretchr/testify/assert"
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
	}
	assert.Contains(t, launcherAppNames(enabled), "Service Discovery",
		"enabled service must register the Service Discovery app")
}
