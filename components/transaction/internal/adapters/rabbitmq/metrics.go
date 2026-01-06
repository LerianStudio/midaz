package rabbitmq

import (
	"context"
	"sync"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
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

	balanceStatusFailedMetric = metrics.Metric{
		Name:        "transaction_balance_status_failed_total",
		Unit:        "1",
		Description: "Total number of transactions with balance_status=FAILED (DLQ)",
	}

	balanceStatusUpdateFailedMetric = metrics.Metric{
		Name:        "transaction_balance_status_update_failed_total",
		Unit:        "1",
		Description: "Total number of balance_status update failures after retries",
	}

	balanceStatusDLQUpdateFailedMetric = metrics.Metric{
		Name:        "transaction_balance_status_dlq_update_failed_total",
		Unit:        "1",
		Description: "Total number of failures updating balance_status during DLQ hook",
	}

	messageLossMetric = metrics.Metric{
		Name:        "message_loss_total",
		Unit:        "1",
		Description: "Total number of messages lost after ack (republish failed)",
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

// RecordBalanceStatusFailed increments the transaction_balance_status_failed_total counter.
// This is called when a transaction's balance update fails after max retries and is marked FAILED.
func (dm *DLQMetrics) RecordBalanceStatusFailed(ctx context.Context, queue string) {
	if dm == nil || dm.factory == nil {
		return
	}

	// IMPORTANT: Do NOT include transaction_id as a metric label (high cardinality / DoS risk).
	// Use structured logs for correlation instead.
	dm.factory.Counter(balanceStatusFailedMetric).
		WithLabels(map[string]string{
			"queue": sanitizeLabelValue(queue),
		}).
		AddOne(ctx)
}

// RecordBalanceStatusFailedWithLogging is a package-level helper that records a balance status failure
// metric and logs the event. Use this instead of DLQMetrics.RecordBalanceStatusFailed when you also
// need logging with the transaction ID.
// Keep transactionID only for logs (not metric labels).
func RecordBalanceStatusFailedWithLogging(ctx context.Context, queue, transactionID string) {
	dm := GetDLQMetrics()
	if dm != nil {
		dm.RecordBalanceStatusFailed(ctx, queue)
	}

	logger, _, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // lib-commons API returns 4 values, only logger needed here

	logger.Warnf("Transaction marked FAILED (DLQ): queue=%s transaction_id=%s", queue, transactionID)
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

// RecordBalanceStatusUpdateFailure records when a status update fails after retries.
// Uses structured logging for alerting systems (Datadog, PagerDuty, etc).
// Note: transaction_id is logged, not in Prometheus label (avoids high cardinality).
func RecordBalanceStatusUpdateFailure(ctx context.Context, targetStatus, transactionID string) {
	dm := GetDLQMetrics()
	if dm != nil && dm.factory != nil {
		dm.factory.Counter(balanceStatusUpdateFailedMetric).
			WithLabels(map[string]string{
				"target_status": sanitizeLabelValue(targetStatus),
			}).
			AddOne(ctx)
	}

	// Structured log for alerting - transaction_id here (not in metric)
	logger, _, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // lib-commons API returns 4 values, only logger needed here

	logger.Errorf("Balance status update failed after retries: target_status=%s transaction_id=%s severity=critical", targetStatus, transactionID)
}

// RecordBalanceStatusDLQUpdateFailure records when a DLQ hook cannot mark a transaction as FAILED.
// This should be rare; it usually indicates DB outage during DLQ routing.
// Note: transaction_id is logged, not in Prometheus label.
func RecordBalanceStatusDLQUpdateFailure(ctx context.Context, queue, targetStatus, transactionID string) {
	dm := GetDLQMetrics()
	if dm != nil && dm.factory != nil {
		dm.factory.Counter(balanceStatusDLQUpdateFailedMetric).
			WithLabels(map[string]string{
				"queue":         sanitizeLabelValue(queue),
				"target_status": targetStatus,
			}).
			AddOne(ctx)
	}

	logger, _, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // lib-commons API returns 4 values, only logger needed here

	logger.Errorf("Balance status update failed during DLQ hook: queue=%s target_status=%s transaction_id=%s severity=critical", queue, targetStatus, transactionID)
}

// recordMessageLoss records when a message is lost after being acked but before republish succeeds.
// This happens in the ack-first pattern when republish fails after the original message was already acked.
// The message cannot be recovered via RabbitMQ redelivery at this point.
func recordMessageLoss(ctx context.Context, queue, reason string) {
	dm := GetDLQMetrics()
	if dm != nil && dm.factory != nil {
		dm.factory.Counter(messageLossMetric).
			WithLabels(map[string]string{
				"queue":  sanitizeLabelValue(queue),
				"reason": sanitizeLabelValue(reason),
			}).
			AddOne(ctx)
	}

	logger, _, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // lib-commons API returns 4 values, only logger needed here

	logger.Errorf("MESSAGE_LOSS: Message lost after ack (republish failed): queue=%s reason=%s severity=critical", queue, reason)
}
