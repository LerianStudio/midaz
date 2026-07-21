// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"sync"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability"
	libRuntime "github.com/LerianStudio/lib-observability/runtime"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/api"
)

// emitReadyzMetrics records the per-dep duration histogram + status counter
// for a single probe execution. MUST be invoked from every probe — including
// skipped/n-a paths — per the canonical contract ("every probe execution").
// Centralised here so the probe functions stay readable and the wsl_v5
// linter does not flip on the duplicated emit pair.
//
// Takes the already-measured elapsed duration (rather than a start time) so
// callers control the measurement window — the SAME elapsed value can then
// be reported both on the metric and in the response's LatencyMs field,
// preventing the "two durations for one probe" bug where the metric closes
// the window before the probe is fully done (e.g. before TLS posture detect).
//
// A nil recorder is treated as a no-op — the probe still runs end-to-end,
// only the metric emission is skipped. Bootstrap injects a real recorder
// via HealthChecker.SetReadyzRecorder.
func (h *HealthChecker) emitReadyzMetrics(ctx context.Context, dep, status string, elapsed time.Duration) {
	h.readyzRecorder.EmitCheckDuration(ctx, dep, status, elapsed)
	h.readyzRecorder.EmitCheckStatus(ctx, dep, status)
}

// Status vocabulary — closed set per the canonical /readyz contract. Any
// other value is non-compliant. Aggregation: top-level "healthy" iff every
// GATING check is in {up, skipped, n/a}; any "down" or "degraded" gating check
// forces 503. Advisory checks (see advisoryChecks — currently streaming) are
// excluded from the aggregate.
//
// StatusSkipped and StatusNA are emitted by the multi-tenant-gated probes
// (redis / tenant_manager report "skipped" in single-tenant mode; rule_cache
// reports "n/a" in multi-tenant mode) and streaming reports "skipped" when
// disabled — downstream consumers depend on the canonical 5-value vocabulary.
const (
	StatusUp       = "up"
	StatusDown     = "down"
	StatusDegraded = "degraded"
	StatusSkipped  = "skipped"
	StatusNA       = "n/a"
)

// Top-level aggregate status values returned in ReadyzResponse.Status.
const (
	StatusHealthy   = "healthy"
	StatusUnhealthy = "unhealthy"
)

// Per-dependency probe timeouts. Per-dep timeouts (rather than a single
// outer timeout) prevent a slow dep from blocking the others — the readyz
// handler must never hang K8s probes.
const (
	probeTimeoutPostgres = 2 * time.Second
	// Rule cache is in-process; "0" timeout means we skip context.WithTimeout
	// entirely. The cache health provider returns synchronously.

	// probeTimeoutRedis bounds the multi-tenant Redis Pub/Sub PING. Tighter
	// than the DB budget — a Pub/Sub PING is a single round-trip.
	probeTimeoutRedis = 1 * time.Second
	// probeTimeoutTenantManager bounds the tenant-manager functional probe
	// (GetActiveTenantsByService), which is an outbound HTTP call.
	probeTimeoutTenantManager = 2 * time.Second
	// probeTimeoutStreaming bounds the streaming producer Healthy() call, which
	// may touch the broker adapter.
	probeTimeoutStreaming = 2 * time.Second
)

// Skip reasons surfaced in ReadyzCheck.Reason when a probe is short-circuited.
// Mirror the exact env-var spelling so operators can grep the response straight
// back to the toggle that produced it.
const (
	reasonMultiTenantDisabled = "MULTI_TENANT_ENABLED=false"
	reasonStreamingDisabled   = "STREAMING_ENABLED=false"
	reasonCircuitBreakerOpen  = "circuit breaker open"
	reasonStreamingDegraded   = "streaming degraded"
)

