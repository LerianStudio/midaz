// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAggregate_CanonicalRule verifies the /readyz aggregation rule:
//
//	"Top-level status is 'healthy' if and only if every check has status
//	in {up, skipped, n/a}. ANY check with status 'down' or 'degraded' MUST
//	yield top-level 'unhealthy' and HTTP 503."
//
// This test is the contract enforcement point. If this test passes, the
// rule is correctly implemented. If it fails, the implementation is wrong.
func TestAggregate_CanonicalRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		checks         map[string]DependencyCheck
		expectedStatus string
		expectedCode   int
	}{
		{
			name:           "empty map yields healthy 200",
			checks:         map[string]DependencyCheck{},
			expectedStatus: "healthy",
			expectedCode:   http.StatusOK,
		},
		{
			name: "all up yields healthy 200",
			checks: map[string]DependencyCheck{
				"mongo":    {Status: StatusUp},
				"rabbitmq": {Status: StatusUp},
				"redis":    {Status: StatusUp},
			},
			expectedStatus: "healthy",
			expectedCode:   http.StatusOK,
		},
		{
			name: "mix of up + skipped yields healthy 200",
			checks: map[string]DependencyCheck{
				"mongo":    {Status: StatusUp},
				"rabbitmq": {Status: StatusSkipped, Reason: "disabled"},
			},
			expectedStatus: "healthy",
			expectedCode:   http.StatusOK,
		},
		{
			name: "mix of up + n/a yields healthy 200",
			checks: map[string]DependencyCheck{
				"mongo":    {Status: StatusUp},
				"rabbitmq": {Status: StatusNA, Reason: "multi-tenant"},
			},
			expectedStatus: "healthy",
			expectedCode:   http.StatusOK,
		},
		{
			name: "mix of skipped + n/a yields healthy 200",
			checks: map[string]DependencyCheck{
				"redis":    {Status: StatusSkipped, Reason: "MULTI_TENANT_ENABLED=false"},
				"mongo":    {Status: StatusNA, Reason: "multi-tenant"},
				"rabbitmq": {Status: StatusNA, Reason: "multi-tenant"},
			},
			expectedStatus: "healthy",
			expectedCode:   http.StatusOK,
		},
		{
			name: "any down yields unhealthy 503",
			checks: map[string]DependencyCheck{
				"mongo":    {Status: StatusUp},
				"rabbitmq": {Status: StatusDown, Error: "connection refused"},
			},
			expectedStatus: "unhealthy",
			expectedCode:   http.StatusServiceUnavailable,
		},
		{
			name: "any degraded yields unhealthy 503",
			checks: map[string]DependencyCheck{
				"mongo": {Status: StatusUp},
				"redis": {Status: StatusDegraded, BreakerState: "half-open"},
			},
			expectedStatus: "unhealthy",
			expectedCode:   http.StatusServiceUnavailable,
		},
		{
			name: "down + degraded both yield unhealthy 503",
			checks: map[string]DependencyCheck{
				"mongo":    {Status: StatusDown, Error: "down"},
				"rabbitmq": {Status: StatusDegraded},
			},
			expectedStatus: "unhealthy",
			expectedCode:   http.StatusServiceUnavailable,
		},
		{
			name: "all down yields unhealthy 503",
			checks: map[string]DependencyCheck{
				"mongo":    {Status: StatusDown},
				"rabbitmq": {Status: StatusDown},
				"redis":    {Status: StatusDown},
			},
			expectedStatus: "unhealthy",
			expectedCode:   http.StatusServiceUnavailable,
		},
		{
			name: "single up yields healthy 200",
			checks: map[string]DependencyCheck{
				"mongo": {Status: StatusUp},
			},
			expectedStatus: "healthy",
			expectedCode:   http.StatusOK,
		},
		{
			name: "single down yields unhealthy 503",
			checks: map[string]DependencyCheck{
				"mongo": {Status: StatusDown},
			},
			expectedStatus: "unhealthy",
			expectedCode:   http.StatusServiceUnavailable,
		},
		// Fix-closed cases (Dispatch 2 LOW-2 promoted): any status not in
		// the closed healthy set {up, skipped, n/a} MUST yield unhealthy
		// 503. Previously the aggregator only checked for {down, degraded}
		// explicitly, so typos like "OK" or future hypothetical values
		// silently reported healthy. The new rule fails closed: an
		// unrecognized status is treated as unhealthy so the bug is
		// surfaced rather than hidden.
		{
			name: "typo OK status yields unhealthy 503",
			checks: map[string]DependencyCheck{
				"mongo": {Status: Status("OK")},
			},
			expectedStatus: "unhealthy",
			expectedCode:   http.StatusServiceUnavailable,
		},
		{
			name: "empty status yields unhealthy 503",
			checks: map[string]DependencyCheck{
				"mongo": {Status: Status("")},
			},
			expectedStatus: "unhealthy",
			expectedCode:   http.StatusServiceUnavailable,
		},
		{
			name: "future hypothetical status yields unhealthy 503",
			checks: map[string]DependencyCheck{
				"mongo": {Status: Status("degraded-recovering")},
			},
			expectedStatus: "unhealthy",
			expectedCode:   http.StatusServiceUnavailable,
		},
		{
			name: "mix of up + unknown still yields unhealthy 503",
			checks: map[string]DependencyCheck{
				"mongo":    {Status: StatusUp},
				"rabbitmq": {Status: Status("unknown-state")},
			},
			expectedStatus: "unhealthy",
			expectedCode:   http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotStatus, gotCode := Aggregate(tt.checks)
			assert.Equal(t, tt.expectedStatus, gotStatus)
			assert.Equal(t, tt.expectedCode, gotCode)
		})
	}
}
