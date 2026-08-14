// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"os"
	"strconv"
	"sync"
)

// defaultTenantCapRetryAfterSeconds is the fallback Retry-After value (seconds)
// applied when TENANT_CAP_RETRY_AFTER_SECONDS is unset, non-numeric, or
// non-positive. The default is intentionally short: supervisor pressure is
// usually cleared by an in-flight tenant rotating out within tens of seconds;
// longer back-offs would unnecessarily stall callers once the cap relaxes.
const defaultTenantCapRetryAfterSeconds = 5

// tenantCapRetryAfterEnvVar is the environment variable operators can set to
// override defaultTenantCapRetryAfterSeconds without rebuilding the binary.
// Value is parsed as a positive integer (seconds); invalid values fall back to
// the default and the misconfiguration is silently ignored to keep startup
// resilient on misconfigured non-production envs.
const tenantCapRetryAfterEnvVar = "TENANT_CAP_RETRY_AFTER_SECONDS"

var (
	// tenantCapRetryAfterOnce guards the one-shot read of
	// TENANT_CAP_RETRY_AFTER_SECONDS so concurrent first-callers do not race
	// each other on env-var parsing. The value is treated as immutable for
	// the lifetime of the process — operators change it via a pod restart,
	// matching the rest of the env-var-driven configuration in tracer.
	tenantCapRetryAfterOnce sync.Once
	tenantCapRetryAfterVal  int
)

// tenantCapRetryAfterSeconds returns the integer Retry-After value (in seconds)
// the per-tenant supervisor middleware emits on its 503 response when the
// tenant cap is reached. Reads TENANT_CAP_RETRY_AFTER_SECONDS once and caches
// the result.
func tenantCapRetryAfterSeconds() int {
	tenantCapRetryAfterOnce.Do(loadTenantCapRetryAfter)

	return tenantCapRetryAfterVal
}

// tenantCapRetryAfterHeader returns the Retry-After value formatted as the
// string ready to drop into c.Set("Retry-After", ...). Centralised so callers
// never deal with strconv themselves.
func tenantCapRetryAfterHeader() string {
	return strconv.Itoa(tenantCapRetryAfterSeconds())
}

// loadTenantCapRetryAfter parses TENANT_CAP_RETRY_AFTER_SECONDS once and
// stores the result. Exposed at package scope (rather than inlined in the
// sync.Once closure) so the test helper can drive it deterministically from
// runtime_env_test.go.
func loadTenantCapRetryAfter() {
	raw := os.Getenv(tenantCapRetryAfterEnvVar)
	if raw == "" {
		tenantCapRetryAfterVal = defaultTenantCapRetryAfterSeconds

		return
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		tenantCapRetryAfterVal = defaultTenantCapRetryAfterSeconds

		return
	}

	tenantCapRetryAfterVal = parsed
}
