// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"time"

	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmevent "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/event"
	tmpostgres "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/postgres"
	tmredis "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/redis"
	libLog "github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
)

// componentsMT bundles every object that the bootstrap builds for
// multi-tenant mode. Keeping them on a single struct makes it trivial to wire
// them into Service and to write unit tests for the wiring helper without
// standing up a real PostgreSQL pool.
//
// The /readyz cycle is single-tenant only — the redisClient and tmBaseURL
// fields used to surface the Tenant Manager + Pub/Sub readyz adapters were
// removed when the cycle scope was reduced to postgres + rule_cache.
type componentsMT struct {
	tmClient      *tmclient.Client
	pgManager     *tmpostgres.Manager
	supervisor    *workers.WorkerSupervisor
	eventListener *tenantListenerApp
}

// wiringDepsMT groups the pre-built dependencies the wiring helper
// needs. They come from the rest of the bootstrap (rule cache, repos, workers'
// config, etc.) and are treated as opaque inputs here.
//
// Compiler configuration (M4): callers provide EITHER a CELAdapter (preferred
// — the wiring helper wraps it in celCompilerAdapter internally) OR a direct
// Compiler (tests mock workers.ExpressionCompiler). Exactly one must be set.
type wiringDepsMT struct {
	SyncRepo   workers.RuleSyncRepository
	UsageRepo  workers.UsageCounterCleanupRepository
	CELAdapter *cel.Adapter
	// Compiler, if set, is used verbatim. Tests inject a mock here; production
	// callers should leave this nil and pass CELAdapter instead so the adapter's
	// lifecycle is owned by the wiring helper (M4).
	Compiler      workers.ExpressionCompiler
	SyncConfig    workers.RuleSyncWorkerConfig
	CleanupConfig workers.UsageCleanupWorkerConfig
	// CleanupWorkerEnabled controls whether the supervisor spawns per-tenant
	// cleanup workers. False ⇒ only sync workers run (H8). CleanupConfig is
	// ignored when this is false.
	CleanupWorkerEnabled bool
	CBTemplate           workers.CircuitBreakerTemplate
	// RuleCache + Clock are supplied via a small interface indirection so the
	// helper does not pull the bootstrap-level imports for them — see the
	// parameter struct in buildComponentsMT.
}

