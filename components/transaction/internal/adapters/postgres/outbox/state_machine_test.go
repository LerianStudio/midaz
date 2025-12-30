package outbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidOutboxTransitions_Defined(t *testing.T) {
	// Verify all statuses are in the transition map
	statuses := []OutboxStatus{StatusPending, StatusProcessing, StatusPublished, StatusFailed, StatusDLQ}
	for _, s := range statuses {
		_, exists := ValidOutboxTransitions[s]
		assert.True(t, exists, "status %s must be in ValidOutboxTransitions", s)
	}
}

func TestOutboxStatus_CanTransitionTo_ValidTransitions(t *testing.T) {
	tests := []struct {
		from OutboxStatus
		to   OutboxStatus
	}{
		{StatusPending, StatusProcessing},
		{StatusProcessing, StatusPublished},
		{StatusProcessing, StatusFailed},
		{StatusFailed, StatusProcessing},
		{StatusFailed, StatusDLQ},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			assert.True(t, tt.from.CanTransitionTo(tt.to),
				"transition from %s to %s should be valid", tt.from, tt.to)
		})
	}
}

func TestOutboxStatus_CanTransitionTo_InvalidTransitions(t *testing.T) {
	tests := []struct {
		from OutboxStatus
		to   OutboxStatus
	}{
		// PENDING can only go to PROCESSING
		{StatusPending, StatusPublished},
		{StatusPending, StatusFailed},
		{StatusPending, StatusDLQ},
		// PROCESSING cannot go back to PENDING or directly to DLQ
		{StatusProcessing, StatusPending},
		{StatusProcessing, StatusDLQ},
		// PUBLISHED is terminal
		{StatusPublished, StatusPending},
		{StatusPublished, StatusProcessing},
		{StatusPublished, StatusFailed},
		{StatusPublished, StatusDLQ},
		// DLQ is terminal
		{StatusDLQ, StatusPending},
		{StatusDLQ, StatusProcessing},
		{StatusDLQ, StatusPublished},
		{StatusDLQ, StatusFailed},
		// FAILED cannot go directly to PUBLISHED
		{StatusFailed, StatusPublished},
		{StatusFailed, StatusPending},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			assert.False(t, tt.from.CanTransitionTo(tt.to),
				"transition from %s to %s should be invalid", tt.from, tt.to)
		})
	}
}

func TestOutboxStatus_IsTerminal(t *testing.T) {
	assert.False(t, StatusPending.IsTerminal(), "PENDING is not terminal")
	assert.False(t, StatusProcessing.IsTerminal(), "PROCESSING is not terminal")
	assert.False(t, StatusFailed.IsTerminal(), "FAILED is not terminal")
	assert.True(t, StatusPublished.IsTerminal(), "PUBLISHED is terminal")
	assert.True(t, StatusDLQ.IsTerminal(), "DLQ is terminal")
}
