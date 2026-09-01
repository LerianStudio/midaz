// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// resetTenantCapRetryAfterForTest clears the cached value so a subsequent call
// to tenantCapRetryAfterSeconds re-reads the environment. Test-only — kept in
// the _test.go file so the production binary never carries this entry point
// (lint enforces the boundary via unused-symbol detection).
func resetTenantCapRetryAfterForTest() {
	tenantCapRetryAfterOnce = sync.Once{}
	tenantCapRetryAfterVal = 0
}

// TestTenantCapRetryAfterSeconds_Default verifies that, with no env override,
// the helper returns the documented default. The default is the contract for
// operators who deploy without TENANT_CAP_RETRY_AFTER_SECONDS — changing the
// number requires deliberately updating the docs and bumping the constant.
//
// Cannot run with t.Parallel() because the helpers it exercises mutate
// process-global state (sync.Once + cached value + os.Setenv). Other tests in
// the package that depend on the cached value must use t.Setenv before this
// runs to avoid order-dependent flakiness.
func TestTenantCapRetryAfterSeconds_Default(t *testing.T) {
	resetTenantCapRetryAfterForTest()
	t.Cleanup(resetTenantCapRetryAfterForTest)

	// Explicitly clear the env var so this test does not pick up the
	// developer's local override. t.Setenv with empty restores correctly on
	// cleanup.
	t.Setenv(tenantCapRetryAfterEnvVar, "")

	got := tenantCapRetryAfterSeconds()
	assert.Equal(t, defaultTenantCapRetryAfterSeconds, got,
		"unset env var must yield the documented default")

	assert.Equal(t, strconv.Itoa(defaultTenantCapRetryAfterSeconds),
		tenantCapRetryAfterHeader(),
		"header helper must format the default cleanly without padding")
}

// TestTenantCapRetryAfterSeconds_Override exercises every path that should
// either honour or reject the operator override. The matrix covers the
// happy path (a positive integer) plus the three failure modes the parser
// must absorb without panicking: non-numeric, zero, negative.
func TestTenantCapRetryAfterSeconds_Override(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     int
		header   string
	}{
		{
			name:     "Success - positive integer override",
			envValue: "12",
			want:     12,
			header:   "12",
		},
		{
			name:     "Success - large value preserved",
			envValue: "120",
			want:     120,
			header:   "120",
		},
		{
			name:     "Edge case - non-numeric falls back to default",
			envValue: "not-a-number",
			want:     defaultTenantCapRetryAfterSeconds,
			header:   strconv.Itoa(defaultTenantCapRetryAfterSeconds),
		},
		{
			name:     "Edge case - zero falls back to default (must be positive)",
			envValue: "0",
			want:     defaultTenantCapRetryAfterSeconds,
			header:   strconv.Itoa(defaultTenantCapRetryAfterSeconds),
		},
		{
			name:     "Edge case - negative falls back to default",
			envValue: "-1",
			want:     defaultTenantCapRetryAfterSeconds,
			header:   strconv.Itoa(defaultTenantCapRetryAfterSeconds),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetTenantCapRetryAfterForTest()
			t.Cleanup(resetTenantCapRetryAfterForTest)
			t.Setenv(tenantCapRetryAfterEnvVar, tt.envValue)

			got := tenantCapRetryAfterSeconds()
			assert.Equal(t, tt.want, got,
				"env value %q should yield Retry-After=%d", tt.envValue, tt.want)

			assert.Equal(t, tt.header, tenantCapRetryAfterHeader(),
				"header helper output should match formatted Retry-After")
		})
	}
}