// ReadyzHandler returns the canonical readiness handler per the Lerian
// /readyz contract. It probes every active dependency with a per-dep
// timeout, aggregates results into ReadyzResponse, and returns 200 only
// when every check is in {up, skipped, n/a}.
//
// The /readyz cycle probes five dependencies: postgres and rule_cache always
// run; redis and tenant_manager are multi-tenant-gated (report "skipped" in
// single-tenant mode); streaming is advisory and gated by STREAMING_ENABLED.
// All but streaming gate the top-level status.
//
// MUST be registered on the public path tree, BEFORE any auth middleware —
// K8s probes are unauthenticated and a 401 here would be interpreted by
// the kubelet as "not ready" and kill the pod.
func (h *HealthChecker) ReadyzHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.UserContext()

		logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

		ctx, span := tracer.Start(ctx, "handler.health.readyz")
		defer span.End()

		// Capture drain posture once at handler entry. Drain does NOT skip
		// the probe work — per-dep timeouts already bound execution and the
		// canonical contract requires the `checks` map to be populated even
		// while draining so operators can see what was healthy at the moment
		// drain started. The `draining: true` flag is the signal that K8s
		// must withhold traffic; the rest of the response shape is identical
		// to the non-drain path.
		draining := h.IsDraining()

		// Run the five probes concurrently. Each probe creates its own child
		// span from the inherited ctx and enforces its own per-dep timeout via
		// context.WithTimeout — parallelism is safe here. Worst-case latency
		// drops from sum(timeouts) to max(timeouts) under partial outage.
		//
		// C2: each probe runs through runtime.SafeGoWithContextAndComponent
		// so a panic inside a probe (e.g. an unexpected nil-pointer in a
		// deps adapter) is recovered with logs + panic_recovered_total
		// counter + span event instead of taking down the HTTP server.
		// KeepRunning policy: a crashing probe MUST NOT crashloop the pod —
		// the readiness signal recovers naturally via the next probe cycle.
		var (
			pgCheck, rcCheck                 api.ReadyzCheck
			redisCheck, tmCheck, streamCheck api.ReadyzCheck
			wg                               sync.WaitGroup
		)

		wg.Add(5)

		libRuntime.SafeGoWithContextAndComponent(
			ctx,
			logger,
			"readyz",
			"probe-postgres",
			libRuntime.KeepRunning,
			func(probeCtx context.Context) {
				defer wg.Done()

				pgCheck = h.probeReadyzPostgres(probeCtx)
			},
		)

		libRuntime.SafeGoWithContextAndComponent(
			ctx,
			logger,
			"readyz",
			"probe-rule_cache",
			libRuntime.KeepRunning,
			func(probeCtx context.Context) {
				defer wg.Done()

				rcCheck = h.probeReadyzRuleCache(probeCtx)
			},
		)

		libRuntime.SafeGoWithContextAndComponent(
			ctx,
			logger,
			"readyz",
			"probe-redis",
			libRuntime.KeepRunning,
			func(probeCtx context.Context) {
				defer wg.Done()

				redisCheck = h.probeReadyzRedis(probeCtx)
			},
		)

		libRuntime.SafeGoWithContextAndComponent(
			ctx,
			logger,
			"readyz",
			"probe-tenant_manager",
			libRuntime.KeepRunning,
			func(probeCtx context.Context) {
				defer wg.Done()

				tmCheck = h.probeReadyzTenantManager(probeCtx)
			},
		)

		libRuntime.SafeGoWithContextAndComponent(
			ctx,
			logger,
			"readyz",
			"probe-streaming",
			libRuntime.KeepRunning,
			func(probeCtx context.Context) {
				defer wg.Done()

				streamCheck = h.probeReadyzStreaming(probeCtx)
			},
		)

		wg.Wait()

		checks := map[string]api.ReadyzCheck{
			"postgres":       pgCheck,
			"rule_cache":     rcCheck,
			"redis":          redisCheck,
			"tenant_manager": tmCheck,
			"streaming":      streamCheck,
		}

		response := api.ReadyzResponse{
			Checks:         checks,
			Version:        h.version,
			DeploymentMode: h.deploymentMode,
		}

		// Drain branch: force unhealthy + 503 regardless of probe outcomes,
		// but preserve the canonical response shape (populated `checks` map,
		// version, deployment_mode). `draining: true` is what K8s reads to
		// route traffic away from this pod.
		if draining {
			response.Status = StatusUnhealthy
			response.Draining = true

			libOtel.HandleSpanBusinessErrorEvent(span, "readyz draining", ErrDependenciesUnhealthy)

			return libHTTP.Respond(c, fiber.StatusServiceUnavailable, response)
		}

		response.Status = aggregateStatus(checks)

		if response.Status == StatusUnhealthy {
			libOtel.HandleSpanError(span, "readyz check failed", ErrDependenciesUnhealthy)

			return libHTTP.Respond(c, fiber.StatusServiceUnavailable, response)
		}

		return libHTTP.Respond(c, fiber.StatusOK, response)
	}
}

