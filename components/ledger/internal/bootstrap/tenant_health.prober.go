// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/bxcodec/dbresolver/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const (
	defaultTenantHealthProbeInterval   = 60 * time.Second
	defaultTenantHealthProbeTimeout    = 2 * time.Second
	defaultTenantHealthProbePassBudget = 30 * time.Second

	// tenantHealthProbeParallelism caps the tenants probed at the same time.
	tenantHealthProbeParallelism = 4

	// Dependency label values. The PostgreSQL names match the readiness checks.
	tenantDependencyPostgresOnboarding  = "postgres_onboarding"
	tenantDependencyPostgresTransaction = "postgres_transaction"
	tenantDependencyMongo               = "mongo"

	// Pass-failure reasons.
	tenantHealthProbeReasonTenantList = "tenant_list"
	tenantHealthProbeReasonLease      = "lease"
	tenantHealthProbeReasonBudget     = "budget"
)

// TenantHealthProbeConfig is the effective prober configuration.
type TenantHealthProbeConfig struct {
	Interval   time.Duration
	Timeout    time.Duration
	PassBudget time.Duration
}

// tenantMongoResolver is satisfied by *tmmongo.Manager.
type tenantMongoResolver interface {
	GetConnection(ctx context.Context, tenantID string) (*mongo.Client, error)
}

// namedPGResolver is one per-tenant PostgreSQL dependency, in registration order.
type namedPGResolver struct {
	dependency string
	resolver   tenantPGResolver
}

// TenantHealthProber probes, in the background and on the leader pod only, each
// active tenant's databases: one SELECT 1 per registered PostgreSQL module and
// one ping of the tenant's MongoDB cluster. Every probe publishes the
// tenant_dependency_up gauge, so a tenant whose database is gone is visible
// within one interval even when it has no traffic. The readiness endpoint
// deliberately does not reflect per-tenant health: a failing tenant there would
// take the pod out of rotation for every tenant. Gauge series are per pod and a
// former leader's keep their last value until restart, so every tick also
// publishes tenant_health_probe_leader to select the current leader's series.
type TenantHealthProber struct {
	logger        libLog.Logger
	lease         reconcilerLease
	cfg           TenantHealthProbeConfig
	tenants       tenantIDSource
	tenantLister  activeTenantLister
	tenantService string
	postgres      []namedPGResolver
	mongo         tenantMongoResolver
	metrics       *metrics.MetricsFactory
	// tracer is the service tracer put on the root context; nil keeps the default scope.
	tracer oteltrace.Tracer

	clock       func() time.Time
	newTicker   func(time.Duration) (<-chan time.Time, func())
	parallelism int
	pingPG      func(ctx context.Context, db dbresolver.DB) error
	pingMongo   func(ctx context.Context, client *mongo.Client) error

	// mu guards last: probes of different tenants report concurrently.
	mu sync.Mutex
	// last holds the outcome of the previous probe of each tenant dependency,
	// keyed by tenantID + "/" + dependency, to log a recovery once.
	last map[string]bool
	// pending holds the tenants a spent budget left for the next pass. Only the
	// pass goroutine touches it.
	pending []string
}

// NewTenantHealthProber builds a prober that probes nothing until tenant
// sources and dependencies are registered. Non-positive durations fall back to
// their defaults.
func NewTenantHealthProber(logger libLog.Logger, lease reconcilerLease, cfg TenantHealthProbeConfig) *TenantHealthProber {
	if cfg.Interval <= 0 {
		cfg.Interval = defaultTenantHealthProbeInterval
	}

	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTenantHealthProbeTimeout
	}

	if cfg.PassBudget <= 0 {
		cfg.PassBudget = defaultTenantHealthProbePassBudget
	}

	return &TenantHealthProber{
		logger: logger,
		lease:  lease,
		cfg:    cfg,
		clock:  time.Now,
		newTicker: func(interval time.Duration) (<-chan time.Time, func()) {
			ticker := time.NewTicker(interval)
			return ticker.C, ticker.Stop
		},
		parallelism: tenantHealthProbeParallelism,
		pingPG:      defaultTenantPGPing,
		pingMongo:   defaultTenantMongoPing,
		last:        map[string]bool{},
	}
}