// buildComponentsMT assembles the tenant-manager client, PG manager,
// worker supervisor, and event listener/dispatcher for multi-tenant mode.
//
// The order mirrors lib-commons v4 semantics:
//
//  1. tenant-manager HTTP client (with circuit breaker + service API key)
//  2. Redis Pub/Sub client (validated with PING; nil-tolerant host means the
//     caller already validated cfg via ValidateMultiTenantConfig)
//  3. tenant-manager PostgreSQL pool manager (one pool per tenant, LRU-capped)
//  4. Worker supervisor (per-tenant sync+cleanup workers) — wired with the
//     tenant-manager client so InitialTenantSync can discover tenants at boot
//  5. EventDispatcher (tenant lifecycle event handler) with supervisor
//     callbacks — OnTenantAdded / OnTenantRemoved
//  6. TenantEventListener (Redis Pub/Sub consumer) driving the dispatcher
//  7. Launcher-compatible tenantListenerApp wrapper around the listener
//
// Returns a non-nil error if any step fails. Caller must call Close on the
// returned tmClient + pgManager (via Service.Shutdown) on service teardown.
func buildComponentsMT(
	cfg *Config,
	logger libLog.Logger,
	deps wiringDepsMT,
	supervisorExtras workers.WorkerSupervisorDeps,
) (*componentsMT, error) {
	if cfg == nil {
		return nil, fmt.Errorf("multi-tenant wiring: cfg is required")
	}

	if logger == nil {
		return nil, fmt.Errorf("multi-tenant wiring: logger is required")
	}

	if !cfg.MultiTenantEnabled {
		return nil, fmt.Errorf("multi-tenant wiring: must only be called when MULTI_TENANT_ENABLED=true")
	}

	// 1. Tenant Manager HTTP client options. The clientOpts assembly + the
	// http:// scheme validation/warning is extracted to keep buildComponentsMT
	// under the gocyclo budget; behavior is unchanged.
	clientOpts, err := buildTMClientOptions(cfg, logger)
	if err != nil {
		return nil, err
	}

	// M5: track every resource opened during wiring and close them in reverse
	// order if any subsequent step fails. Without this, pgManager (opened at
	// step 3) leaks when any of steps 4–7 fail, because only tmClient +
	// redisClient were previously rolled back.
	//
	// `success` flips to true ONLY after the final return statement captures a
	// complete components struct. Any panic or early-return fires the deferred
	// cleanup.
	var (
		success bool
		cleanup []func()
	)

	runCleanup := func() {
		for i := len(cleanup) - 1; i >= 0; i-- {
			cleanup[i]()
		}
	}

	defer func() {
		if !success {
			runCleanup()
		}
	}()

	tmClient, err := tmclient.NewClient(cfg.MultiTenantURL, logger, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("multi-tenant wiring: init tenant-manager client: %w", err)
	}

	cleanup = append(cleanup, func() { _ = tmClient.Close() })

	// 2. Redis Pub/Sub client (validated with PING).
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redisClient, err := tmredis.NewTenantPubSubRedisClient(pingCtx, tmredis.TenantPubSubRedisConfig{
		Host:     cfg.MultiTenantRedisHost,
		Port:     cfg.MultiTenantRedisPort,
		Password: cfg.MultiTenantRedisPassword,
		TLS:      cfg.MultiTenantRedisTLS,
	})
	if err != nil {
		return nil, fmt.Errorf("multi-tenant wiring: init tenant pubsub redis: %w", err)
	}

	cleanup = append(cleanup, func() { _ = redisClient.Close() })

	// Security warning: Redis Pub/Sub has no per-message authentication.
	// Any publisher on the channel can emit tenant.deleted events and trigger
	// cache eviction across all tracer pods. Without TLS, credentials +
	// message payloads also travel in cleartext. Operators MUST restrict
	// PUBLISH on tenant-events to the Tenant Manager identity via Redis ACL.
	// This banner logs once at boot so the mitigation step is impossible
	// to miss.
	if !cfg.MultiTenantRedisTLS {
		logger.With(
			libLog.String("config", "MULTI_TENANT_REDIS_TLS"),
			libLog.Any("tls", false),
		).Log(context.Background(), libLog.LevelWarn,
			"MULTI_TENANT_REDIS_TLS=false: Redis Pub/Sub credentials travel in cleartext and the "+
				"channel is unauthenticated. Restrict PUBLISH on tenant-events to the Tenant "+
				"Manager identity via Redis ACL.")
	}

	// 3. PostgreSQL per-tenant pool manager. Option assembly is extracted to
	// keep this function under the gocyclo budget.
	pgManager := tmpostgres.NewManager(
		tmClient,
		cfg.ApplicationName,
		buildPgManagerOptions(cfg, logger)...,
	)

	cleanup = append(cleanup, func() { _ = pgManager.Close(context.Background()) })

	// 4. Worker supervisor. Fill in the MT-sourced fields, leave caller-owned
	// fields (RuleCache, Clock, Logger, Service) from supervisorExtras. The
	// PoolResolver is built from pgManager so per-tenant workers can resolve
	// their tenant's Postgres pool each cycle and stash it onto the context
	// via tmcore.ContextWithPG — without it, per-tenant workers would poll
	// the root DB.
	poolResolver, err := workers.NewPoolResolver(pgManager)
	if err != nil {
		return nil, fmt.Errorf("multi-tenant wiring: init worker pool resolver: %w", err)
	}

	// M4: resolveCompiler builds the supervisor's workers.ExpressionCompiler
	// from either a CELAdapter (production) or a pre-built Compiler (tests).
	// Extracted to a helper to keep buildComponentsMT under the
	// gocyclo budget.
	mtCompiler, err := resolveCompiler(deps)
	if err != nil {
		return nil, err
	}

	// supervisorDeps assembly (MT-sourced field fill-in + MaxTenants/Service
	// defaulting) is extracted to keep buildComponentsMT under the gocyclo
	// budget; behavior is unchanged.
	supervisorDeps := buildSupervisorDeps(cfg, supervisorExtras, deps, mtCompiler, tmClient, poolResolver)

	supervisor, err := workers.NewWorkerSupervisor(supervisorDeps)
	if err != nil {
		return nil, fmt.Errorf("multi-tenant wiring: init worker supervisor: %w", err)
	}

	// 5. EventDispatcher wires lifecycle callbacks to the supervisor.
	dispatcher := tmevent.NewEventDispatcher(
		nil, // cache: dispatcher creates a safe default when nil
		nil, // loader: lazy-load unused by tracer (supervisor handles tenant onboarding)
		cfg.ApplicationName,
		tmevent.WithPostgres(pgManager),
		tmevent.WithDispatcherLogger(logger),
		tmevent.WithOnTenantAdded(func(ctx context.Context, tenantID string) {
			if err := supervisor.EnsureWorkers(ctx, tenantID); err != nil {
				logger.With(
					libLog.String("operation", "tenant_event.on_added"),
					libLog.String("tenant_id", tenantID),
					libLog.String("error.message", err.Error()),
				).Log(ctx, libLog.LevelError, "failed to spawn workers for tenant on event")
			}
		}),
		tmevent.WithOnTenantRemoved(func(_ context.Context, tenantID string) {
			// StopWorkers is deliberately context-less: it blocks until the
			// tenant's background goroutines have exited, which is a shutdown
			// step that must not be cut short by a cancelled event context.
			//nolint:contextcheck
			supervisor.StopWorkers(tenantID)
		}),
	)

	// 6. TenantEventListener consumes Redis Pub/Sub messages.
	listener, err := tmevent.NewTenantEventListener(
		redisClient,
		dispatcher.HandleEvent,
		tmevent.WithListenerLogger(logger),
		tmevent.WithService(cfg.ApplicationName),
	)
	if err != nil {
		return nil, fmt.Errorf("multi-tenant wiring: init tenant event listener: %w", err)
	}

	// 7. Launcher-compatible listener wrapper.
	listenerApp, err := newTenantListenerApp(listener, logger)
	if err != nil {
		return nil, fmt.Errorf("multi-tenant wiring: wrap tenant event listener: %w", err)
	}

	// All steps succeeded — flip the sentinel so the deferred cleanup no-ops.
	success = true

	return &componentsMT{
		tmClient:      tmClient,
		pgManager:     pgManager,
		supervisor:    supervisor,
		eventListener: listenerApp,
	}, nil
}

