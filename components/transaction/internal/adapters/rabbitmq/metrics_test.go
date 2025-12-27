package rabbitmq

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeLabelValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short string unchanged",
			input:    "short",
			expected: "short",
		},
		{
			name:     "exact length unchanged",
			input:    "a" + string(make([]byte, maxLabelLength-1)),
			expected: "a" + string(make([]byte, maxLabelLength-1)),
		},
		{
			name:     "long string truncated",
			input:    string(make([]byte, maxLabelLength+10)),
			expected: string(make([]byte, maxLabelLength)),
		},
		{
			name:     "empty string unchanged",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := sanitizeLabelValue(tt.input)
			assert.Equal(t, len(tt.expected), len(result))
		})
	}
}

func TestDLQMetrics_NilSafe(t *testing.T) {
	t.Parallel()

	t.Run("nil metrics instance does not panic", func(t *testing.T) {
		t.Parallel()

		var dm *DLQMetrics

		// These should not panic
		assert.NotPanics(t, func() {
			dm.RecordDLQPublishFailure(context.Background(), "test-queue", "test-reason")
		})
		assert.NotPanics(t, func() {
			dm.RecordDLQPublishSuccess(context.Background(), "test-queue")
		})
		assert.NotPanics(t, func() {
			dm.RecordMessageRetry(context.Background(), "test-queue")
		})
	})

	t.Run("metrics with nil factory does not panic", func(t *testing.T) {
		t.Parallel()

		dm := &DLQMetrics{factory: nil}

		assert.NotPanics(t, func() {
			dm.RecordDLQPublishFailure(context.Background(), "test-queue", "test-reason")
		})
		assert.NotPanics(t, func() {
			dm.RecordDLQPublishSuccess(context.Background(), "test-queue")
		})
	})
}

func TestInitDLQMetrics(t *testing.T) {
	t.Run("nil factory is no-op", func(t *testing.T) {
		ResetDLQMetrics() // Clean state
		defer ResetDLQMetrics()

		InitDLQMetrics(nil)

		assert.Nil(t, GetDLQMetrics(), "nil factory should not initialize metrics")
	})
}

func TestResetDLQMetrics(t *testing.T) {
	t.Run("reset clears singleton", func(t *testing.T) {
		// Manually set instance for test
		dlqMetricsMu.Lock()
		dlqMetricsInstance = &DLQMetrics{}
		dlqMetricsMu.Unlock()

		assert.NotNil(t, GetDLQMetrics())

		ResetDLQMetrics()

		assert.Nil(t, GetDLQMetrics())
	})
}

func TestCategorizePublishError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error returns unknown",
			err:      nil,
			expected: "unknown",
		},
		{
			name:     "ErrConfirmChannelClosed",
			err:      ErrConfirmChannelClosed,
			expected: "channel_closed",
		},
		{
			name:     "ErrBrokerNack",
			err:      ErrBrokerNack,
			expected: "broker_nack",
		},
		{
			name:     "ErrConfirmTimeout",
			err:      ErrConfirmTimeout,
			expected: "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := categorizePublishError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDLQMetricDefinitions(t *testing.T) {
	t.Parallel()

	t.Run("dlqPublishFailureMetric is properly defined", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "dlq_publish_failure_total", dlqPublishFailureMetric.Name)
		assert.Equal(t, "1", dlqPublishFailureMetric.Unit)
		assert.NotEmpty(t, dlqPublishFailureMetric.Description)
	})

	t.Run("dlqPublishSuccessMetric is properly defined", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "dlq_publish_success_total", dlqPublishSuccessMetric.Name)
		assert.Equal(t, "1", dlqPublishSuccessMetric.Unit)
		assert.NotEmpty(t, dlqPublishSuccessMetric.Description)
	})

	t.Run("messageRetryMetric is properly defined", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "message_retry_total", messageRetryMetric.Name)
		assert.Equal(t, "1", messageRetryMetric.Unit)
		assert.NotEmpty(t, messageRetryMetric.Description)
	})
}

func TestRecordDLQPublishFailure_NoMetricsInitialized(t *testing.T) {
	ResetDLQMetrics()
	defer ResetDLQMetrics()

	// Should not panic when metrics not initialized
	assert.NotPanics(t, func() {
		recordDLQPublishFailure(context.Background(), "test-queue", "timeout")
	})
}

func TestRecordDLQPublishSuccess_NoMetricsInitialized(t *testing.T) {
	ResetDLQMetrics()
	defer ResetDLQMetrics()

	// Should not panic when metrics not initialized
	assert.NotPanics(t, func() {
		recordDLQPublishSuccess(context.Background(), "test-queue")
	})
}

func TestRecordMessageRetry_NoMetricsInitialized(t *testing.T) {
	ResetDLQMetrics()
	defer ResetDLQMetrics()

	// Should not panic when metrics not initialized
	assert.NotPanics(t, func() {
		recordMessageRetry(context.Background(), "test-queue")
	})
}
