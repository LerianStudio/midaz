// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"testing"

	"github.com/LerianStudio/lib-commons/v5/commons/opentelemetry/metrics"
	"github.com/stretchr/testify/assert"
)

// TestCRMProtectionMetricsDeclarations asserts each CRM protection metric
// declaration exposes the expected Name and Unit. The metric type
// (Counter vs Histogram) is enforced at the emit site, not here, so this
// test only validates the metrics.Metric metadata (Name/Unit).
func TestCRMProtectionMetricsDeclarations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metric   metrics.Metric
		wantName string
		wantUnit string
	}{
		{
			name:     "CRMProtectionModeResolutionTotal",
			metric:   CRMProtectionModeResolutionTotal,
			wantName: "crm_protection_mode_resolution_total",
			wantUnit: "1",
		},
		{
			name:     "CRMProtectionStatusTotal",
			metric:   CRMProtectionStatusTotal,
			wantName: "crm_protection_status_total",
			wantUnit: "1",
		},
		{
			name:     "CRMProtectionEncryptDecryptTotal",
			metric:   CRMProtectionEncryptDecryptTotal,
			wantName: "crm_protection_encrypt_decrypt_total",
			wantUnit: "1",
		},
		{
			name:     "CRMProtectionProviderOperationMs",
			metric:   CRMProtectionProviderOperationMs,
			wantName: "crm_protection_provider_operation_ms",
			wantUnit: "ms",
		},
		{
			name:     "CRMProtectionProviderOperationFailuresTotal",
			metric:   CRMProtectionProviderOperationFailuresTotal,
			wantName: "crm_protection_provider_operation_failures_total",
			wantUnit: "1",
		},
		{
			name:     "CRMProtectionRegistryConflictTotal",
			metric:   CRMProtectionRegistryConflictTotal,
			wantName: "crm_protection_registry_conflict_total",
			wantUnit: "1",
		},
		{
			name:     "CRMProtectionLegacyReadTotal",
			metric:   CRMProtectionLegacyReadTotal,
			wantName: "crm_protection_legacy_read_total",
			wantUnit: "1",
		},
		{
			name:     "CRMProtectionCacheTotal",
			metric:   CRMProtectionCacheTotal,
			wantName: "crm_protection_cache_total",
			wantUnit: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.wantName, tt.metric.Name, "unexpected metric Name")
			assert.Equal(t, tt.wantUnit, tt.metric.Unit, "unexpected metric Unit")
			assert.NotEmpty(t, tt.metric.Description, "metric Description must not be empty")
		})
	}
}

// TestCRMProtectionMetricNamesUnique asserts the eight CRM protection metric
// names are all distinct, preventing accidental registry collisions.
func TestCRMProtectionMetricNamesUnique(t *testing.T) {
	t.Parallel()

	names := []string{
		CRMProtectionModeResolutionTotal.Name,
		CRMProtectionStatusTotal.Name,
		CRMProtectionEncryptDecryptTotal.Name,
		CRMProtectionProviderOperationMs.Name,
		CRMProtectionProviderOperationFailuresTotal.Name,
		CRMProtectionRegistryConflictTotal.Name,
		CRMProtectionLegacyReadTotal.Name,
		CRMProtectionCacheTotal.Name,
	}

	seen := make(map[string]bool, len(names))
	for _, n := range names {
		assert.False(t, seen[n], "duplicate metric name: %s", n)
		seen[n] = true
	}

	assert.Len(t, seen, 8, "expected 8 unique CRM protection metric names")
}
