// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/log"
	libsd "github.com/LerianStudio/lib-service-discovery"
	"github.com/stretchr/testify/require"
)

func TestBuildServiceDiscovery_DisabledReturnsNoopManager(t *testing.T) {
	t.Setenv("SD_ENABLED", "")
	t.Setenv("SERVICE_DISCOVERY_ENABLED", "")
	t.Setenv("SD_ADVERTISE_ADDRESS", "")
	t.Setenv("SERVICE_ADVERTISE_ADDR", "")

	manager, enabled, err := buildServiceDiscovery(libLog.NewNop())

	require.NoError(t, err)
	require.NotNil(t, manager)
	require.False(t, enabled)
}

func TestBuildServiceDiscovery_EnabledWithoutAdvertiseAddrFailsFast(t *testing.T) {
	t.Setenv("SD_ENABLED", "true")
	t.Setenv("SD_ADVERTISE_ADDRESS", "")
	t.Setenv("SERVICE_ADVERTISE_ADDR", "")

	manager, enabled, err := buildServiceDiscovery(libLog.NewNop())

	require.Error(t, err)
	require.Nil(t, manager)
	require.True(t, enabled)
	require.True(t, errors.Is(err, libsd.ErrEmptyAdvertiseAddr))
}
