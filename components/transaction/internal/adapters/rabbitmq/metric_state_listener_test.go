package rabbitmq

import (
	"testing"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	"github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func TestNewMetricStateListener_NilFactory_ReturnsError(t *testing.T) {
	_, err := NewMetricStateListener(nil)
	require.Error(t, err)
	assert.Equal(t, ErrNilMetricsFactory, err)
}

func TestNewMetricStateListener_ValidFactory_ReturnsListener(t *testing.T) {
	mp := sdkmetric.NewMeterProvider()
	meter := mp.Meter("test")
	factory := metrics.NewMetricsFactory(meter, nil)

	listener, err := NewMetricStateListener(factory)

	require.NoError(t, err)
	assert.NotNil(t, listener)
}

func TestMetricStateListener_OnStateChange_UpdatesMetric(t *testing.T) {
	mp := sdkmetric.NewMeterProvider()
	meter := mp.Meter("test")
	factory := metrics.NewMetricsFactory(meter, nil)

	listener, err := NewMetricStateListener(factory)
	require.NoError(t, err)

	// Test state transitions - should not panic
	listener.OnStateChange("rabbitmq-producer", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	listener.OnStateChange("rabbitmq-producer", libCircuitBreaker.StateOpen, libCircuitBreaker.StateHalfOpen)
	listener.OnStateChange("rabbitmq-producer", libCircuitBreaker.StateHalfOpen, libCircuitBreaker.StateClosed)
}

func TestStateToMetricValue(t *testing.T) {
	tests := []struct {
		state    libCircuitBreaker.State
		expected int64
	}{
		{libCircuitBreaker.StateClosed, 0},
		{libCircuitBreaker.StateOpen, 1},
		{libCircuitBreaker.StateHalfOpen, 2},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			assert.Equal(t, tt.expected, stateToMetricValue(tt.state))
		})
	}
}
