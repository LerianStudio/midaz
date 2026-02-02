// Package mcircuitbreaker provides circuit breaker abstractions for Midaz.
// This package defines interfaces that allow the Midaz application to receive
// notifications about circuit breaker state changes without directly depending
// on the lib-commons implementation details.
package mcircuitbreaker

import (
	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
)

// State represents circuit breaker state in Midaz domain.
type State string

const (
	// StateClosed indicates normal operation - requests flow through.
	StateClosed State = "closed"
	// StateOpen indicates circuit is tripped - requests fail fast.
	StateOpen State = "open"
	// StateHalfOpen indicates recovery testing - limited requests allowed.
	StateHalfOpen State = "half-open"
	// StateUnknown indicates the state could not be determined.
	StateUnknown State = "unknown"
)

// Counts represents circuit breaker statistics.
type Counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

// StateChangeEvent contains information about a circuit breaker state transition.
type StateChangeEvent struct {
	ServiceName string
	FromState   State
	ToState     State
	Counts      Counts
}

// StateListener receives notifications when circuit breaker state changes.
// Implement this interface in your application to react to circuit breaker events.
type StateListener interface {
	// OnCircuitBreakerStateChange is called when a circuit breaker changes state.
	OnCircuitBreakerStateChange(event StateChangeEvent)
}

// LibCommonsAdapter wraps a Midaz StateListener to implement lib-commons StateChangeListener.
// This adapter bridges the Midaz domain types with lib-commons types.
type LibCommonsAdapter struct {
	listener StateListener
}

// NewLibCommonsAdapter creates an adapter that forwards lib-commons state changes to a Midaz listener.
func NewLibCommonsAdapter(listener StateListener) *LibCommonsAdapter {
	return &LibCommonsAdapter{listener: listener}
}

// OnStateChange implements lib-commons StateChangeListener interface.
func (a *LibCommonsAdapter) OnStateChange(serviceName string, from libCircuitBreaker.State, to libCircuitBreaker.State, counts libCircuitBreaker.Counts) {
	if a.listener == nil {
		return
	}

	event := StateChangeEvent{
		ServiceName: serviceName,
		FromState:   convertState(from),
		ToState:     convertState(to),
		Counts: Counts{
			Requests:             counts.Requests,
			TotalSuccesses:       counts.TotalSuccesses,
			TotalFailures:        counts.TotalFailures,
			ConsecutiveSuccesses: counts.ConsecutiveSuccesses,
			ConsecutiveFailures:  counts.ConsecutiveFailures,
		},
	}

	a.listener.OnCircuitBreakerStateChange(event)
}

// convertState converts lib-commons State to Midaz State.
func convertState(s libCircuitBreaker.State) State {
	switch s {
	case libCircuitBreaker.StateClosed:
		return StateClosed
	case libCircuitBreaker.StateOpen:
		return StateOpen
	case libCircuitBreaker.StateHalfOpen:
		return StateHalfOpen
	default:
		return StateUnknown
	}
}