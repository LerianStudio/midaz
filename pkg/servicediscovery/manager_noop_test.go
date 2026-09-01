// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build !libsd

package servicediscovery

import (
	"context"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildManager_NoopReturnsDisabled locks the DEFAULT (no-op) build contract:
// BuildManager returns a non-nil Manager, reports discovery disabled, and never
// errors regardless of SD_* env — the default build carries no lib-service-
// discovery dependency, so discovery can never be enabled.
func TestBuildManager_NoopReturnsDisabled(t *testing.T) {
	t.Setenv("SD_ENABLED", "true")
	t.Setenv("SD_EXTERNAL_ADDRESS", "midaz-ledger")

	manager, enabled, err := BuildManager(libLog.NewNop())

	require.NoError(t, err, "the no-op build never fails to build the manager")
	require.NotNil(t, manager)
	assert.False(t, enabled, "the no-op build always reports discovery disabled")
}

// TestManager_ResolvePreferredURL_NoopErrs proves the no-op Manager resolve
// returns an error so ResolveAuthHost degrades to the configured static host,
// matching SD-disabled semantics.
func TestManager_ResolvePreferredURL_NoopErrs(t *testing.T) {
	t.Parallel()

	m := &Manager{}

	got, err := m.ResolvePreferredURL(context.Background(), "plugin-auth", "http://plugin-auth:4000")

	require.Error(t, err, "the no-op resolve must error so callers fall back to the static host")
	assert.Empty(t, got)
}

// TestNoopRunnable_And_BootCloser_Safe proves the no-op Runnable and BootCloser
// compile and no-op safely: Run returns nil, and the boot closer's Disarm /
// CloseOnBootFailure never panic.
func TestNoopRunnable_And_BootCloser_Safe(t *testing.T) {
	t.Parallel()

	desc := BuildServiceDescriptor("midaz-ledger", 3002)

	r := NewRunnable(&Manager{}, desc, libLog.NewNop(), nil)
	require.NotNil(t, r)
	require.NoError(t, r.Run(nil), "no-op Runnable.Run must return nil")

	closer := NewBootCloser(libLog.NewNop(), &Manager{})
	require.NotNil(t, closer)
	require.NotPanics(t, closer.Disarm)
	require.NotPanics(t, closer.CloseOnBootFailure)
}
