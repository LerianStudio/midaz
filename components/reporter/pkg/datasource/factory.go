// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"fmt"
	"strings"

	"github.com/LerianStudio/reporter/pkg"
	"github.com/LerianStudio/reporter/pkg/fetcher"
)

// ProviderConfig holds configuration for selecting and constructing the
// appropriate DataSourceProvider. The factory function NewProvider uses this
// to return either a DirectProvider (legacy, single-tenant) or a
// FetcherProvider (dual-mode, single-tenant or multi-tenant).
type ProviderConfig struct {
	// FetcherEnabled selects FetcherProvider when true, DirectProvider when false.
	FetcherEnabled bool

	// FetcherURL is the base URL of the Fetcher API (required when FetcherEnabled=true).
	FetcherURL string

	// MultiTenantEnabled indicates multi-tenant mode. Requires FetcherEnabled=true
	// because direct mode does not support multi-tenant isolation.
	MultiTenantEnabled bool

	// SafeDataSources is the thread-safe datasource map (used by DirectProvider).
	// May be nil when FetcherEnabled=true.
	SafeDataSources *pkg.SafeDataSources

	// CircuitBreakerManager provides per-datasource circuit breakers (DirectProvider only).
	// Optional — may be nil.
	CircuitBreakerManager *pkg.CircuitBreakerManager

	// HealthChecker provides background health monitoring (DirectProvider only).
	// Optional — may be nil.
	HealthChecker *pkg.HealthChecker

	// M2MTokenProvider provides M2M JWT tokens for inter-service auth.
	// Required for multi-tenant FetcherProvider; nil in single-tenant mode.
	M2MTokenProvider fetcher.M2MTokenProvider

	// FetcherClientOptions allows callers to inject additional FetcherClient
	// functional options (e.g., WithCircuitBreaker, WithHTTPClient).
	FetcherClientOptions []fetcher.FetcherClientOption
}

// NewProvider creates the appropriate DataSourceProvider based on configuration.
// Returns DirectProvider when FetcherEnabled=false, FetcherProvider when true.
//
// Startup validation:
//
//   - FETCHER_ENABLED=true requires FETCHER_URL to be set
//
//   - MULTI_TENANT_ENABLED=true requires FETCHER_ENABLED=true
//
//   - config_runtime.go: ExternalDatasourceConnections() only when FETCHER_ENABLED=false
//
//   - consumer.go: Consumer 2 for extraction callbacks when FETCHER_ENABLED=true
//
//   - Manager and Worker receive provider via bootstrap dependency injection
func NewProvider(cfg ProviderConfig) (DataSourceProvider, error) {
	if err := ValidateProviderConfig(cfg); err != nil {
		return nil, err
	}

	if cfg.FetcherEnabled {
		return buildFetcherProvider(cfg), nil
	}

	return buildDirectProvider(cfg), nil
}

// ValidateProviderConfig checks that the provider configuration is internally
// consistent. Returns a descriptive error if any constraint is violated.
func ValidateProviderConfig(cfg ProviderConfig) error {
	if cfg.FetcherEnabled && strings.TrimSpace(cfg.FetcherURL) == "" {
		return fmt.Errorf("FETCHER_ENABLED=true requires FETCHER_URL to be set")
	}

	if cfg.MultiTenantEnabled && !cfg.FetcherEnabled {
		return fmt.Errorf(
			"MULTI_TENANT_ENABLED=true requires FETCHER_ENABLED=true (Direct mode does not support multi-tenant)",
		)
	}

	return nil
}

// buildDirectProvider constructs a DirectProvider from the configuration.
func buildDirectProvider(cfg ProviderConfig) *DirectProvider {
	return NewDirectProvider(
		cfg.SafeDataSources,
		cfg.CircuitBreakerManager,
		cfg.HealthChecker,
	)
}

// buildFetcherProvider constructs a FetcherProvider from the configuration.
// It creates a FetcherClient with the configured URL and optional M2M auth.
func buildFetcherProvider(cfg ProviderConfig) *FetcherProvider {
	var opts []fetcher.FetcherClientOption

	if cfg.M2MTokenProvider != nil {
		opts = append(opts, fetcher.WithM2MTokenProvider(cfg.M2MTokenProvider))
	}

	opts = append(opts, cfg.FetcherClientOptions...)

	client := fetcher.NewFetcherClient(cfg.FetcherURL, opts...)

	return NewFetcherProvider(client)
}
