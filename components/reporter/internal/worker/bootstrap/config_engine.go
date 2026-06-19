// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	reporterEngine "github.com/LerianStudio/midaz/v4/pkg/reporter/engine"
	libRedis "github.com/LerianStudio/midaz/v4/pkg/reporter/redis"

	fetcherEngine "github.com/LerianStudio/fetcher/pkg/engine"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/postgres"
	clog "github.com/LerianStudio/lib-observability/log"
	"github.com/bxcodec/dbresolver/v2"
	libMongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/trace"
)

// defaultInProcessSchemaCacheTTL is the fallback TTL for the in-process schema
// cache when no positive TTL is configured. Schema is static between migrations,
// so a long TTL is safe — a stale entry only survives until the next worker
// restart or the next migration-driven schema change.
const defaultInProcessSchemaCacheTTL = 10 * time.Minute

// initWorkerEngine constructs the embedded extraction engine and wires it onto
// the worker's UseCase. It is called from initWorkerDependencies AFTER the
// tracer, datasources, and circuit-breaker manager are built. A non-nil error is
// a HARD bootstrap failure: engine.New validates its required ports at
// construction (returning an *EngineError on a nil/typed-nil registry), so a
// wiring miss aborts startup instead of deferring to a nil-pointer panic at the
// first extraction.
//
// It deliberately does NOT pass WithCredentialProtector and does NOT call
// WithEncryptedPersistence(true): the reporter's datasource credentials come
// from env/secret-manager straight to the connectors, and nothing is
// encrypted-at-rest in the reporter's Mongo, so there is no credential to
// protect at the engine boundary.
//
// T07 decision (ExecutionStore deferred): WithExecutionStore is intentionally
// omitted. The reporter already persists report status in its own Mongo
// (ReportDataRepo); a second engine-side execution store would duplicate that
// state. If durable engine-side execution state is wanted later, add a
// SaveExecution-only adapter over the reporter Mongo and pass WithExecutionStore.
func initWorkerEngine(
	cfg *Config,
	logger clog.Logger,
	tracer trace.Tracer,
	externalDataSources *pkg.SafeDataSources,
	circuitBreaker pkg.CircuitBreakerExecutor,
	tenantMongoManager *tmmongo.Manager,
	tenantPostgresManager *tmpostgres.Manager,
	redisRepo libRedis.RedisRepository,
) (*fetcherEngine.Engine, error) {
	resolver := buildEngineResolver(cfg, externalDataSources, tenantMongoManager, tenantPostgresManager)

	registry := reporterEngine.NewRegistry(resolver, circuitBreaker)
	store := reporterEngine.NewConnectionStore(newDatasourceLookup(externalDataSources))
	observability := reporterEngine.NewObservability(tracer)

	opts := []fetcherEngine.Option{
		fetcherEngine.WithConnectorRegistry(registry),
		fetcherEngine.WithConnectionStore(store),
		fetcherEngine.WithObservability(observability),
		fetcherEngine.WithLimits(engineLimits(cfg)),
	}

	// SchemaCache wiring. Database schema is static between migrations, so a
	// cached snapshot spares every report a full information_schema scan (and the
	// per-collection mongo field sampling). A Redis-backed cache is preferred when
	// available (it shares the reconciler's Redis and survives process restarts);
	// otherwise an in-process TTL cache keeps the schema hot for the lifetime of
	// this worker. Either way a cache miss degrades to fresh discovery, so the
	// cache is never load-bearing for correctness.
	schemaCacheTTL := time.Duration(cfg.MultiTenantIdleTimeoutSec) * time.Second
	if schemaCacheTTL <= 0 {
		schemaCacheTTL = defaultInProcessSchemaCacheTTL
	}

	if redisRepo != nil {
		opts = append(opts, fetcherEngine.WithSchemaCache(
			reporterEngine.NewSchemaCache(redisRepo, schemaCacheTTL),
		))
	} else {
		opts = append(opts, fetcherEngine.WithSchemaCache(
			newInProcessSchemaCache(schemaCacheTTL),
		))
	}

	engine, err := fetcherEngine.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to construct embedded extraction engine: %w", err)
	}

	logger.Log(context.Background(), clog.LevelInfo, "Embedded extraction engine constructed",
		clog.Bool("multi_tenant", resolver.IsMultiTenant()),
		clog.Bool("schema_cache", true),
		clog.Bool("schema_cache_redis_backed", redisRepo != nil))

	return engine, nil
}

