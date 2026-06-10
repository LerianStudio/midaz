// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
)

// ProviderConfig holds configuration for constructing a single-tenant
// DataSourceProvider. The remote Fetcher path has been retired: schema
// discovery and validation always run in-process. Multi-tenant deployments use
// NewMultiTenantDirectProvider directly (it requires the lib-commons tenant
// managers, which the bootstrap supplies), so this config covers the
// single-tenant DirectProvider only.
type ProviderConfig struct {
	// SafeDataSources is the thread-safe datasource registry built from env
	// configuration. It supplies datasource IDs, types, schema lists, and
	// CRM/org-scope configuration, plus the lazily-connected env pools used as
	// the schema source in single-tenant mode.
	SafeDataSources *pkg.SafeDataSources

	// CircuitBreakerManager provides per-datasource circuit breakers.
	// Optional — may be nil.
	CircuitBreakerManager *pkg.CircuitBreakerManager

	// HealthChecker provides background health monitoring.
	// Optional — may be nil.
	HealthChecker *pkg.HealthChecker
}

// NewProvider creates a single-tenant in-process DirectProvider. Multi-tenant
// callers use NewMultiTenantDirectProvider instead, since per-tenant schema
// resolution requires the lib-commons tenant managers.
func NewProvider(cfg ProviderConfig) (DataSourceProvider, error) {
	return NewDirectProvider(
		cfg.SafeDataSources,
		cfg.CircuitBreakerManager,
		cfg.HealthChecker,
	), nil
}
