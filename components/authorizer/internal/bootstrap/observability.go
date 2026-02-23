// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"strings"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libMetrics "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"
)

var (
	authorizeRequestsTotal = libMetrics.Metric{
		Name:        "authorizer_authorize_requests_total",
		Unit:        "1",
		Description: "Total number of authorizer authorization requests.",
	}
	authorizeLatencyMs = libMetrics.Metric{
		Name:        "authorizer_authorize_latency_ms",
		Unit:        "ms",
		Description: "Authorization request latency in milliseconds.",
	}
	authorizeOperationsPerRequest = libMetrics.Metric{
		Name:        "authorizer_operations_per_request",
		Unit:        "1",
		Description: "Number of operations per authorization request.",
	}
	authorizeShardsTouchedPerRequest = libMetrics.Metric{
		Name:        "authorizer_shards_touched_per_request",
		Unit:        "1",
		Description: "Number of shards touched per authorization request.",
	}
	engineLockWaitMs = libMetrics.Metric{
		Name:        "authorizer_engine_lock_wait_ms",
		Unit:        "ms",
		Description: "Total lock wait time while acquiring shard locks.",
	}
	engineLockHoldMs = libMetrics.Metric{
		Name:        "authorizer_engine_lock_hold_ms",
		Unit:        "ms",
		Description: "Lock hold duration for shard-locked authorization critical section.",
	}
	walQueueDepth = libMetrics.Metric{
		Name:        "authorizer_wal_queue_depth",
		Unit:        "1",
		Description: "Current pending entries in the authorizer WAL queue.",
	}
	walAppendDroppedTotal = libMetrics.Metric{
		Name:        "authorizer_wal_append_drop_total",
		Unit:        "1",
		Description: "Total number of dropped WAL appends.",
	}
	walWriteErrorsTotal = libMetrics.Metric{
		Name:        "authorizer_wal_write_errors_total",
		Unit:        "1",
		Description: "Total WAL write/flush/sync errors.",
	}
	walFsyncLatencyMs = libMetrics.Metric{
		Name:        "authorizer_wal_fsync_latency_ms",
		Unit:        "ms",
		Description: "WAL fsync latency in milliseconds.",
	}
	redpandaPublishLatencyMs = libMetrics.Metric{
		Name:        "authorizer_redpanda_publish_latency_ms",
		Unit:        "ms",
		Description: "Redpanda publish latency in milliseconds.",
	}
	redpandaPublishErrorsTotal = libMetrics.Metric{
		Name:        "authorizer_redpanda_publish_errors_total",
		Unit:        "1",
		Description: "Total failed publish attempts to Redpanda.",
	}
)

type authorizerMetrics struct {
	factory *libMetrics.MetricsFactory
	logger  libLog.Logger
}

func (m *authorizerMetrics) Enabled() bool {
	return m != nil && m.factory != nil
}

func newAuthorizerMetrics(telemetry *libOpentelemetry.Telemetry, logger libLog.Logger) *authorizerMetrics {
	if telemetry == nil {
		return &authorizerMetrics{logger: logger}
	}

	return &authorizerMetrics{factory: telemetry.MetricsFactory, logger: logger}
}

func (m *authorizerMetrics) RecordAuthorize(
	ctx context.Context,
	method string,
	result string,
	rejectionCode string,
	pending bool,
	transactionStatus string,
	operationCount int,
	shardCount int,
	latency time.Duration,
) {
	if m == nil || m.factory == nil {
		return
	}

	labels := map[string]string{
		"method":           method,
		"result":           result,
		"rejection_code":   normalizeRejectionCode(rejectionCode),
		"pending":          boolLabel(pending),
		"tx_status":        normalizeStatusLabel(transactionStatus),
		"ops_count_bucket": bucketOperationCount(operationCount),
		"shard_bucket":     bucketShardCount(shardCount),
	}

	m.factory.Counter(authorizeRequestsTotal).WithLabels(labels).AddOne(ctx)
	m.factory.Histogram(authorizeLatencyMs).WithLabels(labels).Record(ctx, durationMillis(latency))
	m.factory.Histogram(authorizeOperationsPerRequest).WithLabels(map[string]string{"method": method}).Record(ctx, int64(operationCount))
	m.factory.Histogram(authorizeShardsTouchedPerRequest).WithLabels(map[string]string{"method": method}).Record(ctx, int64(shardCount))
}

