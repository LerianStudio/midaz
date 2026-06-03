// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"testing"

	mongoMock "github.com/LerianStudio/reporter/pkg/mongodb"
	pgMock "github.com/LerianStudio/reporter/pkg/postgres"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"
)

// TestHealthChecker_PerformHealthChecks_EmitsHealthyMetric_PostgresUp wires a
// HealthChecker with a real OTel meter and a mocked PostgreSQL repo that
// successfully serves the lightweight Ping. After performHealthChecks runs,
// the datasource_healthy gauge MUST report 1 and the duration histogram
// MUST have a single observation. This is the production path: a healthy
// datasource produces a continuous time series.
func TestHealthChecker_PerformHealthChecks_EmitsHealthyMetric_PostgresUp(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	mockPgRepo := pgMock.NewMockRepository(ctrl)
	mockPgRepo.EXPECT().
		Ping(gomock.Any()).
		Return(nil)

	dataSources := make(map[string]DataSource)
	dataSources["pg_main"] = DataSource{
		Status:             libConstants.DataSourceStatusAvailable,
		DatabaseType:       PostgreSQLType,
		PostgresRepository: mockPgRepo,
		Initialized:        true,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("pg_main")

	reader, dsMetrics := newDsReaderAndMetrics(t)

	hc, errHC := NewHealthCheckerWithMetrics(&dataSources, cbManager, logger, dsMetrics)
	require.NoError(t, errHC)

	hc.performHealthChecks()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	healthyGot, ok := findDsMetric(t, rm, "datasource_healthy")
	require.True(t, ok, "datasource_healthy must be emitted")

	gauge := healthyGot.Data.(metricdata.Gauge[int64])
	require.Len(t, gauge.DataPoints, 1)

	dsID, _ := gauge.DataPoints[0].Attributes.Value("datasource_id")
	assert.Equal(t, "pg_main", dsID.AsString())
	assert.Equal(t, int64(1), gauge.DataPoints[0].Value)

	durGot, ok := findDsMetric(t, rm, "datasource_check_duration_ms")
	require.True(t, ok, "datasource_check_duration_ms must be emitted")

	hist := durGot.Data.(metricdata.Histogram[float64])
	require.Len(t, hist.DataPoints, 1)
	assert.Equal(t, uint64(1), hist.DataPoints[0].Count)
}

// TestHealthChecker_PerformHealthChecks_EmitsHealthyMetric_MongoDown tests
// the inverse: a MongoDB datasource whose Ping errors must report
// datasource_healthy=0 and still produce a duration sample.
func TestHealthChecker_PerformHealthChecks_EmitsHealthyMetric_MongoDown(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	mockMongoRepo := mongoMock.NewMockRepository(ctrl)
	mockMongoRepo.EXPECT().
		Ping(gomock.Any()).
		Return(assert.AnError)

	dataSources := make(map[string]DataSource)
	dataSources["mongo_orders"] = DataSource{
		Status:            libConstants.DataSourceStatusAvailable,
		DatabaseType:      MongoDBType,
		MongoDBRepository: mockMongoRepo,
		Initialized:       true,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("mongo_orders")

	reader, dsMetrics := newDsReaderAndMetrics(t)

	hc, errHC := NewHealthCheckerWithMetrics(&dataSources, cbManager, logger, dsMetrics)
	require.NoError(t, errHC)

	hc.performHealthChecks()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	healthyGot, ok := findDsMetric(t, rm, "datasource_healthy")
	require.True(t, ok)

	gauge := healthyGot.Data.(metricdata.Gauge[int64])
	require.Len(t, gauge.DataPoints, 1)

	dsID, _ := gauge.DataPoints[0].Attributes.Value("datasource_id")
	assert.Equal(t, "mongo_orders", dsID.AsString())
	assert.Equal(t, int64(0), gauge.DataPoints[0].Value, "down datasource MUST report 0")
}

// TestHealthChecker_PerformHealthChecks_NoMetrics_DoesNotPanic guarantees
// the legacy code path: a HealthChecker created via NewHealthChecker (no
// metrics argument) MUST run performHealthChecks without panicking and
// without emitting any metric points.
func TestHealthChecker_PerformHealthChecks_NoMetrics_DoesNotPanic(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	mockPgRepo := pgMock.NewMockRepository(ctrl)
	// pingDataSource is NOT called when metrics is nil — emitDatasourcePingMetric
	// short-circuits on the nil check. So no Ping expectation.

	dataSources := make(map[string]DataSource)
	dataSources["pg_main"] = DataSource{
		Status:             libConstants.DataSourceStatusAvailable,
		DatabaseType:       PostgreSQLType,
		PostgresRepository: mockPgRepo,
		Initialized:        true,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("pg_main")

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)

	assert.NotPanics(t, hc.performHealthChecks)
}

// TestHealthChecker_PerformHealthChecks_SingleProbePerLoopIteration is the
// regression guard for the Dispatch 2 D fix: when a datasource needs
// healing, the loop body must not double-probe. Previously
// attemptReconnection internally called pingDataSource and the metric
// emitter called it AGAIN at the end of the same iteration — doubling load
// against datasources that were already struggling.
//
// The test pins the expectation: a single iteration emits exactly ONE ping
// (one Ping call) per datasource, even when healing is required.
func TestHealthChecker_PerformHealthChecks_SingleProbePerLoopIteration(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	mockPgRepo := pgMock.NewMockRepository(ctrl)
	// Times(1) is the contract: needsHealing=true → reconnect → emit metric.
	// The whole loop iteration must produce exactly one Ping invocation.
	mockPgRepo.EXPECT().
		Ping(gomock.Any()).
		Return(nil).
		Times(1)

	dataSources := make(map[string]DataSource)
	dataSources["pg_main"] = DataSource{
		Status:             libConstants.DataSourceStatusUnavailable, // forces needsHealing=true
		DatabaseType:       PostgreSQLType,
		PostgresRepository: mockPgRepo,
		Initialized:        false,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("pg_main")

	_, dsMetrics := newDsReaderAndMetrics(t)

	hc, errHC := NewHealthCheckerWithMetrics(&dataSources, cbManager, logger, dsMetrics)
	require.NoError(t, errHC)

	// One iteration → exactly one ping (gomock Times(1) enforces it).
	hc.performHealthChecks()
}

// TestHealthChecker_SetMetrics_InstallsEmitter verifies that SetMetrics can
// install a metrics emitter on a HealthChecker created via the legacy
// constructor — exercising the post-construction wire-in path that the
// Worker bootstrap uses when the meter is available later than the
// HealthChecker.
func TestHealthChecker_SetMetrics_InstallsEmitter(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())

	mockPgRepo := pgMock.NewMockRepository(ctrl)
	mockPgRepo.EXPECT().
		Ping(gomock.Any()).
		Return(nil)

	dataSources := make(map[string]DataSource)
	dataSources["pg_main"] = DataSource{
		Status:             libConstants.DataSourceStatusAvailable,
		DatabaseType:       PostgreSQLType,
		PostgresRepository: mockPgRepo,
		Initialized:        true,
	}

	cbManager := NewCircuitBreakerManager(logger)
	cbManager.GetOrCreate("pg_main")

	reader, dsMetrics := newDsReaderAndMetrics(t)

	hc, errHC := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, errHC)
	hc.SetMetrics(dsMetrics)

	hc.performHealthChecks()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	_, ok := findDsMetric(t, rm, "datasource_healthy")
	assert.True(t, ok, "datasource_healthy must be emitted after SetMetrics")
}
