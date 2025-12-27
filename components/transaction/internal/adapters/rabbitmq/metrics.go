package rabbitmq

import (
	"context"
	"sync"

	"github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"
)

const (
	// maxLabelLength is the maximum length for metric labels to prevent cardinality explosion.
	maxLabelLength = 64
)

// sanitizeLabelValue truncates a label value to prevent metric cardinality issues.
func sanitizeLabelValue(value string) string {
	if len(value) > maxLabelLength {
		return value[:maxLabelLength]
	}

	return value
}

// DLQ metric definitions
var (
	dlqPublishFailureMetric = metrics.Metric{
		Name:        "dlq_publish_failure_total",
		Unit:        "1",
		Description: "Total number of DLQ publish failures (messages permanently lost)",
	}

	dlqPublishSuccessMetric = metrics.Metric{
		Name:        "dlq_publish_success_total",
		Unit:        "1",
		Description: "Total number of successful DLQ publishes",
	}

	messageRetryMetric = metrics.Metric{
		Name:        "message_retry_total",
		Unit:        "1",
		Description: "Total number of message retries before DLQ",
	}
)

// DLQMetrics provides DLQ-related metrics using OpenTelemetry.
type DLQMetrics struct {
	factory *metrics.MetricsFactory
}

// dlqMetricsInstance is the singleton instance for DLQ metrics.
var (
	dlqMetricsInstance *DLQMetrics
	dlqMetricsMu       sync.RWMutex
)

// InitDLQMetrics initializes the DLQ metrics with the provided MetricsFactory.
// This should be called once during application startup after telemetry is initialized.
// It is safe to call multiple times; subsequent calls are no-ops.
func InitDLQMetrics(factory *metrics.MetricsFactory) {
	dlqMetricsMu.Lock()
	defer dlqMetricsMu.Unlock()

	if factory == nil {
		return
	}

	if dlqMetricsInstance != nil {
		return // Already initialized
	}

	dlqMetricsInstance = &DLQMetrics{
		factory: factory,
	}
}

// GetDLQMetrics returns the singleton DLQMetrics instance.
// Returns nil if InitDLQMetrics has not been called.
func GetDLQMetrics() *DLQMetrics {
	dlqMetricsMu.RLock()
	defer dlqMetricsMu.RUnlock()

	return dlqMetricsInstance
}

// ResetDLQMetrics clears the DLQ metrics singleton.
// This is primarily intended for testing to ensure test isolation.
func ResetDLQMetrics() {
	dlqMetricsMu.Lock()
	defer dlqMetricsMu.Unlock()

	dlqMetricsInstance = nil
}

// RecordDLQPublishFailure increments the dlq_publish_failure_total counter.
// This is called when a message cannot be published to DLQ and is permanently lost.
//
// Parameters:
//   - ctx: Context for metric recording (may contain trace correlation)
//   - queue: The original queue name where the message came from
//   - reason: The reason for failure (e.g., "channel_error", "broker_nack", "timeout")
func (dm *DLQMetrics) RecordDLQPublishFailure(ctx context.Context, queue, reason string) {
	if dm == nil || dm.factory == nil {
		return
	}

	dm.factory.Counter(dlqPublishFailureMetric).
		WithLabels(map[string]string{
			"queue":  sanitizeLabelValue(queue),
			"reason": sanitizeLabelValue(reason),
		}).
		AddOne(ctx)
}

// RecordDLQPublishSuccess increments the dlq_publish_success_total counter.
// This is called when a message is successfully published to DLQ.
func (dm *DLQMetrics) RecordDLQPublishSuccess(ctx context.Context, queue string) {
	if dm == nil || dm.factory == nil {
		return
	}

	dm.factory.Counter(dlqPublishSuccessMetric).
		WithLabels(map[string]string{
			"queue": sanitizeLabelValue(queue),
		}).
		AddOne(ctx)
}

// RecordMessageRetry increments the message_retry_total counter.
// This is called when a message is republished for retry.
func (dm *DLQMetrics) RecordMessageRetry(ctx context.Context, queue string) {
	if dm == nil || dm.factory == nil {
		return
	}

	dm.factory.Counter(messageRetryMetric).
		WithLabels(map[string]string{
			"queue": sanitizeLabelValue(queue),
		}).
		AddOne(ctx)
}

// recordDLQPublishFailure is a package-level helper that records a DLQ publish failure.
func recordDLQPublishFailure(ctx context.Context, queue, reason string) {
	dm := GetDLQMetrics()
	if dm != nil {
		dm.RecordDLQPublishFailure(ctx, queue, reason)
	}
}

// recordDLQPublishSuccess is a package-level helper that records a DLQ publish success.
func recordDLQPublishSuccess(ctx context.Context, queue string) {
	dm := GetDLQMetrics()
	if dm != nil {
		dm.RecordDLQPublishSuccess(ctx, queue)
	}
}

// recordMessageRetry is a package-level helper that records a message retry.
func recordMessageRetry(ctx context.Context, queue string) {
	dm := GetDLQMetrics()
	if dm != nil {
		dm.RecordMessageRetry(ctx, queue)
	}
}
