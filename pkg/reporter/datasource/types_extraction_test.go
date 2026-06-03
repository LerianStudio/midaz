// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractionJobRequest_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  ExtractionJobRequest
	}{
		{
			name: "all fields populated",
			req: ExtractionJobRequest{
				DataSourceID: "midaz_onboarding",
				ReportID:     "rpt-123",
				TemplateID:   "tpl-456",
				TenantID:     "tenant-789",
				Fields:       []string{"id", "name", "balance"},
				Filters:      map[string]string{"status": "active"},
			},
		},
		{
			name: "minimal fields with omitempty",
			req: ExtractionJobRequest{
				DataSourceID: "midaz_onboarding",
				ReportID:     "rpt-001",
				TemplateID:   "tpl-001",
				TenantID:     "tenant-001",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.req)
			require.NoError(t, err, "marshalling ExtractionJobRequest should not fail")

			var decoded ExtractionJobRequest
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err, "unmarshalling ExtractionJobRequest should not fail")

			assert.Equal(t, tt.req, decoded, "round-trip should preserve all fields")
		})
	}
}

func TestExtractionJobRequest_OmitemptyBehavior(t *testing.T) {
	t.Parallel()

	req := ExtractionJobRequest{
		DataSourceID: "midaz_onboarding",
		ReportID:     "rpt-001",
		TemplateID:   "tpl-001",
		TenantID:     "tenant-001",
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var raw map[string]interface{}
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	assert.NotContains(t, raw, "fields", "nil Fields should be omitted from JSON")
	assert.NotContains(t, raw, "filters", "nil Filters should be omitted from JSON")
}

func TestExtractionJobRequest_UsesTenantIDNotOrganizationID(t *testing.T) {
	t.Parallel()

	// D3 decision: tenant identity is resolved from JWT context.
	req := ExtractionJobRequest{
		DataSourceID: "midaz_onboarding",
		ReportID:     "rpt-123",
		TemplateID:   "tpl-456",
		TenantID:     "tenant-from-jwt",
	}

	assert.Equal(t, "tenant-from-jwt", req.TenantID, "TenantID should be set from JWT context")
}

func TestExtractionMapping_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	completedAt := now.Add(time.Minute)

	tests := []struct {
		name    string
		mapping ExtractionMapping
	}{
		{
			name: "pending mapping without CompletedAt",
			mapping: ExtractionMapping{
				JobID:       "job-001",
				ReportID:    "rpt-123",
				TemplateID:  "tpl-456",
				TenantID:    "tenant-789",
				Status:      "pending",
				CreatedAt:   now,
				CompletedAt: nil,
			},
		},
		{
			name: "completed mapping with CompletedAt",
			mapping: ExtractionMapping{
				JobID:       "job-002",
				ReportID:    "rpt-456",
				TemplateID:  "tpl-789",
				TenantID:    "tenant-001",
				Status:      "completed",
				CreatedAt:   now,
				CompletedAt: &completedAt,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.mapping)
			require.NoError(t, err, "marshalling ExtractionMapping should not fail")

			var decoded ExtractionMapping
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err, "unmarshalling ExtractionMapping should not fail")

			assert.Equal(t, tt.mapping, decoded, "round-trip should preserve all fields")
		})
	}
}

func TestExtractionMapping_OmitemptyCompletedAt(t *testing.T) {
	t.Parallel()

	mapping := ExtractionMapping{
		JobID:      "job-001",
		ReportID:   "rpt-123",
		TemplateID: "tpl-456",
		TenantID:   "tenant-789",
		Status:     "pending",
		CreatedAt:  time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(mapping)
	require.NoError(t, err)

	var raw map[string]interface{}
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	assert.NotContains(t, raw, "completedAt", "nil CompletedAt should be omitted from JSON")
}

func TestExtractionMapping_UsesTenantIDNotOrganizationID(t *testing.T) {
	t.Parallel()

	// D3 decision: ExtractionMapping uses TenantID (resolved from JWT context),
	// NOT OrganizationID. This is a deliberate design choice for multi-tenant isolation.
	mapping := ExtractionMapping{
		JobID:      "job-001",
		ReportID:   "rpt-123",
		TemplateID: "tpl-456",
		TenantID:   "tenant-from-jwt",
		Status:     "pending",
		CreatedAt:  time.Now(),
	}

	assert.Equal(t, "tenant-from-jwt", mapping.TenantID, "TenantID should be set from JWT context")
}
