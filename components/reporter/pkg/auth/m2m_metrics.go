// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package auth

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// M2MMetrics holds the mandatory M2M OTel instruments for credential caching
// and token-exchange safety signals.
type M2MMetrics struct {
	L1CacheHits         metric.Int64Counter
	L2CacheHits         metric.Int64Counter
	CacheMisses         metric.Int64Counter
	FetchErrors         metric.Int64Counter
	FetchDuration       metric.Float64Histogram
	Invalidations       metric.Int64Counter
	TokenTTLBelowMargin metric.Int64Counter
}

// NewM2MMetrics creates real OTel instruments for M2M credential tracking.
func NewM2MMetrics(meter metric.Meter) (*M2MMetrics, error) {
	l1Hits, err := meter.Int64Counter("m2m_credential_l1_cache_hits",
		metric.WithDescription("Number of L1 (in-memory) cache hits for M2M credentials"))
	if err != nil {
		return nil, fmt.Errorf("create m2m_credential_l1_cache_hits counter: %w", err)
	}

	l2Hits, err := meter.Int64Counter("m2m_credential_l2_cache_hits",
		metric.WithDescription("Number of L2 (Redis) cache hits for M2M credentials"))
	if err != nil {
		return nil, fmt.Errorf("create m2m_credential_l2_cache_hits counter: %w", err)
	}

	misses, err := meter.Int64Counter("m2m_credential_cache_misses",
		metric.WithDescription("Number of cache misses requiring credential fetch from Secrets Manager"))
	if err != nil {
		return nil, fmt.Errorf("create m2m_credential_cache_misses counter: %w", err)
	}

	fetchErrors, err := meter.Int64Counter("m2m_credential_fetch_errors",
		metric.WithDescription("Number of errors when fetching credentials from Secrets Manager"))
	if err != nil {
		return nil, fmt.Errorf("create m2m_credential_fetch_errors counter: %w", err)
	}

	fetchDuration, err := meter.Float64Histogram("m2m_credential_fetch_duration_seconds",
		metric.WithDescription("Duration of credential fetch operations from Secrets Manager"),
		metric.WithUnit("s"))
	if err != nil {
		return nil, fmt.Errorf("create m2m_credential_fetch_duration_seconds histogram: %w", err)
	}

	invalidations, err := meter.Int64Counter("m2m_credential_invalidations",
		metric.WithDescription("Number of credential cache invalidations"))
	if err != nil {
		return nil, fmt.Errorf("create m2m_credential_invalidations counter: %w", err)
	}

	ttlBelowMargin, err := meter.Int64Counter("reporter_m2m_token_ttl_below_margin_total",
		metric.WithDescription("Number of M2M token exchanges where TTL was below safety margin (token rejected)"))
	if err != nil {
		return nil, fmt.Errorf("create reporter_m2m_token_ttl_below_margin_total counter: %w", err)
	}

	return &M2MMetrics{
		L1CacheHits:         l1Hits,
		L2CacheHits:         l2Hits,
		CacheMisses:         misses,
		FetchErrors:         fetchErrors,
		FetchDuration:       fetchDuration,
		Invalidations:       invalidations,
		TokenTTLBelowMargin: ttlBelowMargin,
	}, nil
}

// NoopM2MMetrics returns a no-op M2MMetrics instance for single-tenant mode
// or when a meter provider is not available.
func NoopM2MMetrics() *M2MMetrics {
	provider := noop.NewMeterProvider()
	meter := provider.Meter("noop")

	// noop meter never returns errors, so we can safely ignore them.
	l1Hits, _ := meter.Int64Counter("m2m_credential_l1_cache_hits")
	l2Hits, _ := meter.Int64Counter("m2m_credential_l2_cache_hits")
	misses, _ := meter.Int64Counter("m2m_credential_cache_misses")
	fetchErrors, _ := meter.Int64Counter("m2m_credential_fetch_errors")
	fetchDuration, _ := meter.Float64Histogram("m2m_credential_fetch_duration_seconds")
	invalidations, _ := meter.Int64Counter("m2m_credential_invalidations")
	ttlBelowMargin, _ := meter.Int64Counter("reporter_m2m_token_ttl_below_margin_total")

	return &M2MMetrics{
		L1CacheHits:         l1Hits,
		L2CacheHits:         l2Hits,
		CacheMisses:         misses,
		FetchErrors:         fetchErrors,
		FetchDuration:       fetchDuration,
		Invalidations:       invalidations,
		TokenTTLBelowMargin: ttlBelowMargin,
	}
}