// spanCause chooses the error recorded on a probe's span: the sanitized cause
// from the boundary adapter when present, otherwise the canonical sentinel.
// The wire-facing ReadyzCheck.Error always uses the sentinel — this only
// enriches telemetry.
func spanCause(cause, fallback error) error {
	if cause != nil {
		return cause
	}

	return fallback
}

// probeReadyzRedis pings the multi-tenant Redis Pub/Sub client within the
// per-dep timeout. Redis is multi-tenant-only: in single-tenant mode the probe
// reports "skipped" and never touches the pinger. On any ping failure it
// returns "down" with a canonical sentinel code — the raw go-redis error is
// never surfaced on the wire.
//
// The `tls` field is populated from the configured cfg.MultiTenantRedisTLS bool
// (SetRedisTLS) — never by reflecting on the live connection (anti-pattern
// N4/N5).
func (h *HealthChecker) probeReadyzRedis(ctx context.Context) api.ReadyzCheck {
	//nolint:dogsled // tracker tuple unused in readyz probes; only the tracer is required for child spans
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "readyz.probe.redis")
	defer span.End()

	start := time.Now().UTC()

	if !h.multiTenantEnabled {
		elapsed := time.Since(start)

		h.emitReadyzMetrics(ctx, "redis", StatusSkipped, elapsed)

		return api.ReadyzCheck{
			Status:    StatusSkipped,
			LatencyMs: elapsed.Milliseconds(),
			Reason:    reasonMultiTenantDisabled,
		}
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeoutRedis)
	defer cancel()

	if h.redisPinger == nil {
		elapsed := time.Since(start)

		libOtel.HandleSpanError(span, "redis readyz: connection not established", ErrRedisConnectionNotEstablished)
		h.emitReadyzMetrics(ctx, "redis", StatusDown, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDown,
			LatencyMs: elapsed.Milliseconds(),
			Error:     ErrRedisConnectionNotEstablished.Error(),
		}
	}

	if err := h.redisPinger.Ping(ctx); err != nil {
		elapsed := time.Since(start)

		libOtel.HandleSpanError(span, "redis readyz: ping failed", ErrRedisPingFailed)
		h.emitReadyzMetrics(ctx, "redis", StatusDown, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDown,
			LatencyMs: elapsed.Milliseconds(),
			Error:     ErrRedisPingFailed.Error(),
		}
	}

	elapsed := time.Since(start)

	h.emitReadyzMetrics(ctx, "redis", StatusUp, elapsed)

	return api.ReadyzCheck{
		Status:    StatusUp,
		LatencyMs: elapsed.Milliseconds(),
		TLS:       h.redisTLSEnabled,
	}
}

// probeReadyzTenantManager reports tenant-manager HTTP client readiness. The
// tenant-manager client exposes no dedicated health signal, so the bootstrap
// adapter runs a functional call and classifies its outcome into the tri-state
// {up, degraded, down} — degraded specifically when the client's circuit
// breaker is open. Tenant manager is multi-tenant-only: single-tenant reports
// "skipped".
func (h *HealthChecker) probeReadyzTenantManager(ctx context.Context) api.ReadyzCheck {
	//nolint:dogsled // tracker tuple unused in readyz probes; only the tracer is required for child spans
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "readyz.probe.tenant_manager")
	defer span.End()

	start := time.Now().UTC()

	if !h.multiTenantEnabled {
		elapsed := time.Since(start)

		h.emitReadyzMetrics(ctx, "tenant_manager", StatusSkipped, elapsed)

		return api.ReadyzCheck{
			Status:    StatusSkipped,
			LatencyMs: elapsed.Milliseconds(),
			Reason:    reasonMultiTenantDisabled,
		}
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeoutTenantManager)
	defer cancel()

	if h.tenantManagerProber == nil {
		elapsed := time.Since(start)

		libOtel.HandleSpanError(span, "tenant_manager readyz: prober not configured", ErrTenantManagerUnavailable)
		h.emitReadyzMetrics(ctx, "tenant_manager", StatusDown, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDown,
			LatencyMs: elapsed.Milliseconds(),
			Error:     ErrTenantManagerUnavailable.Error(),
		}
	}

	status, probeErr := h.tenantManagerProber.Probe(ctx)
	elapsed := time.Since(start)

	switch status {
	case StatusUp:
		h.emitReadyzMetrics(ctx, "tenant_manager", StatusUp, elapsed)

		return api.ReadyzCheck{Status: StatusUp, LatencyMs: elapsed.Milliseconds()}
	case StatusDegraded:
		libOtel.HandleSpanError(span, "tenant_manager readyz: degraded",
			spanCause(probeErr, ErrTenantManagerUnavailable))
		h.emitReadyzMetrics(ctx, "tenant_manager", StatusDegraded, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDegraded,
			LatencyMs: elapsed.Milliseconds(),
			Reason:    reasonCircuitBreakerOpen,
		}
	default:
		// Fail closed: StatusDown and any unexpected value collapse to "down".
		libOtel.HandleSpanError(span, "tenant_manager readyz: down",
			spanCause(probeErr, ErrTenantManagerUnavailable))
		h.emitReadyzMetrics(ctx, "tenant_manager", StatusDown, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDown,
			LatencyMs: elapsed.Milliseconds(),
			Error:     ErrTenantManagerUnavailable.Error(),
		}
	}
}

