// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"
)

// contextWithServiceTracer puts the service tracer on the root context of a worker, cron
// or consumer. HTTP requests get it from the telemetry middleware; these loops start from
// context.Background(), so without it every span they open falls back to the
// lib-observability default scope (observability.default) instead of the ledger's. A nil
// tracer leaves ctx unchanged, which keeps that fallback.
func contextWithServiceTracer(ctx context.Context, tracer trace.Tracer) context.Context {
	if tracer == nil {
		return ctx
	}

	return libObservability.ContextWithTracer(ctx, tracer)
}

// serviceTracer returns the tracer the HTTP middleware gives each request: the telemetry
// tracer provider's tracer under the service's instrumentation scope. It returns nil when
// telemetry has no tracer provider, so callers keep their current fallback.
func serviceTracer(telemetry *libOpentelemetry.Telemetry) trace.Tracer {
	if telemetry == nil || telemetry.TracerProvider == nil {
		return nil
	}

	return telemetry.ServiceTracer()
}
