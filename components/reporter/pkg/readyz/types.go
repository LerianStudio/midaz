// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package readyz implements the canonical /readyz contract for Lerian
// services. It provides a closed status vocabulary, a deterministic
// aggregation rule, dependency probe interfaces, drain-state coordination
// for graceful shutdown, and Fiber + net/http handler factories that share
// the same Checker abstraction.
//
// The package is transport-agnostic: concrete dependency checkers in checks.go
// implement the Checker interface and are wired by component-specific
// bootstrap code (Manager via Fiber, Worker via net/http).
//
// Anti-patterns this package deliberately avoids (per the canonical contract):
//   - No response caching (no TTL, no stale-while-revalidate). Every request
//     runs live probes.
//   - No /ready alias path. The contract path is exactly /readyz.
//   - No /health/live + /health/ready split. The Manager and Worker each
//     expose a single /health (liveness) endpoint and a single /readyz.
//   - No substring TLS detection (e.g. strings.Contains(uri, "tls=true")).
//     TLS posture detection lives in tls_detection.go and uses url.Parse.
//   - No reflection on connection objects to discover TLS state.
//   - No scattered SaaS TLS enforcement. Centralized validation lives in
//     bootstrap (Gate 4).
package readyz

import (
	"context"
	"time"
)

// Status is the closed vocabulary for the per-dependency status field in the
// /readyz response. The five values below are the only legal status strings.
//
// Aggregation rule (see Aggregate in aggregation.go):
//   - up | skipped | n/a → counted as healthy
//   - down | degraded   → counted as unhealthy (yields 503)
type Status string

// Status constants — the closed vocabulary. Adding values here is a contract
// change and must be coordinated with all consumers of /readyz.
const (
	// StatusUp means the dependency was probed and is reachable.
	StatusUp Status = "up"

	// StatusDown means the dependency was probed and the probe failed.
	// The DependencyCheck.Error field carries operator-facing context.
	StatusDown Status = "down"

	// StatusDegraded means the dependency is partially unavailable, e.g. a
	// circuit breaker is half-open. Counted as unhealthy by the aggregator.
	StatusDegraded Status = "degraded"

	// StatusSkipped means the dependency is intentionally disabled by config
	// (e.g. FETCHER_ENABLED=false). The DependencyCheck.Reason field carries
	// the human-readable explanation.
	StatusSkipped Status = "skipped"

	// StatusNA means the dependency is not applicable in the current mode
	// (e.g. per-tenant Mongo/RabbitMQ probing in multi-tenant mode is
	// deferred to a future per-tenant readiness gate). The
	// DependencyCheck.Reason field carries the explanation.
	StatusNA Status = "n/a"
)

// DependencyCheck is the per-dependency entry in the /readyz response.
// Optional fields use omitempty so they disappear from the JSON body when
// they have zero values, keeping the wire format compact.
type DependencyCheck struct {
	// Status is one of the closed vocabulary values defined above.
	Status Status `json:"status"`

	// LatencyMs is the probe duration in milliseconds, used for the JSON
	// wire format. Truncates sub-millisecond probes to 0 — that loss is
	// acceptable on the wire (operators read at ms granularity), but
	// metrics must NOT see the truncated value because it bottoms out
	// the histogram. Use Latency for metric emission.
	LatencyMs int64 `json:"latency_ms,omitempty"`

	// Latency is the unrounded probe duration with sub-millisecond
	// resolution. Not marshalled into JSON (the wire format is fixed at
	// LatencyMs). Set in lock-step with LatencyMs by every checker so the
	// handler's metric emission preserves precision (test-reviewer H2).
	//
	// Older checkers that pre-date this field will leave it zero. The
	// handler tolerates that and falls back to time.Duration(LatencyMs)*ms,
	// which is the same lossy path that motivated this field. New checkers
	// MUST populate Latency directly from time.Since(start).
	Latency time.Duration `json:"-"`

	// TLS reports whether the dependency connection is configured to use TLS.
	// Pointer-to-bool so we can distinguish "not detected" (nil) from
	// "explicitly false" (pointer to false). The TLS posture is filled in
	// Gate 3 by tls_detection.go.
	TLS *bool `json:"tls,omitempty"`

	// Error is an operator-facing error message. Set only when Status is
	// down or degraded. MUST NOT contain credentials — callers are expected
	// to redact with pkg.RedactConnectionString or equivalent.
	Error string `json:"error,omitempty"`

	// Reason is a human-readable explanation. Set only when Status is
	// skipped or n/a.
	Reason string `json:"reason,omitempty"`

	// BreakerState is reserved for future circuit-breaker integration
	// (Gate 6). One of: closed, half-open, open. Empty when no breaker is
	// associated with the dependency.
	BreakerState string `json:"breaker_state,omitempty"`
}

// Response is the canonical /readyz response shape. The top-level Status
// field is the aggregation result ("healthy", "unhealthy", or "draining").
// Version, DeploymentMode and Checks are always emitted. The Checks field
// is a map (rather than a pointer) and is encoded as `{}` when empty so
// dashboards / clients keying on `checks` keep working during drain (when
// no probes ran) and during the empty-dependency case.
type Response struct {
	// Status is the aggregated state: "healthy", "unhealthy", or "draining".
	Status string `json:"status"`

	// Reason is set on draining responses to explain why probes were skipped.
	Reason string `json:"reason,omitempty"`

	// Checks maps dependency name to its DependencyCheck entry. Always
	// emitted in the wire format — even when empty (drain mode, services
	// with zero declared dependencies). Handlers MUST set this field to
	// a non-nil map; nil would serialize as `null` instead of `{}`.
	Checks map[string]DependencyCheck `json:"checks"`

	// Version is the running service version (typically OTEL_RESOURCE_SERVICE_VERSION).
	Version string `json:"version"`

	// DeploymentMode echoes the DEPLOYMENT_MODE env var: saas | byoc | local.
	DeploymentMode string `json:"deployment_mode"`
}

// Checker is the per-dependency probe interface. Implementations live in
// checks.go and may be plain structs that close over connection handles.
//
// Check is expected to honor ctx cancellation (use context.WithTimeout in the
// caller) and never panic. It MUST be safe to call concurrently — the
// generic handlers run all checkers in parallel.
type Checker interface {
	// Name returns a stable identifier used as the JSON key for this
	// dependency in the /readyz response (e.g. "mongo", "rabbitmq").
	Name() string

	// Check runs the readiness probe for this dependency and returns a
	// DependencyCheck describing the outcome.
	Check(ctx context.Context) DependencyCheck
}