// WithTenantCache sets the tenants with recent traffic as the tenant source, or
// as the fallback when an active-tenant lister is also set. nil is ignored.
func (p *TenantHealthProber) WithTenantCache(cache tenantIDSource) *TenantHealthProber {
	if cache != nil {
		p.tenants = cache
	}

	return p
}

// WithActiveTenantLister makes the tenant manager's active tenants of service the
// authoritative list. A failed listing falls back to the tenant cache. nil is ignored.
func (p *TenantHealthProber) WithActiveTenantLister(lister activeTenantLister, service string) *TenantHealthProber {
	if lister != nil {
		p.tenantLister = lister
		p.tenantService = service
	}

	return p
}

// WithPostgres registers one per-tenant PostgreSQL dependency. Only the
// postgres_onboarding and postgres_transaction names are accepted, which keeps
// the dependency label bounded; a nil resolver or any other name is ignored.
func (p *TenantHealthProber) WithPostgres(dependency string, resolver tenantPGResolver) *TenantHealthProber {
	if resolver == nil {
		return p
	}

	if dependency != tenantDependencyPostgresOnboarding && dependency != tenantDependencyPostgresTransaction {
		p.logger.Log(context.Background(), libLog.LevelWarn, "Tenant health probe ignored an unknown PostgreSQL dependency",
			libLog.String("dependency", dependency))

		return p
	}

	for _, registered := range p.postgres {
		if registered.dependency == dependency {
			return p
		}
	}

	p.postgres = append(p.postgres, namedPGResolver{dependency: dependency, resolver: resolver})

	return p
}

// WithMongo registers the resolver whose client pings each tenant's MongoDB
// cluster once per pass. nil is ignored.
func (p *TenantHealthProber) WithMongo(resolver tenantMongoResolver) *TenantHealthProber {
	if resolver != nil {
		p.mongo = resolver
	}

	return p
}

// WithMetricsFactory sets the factory for the probe gauge and counters. A nil
// factory disables them.
func (p *TenantHealthProber) WithMetricsFactory(factory *metrics.MetricsFactory) *TenantHealthProber {
	p.metrics = factory

	return p
}

// WithServiceTracer sets the tracer the prober's spans are opened with, so they carry the
// ledger's instrumentation scope. A nil tracer keeps the lib-observability default.
func (p *TenantHealthProber) WithServiceTracer(tracer oteltrace.Tracer) *TenantHealthProber {
	if p == nil {
		return nil
	}

	p.tracer = tracer

	return p
}

// rootContext is the context the loop starts from: empty except for the service tracer.
func (p *TenantHealthProber) rootContext() context.Context {
	return contextWithServiceTracer(context.Background(), p.tracer)
}

