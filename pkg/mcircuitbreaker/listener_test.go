package mcircuitbreaker

import (
	"testing"

	"github.com/stretchr/testify/assert"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
)

func TestStateChangeEvent_ContainsRequiredFields(t *testing.T) {
	event := StateChangeEvent{
		ServiceName: "test-service",
		FromState:   StateClosed,
		ToState:     StateOpen,
		Counts: Counts{
			Requests:            10,
			TotalFailures:       5,
			ConsecutiveFailures: 3,
		},
	}

	assert.Equal(t, "test-service", event.ServiceName)
	assert.Equal(t, StateClosed, event.FromState)
	assert.Equal(t, StateOpen, event.ToState)
	assert.Equal(t, uint32(10), event.Counts.Requests)
	assert.Equal(t, uint32(5), event.Counts.TotalFailures)
	assert.Equal(t, uint32(3), event.Counts.ConsecutiveFailures)
}

func TestStateListener_CanReceiveEvents(t *testing.T) {
	listener := &mockListener{}

	event := StateChangeEvent{
		ServiceName: "rabbitmq-producer",
		FromState:   StateClosed,
		ToState:     StateOpen,
	}

	listener.OnCircuitBreakerStateChange(event)

	assert.Len(t, listener.calls, 1)
	assert.Equal(t, "rabbitmq-producer", listener.calls[0].ServiceName)
}

func TestLibCommonsAdapterListener_ImplementsInterface(t *testing.T) {
	mockMidazListener := &mockListener{}
	adapter := NewLibCommonsAdapter(mockMidazListener)

	// Verify adapter implements lib-commons StateChangeListener
	var _ libCircuitBreaker.StateChangeListener = adapter
}

func TestLibCommonsAdapterListener_ForwardsStateChanges(t *testing.T) {
	mockMidazListener := &mockListener{}
	adapter := NewLibCommonsAdapter(mockMidazListener)

	// Simulate lib-commons callback
	adapter.OnStateChange(
		"rabbitmq-producer",
		libCircuitBreaker.StateClosed,
		libCircuitBreaker.StateOpen,
		libCircuitBreaker.Counts{
			Requests:             10,
			TotalSuccesses:       5,
			TotalFailures:        5,
			ConsecutiveSuccesses: 0,
			ConsecutiveFailures:  3,
		},
	)

	assert.Len(t, mockMidazListener.calls, 1)
	assert.Equal(t, "rabbitmq-producer", mockMidazListener.calls[0].ServiceName)
	assert.Equal(t, StateClosed, mockMidazListener.calls[0].FromState)
	assert.Equal(t, StateOpen, mockMidazListener.calls[0].ToState)
	// Verify all Counts fields are correctly mapped
	assert.Equal(t, uint32(10), mockMidazListener.calls[0].Counts.Requests)
	assert.Equal(t, uint32(5), mockMidazListener.calls[0].Counts.TotalSuccesses)
	assert.Equal(t, uint32(5), mockMidazListener.calls[0].Counts.TotalFailures)
	assert.Equal(t, uint32(0), mockMidazListener.calls[0].Counts.ConsecutiveSuccesses)
	assert.Equal(t, uint32(3), mockMidazListener.calls[0].Counts.ConsecutiveFailures)
}

func TestLibCommonsAdapter_HandlesNilListener(t *testing.T) {
	// Create adapter with nil listener
	adapter := NewLibCommonsAdapter(nil)

	// Should not panic when listener is nil
	adapter.OnStateChange(
		"test-service",
		libCircuitBreaker.StateClosed,
		libCircuitBreaker.StateOpen,
		libCircuitBreaker.Counts{},
	)
	// Test passes if no panic occurred
}

func TestConvertState_AllStates(t *testing.T) {
	tests := []struct {
		name     string
		input    libCircuitBreaker.State
		expected State
	}{
		{
			name:     "closed state",
			input:    libCircuitBreaker.StateClosed,
			expected: StateClosed,
		},
		{
			name:     "open state",
			input:    libCircuitBreaker.StateOpen,
			expected: StateOpen,
		},
		{
			name:     "half-open state",
			input:    libCircuitBreaker.StateHalfOpen,
			expected: StateHalfOpen,
		},
		{
			name:     "unknown state returns StateUnknown",
			input:    libCircuitBreaker.State("invalid-state"), // Invalid state value
			expected: StateUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertState(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLibCommonsAdapter_ForwardsAllStateTransitions(t *testing.T) {
	tests := []struct {
		name         string
		fromState    libCircuitBreaker.State
		toState      libCircuitBreaker.State
		expectedFrom State
		expectedTo   State
	}{
		{
			name:         "closed to open",
			fromState:    libCircuitBreaker.StateClosed,
			toState:      libCircuitBreaker.StateOpen,
			expectedFrom: StateClosed,
			expectedTo:   StateOpen,
		},
		{
			name:         "open to half-open",
			fromState:    libCircuitBreaker.StateOpen,
			toState:      libCircuitBreaker.StateHalfOpen,
			expectedFrom: StateOpen,
			expectedTo:   StateHalfOpen,
		},
		{
			name:         "half-open to closed",
			fromState:    libCircuitBreaker.StateHalfOpen,
			toState:      libCircuitBreaker.StateClosed,
			expectedFrom: StateHalfOpen,
			expectedTo:   StateClosed,
		},
		{
			name:         "half-open to open",
			fromState:    libCircuitBreaker.StateHalfOpen,
			toState:      libCircuitBreaker.StateOpen,
			expectedFrom: StateHalfOpen,
			expectedTo:   StateOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listener := &mockListener{}
			adapter := NewLibCommonsAdapter(listener)

			adapter.OnStateChange("test-service", tt.fromState, tt.toState, libCircuitBreaker.Counts{})

			assert.Len(t, listener.calls, 1)
			assert.Equal(t, tt.expectedFrom, listener.calls[0].FromState)
			assert.Equal(t, tt.expectedTo, listener.calls[0].ToState)
		})
	}
}
