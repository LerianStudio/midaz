// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

// TestBuildPgManagerOptions_SetsTenantManagerModule guards the per-tenant pool
// manager's module identity. lib-commons resolves the operator-configured
// ConnectionSettings for a tenant by indexing
// TenantConfig.Databases[manager.Module()]; with an empty module it falls
// through to the root-level settings tenant-manager never populates and every
// tenant pool silently runs on the library fallback (25/5). The module MUST be
// the tracer catalog module name so the lookup hits the same key tenant-manager
// writes.
func TestBuildPgManagerOptions_SetsTenantManagerModule(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ApplicationName:                        "tracer",
		MultiTenantEnabled:                     true,
		MultiTenantMaxTenantPools:              100,
		MultiTenantIdleTimeoutSec:              300,
		MultiTenantConnectionsCheckIntervalSec: 30,
	}

	manager := tmpostgres.NewManager(nil, cfg.ApplicationName, buildPgManagerOptions(cfg, testutil.NewMockLogger())...)
	require.NotNil(t, manager)

	assert.Equal(t, constant.ModuleName, manager.Module(),
		"pool manager must resolve tenant connection settings under the tracer catalog module")
	assert.Equal(t, "tracer-api", manager.Module(),
		"module must be the exact key tenant-manager publishes under TenantConfig.Databases")
}

// TestBuildPgManagerOptions_ModuleIsNotTheLibraryDefault is the positive
// control for the test above: a manager built WITHOUT the wiring's option list
// reports an empty module, proving Module() really observes the option and the
// assertion above is not vacuously true.
func TestBuildPgManagerOptions_ModuleIsNotTheLibraryDefault(t *testing.T) {
	t.Parallel()

	bare := tmpostgres.NewManager(nil, "tracer")
	require.NotNil(t, bare)

	assert.Empty(t, bare.Module(), "control: lib-commons default module must be empty")
	assert.NotEqual(t, bare.Module(), constant.ModuleName)
}
