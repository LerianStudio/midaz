// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build libsd

package servicediscovery

import (
	"errors"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libsd "github.com/LerianStudio/lib-service-discovery/v2"
	"github.com/stretchr/testify/require"
)

func TestBuildManager_DisabledReturnsNoopManager(t *testing.T) {
	t.Setenv("SD_ENABLED", "")
	t.Setenv("SERVICE_DISCOVERY_ENABLED", "")
	t.Setenv("SD_ADVERTISE_ADDRESS", "")
	t.Setenv("SERVICE_ADVERTISE_ADDR", "")

	manager, enabled, err := BuildManager(libLog.NewNop())

	require.NoError(t, err)
	require.NotNil(t, manager)
	require.False(t, enabled)
}

func TestBuildManager_EnabledWithoutAdvertiseAddrFailsFast(t *testing.T) {
	t.Setenv("SD_ENABLED", "true")
	t.Setenv("SD_EXTERNAL_ADDRESS", "")
	t.Setenv("SD_INTERNAL_ADDRESS", "")
	t.Setenv("SD_ADVERTISE_ADDRESS", "")
	t.Setenv("SERVICE_ADVERTISE_ADDR", "")

	manager, enabled, err := BuildManager(libLog.NewNop())

	require.Error(t, err)
	require.Nil(t, manager)
	require.True(t, enabled)
	require.True(t, errors.Is(err, libsd.ErrNoEndpoint))
}

func TestBuildManager_EnabledWithInternalOnlyAdvertisePasses(t *testing.T) {
	t.Setenv("SD_ENABLED", "true")
	t.Setenv("SD_INTERNAL_ADDRESS", "internal-host:9000")
	t.Setenv("SD_EXTERNAL_ADDRESS", "")
	t.Setenv("SD_ADVERTISE_ADDRESS", "")
	t.Setenv("SERVICE_ADVERTISE_ADDR", "")

	manager, enabled, err := BuildManager(libLog.NewNop())

	require.NoError(t, err)
	require.NotNil(t, manager)
	require.True(t, enabled)
}
