// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"testing"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	"github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func TestNewMetricStateListener(t *testing.T) {
	t.Run("nil factory", func(t *testing.T) {
		listener, err := NewMetricStateListener(nil)
		require.Error(t, err)
		assert.Nil(t, listener)
		assert.ErrorIs(t, err, ErrNilMetricsFactory)
	})

	t.Run("valid factory", func(t *testing.T) {
		meterProvider := sdkmetric.NewMeterProvider()
		factory := metrics.NewMetricsFactory(meterProvider.Meter("test"), nil)

		listener, err := NewMetricStateListener(factory)
		require.NoError(t, err)
		assert.NotNil(t, listener)
	})
}

func TestMetricStateListener_OnStateChange_DoesNotPanic(t *testing.T) {
	meterProvider := sdkmetric.NewMeterProvider()
	factory := metrics.NewMetricsFactory(meterProvider.Meter("test"), nil)

	listener, err := NewMetricStateListener(factory)
	require.NoError(t, err)

	listener.OnStateChange(CircuitBreakerServiceName, libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	listener.OnStateChange(CircuitBreakerServiceName, libCircuitBreaker.StateOpen, libCircuitBreaker.StateHalfOpen)
	listener.OnStateChange(CircuitBreakerServiceName, libCircuitBreaker.StateHalfOpen, libCircuitBreaker.StateClosed)
}

func TestStateToMetricValue(t *testing.T) {
	assert.Equal(t, int64(0), stateToMetricValue(libCircuitBreaker.StateClosed))
	assert.Equal(t, int64(1), stateToMetricValue(libCircuitBreaker.StateOpen))
	assert.Equal(t, int64(2), stateToMetricValue(libCircuitBreaker.StateHalfOpen))
	assert.Equal(t, int64(-1), stateToMetricValue(libCircuitBreaker.State("unknown")))
}
