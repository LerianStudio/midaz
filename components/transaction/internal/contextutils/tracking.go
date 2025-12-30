// Package contextutils provides context utility functions for extracting tracking information.
package contextutils

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"
	"go.opentelemetry.io/otel/trace"
)

// LoggerFromContext extracts only the logger from context, avoiding dogsled linter warnings.
func LoggerFromContext(ctx context.Context) libLog.Logger {
	logger, _, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // helper isolates the pattern
	return logger
}

// TracerFromContext extracts only the tracer from context, avoiding dogsled linter warnings.
func TracerFromContext(ctx context.Context) trace.Tracer {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // helper isolates the pattern
	return tracer
}

// MetricsFromContext extracts only the metrics factory from context, avoiding dogsled linter warnings.
func MetricsFromContext(ctx context.Context) *metrics.MetricsFactory {
	_, _, _, metricFactory := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // helper isolates the pattern
	return metricFactory
}