// buildEngineResolver selects the tenant resolution strategy. Multi-tenant mode
// resolves database-per-tenant via the lib-commons tenant managers; single-tenant
// mode resolves the stable env-configured datasource pools.
func buildEngineResolver(
	cfg *Config,
	externalDataSources *pkg.SafeDataSources,
	tenantMongoManager *tmmongo.Manager,
	tenantPostgresManager *tmpostgres.Manager,
) reporterEngine.TenantResolver {
	if cfg.MultiTenantEnabled {
		return reporterEngine.NewMultiTenantResolver(
			newTenantPostgresAdapter(tenantPostgresManager),
			tenantMongoManager,
			schemaListFunc(externalDataSources),
		)
	}

	return reporterEngine.NewSingleTenantResolver(newSingleTenantDatasources(externalDataSources))
}

// engineLimits maps the worker config onto the engine's configured Limits — the
// bounds the engine enforces on every extraction. Each field falls back to the
// engine DefaultLimits value when its config value is non-positive, so a
// partially-configured deployment still runs bounded. These are the engine-level
// ceilings, distinct from Limits.Effective which validates per-request overrides
// against them.
func engineLimits(cfg *Config) fetcherEngine.Limits {
	defaults := fetcherEngine.DefaultLimits()

	limits := defaults

	if cfg.EngineMaxDatasources > 0 {
		limits.MaxDatasources = cfg.EngineMaxDatasources
	}

	if cfg.EngineMaxTablesPerDatasource > 0 {
		limits.MaxTablesPerDatasource = cfg.EngineMaxTablesPerDatasource
	}

	if cfg.EngineMaxFieldsPerTable > 0 {
		limits.MaxFieldsPerTable = cfg.EngineMaxFieldsPerTable
	}

	if cfg.EngineMaxConcurrency > 0 {
		limits.MaxConcurrency = cfg.EngineMaxConcurrency
	}

	if cfg.EngineTimeoutSec > 0 {
		limits.Timeout = time.Duration(cfg.EngineTimeoutSec) * time.Second
	}

	if cfg.EngineMaxResultBytes > 0 {
		limits.MaxResultBytes = cfg.EngineMaxResultBytes
	}

	return limits
}

// schemaListFunc resolves a datasource's configured PostgreSQL schema list from
// the env-configured datasource map. In database-per-tenant mode the schema set
// is env-derived and identical across tenants, so it is resolved by config name,
// not by tenant.
func schemaListFunc(externalDataSources *pkg.SafeDataSources) func(configName string) []string {
	return func(configName string) []string {
		ds, ok := externalDataSources.Get(configName)
		if !ok || len(ds.Schemas) == 0 {
			return []string{"public"}
		}

		return ds.Schemas
	}
}

// tenantPGManager is the narrow seam this package needs from the lib-commons
// tenant-manager/postgres.Manager: resolve the per-tenant dbresolver.DB for a
// tenant ID. *tmpostgres.Manager satisfies it. Declaring the interface here
// keeps tenantPostgresAdapter unit-testable without standing up a real manager.
type tenantPGManager interface {
	GetDB(ctx context.Context, tenantID string) (dbresolver.DB, error)
}

// Compile-time check: the real lib-commons manager satisfies the seam.
var _ tenantPGManager = (*tmpostgres.Manager)(nil)

// tenantPostgresAdapter bridges the lib-commons tenant-manager/postgres.Manager
// onto the engine's PostgresManager seam. It forwards GetDB verbatim: the tenant
// ID it is asked to resolve is the tenant ID it passes to the manager, and the
// manager's per-tenant dbresolver.DB (which satisfies engine.SQLQuerier) is
// returned unchanged.
//
// CRITICAL ISOLATION INVARIANT: a request for tenant A resolves ONLY tenant A's
// per-tenant pool. The adapter introduces NO fallback — on a manager error it
// propagates the error (fail closed for that request) and NEVER substitutes a
// shared single-tenant pool or any other tenant's connection. The lib-commons
// manager owns per-tenant pool isolation; this adapter must not weaken it.
type tenantPostgresAdapter struct {
	manager tenantPGManager
}

// newTenantPostgresAdapter wraps the lib-commons tenant PostgreSQL manager as an
// engine.PostgresManager.
func newTenantPostgresAdapter(manager tenantPGManager) reporterEngine.PostgresManager {
	return &tenantPostgresAdapter{manager: manager}
}

