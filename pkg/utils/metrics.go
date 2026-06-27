// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"context"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/lib-observability/metrics"
	"github.com/LerianStudio/midaz/v4/pkg"
)

var (
	// DomainOperationsTotal counts business-operation outcomes across all
	// components (D6 mandate). Labels: component, operation, result — all
	// bounded sets (result is success|business_error|technical_error).
	DomainOperationsTotal = metrics.Metric{
		Name:        "domain_operations_total",
		Unit:        "1",
		Description: "Count of business operations by component, operation and result.",
	}

	// DomainOperationDuration tracks business-operation latency. Labels:
	// component, operation.
	DomainOperationDuration = metrics.Metric{
		Name:        "domain_operation_duration_ms",
		Unit:        "ms",
		Description: "Business operation duration in milliseconds by component and operation.",
	}
)

// RecordDomainOperation emits the D6 domain metrics for one business-operation
// completion. Call it at the single exit boundary of a use case (typically via
// defer with a named error). A nil factory is a no-op so single binaries can
// run with metrics disabled; emit failures log at Debug per T11 and never
// affect the operation.
func RecordDomainOperation(ctx context.Context, factory *metrics.MetricsFactory, logger libLog.Logger, component, operation string, start time.Time, err error) {
	if factory == nil {
		return
	}

	result := "success"

	switch {
	case err == nil:
	case pkg.IsBusinessError(err):
		result = "business_error"
	default:
		result = "technical_error"
	}

	counter, cErr := factory.Counter(DomainOperationsTotal)
	if cErr == nil {
		cErr = counter.WithLabels(map[string]string{
			"component": component,
			"operation": operation,
			"result":    result,
		}).Add(ctx, 1)
	}

	histogram, hErr := factory.Histogram(DomainOperationDuration)
	if hErr == nil {
		hErr = histogram.WithLabels(map[string]string{
			"component": component,
			"operation": operation,
		}).Record(ctx, time.Since(start).Milliseconds())
	}

	if logger != nil {
		if cErr != nil {
			logger.Log(ctx, libLog.LevelDebug, "Failed to emit domain operation counter", libLog.Err(cErr))
		}

		if hErr != nil {
			logger.Log(ctx, libLog.LevelDebug, "Failed to emit domain operation histogram", libLog.Err(hErr))
		}
	}
}

