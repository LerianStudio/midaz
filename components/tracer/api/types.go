// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package api

// ReadyzResponse is the canonical readiness probe response per the Lerian
// /readyz contract. Top-level Status is "healthy" iff every check is in
// {up, skipped, n/a}; any "down" or "degraded" check forces "unhealthy" + 503.
type ReadyzResponse struct {
	Status         string                 `json:"status" example:"healthy" enums:"healthy,unhealthy"`
	Draining       bool                   `json:"draining,omitempty" example:"false"`
	Checks         map[string]ReadyzCheck `json:"checks"`
	Version        string                 `json:"version" example:"1.2.3"`
	DeploymentMode string                 `json:"deployment_mode" example:"saas"`
}

// ReadyzCheck is a single dependency probe result for the canonical /readyz
// contract. Status is restricted to the closed vocabulary
// {up, down, degraded, skipped, n/a}; the optional fields are populated per
// status per the Lerian /readyz contract specification.
//
// TLS is a *bool so it can be omitted entirely when the dependency has no TLS
// concept (e.g. an in-process cache) — distinct from "tls=false" (configured
// without TLS).
//
// Reason carries a short, operator-facing explanation that has no natural home
// in Error — it is populated on the non-failing branches where a bare Error
// would be misleading: skipped probes (why the dependency was not checked, e.g.
// "MULTI_TENANT_ENABLED=false") and degraded probes (why the dependency is
// serving but impaired, e.g. "circuit breaker open"). The redis / tenant_manager
// / streaming probes are the producers that made this field earn its place;
// down probes still surface a canonical error code via Error, not Reason.
//
// BreakerState is intentionally still absent. No probe exposes a reliable
// breaker-state signal — the tenant-manager client's circuit breaker is private
// and lib-streaming does not surface one — so advertising the field in the
// OpenAPI spec would mean SDK consumers building dashboards on data that never
// arrives. Re-add only when a producer lands.
type ReadyzCheck struct {
	Status    string `json:"status" example:"up" enums:"up,down,degraded,skipped,n/a"`
	LatencyMs int64  `json:"latency_ms,omitempty" example:"3"`
	TLS       *bool  `json:"tls,omitempty" example:"true"`
	Reason    string `json:"reason,omitempty" example:"MULTI_TENANT_ENABLED=false"`
	Error     string `json:"error,omitempty"`
}
