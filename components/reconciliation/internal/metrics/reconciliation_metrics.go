package reconmetrics

import (
	"context"
	"sync"

	libmetrics "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// Metric definitions.
var (
	runTotalMetric = libmetrics.Metric{
		Name:        "reconciliation_runs_total",
		Unit:        "1",
		Description: "Total number of reconciliation runs",
	}
	runDurationMetric = libmetrics.Metric{
		Name:        "reconciliation_run_duration_ms",
		Unit:        "ms",
		Description: "Reconciliation run duration in milliseconds",
	}
	checkDurationMetric = libmetrics.Metric{
		Name:        "reconciliation_check_duration_ms",
		Unit:        "ms",
		Description: "Reconciliation check duration in milliseconds",
	}
	balanceDiscrepanciesMetric = libmetrics.Metric{
		Name:        "reconciliation_balance_discrepancies",
		Unit:        "1",
		Description: "Current balance discrepancies",
	}
	doubleEntryUnbalancedMetric = libmetrics.Metric{
		Name:        "reconciliation_double_entry_unbalanced",
		Unit:        "1",
		Description: "Current unbalanced transactions",
	}
	orphanTransactionsMetric = libmetrics.Metric{
		Name:        "reconciliation_orphan_transactions",
		Unit:        "1",
		Description: "Current orphan transactions",
	}
	outboxPendingMetric = libmetrics.Metric{
		Name:        "reconciliation_outbox_pending",
		Unit:        "1",
		Description: "Pending outbox entries",
	}
	outboxFailedMetric = libmetrics.Metric{
		Name:        "reconciliation_outbox_failed",
		Unit:        "1",
		Description: "Failed outbox entries",
	}
	dlqEntriesMetric = libmetrics.Metric{
		Name:        "reconciliation_dlq_entries",
		Unit:        "1",
		Description: "DLQ entries",
	}
	redisMismatchMetric = libmetrics.Metric{
		Name:        "reconciliation_redis_mismatches",
		Unit:        "1",
		Description: "Redis mismatches",
	}
	lastRunTimestampMetric = libmetrics.Metric{
		Name:        "reconciliation_last_run_timestamp",
		Unit:        "s",
		Description: "Last reconciliation run timestamp (unix seconds)",
	}
)

// ReconciliationMetrics provides metrics for reconciliation runs.
type ReconciliationMetrics struct {
	factory *libmetrics.MetricsFactory
}

var (
	instance *ReconciliationMetrics
	mu       sync.RWMutex
)

// Init initializes reconciliation metrics.
func Init(factory *libmetrics.MetricsFactory) {
	mu.Lock()
	defer mu.Unlock()

	if factory == nil || instance != nil {
		return
	}

	instance = &ReconciliationMetrics{factory: factory}
}

// Get returns the reconciliation metrics instance.
func Get() *ReconciliationMetrics {
	mu.RLock()
	defer mu.RUnlock()

	return instance
}

// Reset clears metrics (testing only).
func Reset() {
	mu.Lock()
	defer mu.Unlock()

	instance = nil
}

// RecordRun records run metrics.
func (m *ReconciliationMetrics) RecordRun(ctx context.Context, report *domain.ReconciliationReport, durationMs int64) {
	if m == nil || m.factory == nil || report == nil {
		return
	}

	m.factory.Counter(runTotalMetric).
		WithLabels(map[string]string{"status": string(report.Status)}).
		AddOne(ctx)

	m.factory.Histogram(runDurationMetric).
		Record(ctx, durationMs)

	if report.CheckDurations != nil {
		for name, dur := range report.CheckDurations {
			m.factory.Histogram(checkDurationMetric).
				WithLabels(map[string]string{"check": name}).
				Record(ctx, dur)
		}
	}

	if report.BalanceCheck != nil {
		m.factory.Gauge(balanceDiscrepanciesMetric).Set(ctx, int64(report.BalanceCheck.BalancesWithDiscrepancy))
	}

	if report.DoubleEntryCheck != nil {
		m.factory.Gauge(doubleEntryUnbalancedMetric).Set(ctx, int64(report.DoubleEntryCheck.UnbalancedTransactions))
	}

	if report.OrphanCheck != nil {
		m.factory.Gauge(orphanTransactionsMetric).Set(ctx, int64(report.OrphanCheck.OrphanTransactions))
	}

	if report.OutboxCheck != nil {
		m.factory.Gauge(outboxPendingMetric).Set(ctx, report.OutboxCheck.Pending)
		m.factory.Gauge(outboxFailedMetric).Set(ctx, report.OutboxCheck.Failed)
	}

	if report.DLQCheck != nil {
		m.factory.Gauge(dlqEntriesMetric).Set(ctx, report.DLQCheck.Total)
	}

	if report.RedisCheck != nil {
		mismatch := int64(report.RedisCheck.MissingRedis + report.RedisCheck.ValueMismatches + report.RedisCheck.VersionMismatches)
		m.factory.Gauge(redisMismatchMetric).Set(ctx, mismatch)
	}

	m.factory.Gauge(lastRunTimestampMetric).Set(ctx, report.Timestamp.Unix())
}