// GetDB forwards the tenant ID to the lib-commons manager and returns its
// per-tenant connection. On error it propagates the failure unchanged — it never
// falls back to a shared pool, so an MT-postgres extraction can only ever read
// its own tenant's database.
func (a *tenantPostgresAdapter) GetDB(ctx context.Context, tenantID string) (reporterEngine.SQLQuerier, error) {
	db, err := a.manager.GetDB(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Catch a nil dbresolver.DB returned without an error where the concrete
	// type is still visible. Once it is stored in the SQLQuerier return it is
	// boxed twice (dbresolver.DB inside SQLQuerier), so an empty interface here
	// stays non-nil downstream and would nil-deref on the first Ping/Query. The
	// resolver guards the same case at its seam; this is the earliest catch.
	if db == nil {
		return nil, fmt.Errorf("lib-commons tenant PostgreSQL manager returned a nil connection for tenant %q", tenantID)
	}

	return db, nil
}

// singleTenantDatasourcesAdapter bridges the reporter's SafeDataSources onto the
// engine's singleTenantDatasources seam (declared in pkg/reporter/engine). It
// resolves the configured datasource by config name and hands back the live,
// host-owned connection so the engine connector reuses the reporter's pool
// rather than opening its own. Declared in the bootstrap so the engine package
// does not import the bootstrap-heavy datasource map.
type singleTenantDatasourcesAdapter struct {
	datasources *pkg.SafeDataSources
}

// newSingleTenantDatasources builds the single-tenant datasource adapter.
func newSingleTenantDatasources(datasources *pkg.SafeDataSources) reporterEngine.SingleTenantDatasources {
	return &singleTenantDatasourcesAdapter{datasources: datasources}
}

// ResolvePostgres returns the connected *sql.DB and configured schema list for
// the named datasource. It errors when the datasource is missing, of the wrong
// type, or unavailable.
func (a *singleTenantDatasourcesAdapter) ResolvePostgres(_ context.Context, configName string) (*sql.DB, []string, error) {
	ds, ok := a.datasources.Get(configName)
	if !ok {
		return nil, nil, reporterEngine.NewEngineValidationError("datasource not found: " + configName)
	}

	if ds.DatabaseType != pkg.PostgreSQLType {
		return nil, nil, reporterEngine.NewEngineValidationError("datasource is not postgresql: " + configName)
	}

	if ds.DatabaseConfig == nil {
		return nil, nil, reporterEngine.NewEngineUnavailableError("postgresql datasource has no connection: "+configName, nil)
	}

	db, err := ds.DatabaseConfig.GetDB()
	if err != nil {
		return nil, nil, reporterEngine.NewEngineUnavailableError("failed to resolve postgresql connection: "+configName, err)
	}

	schemas := ds.Schemas
	if len(schemas) == 0 {
		schemas = []string{"public"}
	}

	return db, schemas, nil
}

// ResolveMongo returns the connected mongo database for the named datasource. It
// errors when the datasource is missing, of the wrong type, or unavailable.
func (a *singleTenantDatasourcesAdapter) ResolveMongo(ctx context.Context, configName string) (*libMongo.Database, error) {
	ds, ok := a.datasources.Get(configName)
	if !ok {
		return nil, reporterEngine.NewEngineValidationError("datasource not found: " + configName)
	}

	if ds.DatabaseType != pkg.MongoDBType {
		return nil, reporterEngine.NewEngineValidationError("datasource is not mongodb: " + configName)
	}

	repo, ok := ds.MongoDBRepository.(interface {
		GetDatabase(ctx context.Context) (*libMongo.Database, error)
	})
	if !ok || repo == nil {
		return nil, reporterEngine.NewEngineUnavailableError("mongodb datasource has no connection: "+configName, nil)
	}

	db, err := repo.GetDatabase(ctx)
	if err != nil {
		return nil, reporterEngine.NewEngineUnavailableError("failed to resolve mongodb database: "+configName, err)
	}

	return db, nil
}

// datasourceLookupAdapter bridges SafeDataSources onto the engine's
// datasourceLookup seam used by the ConnectionStore: it projects each datasource
// to its secret-free connection fields and enumerates the registered config
// names.
type datasourceLookupAdapter struct {
	datasources *pkg.SafeDataSources
}

// newDatasourceLookup builds the datasource lookup adapter for the
// ConnectionStore.
func newDatasourceLookup(datasources *pkg.SafeDataSources) reporterEngine.DatasourceLookup {
	return &datasourceLookupAdapter{datasources: datasources}
}

// LookupDatasource returns the secret-free connection projection for the named
// datasource. It never returns the password.
func (a *datasourceLookupAdapter) LookupDatasource(configName string) (reporterEngine.DatasourceConnection, bool) {
	ds, ok := a.datasources.Get(configName)
	if !ok {
		return reporterEngine.DatasourceConnection{}, false
	}

	conn := reporterEngine.DatasourceConnection{
		ConfigName:   configName,
		Type:         ds.DatabaseType,
		DatabaseName: databaseNameOf(ds),
		Schemas:      ds.Schemas,
	}

	return conn, true
}

// DatasourceConfigNames returns the config names of every registered datasource.
func (a *datasourceLookupAdapter) DatasourceConfigNames() []string {
	all := a.datasources.GetAll()

	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}

	return names
}