// Run executes a pass immediately and then one per interval until SIGTERM/SIGINT.
// The Launcher parameter is unused, as for the other Midaz workers.
func (p *TenantHealthProber) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(p.rootContext(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	p.logger.Log(ctx, libLog.LevelInfo, "Tenant health prober started",
		libLog.Any("interval_ms", p.cfg.Interval.Milliseconds()),
		libLog.Any("timeout_ms", p.cfg.Timeout.Milliseconds()),
		libLog.Any("pass_budget_ms", p.cfg.PassBudget.Milliseconds()))

	p.run(ctx)

	p.logger.Log(ctx, libLog.LevelInfo, "Tenant health prober shutting down")

	return nil
}

// run executes passes until ctx ends, then gives leadership up once.
func (p *TenantHealthProber) run(ctx context.Context) {
	ticks, stopTicks := p.newTicker(p.cfg.Interval)
	defer stopTicks()
	defer p.lease.Release(ctx)

	for {
		p.pass(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}

// pass runs one probe pass. A panic outside a probe is recovered and logged, and
// the loop keeps running.
func (p *TenantHealthProber) pass(ctx context.Context) {
	defer libRuntime.RecoverWithPolicyAndContext(ctx, p.logger, "ledger", "tenant_health.pass", libRuntime.KeepRunning)

	if ctx.Err() != nil {
		return
	}

	held, fresh, err := p.lease.Hold(ctx)

	p.setLeader(ctx, err == nil && held)

	if err != nil {
		if ctx.Err() != nil {
			return
		}

		p.logger.Log(ctx, libLog.LevelWarn, "Failed to hold tenant health probe lease", libLog.Err(err))
		p.countPassFailure(ctx, tenantHealthProbeReasonLease)

		return
	}

	if !held {
		p.logger.Log(ctx, libLog.LevelDebug, "Another pod leads the tenant health probe; skipping pass")
		return
	}

	if fresh {
		// Another pod may have led meanwhile: outcomes from an earlier tenure are stale.
		p.mu.Lock()
		p.last = map[string]bool{}
		p.mu.Unlock()

		p.pending = nil
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tenant_health.pass")
	defer span.End()

	listed, authoritative := p.tenantIDs(ctx)
	order := dedupTenantIDs(append(append([]string(nil), p.pending...), listed...))

	if len(order) == 0 {
		p.logger.Log(ctx, libLog.LevelDebug, "Tenant health probe found no tenants to probe")
	}

	probed, remaining := p.probeTenants(ctx, order)

	if ctx.Err() != nil {
		return
	}

	p.pending = remaining

	if len(remaining) > 0 {
		p.logger.Log(ctx, libLog.LevelWarn, "tenant health probe pass budget exhausted",
			libLog.Int("probed", probed), libLog.Int("remaining", len(remaining)))
		p.countPassFailure(ctx, tenantHealthProbeReasonBudget)
	}

	if authoritative {
		p.forgetAbsentTenants(listed, remaining)
	}

	span.SetAttributes(
		attribute.Int("app.tenant_health.tenants", len(order)),
		attribute.Int("app.tenant_health.probed", probed),
		attribute.Int("app.tenant_health.remaining", len(remaining)),
	)
}

// probeTenants probes each tenant in order, a bounded number at a time, until the
// pass budget ends. It returns how many tenants started and the ones that did not.
func (p *TenantHealthProber) probeTenants(ctx context.Context, order []string) (int, []string) {
	parallelism := p.parallelism
	if parallelism <= 0 {
		parallelism = tenantHealthProbeParallelism
	}

	budgetEnds := p.clock().Add(p.cfg.PassBudget)
	slots := make(chan struct{}, parallelism)

	var wg sync.WaitGroup

	probed := 0

	var remaining []string

	for index, tenantID := range order {
		slots <- struct{}{}

		if ctx.Err() != nil {
			<-slots
			break
		}

		if !p.clock().Before(budgetEnds) {
			<-slots

			remaining = append([]string(nil), order[index:]...)

			break
		}

		probed++

		wg.Add(1)

		libRuntime.SafeGoWithContextAndComponent(ctx, p.logger, "ledger", "tenant_health.tenant", libRuntime.KeepRunning,
			func(ctx context.Context) {
				defer wg.Done()
				defer func() { <-slots }()

				p.probeTenant(ctx, tenantID)
			})
	}

	wg.Wait()

	return probed, remaining
}

// probeTenant probes every registered dependency of one tenant in turn. A
// failing dependency never stops the next one.
func (p *TenantHealthProber) probeTenant(ctx context.Context, tenantID string) {
	tenantCtx := tmcore.ContextWithTenantID(ctx, tenantID)

	for _, pg := range p.postgres {
		resolver := pg.resolver

		p.probeOne(tenantCtx, tenantID, pg.dependency, func(ctx context.Context) error {
			db, err := resolver.GetDB(ctx, tenantID)
			if err != nil {
				return fmt.Errorf("resolve tenant postgres: %w", err)
			}

			if db == nil {
				return errors.New("resolve tenant postgres: nil handle")
			}

			return p.pingPG(ctx, db)
		})
	}

	if p.mongo != nil {
		p.probeOne(tenantCtx, tenantID, tenantDependencyMongo, func(ctx context.Context) error {
			client, err := p.mongo.GetConnection(ctx, tenantID)
			if err != nil {
				return fmt.Errorf("resolve tenant mongodb: %w", err)
			}

			if client == nil {
				return errors.New("resolve tenant mongodb: nil client")
			}

			return p.pingMongo(ctx, client)
		})
	}
}

// probeOne runs one probe under its own timeout and records the outcome. The
// probe runs in its own goroutine so a resolver that ignores its context still
// ends the wait at the timeout; a panic inside it is recovered as a failure. A
// probe cut short by the end of the pass context records nothing.
func (p *TenantHealthProber) probeOne(ctx context.Context, tenantID, dependency string, probe func(context.Context) error) {
	if ctx.Err() != nil {
		return
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tenant_health.probe")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.tenant_id", tenantID),
		attribute.String("app.tenant_health.dependency", dependency),
	)

	probeCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	done := make(chan error, 1)

	libRuntime.SafeGoWithContextAndComponent(probeCtx, p.logger, "ledger", "tenant_health.probe", libRuntime.KeepRunning,
		func(probeCtx context.Context) {
			defer func() {
				if recovered := recover(); recovered != nil {
					libRuntime.HandlePanicValue(probeCtx, p.logger, recovered, "ledger", "tenant_health.probe")

					done <- fmt.Errorf("tenant dependency probe panicked: %v", recovered)
				}
			}()

			done <- probe(probeCtx)
		})

	var err error

	select {
	case err = <-done:
	case <-probeCtx.Done():
		err = probeCtx.Err()
	}

	if ctx.Err() != nil {
		return
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Tenant dependency probe failed", err)
	}

	p.record(ctx, tenantID, dependency, err)
}

// record publishes one probe outcome and logs it: an Error on every failure and
// an Info once when a dependency comes back.
func (p *TenantHealthProber) record(ctx context.Context, tenantID, dependency string, err error) {
	key := tenantID + "/" + dependency
	up := err == nil

	p.mu.Lock()
	wasUp, seen := p.last[key]
	p.last[key] = up
	p.mu.Unlock()

	labels := map[string]string{"tenant_id": tenantID, "dependency": dependency}

	if up {
		p.setUp(ctx, labels, 1)

		if seen && !wasUp {
			p.logger.Log(ctx, libLog.LevelInfo, "tenant dependency recovered",
				libLog.String("tenant_id", tenantID), libLog.String("dependency", dependency))
		}

		return
	}

	p.setUp(ctx, labels, 0)
	p.countProbeFailure(ctx, labels)
	p.logger.Log(ctx, libLog.LevelError, "tenant dependency probe failed",
		libLog.String("tenant_id", tenantID), libLog.String("dependency", dependency), libLog.Err(err))
}

// tenantIDs lists the tenants of this pass: the tenant manager's active tenants
// when a lister is set, else the cached ones. A failed listing is counted and
// logged and falls back to the cache. The second result is false for that
// fallback, which is only a subset of the active tenants.
func (p *TenantHealthProber) tenantIDs(ctx context.Context) ([]string, bool) {
	if p.tenantLister == nil {
		return p.cachedTenantIDs(), true
	}

	tenants, err := p.tenantLister.GetActiveTenantsByService(ctx, p.tenantService)
	if err != nil {
		if ctx.Err() == nil {
			p.logger.Log(ctx, libLog.LevelWarn, "tenant health probe could not list active tenants; probing cached tenants", libLog.Err(err))
			p.countPassFailure(ctx, tenantHealthProbeReasonTenantList)
		}

		return p.cachedTenantIDs(), false
	}

	ids := make([]string, 0, len(tenants))

	for _, tenant := range tenants {
		if tenant == nil || tenant.ID == "" {
			continue
		}

		ids = append(ids, tenant.ID)
	}

	return ids, true
}

func (p *TenantHealthProber) cachedTenantIDs() []string {
	if p.tenants == nil {
		return nil
	}

	return p.tenants.TenantIDs()
}

// forgetAbsentTenants drops the last outcome of tenants neither listed nor
// carried over. Their gauge series keep the last value until restart.
func (p *TenantHealthProber) forgetAbsentTenants(listed, carried []string) {
	keep := make(map[string]struct{}, len(listed)+len(carried))
	for _, tenantID := range listed {
		keep[tenantID] = struct{}{}
	}

	for _, tenantID := range carried {
		keep[tenantID] = struct{}{}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for key := range p.last {
		if _, ok := keep[tenantOfProbeKey(key)]; !ok {
			delete(p.last, key)
		}
	}
}

// tenantOfProbeKey returns the tenant part of a tenantID + "/" + dependency key.
// Dependency names hold no slash, so the last slash splits the key.
func tenantOfProbeKey(key string) string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '/' {
			return key[:i]
		}
	}

	return key
}

// dedupTenantIDs drops empty and repeated tenant IDs, keeping the first occurrence.
func dedupTenantIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	unique := make([]string, 0, len(ids))

	for _, id := range ids {
		if id == "" {
			continue
		}

		if _, ok := seen[id]; ok {
			continue
		}

		seen[id] = struct{}{}
		unique = append(unique, id)
	}

	return unique
}

// defaultTenantPGPing runs the same SELECT 1 the readiness checks use.
func defaultTenantPGPing(ctx context.Context, db dbresolver.DB) error {
	var one int

	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("SELECT 1 failed: %w", err)
	}

	return nil
}

