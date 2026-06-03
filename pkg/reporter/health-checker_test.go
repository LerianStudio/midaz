// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/constant"
	mongoMock "github.com/LerianStudio/midaz/v3/pkg/reporter/mongodb"
	pgMock "github.com/LerianStudio/midaz/v3/pkg/reporter/postgres"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestHealthChecker_New(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	assert.NotNil(t, hc)
	assert.NotNil(t, hc.dataSources)
	assert.NotNil(t, hc.circuitBreakerManager)
	assert.NotNil(t, hc.logger)
	assert.NotNil(t, hc.stopChan)
}

func TestHealthChecker_StartAndStop(t *testing.T) {
	t.Parallel()

	// Skip this test in short mode since it requires waiting for the health checker's
	// initial 5-second delay to complete before Stop() can take effect
	if testing.Short() {
		t.Skip("Skipping test in short mode - requires waiting for health check delay")
	}

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// Start should not block
	hc.Start()

	// Wait for the initial delay (5 seconds) plus buffer time
	// so that the health check loop enters its select statement
	time.Sleep(6 * time.Second)

	// Stop should gracefully terminate once in the select loop
	done := make(chan struct{})
	go func() {
		hc.Stop()
		close(done)
	}()

	// Should stop quickly once stopChan is closed
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Health checker did not stop within expected time")
	}
}

func TestHealthChecker_StartNotBlocking(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// Start should not block - verify it returns immediately
	started := make(chan struct{})
	go func() {
		hc.Start()
		close(started)
	}()

	select {
	case <-started:
		// Success - Start() returned immediately
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Start() should not block")
	}

	// Note: Not calling Stop() here to avoid the 5s+ delay in tests.
	// The goroutine will be cleaned up when the test exits.
}

