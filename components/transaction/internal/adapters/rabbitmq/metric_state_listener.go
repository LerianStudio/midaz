// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	"github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"go.opentelemetry.io/otel/attribute"
)

// ErrNilMetricsFactory indicates that the metrics factory parameter is nil.
var ErrNilMetricsFactory = errors.New("metrics factory cannot be nil")

// MetricStateListener implements StateChangeListener to update Prometheus metrics
// when circuit breaker state changes.
type MetricStateListener struct {
	factory *metrics.MetricsFactory
}

// NewMetricStateListener creates a new state listener that updates metrics on state changes.
func NewMetricStateListener(factory *metrics.MetricsFactory) (*MetricStateListener, error) {
	if factory == nil {
		return nil, ErrNilMetricsFactory
	}

	return &MetricStateListener{
		factory: factory,
	}, nil
}

// OnStateChange updates the circuit_breaker_state gauge metric when state transitions.
// Values: 0=closed, 1=open, 2=half-open
func (m *MetricStateListener) OnStateChange(serviceName string, from, to libCircuitBreaker.State) {
	value := stateToMetricValue(to)

	m.factory.Gauge(utils.CircuitBreakerState).
		WithAttributes(attribute.String("service", serviceName)).
		Set(context.Background(), value)
}

// stateToMetricValue converts circuit breaker state to metric value.
func stateToMetricValue(state libCircuitBreaker.State) int64 {
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
