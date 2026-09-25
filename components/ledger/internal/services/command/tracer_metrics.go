// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

var tracerDurationBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000}

// emitTracerMetric uses closed vocabularies only. Neither facts, identities nor
// arbitrary errors may become labels, even when supplied by a future caller.
func emitTracerMetric(ctx context.Context, factory *metrics.MetricsFactory, operation, result string, duration time.Duration) {
	if factory == nil {
		return
	}

	switch operation {
	case "admission", "confirm", "release", "recovery":
	default:
		operation = "unknown"
	}

	switch result {
	case "allow", "deny", "review", "fail_open", "unavailable", "context_invalid", "coordination_uncertain", "delivered", "failed", "unresolved", "quarantined":
	default:
		result = "unknown"
	}

	labels := map[string]string{"operation": operation, "result": result}

	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
	if err := factory.AddCounter(ctx, "tracer_coordination_total", "Tracer admission and durable delivery attempts by bounded outcome.", "1", labels, 1); err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Unable to record Tracer coordination metric", libLog.Err(err))
	}

	if err := factory.RecordHistogram(ctx, "tracer_coordination_duration_ms", "Tracer admission and durable delivery duration in milliseconds.", "ms", labels, float64(duration)/float64(time.Millisecond), tracerDurationBuckets); err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Unable to record Tracer coordination duration", libLog.Err(err))
	}
}

type tracerQuarantineCounter interface {
	CountQuarantined(context.Context) (int64, error)
}

func emitTracerQuarantineGauge(ctx context.Context, factory *metrics.MetricsFactory, store TracerObligationStore) {
	if factory == nil {
		return
	}

	counter, ok := store.(tracerQuarantineCounter)
	if !ok {
		return
	}

	count, err := counter.CountQuarantined(ctx)

	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
	if err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Unable to count quarantined Tracer obligations", libLog.Err(err))
		return
	}

	if err := factory.SetGauge(ctx, "tracer_obligations_quarantined", "Current number of quarantined Tracer obligations for the resolved tenant.", "1", nil, count); err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Unable to record quarantined Tracer obligation gauge", libLog.Err(err))
	}
}

func tracerAdmissionMetric(attempt ContextTracerAttempt, outcome reservationOutcome, err error) string {
	if attempt.IntentAttempted && !attempt.Frozen {
		return "coordination_uncertain"
	}

	if err != nil {
		if outcome.Kind == reservationProceed {
			return "fail_open"
		}

		if !tracerAdmissionUnavailable(err) {
			return "context_invalid"
		}

		return "unavailable"
	}

	if attempt.Result == nil {
		return "unavailable"
	}

	switch attempt.Result.Decision {
	case tracercontract.DecisionAllow:
		return "allow"
	case tracercontract.DecisionDeny:
		return "deny"
	case tracercontract.DecisionReview:
		return "review"
	default:
		return "unavailable"
	}
}

// Age is observed for claimed records, not inferred for the entire backlog.
func emitTracerObligationAge(ctx context.Context, factory *metrics.MetricsFactory, record tracerreservation.Pending, now time.Time) {
	if factory == nil || record.CreatedAt.IsZero() || now.Before(record.CreatedAt) {
		return
	}

	state := "unknown"

	switch record.State {
	case tracerreservation.Prepared, tracerreservation.Executing, tracerreservation.Confirmed, tracerreservation.Released:
		state = string(record.State)
	}

	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
	if err := factory.RecordHistogram(ctx, "tracer_obligation_age_ms", "Age of claimed Tracer obligations, including repeated recovery attempts.", "ms", map[string]string{"state": state}, float64(now.Sub(record.CreatedAt))/float64(time.Millisecond), []float64{1000, 10000, 60000, 300000, 3600000, 86400000, 604800000}); err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Unable to record Tracer obligation age", libLog.Err(err))
	}
}