func TestHealthChecker_NeedsHealing(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	tests := []struct {
		name           string
		datasourceName string
		ds             DataSource
		setup          func()
		expected       bool
	}{
		{
			name:           "Unavailable status - needs healing",
			datasourceName: "test_db_1",
			ds: DataSource{
				Status:      libConstants.DataSourceStatusUnavailable,
				Initialized: true,
			},
			setup:    func() {},
			expected: true,
		},
		{
			name:           "Not initialized - needs healing",
			datasourceName: "test_db_2",
			ds: DataSource{
				Status:      libConstants.DataSourceStatusAvailable,
				Initialized: false,
			},
			setup:    func() {},
			expected: true,
		},
		{
			name:           "Available and initialized - no healing needed",
			datasourceName: "test_db_3",
			ds: DataSource{
				Status:      libConstants.DataSourceStatusAvailable,
				Initialized: true,
			},
			setup: func() {
				// Ensure circuit breaker is healthy
				cbManager.GetOrCreate("test_db_3")
			},
			expected: false,
		},
		{
			name:           "Circuit breaker open - needs healing",
			datasourceName: "test_db_4",
			ds: DataSource{
				Status:      libConstants.DataSourceStatusAvailable,
				Initialized: true,
			},
			setup: func() {
				// Force circuit breaker to open state
				cb := cbManager.GetOrCreate("test_db_4")
				// Execute enough failures to trip the circuit breaker
				for i := 0; i < 10; i++ {
					_, _ = cb.Execute(func() (any, error) {
						return nil, assert.AnError
					})
				}
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.setup()
			result := hc.needsHealing(tt.datasourceName, tt.ds)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHealthChecker_GetHealthStatus(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	dataSources["db1"] = DataSource{
		Status:       libConstants.DataSourceStatusAvailable,
		DatabaseType: PostgreSQLType,
		Initialized:  true,
	}
	dataSources["db2"] = DataSource{
		Status:       libConstants.DataSourceStatusUnavailable,
		DatabaseType: MongoDBType,
		Initialized:  false,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("db1")

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	status := hc.GetHealthStatus()

	assert.Len(t, status, 2)
	assert.Contains(t, status["db1"], libConstants.DataSourceStatusAvailable)
	assert.Contains(t, status["db2"], libConstants.DataSourceStatusUnavailable)
}

func TestHealthChecker_GetHealthStatus_Empty(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	status := hc.GetHealthStatus()

	assert.Empty(t, status)
}

func TestHealthChecker_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	for i := 0; i < 5; i++ {
		name := "db" + string(rune('0'+i))
		dataSources[name] = DataSource{
			Status:       libConstants.DataSourceStatusAvailable,
			DatabaseType: PostgreSQLType,
			Initialized:  true,
		}
	}

	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// Test concurrent read access
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			_ = hc.GetHealthStatus()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Concurrent access test timed out")
		}
	}
}

func TestHealthChecker_PingDataSource_UnknownType(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	ds := &DataSource{
		DatabaseType: "unknown_type",
		Initialized:  true,
	}

	// Should return false for unknown database type
	result := hc.pingDataSource(nil, "test_db", ds)
	assert.False(t, result)
}

func TestHealthChecker_PingDataSource_PostgreSQLWithSchemas(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// DataSource with schemas configured but nil repository - should return false
	ds := &DataSource{
		DatabaseType:       PostgreSQLType,
		PostgresRepository: nil,
		Schemas:            []string{"public", "sales"},
		Initialized:        true,
	}

	result := hc.pingDataSource(context.Background(), "test_db", ds)
	assert.False(t, result)
}

func TestHealthChecker_PingDataSource_PostgreSQLEmptySchemas(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// DataSource with empty schemas should default to public, but nil repo returns false
	ds := &DataSource{
		DatabaseType:       PostgreSQLType,
		PostgresRepository: nil,
		Schemas:            []string{},
		Initialized:        true,
	}

	result := hc.pingDataSource(context.Background(), "test_db", ds)
	assert.False(t, result)
}

func TestHealthChecker_PingDataSource_NilPostgresRepository(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	ds := &DataSource{
		DatabaseType:       PostgreSQLType,
		PostgresRepository: nil,
		Initialized:        true,
	}

	result := hc.pingDataSource(nil, "test_db", ds)
	assert.False(t, result)
}

func TestHealthChecker_PingDataSource_NilMongoDBRepository(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	ds := &DataSource{
		DatabaseType:      MongoDBType,
		MongoDBRepository: nil,
		Initialized:       true,
	}

	result := hc.pingDataSource(nil, "test_db", ds)
	assert.False(t, result)
}

func TestHealthChecker_PerformHealthChecks_AllHealthy(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	dataSources["healthy_db"] = DataSource{
		Status:       libConstants.DataSourceStatusAvailable,
		DatabaseType: PostgreSQLType,
		Initialized:  true,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("healthy_db")

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// Should not panic when all datasources are healthy
	assert.NotPanics(t, func() {
		hc.performHealthChecks()
	})
}

func TestHealthChecker_PerformHealthChecks_WithUnavailable(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	dataSources["unavailable_db"] = DataSource{
		Status:       libConstants.DataSourceStatusUnavailable,
		DatabaseType: PostgreSQLType,
		Initialized:  false,
	}

	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// Should not panic when handling unavailable datasources
	assert.NotPanics(t, func() {
		hc.performHealthChecks()
	})
}

func TestHealthChecker_Stop_WithoutStart(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// Stop without Start should not block or panic
	done := make(chan struct{})
	go func() {
		hc.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success - Stop completed immediately when Start was never called
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() blocked when Start() was never called")
	}
}

func TestHealthCheckConstants(t *testing.T) {
	t.Parallel()

	// Verify health check constants are defined properly
	assert.Equal(t, 30*time.Second, constant.HealthCheckInterval)
	assert.Equal(t, 5*time.Second, constant.HealthCheckTimeout)
}

func TestHealthChecker_AttemptReconnection_UnsupportedDBType(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	ds := &DataSource{
		DatabaseType:   "oracle", // unsupported type
		Initialized:    false,
		Status:         libConstants.DataSourceStatusUnavailable,
		DatabaseConfig: nil,
	}

	// attemptReconnection should return false for unsupported type
	result := hc.attemptReconnection("test_unsupported", ds)
	assert.False(t, result)
	assert.Equal(t, libConstants.DataSourceStatusUnavailable, ds.Status)
	assert.NotNil(t, ds.LastError)
}

func TestHealthChecker_AttemptReconnection_NilConnection(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	ds := &DataSource{
		DatabaseType:   PostgreSQLType,
		Initialized:    false,
		Status:         libConstants.DataSourceStatusUnavailable,
		DatabaseConfig: nil, // No connection config
	}

	// Should return false when connection cannot be established
	result := hc.attemptReconnection("test_db", ds)
	assert.False(t, result)
	assert.Equal(t, libConstants.DataSourceStatusUnavailable, ds.Status)
}

func TestHealthChecker_MultipleDataSources(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	dataSources["db1"] = DataSource{
		Status:       libConstants.DataSourceStatusAvailable,
		DatabaseType: PostgreSQLType,
		Initialized:  true,
	}
	dataSources["db2"] = DataSource{
		Status:       libConstants.DataSourceStatusAvailable,
		DatabaseType: MongoDBType,
		Initialized:  true,
	}
	dataSources["db3"] = DataSource{
		Status:       libConstants.DataSourceStatusUnavailable,
		DatabaseType: PostgreSQLType,
		Initialized:  false,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("db1")
	cbManager.GetOrCreate("db2")

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	status := hc.GetHealthStatus()
	assert.Len(t, status, 3)
}

// ---------------------------------------------------------------------------
// pingDataSource tests with mock repositories
// ---------------------------------------------------------------------------

func TestHealthChecker_PingDataSource_PostgreSQL_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)
	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	mockPgRepo := pgMock.NewMockRepository(ctrl)
	// pingDataSource MUST call the lightweight Ping rather than the
	// expensive GetDatabaseSchema. Schemas are intentionally ignored — Ping
	// does not need to know which schemas are configured.
	mockPgRepo.EXPECT().
		Ping(gomock.Any()).
		Return(nil)

	ds := &DataSource{
		DatabaseType:       PostgreSQLType,
		PostgresRepository: mockPgRepo,
		Schemas:            []string{"public", "sales"},
		Initialized:        true,
	}

	result := hc.pingDataSource(context.Background(), "pg_test_db", ds)
	assert.True(t, result)
}

func TestHealthChecker_PingDataSource_PostgreSQL_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)
	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	mockPgRepo := pgMock.NewMockRepository(ctrl)
	mockPgRepo.EXPECT().
		Ping(gomock.Any()).
		Return(assert.AnError)

	ds := &DataSource{
		DatabaseType:       PostgreSQLType,
		PostgresRepository: mockPgRepo,
		Schemas:            nil, // schemas no longer affect Ping behavior
		Initialized:        true,
	}

	result := hc.pingDataSource(context.Background(), "pg_err_db", ds)
	assert.False(t, result)
}

