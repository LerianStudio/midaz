// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

//go:generate mockgen -source=health.go -destination=health_mock.go -package=in

import (
	"context"
	"database/sql"
	"sync/atomic"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/observability"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Sentinel errors for health check failures.
//
// All sentinels live in the canonical pkg/constant/errors.go registry — every
// error surfaced over the wire MUST carry a stable code so dashboards/alerts can
// match by code rather than free-form English text. The aliases below are kept
// so existing call sites in readyz.go / health_handler.go keep compiling without
// churn.
var (
	ErrConnectionNotEstablished = constant.ErrReadyzPgConnectionNotEstablished
	ErrConnectionFailed         = constant.ErrReadyzPgConnectionFailed
	ErrPingFailed               = constant.ErrReadyzPgPingFailed
	ErrDependenciesUnhealthy    = constant.ErrReadyzDependenciesUnhealthy
	ErrCacheNotReady            = constant.ErrReadyzCacheNotReady
	ErrCacheStale               = constant.ErrReadyzCacheStale
)

// Default health check configuration values.
const (
	DefaultHealthCheckTimeout = 3 * time.Second
	// DefaultCacheStalenessThreshold is the lenient tolerance used by the K8s readiness probe.
	// Intentionally higher than RuleSyncWorkerConfig.StalenessThreshold (50s default),
	// which is the internal worker metric for detecting stale cache. The readiness probe
	// uses a wider window to avoid unnecessary pod restarts during transient DB outages.
	DefaultCacheStalenessThreshold = 5 * time.Minute
)

// RuleCacheHealthProvider exposes cache health metrics for the readiness probe.
type RuleCacheHealthProvider interface {
	IsReady(ctx context.Context) bool
	Staleness(ctx context.Context) time.Duration
}

// PostgresDBProvider abstracts PostgreSQL database access for testability.
// This interface allows mocking the database connection in tests.
type PostgresDBProvider interface {
	GetDB(ctx context.Context) (*sql.DB, error)
	IsConnected() bool
}

// postgresConnectionAdapter adapts *libPostgres.Client to PostgresDBProvider.
type postgresConnectionAdapter struct {
	conn *libPostgres.Client
}

// GetDB returns the underlying database connection.
// The dbresolver.DB returned by lib-commons wraps *sql.DB, so we type assert it.
//
// Nil-safe: returns ErrConnectionNotEstablished when the adapter or its
// backing client is nil. Asymmetric with IsConnected was the M6 defect —
// GetDB now guards the same preconditions (M6).
func (p *postgresConnectionAdapter) GetDB(ctx context.Context) (*sql.DB, error) {
	if p == nil || p.conn == nil {
		return nil, ErrConnectionNotEstablished
	}

	db, err := p.conn.Resolver(ctx)
	if err != nil {
		return nil, err
	}

	// The dbresolver.DB has PrimaryDBs() method that returns []*sql.DB
	// For health checks, we need to ping the primary connection
	if dbr, ok := db.(interface{ PrimaryDBs() []*sql.DB }); ok {
		primaries := dbr.PrimaryDBs()
		if len(primaries) == 0 || primaries[0] == nil {
			return nil, ErrConnectionFailed
		}

		return primaries[0], nil
	}

	return nil, ErrConnectionFailed
}

// IsConnected returns whether the connection is established. Nil-safe so a
// typed-nil *postgresConnectionAdapter (which can leak in via a never-wired
// PostgresDBProvider) collapses to "not connected" instead of panicking on
// the readiness fast path. Mirrors the GetDB receiver-nil guard above —
// /readyz must never crash on a half-constructed health checker because
// kubelet would interpret the panic as a hard liveness failure and kill the
// pod, masking the real configuration bug behind a restart loop.
func (p *postgresConnectionAdapter) IsConnected() bool {
	if p == nil || p.conn == nil {
		return false
	}

	connected, err := p.conn.IsConnected()

	return err == nil && connected
}

// HealthChecker holds the connection pools for dependency health checks.
type HealthChecker struct {
	dbProvider              PostgresDBProvider
	cacheHealth             RuleCacheHealthProvider
	cacheStalenessThreshold time.Duration

	// multiTenantEnabled mirrors Config.MultiTenantEnabled. When true the
	// /readyz handler skips the rule_cache probe entirely and reports
	// Status=StatusNA — the cache is per-tenant in MT mode and the K8s probe
	// has no tenant context, so probing the global ("") bucket would always
	// return "down" once it has been evicted at boot (H1, mirrors the
	// boot-time behaviour of buildSelfProbeChecks).
	//
	// Plain bool (not atomic) — written exactly once at bootstrap via
	// SetMultiTenantEnabled BEFORE the HTTP server starts accepting traffic,
	// so there is no race with /readyz reads. Tests use the same setter.
	multiTenantEnabled bool

	// version + deploymentMode are echoed in /readyz responses. Sourced from
	// cfg.OtelServiceVersion + cfg.DeploymentMode at bootstrap time.
	version        string
	deploymentMode string

	// draining is set to true when SIGTERM has been received (Gate 7 wires
	// this). Once true, /readyz short-circuits to 503 + draining=true so K8s
	// drains traffic before shutdown. Atomic for lock-free reads on the hot
	// path — every probe request reads this field.
	draining atomic.Bool

	// pgDSN is the libpq connection string handed to lib-commons at boot.
	// Bootstrap copies it here via SetPostgresDSN so the postgres /readyz
	// probe can populate the `tls` field by parsing the DSN's sslmode (anti-
	// pattern N5 forbids reflecting on the live connection just to read TLS
	// posture). Empty string ⇒ probe reports tls=nil.
	pgDSN string

	// pgTLSDetector parses pgDSN and reports the TLS posture. Defaults to a
	// nil detector (TLS field omitted). Bootstrap injects the real DSN parser
	// from internal/bootstrap/tls_detection.go via SetPostgresTLSDetector —
	// this keeps the `in` package free of any URL/DSN parsing logic and
	// avoids the import cycle that would otherwise arise (bootstrap depends
	// on `in`, not the other way around). The function form (rather than an
	// interface) is intentional: the contract is a single pure function with
	// no state, so an interface would be overkill.
	pgTLSDetector func(dsn string) (bool, error)

	// readyzRecorder is the OTel-backed metrics sink for the /readyz probe.
	// Bootstrap sets this once via SetReadyzRecorder — until then the field
	// is nil and *observability.Recorder method calls short-circuit, so unit
	// tests that don't care about metric emission can run without wiring a
	// recorder. Production wires a Prometheus-bridged recorder so the same
	// series surface on /metrics that the legacy raw-Prometheus emitter
	// produced (preserving the canonical contract).
	readyzRecorder *observability.Recorder
}

// NewHealthChecker creates a new HealthChecker instance with connection pools.
// Uses DefaultHealthCheckTimeout (3s) which is suitable for liveness probes.
// Version + deploymentMode are echoed in /readyz responses; bootstrap sources
// them from cfg.OtelServiceVersion + cfg.DeploymentMode.
func NewHealthChecker(postgresConn *libPostgres.Client, version, deploymentMode string) *HealthChecker {
	var provider PostgresDBProvider

	if postgresConn != nil {
		provider = &postgresConnectionAdapter{conn: postgresConn}
	}

	return &HealthChecker{
		dbProvider:              provider,
		cacheStalenessThreshold: DefaultCacheStalenessThreshold,
		version:                 version,
		deploymentMode:          deploymentMode,
	}
}

// NewTestableHealthChecker creates a HealthChecker with an injectable PostgresDBProvider.
// This constructor is intended for testing, allowing mock database connections.
// version + deploymentMode default to empty strings.
func NewTestableHealthChecker(provider PostgresDBProvider) *HealthChecker {
	return NewTestableHealthCheckerWithMeta(provider, "", "")
}

// NewTestableHealthCheckerWithMeta is the test constructor with explicit
// version + deploymentMode wiring. Use this when tests assert the /readyz
// response echoes those values.
func NewTestableHealthCheckerWithMeta(provider PostgresDBProvider, version, deploymentMode string) *HealthChecker {
	return &HealthChecker{
		dbProvider:              provider,
		cacheStalenessThreshold: DefaultCacheStalenessThreshold,
		version:                 version,
		deploymentMode:          deploymentMode,
	}
}

// SetCacheHealthProvider attaches a cache health provider to the health checker.
// Must be called after cache warm-up completes.
func (h *HealthChecker) SetCacheHealthProvider(provider RuleCacheHealthProvider) {
	h.cacheHealth = provider
}

// SetPostgresDSN registers the libpq connection string used to probe TLS
// posture in the postgres /readyz check. Bootstrap calls this once at boot
// after building the DSN; tests may pass a hand-crafted DSN to assert
// specific TLS branches. There is no PostgresDBProvider equivalent because
// lib-commons does not expose the DSN through its client API — and reading
// it off a live *sql.DB would require reflection (anti-pattern N5).
func (h *HealthChecker) SetPostgresDSN(dsn string) {
	h.pgDSN = dsn
}

// SetPostgresTLSDetector injects the DSN parser used by the postgres /readyz
// probe to populate the `tls` field. The detector is a function rather than
// an interface — the contract is a single pure parse-and-classify call with
// no state. Bootstrap wires the real parser from
// internal/bootstrap/tls_detection.go; tests may inject a stub to drive
// specific branches without crafting a full DSN.
func (h *HealthChecker) SetPostgresTLSDetector(detector func(dsn string) (bool, error)) {
	h.pgTLSDetector = detector
}

// SetReadyzRecorder wires the OTel-backed metrics sink for the /readyz probe.
// Called once from bootstrap after the Prometheus-bridged factory has been
// constructed (see observability.NewPrometheusBackedFactory). A nil recorder
// is treated as a no-op — the probe still runs end-to-end, only the metric
// emission is skipped. There is no concurrency concern: bootstrap sets this
// before the HTTP server starts accepting traffic.
func (h *HealthChecker) SetReadyzRecorder(r *observability.Recorder) {
	if h == nil {
		return
	}

	h.readyzRecorder = r
}

// ReadyzRecorder exposes the wired metrics recorder so the bootstrap-time
// self-probe can emit selfprobe_result through the same factory the
// /readyz handler uses. Returns nil until SetReadyzRecorder has been called.
func (h *HealthChecker) ReadyzRecorder() *observability.Recorder {
	if h == nil {
		return nil
	}

	return h.readyzRecorder
}

// PostgresProvider exposes the wired PostgresDBProvider so bootstrap-time
// adapters (e.g. the startup self-probe) can reuse the same provider that
// the per-request /readyz probe uses. Returns nil when no provider was
// supplied at construction time.
//
// Read-only accessor — the provider is set once at NewHealthChecker time
// and never mutated afterwards.
func (h *HealthChecker) PostgresProvider() PostgresDBProvider {
	if h == nil {
		return nil
	}

	return h.dbProvider
}

// CacheHealthProvider exposes the wired RuleCacheHealthProvider for the same
// reason as PostgresProvider — it lets the bootstrap-time self-probe reuse
// the per-request /readyz probe's source of truth. Returns nil until
// SetCacheHealthProvider has been called.
func (h *HealthChecker) CacheHealthProvider() RuleCacheHealthProvider {
	if h == nil {
		return nil
	}

	return h.cacheHealth
}

// MarkDraining flips the drain sentinel. Once set, /readyz returns 503 with
// draining=true regardless of dep health, so K8s removes the pod from the
// service endpoints before shutdown completes. Reserved for Gate 7
// (SIGTERM wiring); Gate 2 only exposes the field + method + handler branch.
func (h *HealthChecker) MarkDraining() {
	h.draining.Store(true)
}

// IsDraining reports whether MarkDraining() has been invoked. Exposed for
// tests and for the /readyz handler short-circuit.
func (h *HealthChecker) IsDraining() bool {
	return h.draining.Load()
}

// SetMultiTenantEnabled flips the MT mode flag. Bootstrap calls this once
// during construction (immediately after NewHealthChecker) so the /readyz
// handler knows whether to gate the rule_cache probe (H1).
//
// In MT mode the global cache is intentionally NOT warmed and the empty-tenant
// bucket is evicted at boot — see conditionalWarmUpCache. The K8s probe runs
// without a tenant context, so probing the global cache would always report
// "down" and crashloop the pod. With this flag set, the probe instead returns
// Status=StatusNA so /readyz keeps reflecting the (still-meaningful) postgres
// dependency without polluting the cache lane.
//
// Per-tenant cache health is surfaced via the tenant_consumers_active and
// tenant_connections_* metrics, NOT /readyz.
func (h *HealthChecker) SetMultiTenantEnabled(enabled bool) {
	if h == nil {
		return
	}

	h.multiTenantEnabled = enabled
}

// IsMultiTenantEnabled reports whether the checker is configured for MT mode.
// Exposed for tests (and for the readyz handler's MT gate) — production code
// reads it indirectly via the probe-skip branch in probeReadyzRuleCache.
func (h *HealthChecker) IsMultiTenantEnabled() bool {
	if h == nil {
		return false
	}

	return h.multiTenantEnabled
}

// SetCacheStalenessThreshold overrides the default 5-minute threshold used
// by the rule_cache /readyz probe to flip "ready but stale" into "degraded".
// Bootstrap calls this with the operator-tunable value derived from
// READYZ_CACHE_STALENESS_THRESHOLD_SECONDS (H3); a non-positive value is
// ignored and the default is preserved.
//
// Sized to outlive realistic Postgres failovers (default 5 min) so a brief DB
// hiccup does not flip every pod's /readyz to 503 and trigger a fleet-wide
// blast radius — the trade-off is that operators with a tighter SLO can
// shorten the window via env var without a code change.
func (h *HealthChecker) SetCacheStalenessThreshold(threshold time.Duration) {
	if h == nil || threshold <= 0 {
		return
	}

	h.cacheStalenessThreshold = threshold
}

// CacheStalenessThreshold returns the currently configured rule_cache
// staleness threshold. Exposed so tests can assert overrides land, and so
// SREs can read the live value via a debug endpoint if added later.
func (h *HealthChecker) CacheStalenessThreshold() time.Duration {
	if h == nil {
		return 0
	}

	return h.cacheStalenessThreshold
}
