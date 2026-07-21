// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package services provides core business services for the Tracer application.
package services

import (
	libMetrics "github.com/LerianStudio/lib-observability/metrics"
)

// Metric is an alias for libMetrics.Metric to allow local usage without importing.
type Metric = libMetrics.Metric

// MetricAuditPersistFailures tracks audit record persistence failures.
// Name follows TRD Section 9.3 convention with tracer_ prefix.
// This metric signals when transaction validation audit records could not be
// persisted to the database. Non-zero values indicate potential compliance gaps
// (SOX/GLBA audit trail requirements) that require investigation.
// Labels: none (request_id available in logs/spans for investigation)
var MetricAuditPersistFailures = Metric{
	Name:        "tracer_audit_persist_failures_total",
	Unit:        "1",
	Description: "Total audit record persistence failures (compliance risk)",
}

// MetricValidationRollbackFailures tracks usage rollback failures during REVIEW decisions.
// Name follows TRD Section 9.3 convention with tracer_ prefix.
// This metric signals when usage counters could not be rolled back after a REVIEW decision.
// Non-zero values indicate eventual consistency gaps that will self-correct at period boundaries.
// Labels: none (request_id available in logs/spans for investigation)
var MetricValidationRollbackFailures = Metric{
	Name:        "tracer_validation_rollback_failures_total",
	Unit:        "1",
	Description: "Total usage rollback failures for REVIEW decisions",
}
