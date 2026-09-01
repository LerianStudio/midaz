// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build libsd

package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/LerianStudio/lib-observability/v2/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgsd "github.com/LerianStudio/midaz/v4/pkg/servicediscovery"
)

// TestWireServiceDiscovery_EnabledUsesRealRecorder asserts that when discovery is
// enabled with a real MetricsFactory the wiring carries the OTel-backed recorder
// (not the no-op), so register/deregister/resolve metrics actually flow.
//
// SD can only be enabled in the //go:build libsd build (the default build no-ops
// SD — see pkg/servicediscovery TODO(3482)), so this enabled-path coverage lives
// behind the tag.
func TestWireServiceDiscovery_EnabledUsesRealRecorder(t *testing.T) {
	t.Setenv("SD_ENABLED", "true")
	t.Setenv("SD_ADVERTISE_ADDRESS", "midaz-ledger")

	// AuthEnabled=false so ResolveAuthHost is skipped and the test never dials a
	// registry; the recorder posture is what is under test.
	cfg := &Config{ServerAddress: ":3002", AuthEnabled: false, AuthHost: "http://plugin-auth:4000"}

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
	t.Setenv("SD_ADVERTISE_ADDRESS", "midaz-ledger")

	cfg := &Config{ServerAddress: ":3002", AuthEnabled: false, AuthHost: "http://plugin-auth:4000"}

	sd, err := wireServiceDiscovery(cfg, libLog.NewNop(), nil)

	require.NoError(t, err)
	require.True(t, sd.enabled)
	require.NotNil(t, sd.recorder)

	_, isNop := sd.recorder.(pkgsd.NopMetricsRecorder)
	assert.True(t, isNop, "nil factory must degrade to a NopMetricsRecorder")
}
