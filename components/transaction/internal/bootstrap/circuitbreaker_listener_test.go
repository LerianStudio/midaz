package bootstrap

import (
	"testing"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCircuitBreakerListener(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()

	listener := NewCircuitBreakerListener(logger, nil, nil)

	require.NotNil(t, listener)
	assert.Equal(t, logger, listener.logger)
	assert.Nil(t, listener.telemetry)
	assert.Nil(t, listener.manager)
}

func TestNewCircuitBreakerListener_WithTelemetry(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	telemetry := &libOpentelemetry.Telemetry{}

	listener := NewCircuitBreakerListener(logger, telemetry, nil)

	require.NotNil(t, listener)
	assert.Equal(t, logger, listener.logger)
	assert.Equal(t, telemetry, listener.telemetry)
}

func TestNewCircuitBreakerListener_WithManager(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	manager := libCircuitBreaker.NewManager(logger)

	listener := NewCircuitBreakerListener(logger, nil, manager)

	require.NotNil(t, listener)
	assert.Equal(t, manager, listener.manager)
}

func TestCircuitBreakerListener_OnStateChange_ToOpen(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	listener := NewCircuitBreakerListener(logger, nil, nil)

	assert.NotPanics(t, func() {
		listener.OnStateChange("rabbitmq", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	})
}

func TestCircuitBreakerListener_OnStateChange_ToHalfOpen(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	listener := NewCircuitBreakerListener(logger, nil, nil)

	assert.NotPanics(t, func() {
		listener.OnStateChange("rabbitmq", libCircuitBreaker.StateOpen, libCircuitBreaker.StateHalfOpen)
	})
}

func TestCircuitBreakerListener_OnStateChange_ToClosed(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	listener := NewCircuitBreakerListener(logger, nil, nil)

	assert.NotPanics(t, func() {
		listener.OnStateChange("rabbitmq", libCircuitBreaker.StateHalfOpen, libCircuitBreaker.StateClosed)
	})
}

func TestCircuitBreakerListener_OnStateChange_WithManager(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	manager := libCircuitBreaker.NewManager(logger)
	manager.GetOrCreate("rabbitmq", libCircuitBreaker.Config{})
	listener := NewCircuitBreakerListener(logger, nil, manager)

	assert.NotPanics(t, func() {
		listener.OnStateChange("rabbitmq", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	})
}

func TestCircuitBreakerListener_OnStateChange_NilManager_NoPanic(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	listener := NewCircuitBreakerListener(logger, nil, nil)

	assert.NotPanics(t, func() {
		listener.OnStateChange("rabbitmq", libCircuitBreaker.StateClosed, libCircuitBreaker.StateOpen)
	})
}

func TestCircuitBreakerListener_emitMetrics_NilTelemetry(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	listener := NewCircuitBreakerListener(logger, nil, nil)

	assert.NotPanics(t, func() {
		listener.emitMetrics("rabbitmq", 1, libCircuitBreaker.Counts{})
	})
}

func TestCircuitBreakerListener_emitMetrics_NilMetricsFactory(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	telemetry := &libOpentelemetry.Telemetry{
		MetricsFactory: nil,
	}
	listener := NewCircuitBreakerListener(logger, telemetry, nil)

	assert.NotPanics(t, func() {
		listener.emitMetrics("rabbitmq", 1, libCircuitBreaker.Counts{})
	})
}

func TestCircuitBreakerListener_getCounts_NilManager(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	listener := NewCircuitBreakerListener(logger, nil, nil)

	counts := listener.getCounts("rabbitmq")

	assert.Equal(t, libCircuitBreaker.Counts{}, counts)
}

func TestCircuitBreakerListener_getCounts_WithManager(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	manager := libCircuitBreaker.NewManager(logger)
	manager.GetOrCreate("rabbitmq", libCircuitBreaker.Config{})
	listener := NewCircuitBreakerListener(logger, nil, manager)

	counts := listener.getCounts("rabbitmq")

	require.NotNil(t, counts)
}

func TestCircuitBreakerListener_calculateFailureRatioPercent_ZeroRequests(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	listener := NewCircuitBreakerListener(logger, nil, nil)

	counts := libCircuitBreaker.Counts{Requests: 0, TotalFailures: 0}
	ratio := listener.calculateFailureRatioPercent(counts)

	assert.Equal(t, float64(0), ratio)
}

func TestCircuitBreakerListener_calculateFailureRatioPercent_WithFailures(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	listener := NewCircuitBreakerListener(logger, nil, nil)

	counts := libCircuitBreaker.Counts{Requests: 100, TotalFailures: 25}
	ratio := listener.calculateFailureRatioPercent(counts)

	assert.Equal(t, float64(25), ratio)
}

func TestCircuitBreakerListener_calculateFailureRatioPercent_AllFailures(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	listener := NewCircuitBreakerListener(logger, nil, nil)

	counts := libCircuitBreaker.Counts{Requests: 50, TotalFailures: 50}
	ratio := listener.calculateFailureRatioPercent(counts)

	assert.Equal(t, float64(100), ratio)
}

func TestCircuitBreakerListener_buildTelemetryContext_NilTelemetry(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	listener := NewCircuitBreakerListener(logger, nil, nil)

	ctx := listener.buildTelemetryContext()

	require.NotNil(t, ctx)
}

func TestCircuitBreakerListener_buildTelemetryContext_WithLogger(t *testing.T) {
	t.Parallel()

	logger := newTestLogger()
	listener := NewCircuitBreakerListener(logger, nil, nil)

	ctx := listener.buildTelemetryContext()

	require.NotNil(t, ctx)
}

func TestStateToInt_Closed(t *testing.T) {
	t.Parallel()

	result := stateToInt(libCircuitBreaker.StateClosed)

	assert.Equal(t, int64(0), result)
}

func TestStateToInt_Open(t *testing.T) {
	t.Parallel()

	result := stateToInt(libCircuitBreaker.StateOpen)

	assert.Equal(t, int64(1), result)
}

func TestStateToInt_HalfOpen(t *testing.T) {
	t.Parallel()

	result := stateToInt(libCircuitBreaker.StateHalfOpen)

	assert.Equal(t, int64(2), result)
}

func TestStateToInt_Unknown(t *testing.T) {
	t.Parallel()

	result := stateToInt(libCircuitBreaker.State("unknown"))

	assert.Equal(t, int64(-1), result)
}

func TestCircuitBreakerListener_OnStateChange_AllStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		from          libCircuitBreaker.State
		to            libCircuitBreaker.State
		expectedState int64
	}{
		{
			name:          "closed to open",
			from:          libCircuitBreaker.StateClosed,
			to:            libCircuitBreaker.StateOpen,
			expectedState: 1,
		},
		{
			name:          "open to half-open",
			from:          libCircuitBreaker.StateOpen,
			to:            libCircuitBreaker.StateHalfOpen,
			expectedState: 2,
		},
		{
			name:          "half-open to closed",
			from:          libCircuitBreaker.StateHalfOpen,
			to:            libCircuitBreaker.StateClosed,
			expectedState: 0,
		},
		{
			name:          "half-open to open",
			from:          libCircuitBreaker.StateHalfOpen,
			to:            libCircuitBreaker.StateOpen,
			expectedState: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := newTestLogger()
			listener := NewCircuitBreakerListener(logger, nil, nil)

			assert.NotPanics(t, func() {
				listener.OnStateChange("test-service", tt.from, tt.to)
			})
			assert.Equal(t, tt.expectedState, stateToInt(tt.to))
		})
	}
}