// databaseNameOf reads the configured database name off whichever connection a
// datasource carries. The reporter's DataSource stores the postgres database name
// on its connection config and the mongo database name on a dedicated field.
func databaseNameOf(ds pkg.DataSource) string {
	switch ds.DatabaseType {
	case pkg.PostgreSQLType:
		if ds.DatabaseConfig != nil {
			return ds.DatabaseConfig.DBName
		}
	case pkg.MongoDBType:
		return ds.MongoDBName
	}

	return ""
}

// inProcessSchemaCache is a minimal in-memory, TTL-bounded fetcher.SchemaCache
// used when no Redis-backed cache is wired. It spares each report a fresh
// information_schema scan / mongo field sampling by holding discovered snapshots
// for the worker's lifetime. Entries are keyed by tenant ID + datasource config
// name so a snapshot is never served across tenants; in single-tenant mode the
// tenant ID is empty and the key collapses to the config name. A miss (absent or
// expired) degrades to fresh discovery, so the cache is never load-bearing.
type inProcessSchemaCache struct {
	ttl time.Duration

	mu      sync.RWMutex
	entries map[string]inProcessSchemaEntry
}

// inProcessSchemaEntry is one cached snapshot and its expiry instant.
type inProcessSchemaEntry struct {
	snapshot  fetcherEngine.SchemaSnapshot
	expiresAt time.Time
}

// Compile-time check that inProcessSchemaCache satisfies the engine's optional
// SchemaCache port.
var _ fetcherEngine.SchemaCache = (*inProcessSchemaCache)(nil)

// newInProcessSchemaCache builds an in-process TTL schema cache. A non-positive
// ttl is normalized to defaultInProcessSchemaCacheTTL.
func newInProcessSchemaCache(ttl time.Duration) *inProcessSchemaCache {
	if ttl <= 0 {
		ttl = defaultInProcessSchemaCacheTTL
	}

	return &inProcessSchemaCache{
		ttl:     ttl,
		entries: make(map[string]inProcessSchemaEntry),
	}
}

// GetSchema returns the cached snapshot for the tenant+datasource pair. An absent
// or expired entry reports ok=false with a nil error so the engine falls back to
// fresh discovery.
func (c *inProcessSchemaCache) GetSchema(_ context.Context, tenant fetcherEngine.TenantContext, configName string) (fetcherEngine.SchemaSnapshot, bool, error) {
	key := inProcessSchemaKey(tenant.TenantID, configName)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return fetcherEngine.SchemaSnapshot{}, false, nil
	}

	return entry.snapshot, true, nil
}

// PutSchema stores the snapshot under the tenant-scoped key with the configured
// TTL. It never fails the caller.
func (c *inProcessSchemaCache) PutSchema(_ context.Context, tenant fetcherEngine.TenantContext, snapshot fetcherEngine.SchemaSnapshot) error {
	key := inProcessSchemaKey(tenant.TenantID, snapshot.ConfigName)

	c.mu.Lock()
	c.entries[key] = inProcessSchemaEntry{
		snapshot:  snapshot,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()

	return nil
}

// inProcessSchemaKey partitions the cache by tenant so no cross-tenant schema
// serving is structurally possible.
func inProcessSchemaKey(tenantID, configName string) string {
	return tenantID + ":" + configName
}
