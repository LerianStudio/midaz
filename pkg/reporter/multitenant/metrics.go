// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package multitenant

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

// Metrics holds the four canonical multi-tenant OTel instruments.
// All fields are guaranteed non-nil after construction via NewMetrics or NoopMetrics.
// Callers can record values without nil checks regardless of whether multi-tenant
// mode is enabled.
type Metrics struct {
	// TenantConnectionsTotal counts the total number of tenant database connections created.
	TenantConnectionsTotal metric.Int64Counter

	// TenantConnectionErrorsTotal counts connection failures segmented by tenant identifier.
	TenantConnectionErrorsTotal metric.Int64Counter

	// TenantConsumersActive tracks the current number of active message consumers.
	// Uses UpDownCounter because the value can both increase and decrease.
	TenantConsumersActive metric.Int64UpDownCounter

	// TenantMessagesProcessedTotal counts messages processed, broken down by tenant.
	TenantMessagesProcessedTotal metric.Int64Counter
}

// NewMetrics creates a Metrics instance with real OTel instruments registered on the
// provided meter. Use this when MULTI_TENANT_ENABLED=true so that metrics are exported
// to the configured OTel collector.
func NewMetrics(meter metric.Meter) (*Metrics, error) {
	connectionsTotal, err := meter.Int64Counter(
		"tenant_connections_total",
		metric.WithDescription("Total tenant database connections created"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create tenant_connections_total counter: %w", err)
	}

	connectionErrorsTotal, err := meter.Int64Counter(
		"tenant_connection_errors_total",
		metric.WithDescription("Connection failures per tenant"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create tenant_connection_errors_total counter: %w", err)
	}

	consumersActive, err := meter.Int64UpDownCounter(
		"tenant_consumers_active",
		metric.WithDescription("Active message consumers per tenant"),
		metric.WithUnit("{consumer}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create tenant_consumers_active up_down_counter: %w", err)
	}

	messagesProcessedTotal, err := meter.Int64Counter(
		"tenant_messages_processed_total",
		metric.WithDescription("Messages processed per tenant"),
		metric.WithUnit("{message}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create tenant_messages_processed_total counter: %w", err)
	}

	return &Metrics{
		TenantConnectionsTotal:       connectionsTotal,
		TenantConnectionErrorsTotal:  connectionErrorsTotal,
		TenantConsumersActive:        consumersActive,
		TenantMessagesProcessedTotal: messagesProcessedTotal,
	}, nil
}
