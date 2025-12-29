package mmigration

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics for migration operations.
var (
	// MigrationDurationSeconds tracks time spent in migration operations.
	MigrationDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "midaz",
			Subsystem: "migration",
			Name:      "duration_seconds",
			Help:      "Time spent in migration operations",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"component", "operation", "status"},
	)

	// MigrationRecoveryTotal counts migration recovery attempts.
	MigrationRecoveryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "midaz",
			Subsystem: "migration",
			Name:      "recovery_total",
			Help:      "Total count of migration recovery attempts",
		},
		[]string{"component", "status"},
	)

	// MigrationLockWaitSeconds tracks time spent waiting for advisory locks.
	MigrationLockWaitSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "midaz",
			Subsystem: "migration",
			Name:      "lock_wait_seconds",
			Help:      "Time spent waiting for migration advisory lock",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"component", "acquired"},
	)

	// MigrationStatusGauge indicates current migration status.
	MigrationStatusGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "midaz",
			Subsystem: "migration",
			Name:      "status",
			Help:      "Current migration status (1=healthy, 0=unhealthy)",
		},
		[]string{"component"},
	)

	// MigrationVersionGauge tracks current migration version.
	MigrationVersionGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "midaz",
			Subsystem: "migration",
			Name:      "version",
			Help:      "Current migration version number",
		},
		[]string{"component"},
	)
)

// RecordMigrationDuration records the duration of a migration operation.
func (w *MigrationWrapper) RecordMigrationDuration(operation, status string, durationSeconds float64) {
	MigrationDurationSeconds.WithLabelValues(w.config.Component, operation, status).Observe(durationSeconds)
}

// RecordRecoveryAttempt records a migration recovery attempt.
func (w *MigrationWrapper) RecordRecoveryAttempt(successful bool) {
	status := "success"
	if !successful {
		status = "failure"
	}

	MigrationRecoveryTotal.WithLabelValues(w.config.Component, status).Inc()
}

// RecordLockWait records time spent waiting for advisory lock.
func (w *MigrationWrapper) RecordLockWait(acquired bool, durationSeconds float64) {
	acquiredStr := "true"
	if !acquired {
		acquiredStr = "false"
	}

	MigrationLockWaitSeconds.WithLabelValues(w.config.Component, acquiredStr).Observe(durationSeconds)
}

// UpdateStatusMetrics updates the status gauge metrics.
func (w *MigrationWrapper) UpdateStatusMetrics() {
	status := w.GetStatus()

	healthy := float64(0)
	if status.IsHealthy() {
		healthy = 1
	}

	MigrationStatusGauge.WithLabelValues(w.config.Component).Set(healthy)
	MigrationVersionGauge.WithLabelValues(w.config.Component).Set(float64(status.Version))
}