// probeReadyzStreaming reports streaming/RedPanda producer readiness via the
// bootstrap adapter over lib-streaming's Emitter.Healthy tri-state. Independent
// of multi-tenancy: gated by the STREAMING_ENABLED master flag, reporting
// "skipped" when disabled (more honest than probing a NoopEmitter that always
// reports healthy).
func (h *HealthChecker) probeReadyzStreaming(ctx context.Context) api.ReadyzCheck {
	//nolint:dogsled // tracker tuple unused in readyz probes; only the tracer is required for child spans
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "readyz.probe.streaming")
	defer span.End()

	start := time.Now().UTC()

	if !h.streamingEnabled {
		elapsed := time.Since(start)

		h.emitReadyzMetrics(ctx, "streaming", StatusSkipped, elapsed)

		return api.ReadyzCheck{
			Status:    StatusSkipped,
			LatencyMs: elapsed.Milliseconds(),
			Reason:    reasonStreamingDisabled,
		}
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeoutStreaming)
	defer cancel()

	if h.streamingProber == nil {
		elapsed := time.Since(start)

		libOtel.HandleSpanError(span, "streaming readyz: prober not configured", ErrStreamingUnhealthy)
		h.emitReadyzMetrics(ctx, "streaming", StatusDown, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDown,
			LatencyMs: elapsed.Milliseconds(),
			Error:     ErrStreamingUnhealthy.Error(),
		}
	}

	status, probeErr := h.streamingProber.Probe(ctx)
	elapsed := time.Since(start)

	switch status {
	case StatusUp:
		h.emitReadyzMetrics(ctx, "streaming", StatusUp, elapsed)

		return api.ReadyzCheck{Status: StatusUp, LatencyMs: elapsed.Milliseconds()}
	case StatusDegraded:
		libOtel.HandleSpanError(span, "streaming readyz: degraded",
			spanCause(probeErr, ErrStreamingUnhealthy))
		h.emitReadyzMetrics(ctx, "streaming", StatusDegraded, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDegraded,
			LatencyMs: elapsed.Milliseconds(),
			Reason:    reasonStreamingDegraded,
		}
	default:
		// Fail closed: StatusDown and any unexpected value collapse to "down".
		libOtel.HandleSpanError(span, "streaming readyz: down",
			spanCause(probeErr, ErrStreamingUnhealthy))
		h.emitReadyzMetrics(ctx, "streaming", StatusDown, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDown,
			LatencyMs: elapsed.Milliseconds(),
			Error:     ErrStreamingUnhealthy.Error(),
		}
	}
}

// advisoryChecks are checks that appear in the response `checks` map (with
// their real status and metrics) but do NOT contribute to the top-level
// aggregate / HTTP status. Streaming is IMPORTANT-posture and non-blocking: a
// broker hiccup must never pull the pod from rotation, so its up/down/degraded
// status is informational only.
var advisoryChecks = map[string]struct{}{
	"streaming": {},
}

