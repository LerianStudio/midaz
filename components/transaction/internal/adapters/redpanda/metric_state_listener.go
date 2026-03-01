// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/attribute"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	"github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"

	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// ErrNilMetricsFactory indicates that the metrics factory parameter is nil.
var ErrNilMetricsFactory = errors.New("metrics factory cannot be nil")

// MetricStateListener implements StateChangeListener to update metrics on state changes.
type MetricStateListener struct {
	factory *metrics.MetricsFactory
}

// NewMetricStateListener creates a new state listener that updates metrics on state changes.
func NewMetricStateListener(factory *metrics.MetricsFactory) (*MetricStateListener, error) {
	if factory == nil {
		return nil, ErrNilMetricsFactory
	}

	return &MetricStateListener{factory: factory}, nil
}

// OnStateChange updates the circuit_breaker_state gauge metric when state transitions.
func (m *MetricStateListener) OnStateChange(serviceName string, _, to libCircuitBreaker.State) {
	value := stateToMetricValue(to)

	m.factory.Gauge(utils.CircuitBreakerState).
		WithAttributes(attribute.String("service", serviceName)).
		Set(context.Background(), value)
}

const (
	metricStateClosed   int64 = 0
	metricStateOpen     int64 = 1
	metricStateHalfOpen int64 = 2
	metricStateUnknown  int64 = -1
)

func stateToMetricValue(state libCircuitBreaker.State) int64 {
	switch state {
	case libCircuitBreaker.StateClosed:
		return metricStateClosed
	case libCircuitBreaker.StateOpen:
		return metricStateOpen
	case libCircuitBreaker.StateHalfOpen:
		return metricStateHalfOpen
	default:
		return metricStateUnknown
	}
}
