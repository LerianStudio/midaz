// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"testing"

	pkg "github.com/LerianStudio/midaz/v3/pkg/reporter"
	pg "github.com/LerianStudio/midaz/v3/pkg/reporter/postgres"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	libObservability "github.com/LerianStudio/lib-observability"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time check: DirectProvider must implement DataSourceProvider.
var _ DataSourceProvider = (*DirectProvider)(nil)

// newTestSafeDataSources creates a SafeDataSources for testing with the given map
// and registers the IDs in the immutable registry.
func newTestSafeDataSources(t *testing.T, dsMap map[string]pkg.DataSource) *pkg.SafeDataSources {
	t.Helper()

	pkg.ResetRegisteredDataSourceIDsForTesting()

	ids := make([]string, 0, len(dsMap))
	for k := range dsMap {
		ids = append(ids, k)
	}

	pkg.RegisterDataSourceIDsForTesting(ids)

	return pkg.NewSafeDataSources(dsMap)
}

func TestDirectProvider_Constructor(t *testing.T) {
	tests := []struct {
		name      string
		sds       *pkg.SafeDataSources
		cbManager *pkg.CircuitBreakerManager
		hcRunner  *pkg.HealthChecker
	}{
		{
			name:      "creates with nil optional dependencies",
			sds:       pkg.NewSafeDataSources(nil),
			cbManager: nil,
			hcRunner:  nil,
		},
		{
			name:      "creates with non-nil dependencies",
			sds:       pkg.NewSafeDataSources(nil),
			cbManager: createTestCircuitBreakerManager(),
			hcRunner:  createTestHealthChecker(t, nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewDirectProvider(tt.sds, tt.cbManager, tt.hcRunner)

			require.NotNil(t, provider)
			assert.Equal(t, tt.sds, provider.safeDatasources)
			assert.Equal(t, tt.cbManager, provider.circuitBreakerManager)
			assert.Equal(t, tt.hcRunner, provider.healthChecker)
		})
	}
}

func TestDirectProvider_ListDataSources(t *testing.T) {
	tests := []struct {
		name    string
		dsMap   map[string]pkg.DataSource
		wantLen int
		wantErr bool
	}{
		{
			name: "returns all configured datasources with correct fields",
			dsMap: map[string]pkg.DataSource{
				"pg_sales": {
					DatabaseType: pkg.PostgreSQLType,
					Initialized:  true,
					Status:       libConstants.DataSourceStatusAvailable,
					DatabaseConfig: &pg.Connection{
						DBName: "sales_db",
					},
				},
			},
			wantLen: 1,
			wantErr: false,
		},
		{
			name:    "returns empty list when no datasources configured",
			dsMap:   map[string]pkg.DataSource{},
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "includes unavailable datasources in listing",
			dsMap: map[string]pkg.DataSource{
				"pg_down": {
					DatabaseType: pkg.PostgreSQLType,
					Initialized:  false,
					Status:       libConstants.DataSourceStatusUnavailable,
					DatabaseConfig: &pg.Connection{
						DBName: "down_db",
					},
				},
			},
			wantLen: 1,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sds := newTestSafeDataSources(t, tt.dsMap)
			provider := NewDirectProvider(sds, nil, nil)

			result, err := provider.ListDataSources(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, result, tt.wantLen)

			// M2: Assert Name, Type, and Status fields for each datasource
			for _, info := range result {
				ds, exists := tt.dsMap[info.ID]
				require.True(t, exists, "unexpected datasource ID: %s", info.ID)
				assert.Equal(t, ds.DatabaseType, info.Type, "Type mismatch for %s", info.ID)
				assert.Equal(t, ds.Status, info.Status, "Status mismatch for %s", info.ID)

				if ds.DatabaseConfig != nil {
					assert.Equal(t, ds.DatabaseConfig.DBName, info.Name, "Name mismatch for %s", info.ID)
				}
			}
		})
	}
}

func TestDirectProvider_HealthCheck(t *testing.T) {
	t.Run("returns status from SafeDataSources when no health checker", func(t *testing.T) {
		dsMap := map[string]pkg.DataSource{
			"pg_main": {
				DatabaseType: pkg.PostgreSQLType,
				Initialized:  true,
				Status:       libConstants.DataSourceStatusAvailable,
			},
			"pg_down": {
				DatabaseType: pkg.PostgreSQLType,
				Initialized:  false,
				Status:       libConstants.DataSourceStatusUnavailable,
			},
		}

		sds := newTestSafeDataSources(t, dsMap)
		provider := NewDirectProvider(sds, nil, nil)

		result, err := provider.HealthCheck(context.Background())

		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.True(t, result["pg_main"], "available+initialized should be true")
		assert.False(t, result["pg_down"], "unavailable+uninitialized should be false")
	})

	// C1: Exercise the HealthChecker branch with a real HealthChecker
	t.Run("delegates to health checker when available", func(t *testing.T) {
		rawDS := map[string]pkg.DataSource{
			"pg_main": {
				DatabaseType: pkg.PostgreSQLType,
				Initialized:  true,
				Status:       libConstants.DataSourceStatusAvailable,
			},
			"mongo_crm": {
				DatabaseType: pkg.MongoDBType,
				Initialized:  false,
				Status:       libConstants.DataSourceStatusUnavailable,
			},
		}

		sds := newTestSafeDataSources(t, rawDS)

		// Create a real HealthChecker backed by the same datasource map.
		// GetHealthStatus() reads from *dataSources and appends CB state.
		hc := createTestHealthChecker(t, &rawDS)

		provider := NewDirectProvider(sds, nil, hc)

		result, err := provider.HealthCheck(context.Background())

		require.NoError(t, err)
		assert.Len(t, result, 2)

		// pg_main status = "available (CB: ...)" -> starts with "available" -> true
		assert.True(t, result["pg_main"], "available DS should map to true via HealthChecker")
		// mongo_crm status = "unavailable (CB: ...)" -> does NOT start with "available" -> false
		assert.False(t, result["mongo_crm"], "unavailable DS should map to false via HealthChecker")
	})

	t.Run("health checker returns false for unknown datasource ID", func(t *testing.T) {
		// rawDS has one entry; the HealthChecker is built from an empty map
		// so it won't return a status for "pg_orphan".
		emptyRaw := map[string]pkg.DataSource{}
		hc := createTestHealthChecker(t, &emptyRaw)

		dsMap := map[string]pkg.DataSource{
			"pg_orphan": {
				DatabaseType: pkg.PostgreSQLType,
				Initialized:  true,
				Status:       libConstants.DataSourceStatusAvailable,
			},
		}

		sds := newTestSafeDataSources(t, dsMap)
		provider := NewDirectProvider(sds, nil, hc)

		result, err := provider.HealthCheck(context.Background())

		require.NoError(t, err)
		assert.False(t, result["pg_orphan"], "missing from HealthChecker should be false")
	})
}

// createTestCircuitBreakerManager builds a CircuitBreakerManager for tests.
func createTestCircuitBreakerManager() *pkg.CircuitBreakerManager {
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	return pkg.NewCircuitBreakerManager(logger)
}

// createTestHealthChecker builds a HealthChecker for tests.
// If rawDS is nil, an empty map is used.
func createTestHealthChecker(t *testing.T, rawDS *map[string]pkg.DataSource) *pkg.HealthChecker {
	t.Helper()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbManager := pkg.NewCircuitBreakerManager(logger)

	if rawDS == nil {
		empty := make(map[string]pkg.DataSource)
		rawDS = &empty
	}

	hc, err := pkg.NewHealthChecker(rawDS, cbManager, logger)
	require.NoError(t, err)

	return hc
}
