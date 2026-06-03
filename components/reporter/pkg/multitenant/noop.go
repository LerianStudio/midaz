// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package multitenant

import (
	"go.opentelemetry.io/otel/metric/noop"
)

// NoopMetrics returns a Metrics instance backed by no-op OTel instruments.
// Use this when MULTI_TENANT_ENABLED=false to avoid any OTel registration
// overhead while keeping the same API surface. All returned instruments are
// safe to call (Add, etc.) without nil checks and incur zero overhead.
func NoopMetrics() *Metrics {
	meter := noop.NewMeterProvider().Meter("noop")

	// noop meter never returns errors, so we can safely ignore them.
	connectionsTotal, _ := meter.Int64Counter("tenant_connections_total")
	connectionErrorsTotal, _ := meter.Int64Counter("tenant_connection_errors_total")
	consumersActive, _ := meter.Int64UpDownCounter("tenant_consumers_active")
	messagesProcessedTotal, _ := meter.Int64Counter("tenant_messages_processed_total")

	return &Metrics{
		TenantConnectionsTotal:       connectionsTotal,
		TenantConnectionErrorsTotal:  connectionErrorsTotal,
		TenantConsumersActive:        consumersActive,
		TenantMessagesProcessedTotal: messagesProcessedTotal,
	}
}
