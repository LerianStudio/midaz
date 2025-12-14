package mruntime

import (
	"context"
	"sync"

	"github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"
)

const (
	// maxLabelLength is the maximum length for metric labels to prevent cardinality explosion.
	maxLabelLength = 64
)

// sanitizeLabel truncates a label value to prevent metric cardinality issues.
func sanitizeLabel(value string) string {
	if len(value) > maxLabelLength {
		return value[:maxLabelLength]
	}
	return value
}

// PanicMetrics provides panic-related metrics using OpenTelemetry.
// It wraps lib-commons' MetricsFactory for consistent metric handling.
type PanicMetrics struct {
	factory *metrics.MetricsFactory
}

// panicRecoveredMetric defines the metric for counting recovered panics.
var panicRecoveredMetric = metrics.Metric{
	Name:        "panic_recovered_total",
	Unit:        "1",
	Description: "Total number of recovered panics",
}

// panicMetricsInstance is the singleton instance for panic metrics.
// It is initialized lazily via InitPanicMetrics.
var (
	panicMetricsInstance *PanicMetrics
	panicMetricsMu       sync.RWMutex
)

// InitPanicMetrics initializes the panic metrics with the provided MetricsFactory.
// This should be called once during application startup after telemetry is initialized.
// It is safe to call multiple times; subsequent calls are no-ops.
//
// Example:
//
//	tl := opentelemetry.InitializeTelemetry(cfg)
//	mruntime.InitPanicMetrics(tl.MetricsFactory)
func InitPanicMetrics(factory *metrics.MetricsFactory) {
	panicMetricsMu.Lock()
	defer panicMetricsMu.Unlock()

	if factory == nil {
		return
	}

	if panicMetricsInstance != nil {
		return // Already initialized
	}

	panicMetricsInstance = &PanicMetrics{
		factory: factory,
	}
}

// GetPanicMetrics returns the singleton PanicMetrics instance.
// Returns nil if InitPanicMetrics has not been called.
func GetPanicMetrics() *PanicMetrics {
	panicMetricsMu.RLock()
	defer panicMetricsMu.RUnlock()

	return panicMetricsInstance
}

// ResetPanicMetrics clears the panic metrics singleton.
// This is primarily intended for testing to ensure test isolation.
// In production, this should generally not be called.
func ResetPanicMetrics() {
	panicMetricsMu.Lock()
	defer panicMetricsMu.Unlock()

	panicMetricsInstance = nil
}

// RecordPanicRecovered increments the panic_recovered_total counter with the given labels.
// If metrics are not initialized, this is a no-op.
//
// Parameters:
//   - ctx: Context for metric recording (may contain trace correlation)
//   - component: The component where the panic occurred (e.g., "transaction", "onboarding", "crm")
//   - goroutineName: The name of the goroutine or handler (e.g., "http_handler", "rabbitmq_worker")
func (pm *PanicMetrics) RecordPanicRecovered(ctx context.Context, component, goroutineName string) {
	if pm == nil || pm.factory == nil {
		return
	}

	pm.factory.Counter(panicRecoveredMetric).
		WithLabels(map[string]string{
			"component":      sanitizeLabel(component),
			"goroutine_name": sanitizeLabel(goroutineName),
		}).
		AddOne(ctx)
}

// recordPanicMetric is a package-level helper that records a panic metric if metrics are initialized.
// This is called internally by recovery functions.
func recordPanicMetric(ctx context.Context, component, goroutineName string) {
	pm := GetPanicMetrics()
	if pm != nil {
		pm.RecordPanicRecovered(ctx, component, goroutineName)
	}
}
