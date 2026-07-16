// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"sync"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/lib-observability/metrics"
	libsd "github.com/LerianStudio/lib-service-discovery"
	pkgsd "github.com/LerianStudio/midaz/v3/pkg/servicediscovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
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

// closeStubRegistry is a minimal in-memory libsd.Registry whose Watch returns a
// live channel so a Resolve lazy-spawns a background watcher goroutine that only
// dies when the manager's base context is cancelled by Close. It records Close via
// the optional Close() seam that libsd.Manager.Close delegates to, so a test can
// assert closeManagerOnBootFailure actually closed the manager.
type closeStubRegistry struct {
	mu         sync.Mutex
	closeCalls int
	watchCh    chan libsd.Event
}

func newCloseStubRegistry() *closeStubRegistry {
	return &closeStubRegistry{watchCh: make(chan libsd.Event)}
}

func (s *closeStubRegistry) Register(_ context.Context, _ libsd.Service) error { return nil }
func (s *closeStubRegistry) Deregister(_ context.Context, _ string) error      { return nil }
func (s *closeStubRegistry) Resolve(_ context.Context, _, _ string) (libsd.Service, error) {
	return libsd.Service{}, errors.New("no healthy instances")
}

// Watch returns the shared live channel; the watcher goroutine blocks on it until
// the manager's base context is cancelled by Close.
func (s *closeStubRegistry) Watch(_ context.Context, _ string) (<-chan libsd.Event, error) {
	return s.watchCh, nil
}

// Close satisfies the optional seam libsd.Manager.Close delegates to.
func (s *closeStubRegistry) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closeCalls++

	return nil
}

func (s *closeStubRegistry) closeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.closeCalls
}

// TestCloseManagerOnBootFailure_ClosesManagerAndStopsWatcher proves the boot-failure
// helper tears down the boot-time watcher goroutine (goleak-guarded) and closes the
// manager exactly once. It builds an enabled manager over a stub registry, triggers
// a Resolve to lazy-spawn the watcher, then asserts closeManagerOnBootFailure leaves
// no leaked goroutine and Close was invoked once.
func TestCloseManagerOnBootFailure_ClosesManagerAndStopsWatcher(t *testing.T) {
	// Ignore the lib-commons tenant-manager in-memory cache cleanup loop: sibling
	// tests in this package spawn it and it is not torn down by this test. The SD
	// watcher this test is responsible for still surfaces if it leaks, so the
	// assertion stays binding on the code under test.
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/cache.(*InMemoryCache).cleanupLoop"))

	stub := newCloseStubRegistry()

	mgr, err := libsd.New(
		libsd.Config{Enabled: true, ConsulAddr: "consul:8500", AdvertiseAddr: "svc.test:4003"},
		libsd.WithLogger(libLog.NewNop()),
		libsd.WithRegistry(stub),
	)
	require.NoError(t, err)

	// Resolve lazy-spawns the managed watcher goroutine (Watch returns a live
	// channel) that only exits when Close cancels the manager base context.
	_, _ = mgr.Resolve(context.Background(), "plugin-auth", "fallback:4000")

	closeManagerOnBootFailure(libLog.NewNop(), mgr)

	assert.Equal(t, 1, stub.closeCount(), "boot-failure close must close the manager exactly once")
}

// TestCloseManagerOnBootFailure_NilManagerNoOp confirms the helper is nil-safe: it
// neither panics nor closes anything when handed a nil manager.
func TestCloseManagerOnBootFailure_NilManagerNoOp(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		closeManagerOnBootFailure(libLog.NewNop(), nil)
	})
}
