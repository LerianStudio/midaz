// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry/metrics"

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
)