// buildPgManagerOptions assembles the option list for tmpostgres.NewManager.
//
// MaxOpenConns / MaxIdleConns are forwarded only when explicitly configured —
// passing 0 to lib-commons' WithMaxOpenConns is treated as "unbounded" and
// would override its sensible fallback (25/5). Production leaves both env
// vars unset; integration tests pin them to small values so cumulative
// connections across repeated reboots stay below testcontainer
// max_connections=100. Extracted so buildComponentsMT stays under
// the gocyclo budget.
func buildPgManagerOptions(cfg *Config, logger libLog.Logger) []tmpostgres.Option {
	opts := []tmpostgres.Option{
		tmpostgres.WithLogger(logger),
		tmpostgres.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools),
		tmpostgres.WithIdleTimeout(time.Duration(cfg.MultiTenantIdleTimeoutSec) * time.Second),
		tmpostgres.WithConnectionsCheckInterval(time.Duration(cfg.MultiTenantConnectionsCheckIntervalSec) * time.Second),
	}

	if cfg.MultiTenantMaxOpenConnsPerTenant > 0 {
		opts = append(opts, tmpostgres.WithMaxOpenConns(cfg.MultiTenantMaxOpenConnsPerTenant))
	}

	if cfg.MultiTenantMaxIdleConnsPerTenant > 0 {
		opts = append(opts, tmpostgres.WithMaxIdleConns(cfg.MultiTenantMaxIdleConnsPerTenant))
	}

	return opts
}

// resolveCompiler returns the workers.ExpressionCompiler the supervisor will
// use. Production callers pass a CELAdapter; tests pass a pre-built mock
// Compiler. Exactly one must be set (M4).
func resolveCompiler(deps wiringDepsMT) (workers.ExpressionCompiler, error) {
	switch {
	case deps.Compiler != nil && deps.CELAdapter != nil:
		return nil, fmt.Errorf("multi-tenant wiring: set exactly one of Compiler or CELAdapter, not both")
	case deps.Compiler != nil:
		return deps.Compiler, nil
	case deps.CELAdapter != nil:
		return &celCompilerAdapter{adapter: deps.CELAdapter}, nil
	default:
		return nil, fmt.Errorf("multi-tenant wiring: Compiler or CELAdapter is required")
	}
}