// aggregateStatus implements the canonical aggregation rule: top-level
// "healthy" iff every GATING check is in {up, skipped, n/a}. Any "down" or
// "degraded" gating check forces "unhealthy".
//
// Advisory checks (see advisoryChecks) are skipped entirely: they still appear
// verbatim in the response `checks` map, but their status never influences the
// aggregate or the HTTP code.
//
// Fails CLOSED on unknown probe states: a gating probe that returns a Status
// outside the canonical 5-value vocabulary ("", "OK", "READY", a typo) is
// treated as "unhealthy" and forces 503. Without this default, a buggy probe
// that returned an empty string would silently aggregate to "healthy" —
// fail-OPEN, which is the dangerous direction for a readiness signal.
func aggregateStatus(checks map[string]api.ReadyzCheck) string {
	for name, c := range checks {
		if _, advisory := advisoryChecks[name]; advisory {
			continue
		}

		switch c.Status {
		case StatusUp, StatusSkipped, StatusNA:
			// Healthy contribution — keep walking.
		case StatusDown, StatusDegraded:
			return StatusUnhealthy
		default:
			// Unknown probe status — fail closed.
			return StatusUnhealthy
		}
	}

	return StatusHealthy
}

// probeReadyzPostgres pings the primary PostgreSQL connection within the
// per-dep timeout. Returns "down" with a sanitized error message on any
// failure (no SQL error details surfaced — that would leak internal state).
//
// The `tls` response field is populated by parsing the DSN registered via
// SetPostgresDSN — anti-pattern N5 forbids reflecting on the live connection
// to read TLS posture, so the helper inspects the libpq connection string
// directly. When the DSN is empty (not registered) or fails to parse, the
// `tls` field is omitted (nil); the probe still reports up/down based on
// the ping result.
func (h *HealthChecker) probeReadyzPostgres(ctx context.Context) api.ReadyzCheck {
	//nolint:dogsled // tracker tuple unused in readyz probes; only the tracer is required for child spans
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "readyz.probe.postgres")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, probeTimeoutPostgres)
	defer cancel()

	start := time.Now().UTC()

	if h.dbProvider == nil || !h.dbProvider.IsConnected() {
		elapsed := time.Since(start)

		libOtel.HandleSpanError(span, "postgres readyz: connection not established", ErrConnectionNotEstablished)
		h.emitReadyzMetrics(ctx, "postgres", StatusDown, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDown,
			LatencyMs: elapsed.Milliseconds(),
			Error:     ErrConnectionNotEstablished.Error(),
		}
	}

	db, err := h.dbProvider.GetDB(ctx)
	if err != nil {
		elapsed := time.Since(start)

		libOtel.HandleSpanError(span, "postgres readyz: get db failed", ErrConnectionFailed)
		h.emitReadyzMetrics(ctx, "postgres", StatusDown, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDown,
			LatencyMs: elapsed.Milliseconds(),
			Error:     ErrConnectionFailed.Error(),
		}
	}

	if err := db.PingContext(ctx); err != nil {
		elapsed := time.Since(start)

		libOtel.HandleSpanError(span, "postgres readyz: ping failed", ErrPingFailed)
		h.emitReadyzMetrics(ctx, "postgres", StatusDown, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDown,
			LatencyMs: elapsed.Milliseconds(),
			Error:     ErrPingFailed.Error(),
		}
	}

	// TLS detect is part of the probe work — compute elapsed AFTER it so the
	// metric and the response's LatencyMs reflect the same measurement window.
	tls := h.detectPostgresTLS(span)
	elapsed := time.Since(start)

	h.emitReadyzMetrics(ctx, "postgres", StatusUp, elapsed)

	return api.ReadyzCheck{
		Status:    StatusUp,
		LatencyMs: elapsed.Milliseconds(),
		TLS:       tls,
	}
}

// detectPostgresTLS resolves the TLS posture for the postgres readyz probe.
// Returns nil when (a) no detector has been wired, (b) no DSN has been
// registered, or (c) parsing the DSN fails. In case (c) the parse error is
// recorded on the span as a non-business event so operators can spot
// misconfigurations without flipping the probe to "down" — a parseable-but-
// "ssl-disabled" DSN is operationally distinct from "we couldn't determine
// the posture".
func (h *HealthChecker) detectPostgresTLS(span trace.Span) *bool {
	if h.pgTLSDetector == nil || h.pgDSN == "" {
		return nil
	}

	enabled, err := h.pgTLSDetector(h.pgDSN)
	if err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "postgres readyz: tls posture parse failed", err)

		return nil
	}

	return &enabled
}

