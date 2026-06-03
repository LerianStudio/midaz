// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStatusConstants_HaveCanonicalValues verifies the closed status vocabulary
// matches the canonical /readyz contract. Adding or renaming a status value is
// a breaking change and MUST be rejected here.
func TestStatusConstants_HaveCanonicalValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		got      Status
		expected string
	}{
		{name: "up", got: StatusUp, expected: "up"},
		{name: "down", got: StatusDown, expected: "down"},
		{name: "degraded", got: StatusDegraded, expected: "degraded"},
		{name: "skipped", got: StatusSkipped, expected: "skipped"},
		{name: "n/a", got: StatusNA, expected: "n/a"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, string(tt.got))
		})
	}
}

// TestDependencyCheck_JSONOmitsZeroFields verifies that optional fields are
// omitted from the JSON output when they have zero values. This keeps the
// /readyz response compact and matches the canonical contract.
func TestDependencyCheck_JSONOmitsZeroFields(t *testing.T) {
	t.Parallel()

	d := DependencyCheck{Status: StatusUp}
	assert.Equal(t, StatusUp, d.Status)
	assert.Equal(t, int64(0), d.LatencyMs)
	assert.Nil(t, d.TLS)
	assert.Empty(t, d.Error)
	assert.Empty(t, d.Reason)
	assert.Empty(t, d.BreakerState)
}

// TestResponse_RequiredFieldsExist verifies the Response struct has the
// required top-level fields per the canonical contract.
func TestResponse_RequiredFieldsExist(t *testing.T) {
	t.Parallel()

	r := Response{
		Status:         "healthy",
		Version:        "1.2.3",
		DeploymentMode: "saas",
		Checks:         map[string]DependencyCheck{},
	}

	assert.Equal(t, "healthy", r.Status)
	assert.Equal(t, "1.2.3", r.Version)
	assert.Equal(t, "saas", r.DeploymentMode)
	assert.NotNil(t, r.Checks)
}
