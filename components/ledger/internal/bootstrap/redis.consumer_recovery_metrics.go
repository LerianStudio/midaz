// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
)

const (
	recoveryMetricOutcomeCompleted          = "completed"
	recoveryMetricOutcomeContextCanceled    = "context_canceled"
	recoveryMetricOutcomeNotConfigured      = "not_configured"
	recoveryMetricOutcomeFinalizationFailed = "finalization_failed"
	recoveryMetricOutcomeAckFailed          = "ack_failed"
	recoveryMetricOutcomeRecordChanged      = "record_changed"
	recoveryMetricOutcomeInvalidAck         = "invalid_ack"

	recoveryMetricName         = "engine_recovery_total"
	recoveryMetricDurationName = "engine_recovery_duration_ms"
	recoveryMetricBacklogName  = "engine_recovery_backlog_count"
	recoveryMetricOutcomeLabel = "outcome"
	recoveryMetricSourceLabel  = "source"
	recoveryMetricCounterUnit  = "1"
	recoveryMetricDurationUnit = "ms"
	recoveryMetricCounterDesc  = "Number of bounded engine recovery outcomes."
	recoveryMetricDurationDesc = "Duration of engine recovery finalization in milliseconds."
	recoveryMetricBacklogDesc  = "Current recovery records awaiting processing by bounded queue source."
)

var recoveryMetricDurationBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000}

func (r *recoveryRecordCompleter) emitRecoveryMetrics(ctx context.Context, source txRedis.RecoveryQueueSource, outcome string, duration time.Duration) {
	if r.metricsFactory == nil {
		return
	}

	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
	labels := map[string]string{
		recoveryMetricOutcomeLabel: recoveryMetricOutcome(outcome),
		recoveryMetricSourceLabel:  recoveryMetricSource(source),
	}

	if err := r.metricsFactory.AddCounter(ctx, recoveryMetricName, recoveryMetricCounterDesc, recoveryMetricCounterUnit, labels, 1); err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Unable to record engine recovery outcome metric", libLog.Err(err))
	}

	if err := r.metricsFactory.RecordHistogram(ctx, recoveryMetricDurationName, recoveryMetricDurationDesc, recoveryMetricDurationUnit, labels, float64(duration)/float64(time.Millisecond), recoveryMetricDurationBuckets); err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Unable to record engine recovery duration metric", libLog.Err(err))
	}
}

func recoveryMetricOutcome(outcome string) string {
	switch outcome {
	case recoveryMetricOutcomeCompleted,
		recoveryMetricOutcomeContextCanceled,
		recoveryMetricOutcomeNotConfigured,
		recoveryMetricOutcomeFinalizationFailed,
		recoveryMetricOutcomeAckFailed,
		recoveryMetricOutcomeRecordChanged,
		recoveryMetricOutcomeInvalidAck:
		return outcome
	default:
		return "unknown"
	}
}

func (r *recoveryRecordCompleter) emitRecoveryBacklog(
	ctx context.Context,
	source txRedis.RecoveryQueueSource,
	depth int,
) {
	if r == nil || r.metricsFactory == nil {
		return
	}

	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)

	labels := map[string]string{recoveryMetricSourceLabel: recoveryMetricSource(source)}
	if err := r.metricsFactory.SetGauge(
		ctx,
		recoveryMetricBacklogName,
		recoveryMetricBacklogDesc,
		recoveryMetricCounterUnit,
		labels,
		int64(depth),
	); err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Unable to record engine recovery backlog metric", libLog.Err(err))
	}
}

func recoveryMetricSource(source txRedis.RecoveryQueueSource) string {
	switch source {
	case txRedis.RecoveryQueueSourceLegacyBackup, txRedis.RecoveryQueueSourceEngineRecover:
		return string(source)
	default:
		return "unknown"
	}
}
