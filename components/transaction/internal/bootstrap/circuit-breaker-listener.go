package bootstrap

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// CircuitBreakerListener implements StateChangeListener for circuit breaker observability.
// It logs state transitions with failure counts and emits gauge metrics to OpenTelemetry.
type CircuitBreakerListener struct {
	logger    libLog.Logger
	telemetry *libOpentelemetry.Telemetry
	manager   libCircuitBreaker.Manager
}

// NewCircuitBreakerListener creates a new listener with proper telemetry context.
func NewCircuitBreakerListener(logger libLog.Logger, telemetry *libOpentelemetry.Telemetry, manager libCircuitBreaker.Manager) *CircuitBreakerListener {
	return &CircuitBreakerListener{
		logger:    logger,
		telemetry: telemetry,
		manager:   manager,
	}
}

// OnStateChange logs circuit breaker state transitions and emits gauge metrics.
// State values: 0=closed, 1=open, 2=half_open
func (l *CircuitBreakerListener) OnStateChange(serviceName string, from libCircuitBreaker.State, to libCircuitBreaker.State) {
	stateValue := stateToInt(to)
	counts := l.getCounts(serviceName)

	switch to {
	case libCircuitBreaker.StateOpen:
		l.logger.Warnf(
			"Circuit breaker [%s] OPENED: %s -> %s | consecutive_failures=%d, total_failures=%d, requests=%d, failure_ratio=%.1f%%",
			serviceName, from, to,
			counts.ConsecutiveFailures,
			counts.TotalFailures,
			counts.Requests,
			l.calculateFailureRatioPercent(counts),
		)
	case libCircuitBreaker.StateHalfOpen:
		l.logger.Infof(
			"Circuit breaker [%s] HALF-OPEN: %s -> %s | testing recovery, consecutive_successes=%d",
			serviceName, from, to,
			counts.ConsecutiveSuccesses,
		)
	case libCircuitBreaker.StateClosed:
		l.logger.Infof(
			"Circuit breaker [%s] CLOSED: %s -> %s | normal operation resumed, consecutive_successes=%d",
			serviceName, from, to,
			counts.ConsecutiveSuccesses,
		)
	}

	l.emitMetrics(serviceName, stateValue, counts)
}

// getCounts retrieves the current counts from the circuit breaker.
func (l *CircuitBreakerListener) getCounts(serviceName string) libCircuitBreaker.Counts {
	if l.manager == nil {
		return libCircuitBreaker.Counts{}
	}

	return l.manager.GetCounts(serviceName)
}

// calculateFailureRatioPercent calculates the failure ratio as percentage (0-100).
func (l *CircuitBreakerListener) calculateFailureRatioPercent(counts libCircuitBreaker.Counts) float64 {
	if counts.Requests == 0 {
		return 0
	}

	return float64(counts.TotalFailures) / float64(counts.Requests) * 100
}

// emitMetrics emits all circuit breaker metrics with proper telemetry context.
func (l *CircuitBreakerListener) emitMetrics(serviceName string, stateValue int64, counts libCircuitBreaker.Counts) {
	if l.telemetry == nil || l.telemetry.MetricsFactory == nil {
		return
	}

	ctx := l.buildTelemetryContext()
	labels := map[string]string{"service_name": serviceName}

	l.telemetry.MetricsFactory.Gauge(utils.CircuitBreakerState).
		WithLabels(labels).Set(ctx, stateValue)

	l.telemetry.MetricsFactory.Gauge(utils.CircuitBreakerConsecutiveFailures).
		WithLabels(labels).Set(ctx, int64(counts.ConsecutiveFailures))

	l.telemetry.MetricsFactory.Gauge(utils.CircuitBreakerTotalFailures).
		WithLabels(labels).Set(ctx, int64(counts.TotalFailures))

	l.telemetry.MetricsFactory.Gauge(utils.CircuitBreakerTotalRequests).
		WithLabels(labels).Set(ctx, int64(counts.Requests))

	failureRatioBps := int64(l.calculateFailureRatioPercent(counts) * 100)
	l.telemetry.MetricsFactory.Gauge(utils.CircuitBreakerFailureRatio).
		WithLabels(labels).Set(ctx, failureRatioBps)
}

// buildTelemetryContext creates a context with all telemetry components.
func (l *CircuitBreakerListener) buildTelemetryContext() context.Context {
	ctx := context.Background()

	if l.logger != nil {
		ctx = libCommons.ContextWithLogger(ctx, l.logger)
	}

	if l.telemetry != nil {
		if l.telemetry.MetricsFactory != nil {
			ctx = libCommons.ContextWithMetricFactory(ctx, l.telemetry.MetricsFactory)
		}
	}

	return ctx
}

// stateToInt converts circuit breaker state to integer for metrics.
func stateToInt(state libCircuitBreaker.State) int64 {
	switch state {
	case libCircuitBreaker.StateClosed:
		return 0
	case libCircuitBreaker.StateOpen:
		return 1
	case libCircuitBreaker.StateHalfOpen:
		return 2
	default:
		return -1
	}
}