func (m *authorizerMetrics) RecordPublish(ctx context.Context, topic string, err error, latency time.Duration) {
	if m == nil || m.factory == nil {
		return
	}

	labels := map[string]string{"topic": normalizeTopic(topic)}
	m.factory.Histogram(redpandaPublishLatencyMs).WithLabels(labels).Record(ctx, durationMillis(latency))
	if err != nil {
		m.factory.Counter(redpandaPublishErrorsTotal).WithLabels(labels).AddOne(ctx)
	}
}

func (m *authorizerMetrics) ObserveAuthorizeLockWait(shardCount int, wait time.Duration) {
	if m == nil || m.factory == nil {
		return
	}

	m.factory.Histogram(engineLockWaitMs).WithLabels(map[string]string{"shard_bucket": bucketShardCount(shardCount)}).
		Record(context.Background(), durationMillis(wait))
}

func (m *authorizerMetrics) ObserveAuthorizeLockHold(shardCount int, hold time.Duration) {
	if m == nil || m.factory == nil {
		return
	}

	m.factory.Histogram(engineLockHoldMs).WithLabels(map[string]string{"shard_bucket": bucketShardCount(shardCount)}).
		Record(context.Background(), durationMillis(hold))
}

func (m *authorizerMetrics) ObserveWALAppendFailure(err error) {
	if m == nil {
		return
	}

	if m.factory != nil {
		m.factory.Counter(walAppendDroppedTotal).WithLabels(map[string]string{"reason": "append_failed"}).AddOne(context.Background())
	}

	if m.logger != nil && err != nil {
		m.logger.Errorf("Authorizer WAL append failed (fail-closed): %v", err)
	}
}

func (m *authorizerMetrics) ObserveWALQueueDepth(depth int) {
	if m == nil || m.factory == nil {
		return
	}

	m.factory.Gauge(walQueueDepth).Set(context.Background(), int64(depth))
}

func (m *authorizerMetrics) ObserveWALAppendDropped(err error) {
	if m == nil {
		return
	}

	if m.factory != nil {
		m.factory.Counter(walAppendDroppedTotal).WithLabels(map[string]string{"reason": "buffer_full"}).AddOne(context.Background())
	}

	if m.logger != nil && err != nil {
		m.logger.Warnf("Authorizer WAL append dropped: %v", err)
	}
}

func (m *authorizerMetrics) ObserveWALWriteError(stage string, err error) {
	if m == nil {
		return
	}

	if m.factory != nil {
		m.factory.Counter(walWriteErrorsTotal).WithLabels(map[string]string{"stage": normalizeStage(stage)}).AddOne(context.Background())
	}

	if m.logger != nil && err != nil {
		m.logger.Errorf("Authorizer WAL %s error: %v", normalizeStage(stage), err)
	}
}

func (m *authorizerMetrics) ObserveWALFsyncLatency(latency time.Duration) {
	if m == nil || m.factory == nil {
		return
	}

	m.factory.Histogram(walFsyncLatencyMs).Record(context.Background(), durationMillis(latency))
}

func bucketOperationCount(operationCount int) string {
	switch {
	case operationCount <= 0:
		return "0"
	case operationCount == 1:
		return "1"
	case operationCount <= 4:
		return "2_4"
	case operationCount <= 10:
		return "5_10"
	default:
		return "11_plus"
	}
}

func bucketShardCount(shardCount int) string {
	switch {
	case shardCount <= 0:
		return "0"
	case shardCount == 1:
		return "1"
	case shardCount == 2:
		return "2"
	case shardCount <= 4:
		return "3_4"
	default:
		return "5_plus"
	}
}

func normalizeStatusLabel(value string) string {
	status := strings.ToLower(strings.TrimSpace(value))

	switch status {
	case "", "created", "pending", "approved", "canceled", "approved_compensate":
		if status == "" {
			return "empty"
		}

		return status
	default:
		return "other"
	}
}

func normalizeRejectionCode(value string) string {
	rejectionCode := strings.ToLower(strings.TrimSpace(value))
	if rejectionCode == "" {
		return "none"
	}

	if len(rejectionCode) > 64 {
		return rejectionCode[:64]
	}

	return rejectionCode
}

func normalizeStage(value string) string {
	stage := strings.TrimSpace(strings.ToLower(value))
	if stage == "" {
		return "unknown"
	}

	return stage
}

func normalizeTopic(value string) string {
	topic := strings.TrimSpace(strings.ToLower(value))
	if topic == "" {
		return "unknown"
	}

	if len(topic) > 64 {
		return topic[:64]
	}

	return topic
}

func boolLabel(value bool) string {
	if value {
		return "true"
	}

	return "false"
}

func durationMillis(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}

	return d.Milliseconds()
}