// defaultTenantMongoPing pings the primary of the tenant's MongoDB cluster.
func defaultTenantMongoPing(ctx context.Context, client *mongo.Client) error {
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	return nil
}

func (p *TenantHealthProber) setUp(ctx context.Context, labels map[string]string, value int64) {
	if p.metrics == nil {
		return
	}

	gauge, err := p.metrics.Gauge(utils.TenantDependencyUp)
	if err != nil {
		p.logger.Log(ctx, libLog.LevelDebug, "Failed to create tenant dependency gauge", libLog.Err(err))
		return
	}

	if err := gauge.WithLabels(labels).Set(ctx, value); err != nil {
		p.logger.Log(ctx, libLog.LevelDebug, "Failed to emit tenant dependency gauge", libLog.Err(err))
	}
}

// setLeader publishes whether this pod held the lease on this tick. The
// tenant_dependency_up series of a former leader keep their last value, so this
// gauge is what selects the current leader's series.
func (p *TenantHealthProber) setLeader(ctx context.Context, leader bool) {
	if p.metrics == nil {
		return
	}

	gauge, err := p.metrics.Gauge(utils.TenantHealthProbeLeader)
	if err != nil {
		p.logger.Log(ctx, libLog.LevelDebug, "Failed to create tenant health probe leader gauge", libLog.Err(err))
		return
	}

	var value int64
	if leader {
		value = 1
	}

	if err := gauge.Set(ctx, value); err != nil {
		p.logger.Log(ctx, libLog.LevelDebug, "Failed to emit tenant health probe leader gauge", libLog.Err(err))
	}
}

func (p *TenantHealthProber) countProbeFailure(ctx context.Context, labels map[string]string) {
	p.addOne(ctx, utils.TenantDependencyProbeFailures, labels)
}

func (p *TenantHealthProber) countPassFailure(ctx context.Context, reason string) {
	p.addOne(ctx, utils.TenantHealthProbePassFailures, map[string]string{"reason": reason})
}

func (p *TenantHealthProber) addOne(ctx context.Context, metric metrics.Metric, labels map[string]string) {
	if p.metrics == nil {
		return
	}

	counter, err := p.metrics.Counter(metric)
	if err != nil {
		p.logger.Log(ctx, libLog.LevelDebug, "Failed to create tenant health probe counter",
			libLog.String("metric", metric.Name), libLog.Err(err))

		return
	}

	if err := counter.WithLabels(labels).AddOne(ctx); err != nil {
		p.logger.Log(ctx, libLog.LevelDebug, "Failed to emit tenant health probe counter",
			libLog.String("metric", metric.Name), libLog.Err(err))
	}
}
