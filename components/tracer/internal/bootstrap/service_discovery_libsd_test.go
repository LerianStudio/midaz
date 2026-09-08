// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build libsd

package bootstrap

import (
	"errors"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	libsd "github.com/LerianStudio/lib-service-discovery/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgsd "github.com/LerianStudio/midaz/v4/pkg/servicediscovery"
)

// TestWireServiceDiscovery_EnabledUsesRealRecorder asserts that when discovery is
// enabled with a real MetricsFactory the wiring carries the OTel-backed recorder
// (not the no-op), so register/deregister/resolve metrics actually flow.
// PluginAuthEnabled=false so ResolveAuthHost is skipped and the test never dials
// a registry; the recorder posture is what is under test.
//
// SD can only be enabled in the //go:build libsd build (the default build no-ops
// SD — see pkg/servicediscovery TODO(3482)), so this enabled-path coverage lives
// behind the tag.
func TestWireServiceDiscovery_EnabledUsesRealRecorder(t *testing.T) {
	clearServiceDiscoveryEnv(t)
	t.Setenv("SD_ENABLED", "true")
	t.Setenv("SD_EXTERNAL_ADDRESS", "midaz-tracer")

	cfg := &Config{ServerAddress: ":4020", PluginAuthEnabled: false, PluginAuthAddress: "http://plugin-auth:4000"}

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop(), metrics.NewNopFactory())

	require.NoError(t, err)
	require.True(t, sd.enabled)
	require.NotNil(t, sd.recorder)
	assert.Equal(t, "midaz-tracer", sd.descriptor.Name,
		"enabled discovery must advertise under the midaz-tracer registry name")

	_, isNop := sd.recorder.(pkgsd.NopMetricsRecorder)
	assert.False(t, isNop,
		"SD enabled with a real factory must yield the OTel-backed recorder")
}

// TestWireServiceDiscovery_EnabledNilFactoryDegradesToNop asserts that when SD is
// enabled but telemetry is off (nil factory), the recorder degrades to a no-op via
// NewMetricsFactoryRecorder — safe, and never a nil deref at the call sites.
func TestWireServiceDiscovery_EnabledNilFactoryDegradesToNop(t *testing.T) {
	clearServiceDiscoveryEnv(t)
	t.Setenv("SD_ENABLED", "true")
	t.Setenv("SD_EXTERNAL_ADDRESS", "midaz-tracer")

	cfg := &Config{ServerAddress: ":4020", PluginAuthEnabled: false, PluginAuthAddress: "http://plugin-auth:4000"}

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop(), nil)

	require.NoError(t, err)
	require.True(t, sd.enabled)
	require.NotNil(t, sd.recorder)

	_, isNop := sd.recorder.(pkgsd.NopMetricsRecorder)
	assert.True(t, isNop, "nil factory must degrade to a NopMetricsRecorder")
}

// TestWireServiceDiscovery_EnabledNoEndpointFailsFast locks the wrapper's
// fail-fast contract: enabling discovery with no advertise endpoint must abort
// boot with an error wrapping libsd.ErrNoEndpoint.
func TestWireServiceDiscovery_EnabledNoEndpointFailsFast(t *testing.T) {
	clearServiceDiscoveryEnv(t)
	t.Setenv("SD_ENABLED", "true")

	cfg := &Config{ServerAddress: ":4020", PluginAuthEnabled: false, PluginAuthAddress: "http://plugin-auth:4000"}

	_, err := wireServiceDiscovery(cfg, libLog.NewNop(), metrics.NewNopFactory())

	require.Error(t, err, "enabling discovery with no advertise endpoint must fail boot")
	assert.True(t, errors.Is(err, libsd.ErrNoEndpoint),
		"the error must wrap libsd.ErrNoEndpoint")
}

// TestWireServiceDiscovery_EnabledMalformedServerAddressFailsFast is the mirror
// of the disabled-parity test: with discovery ENABLED the advertised port IS
// parsed, so a malformed SERVER_ADDRESS must abort boot. SD_EXTERNAL_ADDRESS is
// set so BuildManager does not fail-fast on a missing endpoint first — the
// ParseServerPort error branch is the one under test. The wiring returns the
// zero value on failure.
func TestWireServiceDiscovery_EnabledMalformedServerAddressFailsFast(t *testing.T) {
	clearServiceDiscoveryEnv(t)
	t.Setenv("SD_ENABLED", "true")
	t.Setenv("SD_EXTERNAL_ADDRESS", "midaz-tracer")

	cfg := &Config{ServerAddress: "not-a-valid-address", PluginAuthEnabled: false, PluginAuthAddress: "http://plugin-auth:4000"}

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop(), metrics.NewNopFactory())

	require.Error(t, err, "malformed SERVER_ADDRESS must abort boot when discovery is enabled")
	require.False(t, sd.enabled, "failed wiring must return the zero value")
	require.Nil(t, sd.manager, "failed wiring must return the zero value")
	assert.Empty(t, sd.descriptor.ID, "failed wiring must return the zero-value descriptor")
}
