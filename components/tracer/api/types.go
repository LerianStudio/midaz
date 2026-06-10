// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package api

// ErrorResponse represents a standard error response.
// Used in Swagger documentation for error responses.
type ErrorResponse struct {
	Code    string `json:"code" validate:"required" example:"0053"`
	Title   string `json:"title" validate:"required" example:"Bad Request"`
	Message string `json:"message" validate:"required" example:"Invalid input provided"`
} // @name ErrorResponse

// VersionResponse represents the response of the version endpoint.
type VersionResponse struct {
	Version     string `json:"version" example:"1.0.0"`
	RequestDate string `json:"requestDate" example:"2025-01-01T00:00:00Z"`
	Commit      string `json:"commit" example:"a1b2c3d"`
	BuildTime   string `json:"buildTime" example:"2025-01-01T00:00:00Z"`
	Dirty       bool   `json:"dirty" example:"false"`
} // @name VersionResponse

// ReadyzResponse is the canonical readiness probe response per the Lerian
// /readyz contract. Top-level Status is "healthy" iff every check is in
// {up, skipped, n/a}; any "down" or "degraded" check forces "unhealthy" + 503.
type ReadyzResponse struct {
	Status         string                 `json:"status" example:"healthy" enums:"healthy,unhealthy"`
	Draining       bool                   `json:"draining,omitempty" example:"false"`
	Checks         map[string]ReadyzCheck `json:"checks"`
	Version        string                 `json:"version" example:"1.2.3"`
	DeploymentMode string                 `json:"deployment_mode" example:"saas"`
} // @name ReadyzResponse

// ReadyzCheck is a single dependency probe result for the canonical /readyz
// contract. Status is restricted to the closed vocabulary
// {up, down, degraded, skipped, n/a}; the optional fields are populated per
// status per the Lerian /readyz contract specification.
//
// TLS is a *bool so it can be omitted entirely when the dependency has no TLS
// concept (e.g. an in-process cache) — distinct from "tls=false" (configured
// without TLS).
//
// Reason and BreakerState fields are intentionally absent. The struct
// previously declared them, but no producer in the readyz pipeline ever
// populated them — advertising fields in the OpenAPI spec that no code emits
// is worse than not advertising them at all, because external SDK consumers
// would build dashboards on data that never arrives. Re-add only when a
// producer lands.
type ReadyzCheck struct {
	Status    string `json:"status" example:"up" enums:"up,down,degraded,skipped,n/a"`
	LatencyMs int64  `json:"latency_ms,omitempty" example:"3"`
	TLS       *bool  `json:"tls,omitempty" example:"true"`
	Error     string `json:"error,omitempty"`
} // @name ReadyzCheck
