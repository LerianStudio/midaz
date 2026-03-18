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
)