func TestHealthChecker_PingDataSource_MongoDB_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)
	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	mockMongoRepo := mongoMock.NewMockRepository(ctrl)
	mockMongoRepo.EXPECT().
		Ping(gomock.Any()).
		Return(nil)

	ds := &DataSource{
		DatabaseType:      MongoDBType,
		MongoDBRepository: mockMongoRepo,
		Initialized:       true,
	}

	result := hc.pingDataSource(context.Background(), "mongo_test_db", ds)
	assert.True(t, result)
}

func TestHealthChecker_PingDataSource_MongoDB_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)
	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	mockMongoRepo := mongoMock.NewMockRepository(ctrl)
	mockMongoRepo.EXPECT().
		Ping(gomock.Any()).
		Return(assert.AnError)

	ds := &DataSource{
		DatabaseType:      MongoDBType,
		MongoDBRepository: mockMongoRepo,
		Initialized:       true,
	}

	result := hc.pingDataSource(context.Background(), "mongo_err_db", ds)
	assert.False(t, result)
}

// ---------------------------------------------------------------------------
// GetHealthStatus with circuit breaker states
// ---------------------------------------------------------------------------

func TestHealthChecker_GetHealthStatus_WithCircuitBreakerStates(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	dataSources["healthy_db"] = DataSource{
		Status:       libConstants.DataSourceStatusAvailable,
		DatabaseType: PostgreSQLType,
		Initialized:  true,
	}
	dataSources["unhealthy_db"] = DataSource{
		Status:       libConstants.DataSourceStatusUnavailable,
		DatabaseType: MongoDBType,
		Initialized:  false,
	}
	dataSources["degraded_db"] = DataSource{
		Status:       libConstants.DataSourceStatusDegraded,
		DatabaseType: PostgreSQLType,
		Initialized:  true,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("healthy_db")

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	status := hc.GetHealthStatus()

	assert.Len(t, status, 3)
	assert.Contains(t, status["healthy_db"], libConstants.DataSourceStatusAvailable)
	assert.Contains(t, status["healthy_db"], "CB:")
	assert.Contains(t, status["unhealthy_db"], libConstants.DataSourceStatusUnavailable)
	assert.Contains(t, status["degraded_db"], libConstants.DataSourceStatusDegraded)
}

// ---------------------------------------------------------------------------
// needsHealing edge cases
// ---------------------------------------------------------------------------

func TestHealthChecker_NeedsHealing_CircuitBreakerHealthyButNotOpen(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// Datasource is available, initialized, and CB is not open (half-open)
	// In half-open state, IsHealthy returns true, so needsHealing returns false
	ds := DataSource{
		Status:      libConstants.DataSourceStatusAvailable,
		Initialized: true,
	}

	// The circuit breaker is freshly created (closed state) and healthy
	cbManager.GetOrCreate("halfopen_db")
	result := hc.needsHealing("halfopen_db", ds)
	assert.False(t, result, "healthy initialized datasource with closed CB should not need healing")
}

// ---------------------------------------------------------------------------
// attemptReconnection – MongoDB type (ConnectToDataSource fails because the
// datasource is not registered in the immutable registry)
// ---------------------------------------------------------------------------

func TestHealthChecker_AttemptReconnection_MongoDBType(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	ds := &DataSource{
		DatabaseType: MongoDBType,
		MongoURI:     "mongodb://localhost:27017/test",
		MongoDBName:  "test",
		Initialized:  false,
		Status:       libConstants.DataSourceStatusUnavailable,
	}

	// attemptReconnection should return false because the datasource ID is
	// not registered in the immutable registry – ConnectToDataSource rejects it.
	result := hc.attemptReconnection("mongo_unreg_db", ds)
	assert.False(t, result)
	assert.Equal(t, libConstants.DataSourceStatusUnavailable, ds.Status)
	assert.NotNil(t, ds.LastError)
}

// ---------------------------------------------------------------------------
// performHealthChecks – mixed healthy and unhealthy datasources
// ---------------------------------------------------------------------------

func TestHealthChecker_PerformHealthChecks_MixedHealthyAndUnhealthy(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	dataSources["healthy_pg"] = DataSource{
		Status:       libConstants.DataSourceStatusAvailable,
		DatabaseType: PostgreSQLType,
		Initialized:  true,
	}
	dataSources["unavailable_pg"] = DataSource{
		Status:       libConstants.DataSourceStatusUnavailable,
		DatabaseType: PostgreSQLType,
		Initialized:  false,
	}
	dataSources["healthy_mongo"] = DataSource{
		Status:       libConstants.DataSourceStatusAvailable,
		DatabaseType: MongoDBType,
		Initialized:  true,
	}
	dataSources["degraded_pg"] = DataSource{
		Status:       libConstants.DataSourceStatusDegraded,
		DatabaseType: PostgreSQLType,
		Initialized:  true,
	}

	cbManager := NewCircuitBreakerManager(logger)
	// Create circuit breakers for the healthy ones to keep them healthy
	cbManager.GetOrCreate("healthy_pg")
	cbManager.GetOrCreate("healthy_mongo")
	cbManager.GetOrCreate("degraded_pg")

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// Should not panic; the snapshot-copy logic handles concurrent access correctly.
	assert.NotPanics(t, func() {
		hc.performHealthChecks()
	})

	// Verify healthy datasources are unchanged
	assert.Equal(t, libConstants.DataSourceStatusAvailable, dataSources["healthy_pg"].Status)
	assert.Equal(t, libConstants.DataSourceStatusAvailable, dataSources["healthy_mongo"].Status)
}

// ---------------------------------------------------------------------------
// performHealthChecks – datasource that is not initialized triggers healing
// ---------------------------------------------------------------------------

func TestHealthChecker_PerformHealthChecks_NotInitialized(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	dataSources["uninit_db"] = DataSource{
		Status:       libConstants.DataSourceStatusAvailable,
		DatabaseType: PostgreSQLType,
		Initialized:  false, // not initialized – needsHealing returns true
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("uninit_db")

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// performHealthChecks should attempt healing for the uninitialised datasource
	// and NOT panic even though ConnectToDataSource will fail.
	assert.NotPanics(t, func() {
		hc.performHealthChecks()
	})
}

// ---------------------------------------------------------------------------
// performHealthChecks – circuit breaker open triggers healing path
// ---------------------------------------------------------------------------

func TestHealthChecker_PerformHealthChecks_CircuitBreakerOpen(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	dataSources["cb_open_db"] = DataSource{
		Status:       libConstants.DataSourceStatusAvailable,
		DatabaseType: PostgreSQLType,
		Initialized:  true,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cb := cbManager.GetOrCreate("cb_open_db")
	// Trip the circuit breaker to open state
	for i := 0; i < 20; i++ {
		_, _ = cb.Execute(func() (any, error) {
			return nil, assert.AnError
		})
	}
	// Verify it is open
	assert.Equal(t, constant.CircuitBreakerStateOpen, cbManager.GetState("cb_open_db"))

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// performHealthChecks should attempt healing for the CB-open datasource
	// and NOT panic. The reconnection will fail, but the flow is exercised.
	assert.NotPanics(t, func() {
		hc.performHealthChecks()
	})
}

// ---------------------------------------------------------------------------
// GetHealthStatus – datasources with different circuit breaker states
// ---------------------------------------------------------------------------

func TestHealthChecker_GetHealthStatus_MultipleCircuitBreakerStates(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	dataSources["closed_cb_db"] = DataSource{
		Status:       libConstants.DataSourceStatusAvailable,
		DatabaseType: PostgreSQLType,
		Initialized:  true,
	}
	dataSources["open_cb_db"] = DataSource{
		Status:       libConstants.DataSourceStatusUnavailable,
		DatabaseType: PostgreSQLType,
		Initialized:  false,
	}
	dataSources["no_cb_db"] = DataSource{
		Status:       libConstants.DataSourceStatusAvailable,
		DatabaseType: MongoDBType,
		Initialized:  true,
	}

	cbManager := NewCircuitBreakerManager(logger)
	// closed_cb_db has a closed circuit breaker
	cbManager.GetOrCreate("closed_cb_db")

	// open_cb_db has an open circuit breaker
	cb := cbManager.GetOrCreate("open_cb_db")
	for i := 0; i < 20; i++ {
		_, _ = cb.Execute(func() (any, error) {
			return nil, assert.AnError
		})
	}

	// no_cb_db has no circuit breaker created (returns "not_initialized")

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	status := hc.GetHealthStatus()

	assert.Len(t, status, 3)

	// Verify each entry contains both the datasource status AND the CB state
	assert.Contains(t, status["closed_cb_db"], libConstants.DataSourceStatusAvailable)
	assert.Contains(t, status["closed_cb_db"], "CB: "+constant.CircuitBreakerStateClosed)

	assert.Contains(t, status["open_cb_db"], libConstants.DataSourceStatusUnavailable)
	assert.Contains(t, status["open_cb_db"], "CB: "+constant.CircuitBreakerStateOpen)

	assert.Contains(t, status["no_cb_db"], libConstants.DataSourceStatusAvailable)
	assert.Contains(t, status["no_cb_db"], "CB: not_initialized")
}

// ---------------------------------------------------------------------------
// needsHealing – degraded status DOES need healing
// ---------------------------------------------------------------------------
//
// A Degraded datasource is the output of the metric-only ping path
// (a previously-Available datasource whose periodic ping just failed).
// It MUST be treated as needing healing so the next loop re-establishes
// the connection. Without this, a degraded datasource would stay
// degraded forever and the healing path would never trigger.

func TestHealthChecker_NeedsHealing_DegradedStatusHealthyCB(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("degraded_db")

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// Degraded, initialized, CB closed: Degraded explicitly needs healing
	// — the periodic ping just failed and the next loop must
	// re-establish the connection.
	ds := DataSource{
		Status:      libConstants.DataSourceStatusDegraded,
		Initialized: true,
	}

	result := hc.needsHealing("degraded_db", ds)
	assert.True(t, result, "degraded datasource must need healing so the reconnection path runs next loop")
}

// ---------------------------------------------------------------------------
// needsHealing – multiple conditions true simultaneously
// ---------------------------------------------------------------------------

func TestHealthChecker_NeedsHealing_MultipleConditionsTrue(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// Datasource is unavailable AND not initialized – first condition triggers
	ds := DataSource{
		Status:      libConstants.DataSourceStatusUnavailable,
		Initialized: false,
	}

	result := hc.needsHealing("multi_cond_db", ds)
	assert.True(t, result, "unavailable and uninitialized datasource should need healing")
}

// ---------------------------------------------------------------------------
// attemptReconnection – verifies RetryCount is reset and LastAttempt is updated
// ---------------------------------------------------------------------------

func TestHealthChecker_AttemptReconnection_ResetsRetryCountAndTimestamp(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	oldTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	ds := &DataSource{
		DatabaseType: PostgreSQLType,
		Initialized:  false,
		Status:       libConstants.DataSourceStatusUnavailable,
		RetryCount:   5,
		LastAttempt:  oldTime,
	}

	before := time.Now()
	_ = hc.attemptReconnection("retry_reset_db", ds)

	// RetryCount should be reset to 0 (set at the start of attemptReconnection)
	assert.Equal(t, 0, ds.RetryCount)

	// LastAttempt should be updated to a recent time
	assert.True(t, ds.LastAttempt.After(before) || ds.LastAttempt.Equal(before),
		"LastAttempt should be updated to current time")
}

// ---------------------------------------------------------------------------
// pingDataSource – PostgreSQL with mocked repo and custom schemas (success)
//
// Schemas configuration is preserved on the DataSource so the rest of the
// system can use it for query-time validation, but Ping itself MUST NOT
// receive or care about schemas — the new lightweight implementation
// (SELECT 1 via *sql.DB.PingContext) is schema-agnostic.
// ---------------------------------------------------------------------------

func TestHealthChecker_PingDataSource_PostgreSQL_CustomSchemas_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)
	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	mockPgRepo := pgMock.NewMockRepository(ctrl)
	mockPgRepo.EXPECT().
		Ping(gomock.Any()).
		Return(nil)

	ds := &DataSource{
		DatabaseType:       PostgreSQLType,
		PostgresRepository: mockPgRepo,
		Schemas:            []string{"inventory", "billing"},
		Initialized:        true,
	}

	result := hc.pingDataSource(context.Background(), "pg_custom_schemas", ds)
	assert.True(t, result)
}

// ---------------------------------------------------------------------------
// pingDataSource – MongoDB with mocked repo (error path)
// ---------------------------------------------------------------------------

func TestHealthChecker_PingDataSource_MongoDB_ErrorFromRepo(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	cbManager := NewCircuitBreakerManager(logger)
	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	mockMongoRepo := mongoMock.NewMockRepository(ctrl)
	mockMongoRepo.EXPECT().
		Ping(gomock.Any()).
		Return(assert.AnError)

	ds := &DataSource{
		DatabaseType:      MongoDBType,
		MongoDBRepository: mockMongoRepo,
		Initialized:       true,
	}

	result := hc.pingDataSource(context.Background(), "mongo_err_repo", ds)
	assert.False(t, result)
}

// ---------------------------------------------------------------------------
// performHealthChecks – all datasources need healing (unavailable count path)
// ---------------------------------------------------------------------------

func TestHealthChecker_PerformHealthChecks_AllNeedHealing(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	dataSources["down_db1"] = DataSource{
		Status:       libConstants.DataSourceStatusUnavailable,
		DatabaseType: PostgreSQLType,
		Initialized:  false,
	}
	dataSources["down_db2"] = DataSource{
		Status:       libConstants.DataSourceStatusUnavailable,
		DatabaseType: MongoDBType,
		Initialized:  false,
	}

	cbManager := NewCircuitBreakerManager(logger)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// Both datasources need healing; ConnectToDataSource will fail for both
	// because the IDs are not registered. The unavailableCount > 0 logging
	// branch is exercised.
	assert.NotPanics(t, func() {
		hc.performHealthChecks()
	})
}

// ---------------------------------------------------------------------------
// Concurrent performHealthChecks – snapshot isolation
// ---------------------------------------------------------------------------

func TestHealthChecker_PerformHealthChecks_ConcurrentCalls(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	dataSources["concurrent_db1"] = DataSource{
		Status:       libConstants.DataSourceStatusAvailable,
		DatabaseType: PostgreSQLType,
		Initialized:  true,
	}
	dataSources["concurrent_db2"] = DataSource{
		Status:       libConstants.DataSourceStatusUnavailable,
		DatabaseType: MongoDBType,
		Initialized:  false,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("concurrent_db1")

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	// Run multiple performHealthChecks concurrently to verify the
	// snapshot-copy + mutex logic does not race.
	done := make(chan bool, 5)

	for i := 0; i < 5; i++ {
		go func() {
			hc.performHealthChecks()
			done <- true
		}()
	}

	for i := 0; i < 5; i++ {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("Concurrent performHealthChecks timed out")
		}
	}
}

// ---------------------------------------------------------------------------
// GetHealthStatus – single datasource with known CB state
// ---------------------------------------------------------------------------

func TestHealthChecker_GetHealthStatus_SingleDatasource(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	dataSources := make(map[string]DataSource)
	dataSources["solo_db"] = DataSource{
		Status:       libConstants.DataSourceStatusAvailable,
		DatabaseType: PostgreSQLType,
		Initialized:  true,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("solo_db")

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	status := hc.GetHealthStatus()

	assert.Len(t, status, 1)
	expected := libConstants.DataSourceStatusAvailable + " (CB: " + constant.CircuitBreakerStateClosed + ")"
	assert.Equal(t, expected, status["solo_db"])
}

// ---------------------------------------------------------------------------
// Periodic-ping degradation
// ---------------------------------------------------------------------------
//
// When metrics are configured AND a previously-healthy datasource fails its
// periodic ping (i.e., needsHealing was false at the start of the loop),
// performHealthChecks MUST flip ds.Status to Degraded so the next loop's
// needsHealing check picks it up. Otherwise the datasource silently stays
// "Available" forever even though every ping is failing.
func TestHealthChecker_PerformHealthChecks_HealthyButFailingPing_MarksDegraded(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	mockMongoRepo := mongoMock.NewMockRepository(ctrl)
	// Ping fails — simulates a healthy-at-start datasource that has just
	// become unreachable mid-loop.
	mockMongoRepo.EXPECT().
		Ping(gomock.Any()).
		Return(assert.AnError)

	dataSources := make(map[string]DataSource)
	dataSources["mongo_periodic"] = DataSource{
		Status:            libConstants.DataSourceStatusAvailable,
		DatabaseType:      MongoDBType,
		MongoDBRepository: mockMongoRepo,
		Initialized:       true,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("mongo_periodic")

	// Metrics required to force the metric-only ping path
	// (reconnectAttempted=false, metrics!=nil).
	dsMetrics, err := NewDatasourceMetrics(nil)
	if err != nil {
		t.Fatalf("NewDatasourceMetrics: %v", err)
	}

	hc, errHC := NewHealthCheckerWithMetrics(&dataSources, cbManager, logger, dsMetrics)
	require.NoError(t, errHC)

	hc.performHealthChecks()

	// After the failing ping, the datasource MUST be flipped to Degraded so
	// the next loop's needsHealing check fires. Without this flip the status
	// stayed at Available and the healing path never triggered.
	updated := dataSources["mongo_periodic"]
	assert.Equal(t, libConstants.DataSourceStatusDegraded, updated.Status,
		"failed periodic ping must flip Status to Degraded")
	assert.NotNil(t, updated.LastError,
		"degraded datasource must record a LastError for operator visibility")

	// Sanity: needsHealing must now report true so the next pass triggers
	// the healing path.
	assert.True(t, hc.needsHealing("mongo_periodic", updated),
		"needsHealing must return true once Status flipped to Degraded")
}

// TestHealthChecker_PerformHealthChecks_HealthyAndPingingOK_StaysAvailable is
// the negative twin of the test above: a successful periodic ping must NOT
// touch Status — the datasource was Available and stays Available.
func TestHealthChecker_PerformHealthChecks_HealthyAndPingingOK_StaysAvailable(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	mockMongoRepo := mongoMock.NewMockRepository(ctrl)
	mockMongoRepo.EXPECT().
		Ping(gomock.Any()).
		Return(nil)

	dataSources := make(map[string]DataSource)
	dataSources["mongo_periodic_ok"] = DataSource{
		Status:            libConstants.DataSourceStatusAvailable,
		DatabaseType:      MongoDBType,
		MongoDBRepository: mockMongoRepo,
		Initialized:       true,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("mongo_periodic_ok")

	dsMetrics, err := NewDatasourceMetrics(nil)
	if err != nil {
		t.Fatalf("NewDatasourceMetrics: %v", err)
	}

	hc, errHC := NewHealthCheckerWithMetrics(&dataSources, cbManager, logger, dsMetrics)
	require.NoError(t, errHC)

	hc.performHealthChecks()

	updated := dataSources["mongo_periodic_ok"]
	assert.Equal(t, libConstants.DataSourceStatusAvailable, updated.Status,
		"healthy datasource that pings successfully must stay Available")
}