// probeReadyzRuleCache reports cache readiness:
//
//   - !ready ⇒ status "down"
//   - stale  ⇒ status "degraded" (intentional behavior change vs. legacy /ready
//     which returned 200/DEGRADED — the canonical contract forces 503 here)
//   - else   ⇒ status "up"
//
// The cache is in-process so no context.WithTimeout wrapper is needed; the
// underlying provider returns synchronously.
//
// LatencyMs is measured via time.Since(start).Milliseconds() and always
// populated — even though the in-process cache nearly always rounds to 0,
// the contract field is honest about what was measured.
//
// The /readyz cycle is single-tenant only: this probe reports the cache
// state of the global cache provider.
//
// TLS field: the rule cache is in-process — there is no transport, no TLS
// concept. The probe leaves ReadyzCheck.TLS nil so it is omitted from the
// JSON response (per ReadyzCheck.TLS json:"tls,omitempty"). This is
// distinct from "tls=false", which would (incorrectly) imply the dep was
// configured without TLS.
func (h *HealthChecker) probeReadyzRuleCache(ctx context.Context) api.ReadyzCheck {
	//nolint:dogsled // tracker tuple unused in readyz probes; only the tracer is required for child spans
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "readyz.probe.rule_cache")
	defer span.End()

	start := time.Now().UTC()

	// H1: in multi-tenant mode the global rule cache is intentionally NOT
	// warmed at boot — the empty-tenant bucket is evicted in
	// conditionalWarmUpCache, and per-tenant buckets are populated lazily by
	// the per-tenant RuleSyncWorker. The K8s probe runs without tenant
	// context, so probing the global cache here would always report "down"
	// and crashloop every pod fleet-wide. Mirror the boot self-probe gate
	// (buildSelfProbeChecks) and report Status=n/a so /readyz keeps
	// reflecting postgres health without polluting the cache lane. Per-tenant
	// cache health is exposed via the tenant_consumers_active metric.
	if h.multiTenantEnabled {
		elapsed := time.Since(start)

		h.emitReadyzMetrics(ctx, "rule_cache", StatusNA, elapsed)

		return api.ReadyzCheck{
			Status:    StatusNA,
			LatencyMs: elapsed.Milliseconds(),
		}
	}

	if h.cacheHealth == nil {
		// No cache wired yet (e.g. warm-up incomplete). Report "down" so K8s
		// holds traffic until SetCacheHealthProvider is called.
		elapsed := time.Since(start)

		libOtel.HandleSpanError(span, "rule_cache readyz: provider not configured", ErrCacheNotReady)
		h.emitReadyzMetrics(ctx, "rule_cache", StatusDown, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDown,
			LatencyMs: elapsed.Milliseconds(),
			Error:     ErrCacheNotReady.Error(),
		}
	}

	if !h.cacheHealth.IsReady(ctx) {
		elapsed := time.Since(start)

		libOtel.HandleSpanError(span, "rule_cache readyz: not ready", ErrCacheNotReady)
		h.emitReadyzMetrics(ctx, "rule_cache", StatusDown, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDown,
			LatencyMs: elapsed.Milliseconds(),
			Error:     ErrCacheNotReady.Error(),
		}
	}

	if h.cacheHealth.Staleness(ctx) > h.cacheStalenessThreshold {
		elapsed := time.Since(start)

		libOtel.HandleSpanError(span, "rule_cache readyz: stale", ErrCacheStale)
		h.emitReadyzMetrics(ctx, "rule_cache", StatusDegraded, elapsed)

		return api.ReadyzCheck{
			Status:    StatusDegraded,
			LatencyMs: elapsed.Milliseconds(),
			Error:     ErrCacheStale.Error(),
		}
	}

	elapsed := time.Since(start)

	h.emitReadyzMetrics(ctx, "rule_cache", StatusUp, elapsed)

	// LatencyMs is honest: for the in-process cache the value almost always
	// rounds to 0, but the contract field is always populated so consumers
	// can rely on its presence.
	return api.ReadyzCheck{
		Status:    StatusUp,
		LatencyMs: elapsed.Milliseconds(),
	}
}
