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
			Requests:            10,
			TotalFailures:       5,
			ConsecutiveFailures: 3,
		},
	)

	assert.Len(t, mockMidazListener.calls, 1)
	assert.Equal(t, "rabbitmq-producer", mockMidazListener.calls[0].ServiceName)
	assert.Equal(t, StateClosed, mockMidazListener.calls[0].FromState)
	assert.Equal(t, StateOpen, mockMidazListener.calls[0].ToState)
}