var (
	BalanceSynced = metrics.Metric{
		Name:        "balance_synced",
		Unit:        "1",
		Description: "Measures the number of balances synced.",
	}

	// BalanceSyncBatchFailures counts batch sync operation failures.
	BalanceSyncBatchFailures = metrics.Metric{
		Name:        "balance_sync_batch_failures_total",
		Unit:        "1",
		Description: "Total batch sync operation failures.",
	}

	// BalanceSyncCleanupFailures counts schedule cleanup failures after successful DB sync.
	BalanceSyncCleanupFailures = metrics.Metric{
		Name:        "balance_sync_cleanup_failures_total",
		Unit:        "1",
		Description: "Total schedule cleanup failures after successful balance sync.",
	}

	// BalanceSyncTenantSkip counts tenants skipped by the balance sync worker when
	// their PostgreSQL connection cannot be resolved for a cycle.
	BalanceSyncTenantSkip = metrics.Metric{
		Name:        "balance_sync_tenant_skip_total",
		Unit:        "1",
		Description: "Total tenants skipped by the balance sync worker due to connection resolution failure.",
	}

	// Redis backup-queue (poison record) observability metrics.

	// RedisBackupQueueDepth is the number of records currently in the backup queue.
	RedisBackupQueueDepth = metrics.Metric{
		Name:        "redis_backup_queue_depth",
		Unit:        "1",
		Description: "Number of records currently in the Redis transaction backup queue.",
	}

	// RedisBackupQueueOldestAgeSeconds is the age of the oldest record in the backup queue.
	RedisBackupQueueOldestAgeSeconds = metrics.Metric{
		Name:        "redis_backup_queue_oldest_age_seconds",
		Unit:        "s",
		Description: "Age in seconds of the oldest record in the Redis transaction backup queue.",
	}

	// RedisBackupQuarantineTotal counts poison records moved to the durable quarantine table.
	RedisBackupQuarantineTotal = metrics.Metric{
		Name:        "redis_backup_quarantine_total",
		Unit:        "1",
		Description: "Total poison backup records moved to the Postgres quarantine table.",
	}

	// RedisBackupReplayRecomputedBalancesAfterTotal counts backup-replay records
	// whose operation audit rows were rebuilt without Lua's authoritative
	// after-balances, so overdraft transactions may carry naive before-amount
	// arithmetic (audit divergence tracked under T-006.1 / T-009).
	RedisBackupReplayRecomputedBalancesAfterTotal = metrics.Metric{
		Name:        "redis_backup_replay_recomputed_balances_after_total",
		Unit:        "1",
		Description: "Total backup-replay records whose after-balances were recomputed rather than replayed from Lua, risking overdraft audit divergence.",
	}

	// CircuitBreakerState indicates the current state of the RabbitMQ circuit breaker.
	// Values: 0 = closed (healthy), 1 = open (unhealthy), 2 = half-open (recovering)
	CircuitBreakerState = metrics.Metric{
		Name:        "circuit_breaker_state",
		Unit:        "1",
		Description: "Current state of the circuit breaker (0=closed, 1=open, 2=half-open).",
	}

	// Multi-tenant metrics (canonical per Ring Standards multi-tenant.md)

	// TenantConnectionsTotal counts total tenant connections created.
	TenantConnectionsTotal = metrics.Metric{
		Name:        "tenant_connections_total",
		Unit:        "1",
		Description: "Total tenant database connections created.",
	}

	// TenantConnectionErrorsTotal counts connection failures per tenant.
	TenantConnectionErrorsTotal = metrics.Metric{
		Name:        "tenant_connection_errors_total",
		Unit:        "1",
		Description: "Total tenant database connection failures.",
	}

	// TenantConsumersActive tracks active message consumers.
	TenantConsumersActive = metrics.Metric{
		Name:        "tenant_consumers_active",
		Unit:        "1",
		Description: "Number of active tenant message consumers.",
	}

	// TenantMessagesProcessedTotal counts messages processed per tenant.
	TenantMessagesProcessedTotal = metrics.Metric{
		Name:        "tenant_messages_processed_total",
		Unit:        "1",
		Description: "Total messages processed per tenant.",
	}

	// Bulk Recorder metrics for transaction layer bulk insert operations

	// BulkRecorderTransactionsAttempted counts transactions sent to bulk INSERT.
	BulkRecorderTransactionsAttempted = metrics.Metric{
		Name:        "bulk_recorder_transactions_attempted_total",
		Unit:        "1",
		Description: "Total transactions sent to bulk INSERT.",
	}

	// BulkRecorderTransactionsInserted counts transactions actually inserted.
	BulkRecorderTransactionsInserted = metrics.Metric{
		Name:        "bulk_recorder_transactions_inserted_total",
		Unit:        "1",
		Description: "Total transactions actually inserted (excluding duplicates).",
	}

	// BulkRecorderTransactionsIgnored counts transactions skipped due to duplicates.
	BulkRecorderTransactionsIgnored = metrics.Metric{
		Name:        "bulk_recorder_transactions_ignored_total",
		Unit:        "1",
		Description: "Total transactions skipped (duplicates via ON CONFLICT DO NOTHING).",
	}

	// BulkRecorderOperationsAttempted counts operations sent to bulk INSERT.
	BulkRecorderOperationsAttempted = metrics.Metric{
		Name:        "bulk_recorder_operations_attempted_total",
		Unit:        "1",
		Description: "Total operations sent to bulk INSERT.",
	}

	// BulkRecorderOperationsInserted counts operations actually inserted.
	BulkRecorderOperationsInserted = metrics.Metric{
		Name:        "bulk_recorder_operations_inserted_total",
		Unit:        "1",
		Description: "Total operations actually inserted (excluding duplicates).",
	}

	// BulkRecorderOperationsIgnored counts operations skipped due to duplicates.
	BulkRecorderOperationsIgnored = metrics.Metric{
		Name:        "bulk_recorder_operations_ignored_total",
		Unit:        "1",
		Description: "Total operations skipped (duplicates via ON CONFLICT DO NOTHING).",
	}

	// BulkRecorderBulkSize tracks the number of messages per bulk.
	BulkRecorderBulkSize = metrics.Metric{
		Name:        "bulk_recorder_bulk_size",
		Unit:        "1",
		Description: "Number of messages per bulk processing batch.",
	}

	// BulkRecorderBulkDuration tracks the time taken for each bulk processing.
	BulkRecorderBulkDuration = metrics.Metric{
		Name:        "bulk_recorder_bulk_duration_ms",
		Unit:        "ms",
		Description: "Time taken for bulk processing in milliseconds.",
	}

	// BulkRecorderFallbackTotal counts fallback activations when bulk fails.
	BulkRecorderFallbackTotal = metrics.Metric{
		Name:        "bulk_recorder_fallback_total",
		Unit:        "1",
		Description: "Total fallback activations when bulk processing fails.",
	}

	// Readyz endpoint metrics

	// ReadyzCheckDuration tracks the duration of individual health check probes.
	ReadyzCheckDuration = metrics.Metric{
		Name:        "readyz_check_duration_ms",
		Unit:        "ms",
		Description: "Duration of individual health check probes in milliseconds.",
	}

	// ReadyzCheckStatus counts health check outcomes by checker and status.
	ReadyzCheckStatus = metrics.Metric{
		Name:        "readyz_check_status_total",
		Unit:        "1",
		Description: "Count of health check outcomes by checker and status.",
	}

	// ReadyzRequestsTotal counts total readyz endpoint requests.
	ReadyzRequestsTotal = metrics.Metric{
		Name:        "readyz_requests_total",
		Unit:        "1",
		Description: "Total number of readyz endpoint requests.",
	}

	// CRM protection metrics for field encryption/decryption observability.
	// The metric type (Counter vs Histogram) is enforced at the emit site;
	// metrics.Metric only carries Name/Unit/Description here.

	// CRMProtectionModeResolutionTotal counts protection mode resolutions.
	// Counter. Label: mode (legacy/envelope).
	CRMProtectionModeResolutionTotal = metrics.Metric{
		Name:        "crm_protection_mode_resolution_total",
		Unit:        "1",
		Description: "Total protection mode resolutions by mode (legacy/envelope).",
	}

	// CRMProtectionStatusTotal counts protection status outcomes.
	// Counter. Label: status.
	CRMProtectionStatusTotal = metrics.Metric{
		Name:        "crm_protection_status_total",
		Unit:        "1",
		Description: "Total protection status outcomes by status.",
	}

	// CRMProtectionEncryptDecryptTotal counts encrypt/decrypt operations.
	// Counter. Labels: path (legacy/envelope), outcome (success/failure),
	// error_type (empty on success).
	CRMProtectionEncryptDecryptTotal = metrics.Metric{
		Name:        "crm_protection_encrypt_decrypt_total",
		Unit:        "1",
		Description: "Total encrypt/decrypt operations by path, outcome, and error_type.",
	}

	// CRMProtectionProviderOperationMs measures provider operation duration.
	// Histogram (milliseconds). Labels: operation (wrap/unwrap), provider.
	// Milliseconds are used because lib-commons v5 only exposes Int64Histogram,
	// so sub-second KMS latencies (10-200ms) would truncate to zero in seconds;
	// this mirrors the ReadyzCheckDuration precedent above.
	CRMProtectionProviderOperationMs = metrics.Metric{
		Name:        "crm_protection_provider_operation_ms",
		Unit:        "ms",
		Description: "Duration of provider wrap/unwrap operations in milliseconds by operation and provider.",
	}

	// CRMProtectionProviderOperationFailuresTotal counts provider operation failures.
	// Counter. Labels: operation, error_code.
	CRMProtectionProviderOperationFailuresTotal = metrics.Metric{
		Name:        "crm_protection_provider_operation_failures_total",
		Unit:        "1",
		Description: "Total provider operation failures by operation and error_code.",
	}

	// CRMProtectionRegistryConflictTotal counts protection registry conflicts.
	// Counter. No labels. DEFERRED-emit: declared so the catalog is complete, but
	// no reachable emit site exists yet (same class as the rotation-guard metric);
	// wire it when the registry Update conflict path lands.
	CRMProtectionRegistryConflictTotal = metrics.Metric{
		Name:        "crm_protection_registry_conflict_total",
		Unit:        "1",
		Description: "Total protection registry conflicts.",
	}

	// CRMProtectionLegacyReadTotal counts legacy-path reads.
	// Counter. Label: organization_status.
	CRMProtectionLegacyReadTotal = metrics.Metric{
		Name:        "crm_protection_legacy_read_total",
		Unit:        "1",
		Description: "Total legacy-path reads by organization_status.",
	}

	// CRMProtectionCacheTotal counts protection cache lookups.
	// Counter. Labels: operation, result (hit/miss).
	CRMProtectionCacheTotal = metrics.Metric{
		Name:        "crm_protection_cache_total",
		Unit:        "1",
		Description: "Total protection cache lookups by operation and result (hit/miss).",
	}
)
