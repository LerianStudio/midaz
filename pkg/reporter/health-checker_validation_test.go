// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Constructor input validation. The HealthChecker is consumed by the
// /readyz path and the periodic background loop; both call paths
// dereference dataSources and circuitBreakerManager. A nil pointer in
// either field would surface as a runtime nil-pointer panic far away
// from the construction site, so the constructors fail fast at build
// time and surface a typed error to the caller.

func TestNewHealthChecker_NilDataSources_ReturnsError(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbManager := NewCircuitBreakerManager(logger)

	hc, err := NewHealthChecker(nil, cbManager, logger)
	require.Error(t, err)
	assert.Nil(t, hc)
	assert.Contains(t, err.Error(), "dataSources")
}

func TestNewHealthChecker_NilCircuitBreaker_ReturnsError(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	dataSources := make(map[string]DataSource)

	hc, err := NewHealthChecker(&dataSources, nil, logger)
	require.Error(t, err)
	assert.Nil(t, hc)
	assert.Contains(t, err.Error(), "circuitBreakerManager")
}

func TestNewHealthChecker_NilLogger_ReturnsError(t *testing.T) {
	t.Parallel()

	dataSources := make(map[string]DataSource)
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbManager := NewCircuitBreakerManager(logger)

	hc, err := NewHealthChecker(&dataSources, cbManager, nil)
	require.Error(t, err)
	assert.Nil(t, hc)
	assert.Contains(t, err.Error(), "logger")
}

func TestNewHealthChecker_AllValid_ReturnsChecker(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbManager := NewCircuitBreakerManager(logger)
	dataSources := make(map[string]DataSource)

	hc, err := NewHealthChecker(&dataSources, cbManager, logger)
	require.NoError(t, err)
	require.NotNil(t, hc)
}

func TestNewHealthCheckerWithMetrics_NilDataSources_ReturnsError(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbManager := NewCircuitBreakerManager(logger)

	hc, err := NewHealthCheckerWithMetrics(nil, cbManager, logger, nil)
	require.Error(t, err)
	assert.Nil(t, hc)
	assert.Contains(t, err.Error(), "dataSources")
}

func TestNewHealthCheckerWithMetrics_NilCircuitBreaker_ReturnsError(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	dataSources := make(map[string]DataSource)

	hc, err := NewHealthCheckerWithMetrics(&dataSources, nil, logger, nil)
	require.Error(t, err)
	assert.Nil(t, hc)
	assert.Contains(t, err.Error(), "circuitBreakerManager")
}

func TestNewHealthCheckerWithMetrics_NilLogger_ReturnsError(t *testing.T) {
	t.Parallel()

	dataSources := make(map[string]DataSource)
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbManager := NewCircuitBreakerManager(logger)

	hc, err := NewHealthCheckerWithMetrics(&dataSources, cbManager, nil, nil)
	require.Error(t, err)
	assert.Nil(t, hc)
	assert.Contains(t, err.Error(), "logger")
}

// Nil metrics is explicitly tolerated — it is the documented opt-out
// for callers that do not want emission. Constructor must succeed.
func TestNewHealthCheckerWithMetrics_NilMetrics_AllowedReturnsChecker(t *testing.T) {
	t.Parallel()

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	cbManager := NewCircuitBreakerManager(logger)
	dataSources := make(map[string]DataSource)

	hc, err := NewHealthCheckerWithMetrics(&dataSources, cbManager, logger, nil)
	require.NoError(t, err)
	require.NotNil(t, hc)
}
