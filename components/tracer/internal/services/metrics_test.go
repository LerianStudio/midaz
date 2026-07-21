// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMetricAuditPersistFailures_Definition verifies the metric is properly defined.
func TestMetricAuditPersistFailures_Definition(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "tracer_audit_persist_failures_total", MetricAuditPersistFailures.Name,
		"metric name should follow tracer_ prefix convention")
	assert.Equal(t, "1", MetricAuditPersistFailures.Unit,
		"unit should be '1' for counters")
	assert.NotEmpty(t, MetricAuditPersistFailures.Description,
		"description should be non-empty for documentation")
	assert.Contains(t, MetricAuditPersistFailures.Description, "audit",
		"description should mention audit context")
}

// TestMetricValidationRollbackFailures_Definition verifies the metric is properly defined.
func TestMetricValidationRollbackFailures_Definition(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "tracer_validation_rollback_failures_total", MetricValidationRollbackFailures.Name,
		"metric name should follow tracer_ prefix convention")
	assert.Equal(t, "1", MetricValidationRollbackFailures.Unit,
		"unit should be '1' for counters")
	assert.NotEmpty(t, MetricValidationRollbackFailures.Description,
		"description should be non-empty for documentation")
	assert.Contains(t, MetricValidationRollbackFailures.Description, "rollback",
		"description should mention rollback context")
}
