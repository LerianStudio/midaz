// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libMetrics "github.com/LerianStudio/lib-observability/v4/metrics"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

var metricContextLimitEligibilityFailures = libMetrics.Metric{
	Name:        "tracer_context_limit_eligibility_failures_total",
	Unit:        "1",
	Description: "Total shared admissions rejected because an active limit is not eligible for account-scoped context evaluation.",
}

func recordContextLimitEligibilityFailure(ctx context.Context, factory *libMetrics.MetricsFactory, logger libLog.Logger, err error) {
	if !errors.Is(err, constant.ErrContextLimitsUnavailable) {
		return
	}

	if logger != nil {
		logger.Log(ctx, libLog.LevelError,
			"Shared admission rejected by an ineligible active limit; run the per-tenant eligibility report before activation")
	}

	if factory == nil {
		return
	}

	counter, metricErr := factory.Counter(metricContextLimitEligibilityFailures)
	if metricErr == nil {
		metricErr = counter.Add(ctx, 1)
	}

	if metricErr != nil && logger != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit context limit eligibility counter", libLog.Err(metricErr))
	}
}
