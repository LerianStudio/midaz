// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
)

const (
	engineWriteBehindProjectionMetric     = "engine_write_behind_projection_total"
	engineWriteBehindProjectionMetricDesc = "Number of applied accounting results routed to bounded projection paths."
)

func (uc *UseCase) recordEngineWriteBehindProjection(ctx context.Context, path, outcome string) {
	if uc == nil || uc.MetricsFactory == nil {
		return
	}

	if err := uc.MetricsFactory.AddCounter(ctx, engineWriteBehindProjectionMetric, engineWriteBehindProjectionMetricDesc, "1",
		engineWriteBehindMetricLabels(path, outcome), 1); err != nil {
		logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
		logger.Log(ctx, libLog.LevelDebug, "Failed to record engine write-behind projection metric", libLog.Err(err))
	}
}

func engineWriteBehindMetricLabels(path, outcome string) map[string]string {
	return map[string]string{
		"path":    boundedEngineWriteBehindPath(path),
		"outcome": boundedEngineWriteBehindOutcome(outcome),
	}
}

func boundedEngineWriteBehindPath(path string) string {
	switch path {
	case "async", "fallback", "sync", "bulk":
		return path
	default:
		return "unknown"
	}
}

func boundedEngineWriteBehindOutcome(outcome string) string {
	switch outcome {
	case "published", "completed", "deferred", "failed":
		return outcome
	default:
		return "unknown"
	}
}