// buildTMClientOptions assembles the tenant-manager HTTP client options and
// enforces the http:// scheme policy (cleartext requires explicit opt-in,
// emitting a one-time security warning). Extracted from buildComponentsMT to
// keep it under the gocyclo budget; behavior is unchanged.
//
// lib-commons v4 enforces HTTPS by default. When the operator has explicitly
// configured an http:// URL (typical in dev/test clusters where the Tenant
// Manager is reachable only on a private network), mirror that choice by
// opting in to WithAllowInsecureHTTP. Production deployments MUST configure an
// https:// URL in MULTI_TENANT_URL.
func buildTMClientOptions(cfg *Config, logger libLog.Logger) ([]tmclient.ClientOption, error) {
	clientOpts := []tmclient.ClientOption{
		tmclient.WithServiceAPIKey(cfg.MultiTenantServiceAPIKey),
		tmclient.WithCircuitBreaker(
			cfg.MultiTenantCircuitBreakerThreshold,
			time.Duration(cfg.MultiTenantCircuitBreakerTimeoutSec)*time.Second,
		),
		tmclient.WithTimeout(time.Duration(cfg.MultiTenantTimeout) * time.Second),
		// MULTI_TENANT_CACHE_TTL_SEC (default 120s) controls how long the
		// tenant-manager HTTP client caches /settings responses. Without this
		// option lib-commons defaults to 1h, which would silently ignore the
		// operator-configured value.
		tmclient.WithCacheTTL(time.Duration(cfg.MultiTenantCacheTTLSec) * time.Second),
	}

	if strings.HasPrefix(cfg.MultiTenantURL, "http://") {
		if !cfg.MultiTenantAllowInsecureHTTP {
			return nil, fmt.Errorf(
				"multi-tenant config: MULTI_TENANT_URL uses http:// but MULTI_TENANT_ALLOW_INSECURE_HTTP is false. " +
					"Set MULTI_TENANT_ALLOW_INSECURE_HTTP=true to explicitly opt in (NOT for production — " +
					"credentials travel in cleartext)")
		}

		logger.With(
			libLog.String("config", "MULTI_TENANT_URL"),
			libLog.String("scheme", "http"),
		).Log(context.Background(), libLog.LevelWarn,
			"SECURITY: MULTI_TENANT_URL uses cleartext HTTP — service API key and tenant credentials travel in plaintext")

		clientOpts = append(clientOpts, tmclient.WithAllowInsecureHTTP())
	}

	return clientOpts, nil
}

// buildSupervisorDeps fills the MT-sourced fields onto the caller-provided
// supervisorExtras and applies MaxTenants/Service defaults. Extracted from
// buildComponentsMT to keep it under the gocyclo budget; behavior is
// unchanged.
func buildSupervisorDeps(
	cfg *Config,
	supervisorExtras workers.WorkerSupervisorDeps,
	deps wiringDepsMT,
	mtCompiler workers.ExpressionCompiler,
	tmClient *tmclient.Client,
	poolResolver workers.WorkerPoolResolver,
) workers.WorkerSupervisorDeps {
	supervisorDeps := supervisorExtras
	supervisorDeps.SyncRepo = deps.SyncRepo
	supervisorDeps.UsageRepo = deps.UsageRepo
	supervisorDeps.Compiler = mtCompiler
	supervisorDeps.SyncConfig = deps.SyncConfig
	supervisorDeps.CleanupConfig = deps.CleanupConfig
	supervisorDeps.CleanupWorkerEnabled = deps.CleanupWorkerEnabled
	supervisorDeps.CBTemplate = deps.CBTemplate
	supervisorDeps.TenantList = tmClient
	supervisorDeps.PoolResolver = poolResolver

	if supervisorDeps.MaxTenants == 0 {
		supervisorDeps.MaxTenants = cfg.MultiTenantMaxTenantPools
	}

	if supervisorDeps.Service == "" {
		supervisorDeps.Service = cfg.ApplicationName
	}

	return supervisorDeps
}
