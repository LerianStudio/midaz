// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import "context"

// Result labels attached to Service Discovery metrics. They are the single source
// of truth for both call sites and the OTel-backed recorder implementation.
const (
	ResultOK       = "ok"
	ResultError    = "error"
	ResultResolved = "resolved"
	ResultFallback = "fallback"
)

// MetricsRecorder records Service Discovery metrics. Implementations are optional;
// a nil recorder is treated as a no-op via orNop. The interface is intentionally
// decoupled from OTel/lib-observability so call sites stay unit-testable with a
// plain stub.
type MetricsRecorder interface {
	// RegisterInitiated records that a service registration was initiated
	// (registration is fire-and-forget, so only initiations are observable).
	RegisterInitiated(ctx context.Context)
	// DeregisterResult records a synchronous deregistration outcome
	// (result: ResultOK|ResultError).
	DeregisterResult(ctx context.Context, result string)
	// ResolveResult records a resolve outcome for a service
	// (result: ResultResolved|ResultFallback|ResultError) together with its
	// duration in milliseconds.
	ResolveResult(ctx context.Context, service, result string, durationMs int64)
}

// NopMetricsRecorder is a zero-size MetricsRecorder whose methods do nothing. It
// backs orNop so call sites never need a nil check.
type NopMetricsRecorder struct{}

var _ MetricsRecorder = NopMetricsRecorder{}

func (NopMetricsRecorder) RegisterInitiated(_ context.Context) {}

func (NopMetricsRecorder) DeregisterResult(_ context.Context, _ string) {}

func (NopMetricsRecorder) ResolveResult(_ context.Context, _, _ string, _ int64) {}

// orNop returns r, or a NopMetricsRecorder when r is nil, so callers can invoke
// recorder methods unconditionally. It is the nil-guard the register/deregister/
// resolve call sites route every recorder through.
func orNop(r MetricsRecorder) MetricsRecorder {
	if r == nil {
		return NopMetricsRecorder{}
	}

	return r
}
