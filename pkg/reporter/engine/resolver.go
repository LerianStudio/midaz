// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package engine adapts the midaz reporter's existing PostgreSQL and MongoDB
// database access onto the embedded fetcher extraction engine
// (github.com/LerianStudio/fetcher/pkg/engine) host ports. It provides a
// tenant-aware ConnectorRegistry/ConnectorFactory/Connector implementation that
// reuses the reporter's connection pools and circuit breakers, streaming
// extraction rows one at a time over pgx rows and the mongo driver cursor.
package engine

import (
	"context"
	"database/sql"

	libCommonsMongo "go.mongodb.org/mongo-driver/v2/mongo"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
)

// sqlQuerier is the minimal read surface a PostgreSQL connection must expose for
// streaming extraction. Both the single-tenant *sql.DB and the multi-tenant
// dbresolver.DB (lib-commons tenant-manager) satisfy it, so the connector code
// never depends on which provenance resolved the handle.
type sqlQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	PingContext(ctx context.Context) error
}

// Compile-time check: the stdlib *sql.DB satisfies sqlQuerier.
var _ sqlQuerier = (*sql.DB)(nil)

// postgresHandle is the resolved PostgreSQL connection for one tenant: the read
// surface plus the schema list the datasource is configured to expose. It
// carries no credentials.
type postgresHandle struct {
	db      sqlQuerier
	schemas []string
}

// mongoHandle is the resolved MongoDB database for one tenant.
type mongoHandle struct {
	db *libCommonsMongo.Database
}

// PostgresManager is the multi-tenant PostgreSQL resolution seam, satisfied by
// lib-commons tenant-manager/postgres.Manager. It returns the per-tenant
// dbresolver.DB read surface. Declaring the narrow interface here (rather than
// importing the concrete Manager) keeps this package unit-testable and inverts
// the dependency onto the lib-commons machinery the rest of midaz uses.
type PostgresManager interface {
	// GetDB returns the resolved dbresolver.DB for the tenant. The returned
	// value satisfies sqlQuerier.
	GetDB(ctx context.Context, tenantID string) (sqlQuerier, error)
}

// MongoManager is the multi-tenant MongoDB resolution seam, satisfied by
// lib-commons tenant-manager/mongo.Manager. It returns the per-tenant database
// handle resolved from the tenant config.
type MongoManager interface {
	GetDatabaseForTenant(ctx context.Context, tenantID string) (*libCommonsMongo.Database, error)
}

// TenantResolver resolves the correct per-tenant PostgreSQL or MongoDB handle
// for an extraction. It is the load-bearing seam of the migration's critical
// path: it moves multi-tenant DB resolution in-process, replacing the remote
// fetcher's per-tenant pooling.
//
// Two implementations exist: singleTenantResolver (the stable env-configured
// datasource, ignoring tenant identity) and multiTenantResolver (lib-commons
// tenant managers, resolving database-per-tenant from the tenant ID). The host
// picks one at bootstrap; the connector code is identical for both.
type TenantResolver interface {
	// ResolvePostgres returns the PostgreSQL handle for the given tenant and
	// datasource config name. tenantID is empty in single-tenant mode.
	ResolvePostgres(ctx context.Context, tenantID, configName string) (postgresHandle, error)
	// ResolveMongo returns the MongoDB handle for the given tenant and
	// datasource config name. tenantID is empty in single-tenant mode.
	ResolveMongo(ctx context.Context, tenantID, configName string) (mongoHandle, error)
	// IsMultiTenant reports whether tenant identity is required.
	IsMultiTenant() bool
}

// singleTenantDatasources is the read surface this package needs from the
// reporter's SafeDataSources: resolve a configured datasource by its config
// name and connect it on demand. It is satisfied by an adapter over
// pkg/reporter.SafeDataSources, declared in the worker bootstrap, so this
// package does not import the bootstrap-heavy datasource map directly.
type singleTenantDatasources interface {
	// ResolvePostgres returns the connected *sql.DB and configured schema list
	// for the named datasource. It returns an error when the datasource is
	// missing, of the wrong type, or unavailable.
	ResolvePostgres(ctx context.Context, configName string) (*sql.DB, []string, error)
	// ResolveMongo returns the connected mongo database for the named
	// datasource. It returns an error when the datasource is missing, of the
	// wrong type, or unavailable.
	ResolveMongo(ctx context.Context, configName string) (*libCommonsMongo.Database, error)
}

// singleTenantResolver resolves to the stable, env-configured datasource pools.
// It ignores tenant identity entirely: in single-tenant mode (formerly
// FETCHER_ENABLED=false) the whole process serves one logical tenant and the
// datasources are global.
type singleTenantResolver struct {
	datasources singleTenantDatasources
}

// NewSingleTenantResolver builds a TenantResolver backed by the reporter's
// env-configured datasources.
func NewSingleTenantResolver(datasources singleTenantDatasources) TenantResolver {
	return &singleTenantResolver{datasources: datasources}
}

func (r *singleTenantResolver) IsMultiTenant() bool { return false }

func (r *singleTenantResolver) ResolvePostgres(ctx context.Context, _ string, configName string) (postgresHandle, error) {
	db, schemas, err := r.datasources.ResolvePostgres(ctx, configName)
	if err != nil {
		return postgresHandle{}, err
	}

	// Guard against a concrete nil *sql.DB returned without an error: once it is
	// stored in the sqlQuerier interface field it is no longer == nil, so the
	// factory's handle.db == nil check cannot catch it. Catch it here, where the
	// concrete type is still visible, so a downstream Ping/Query never nil-derefs.
	if db == nil {
		return postgresHandle{}, NewEngineUnavailableError("single-tenant datasource resolved a nil postgres connection", nil)
	}

	return postgresHandle{db: db, schemas: schemas}, nil
}

func (r *singleTenantResolver) ResolveMongo(ctx context.Context, _ string, configName string) (mongoHandle, error) {
	db, err := r.datasources.ResolveMongo(ctx, configName)
	if err != nil {
		return mongoHandle{}, err
	}

	return mongoHandle{db: db}, nil
}

// multiTenantResolver resolves per-tenant database handles via the lib-commons
// tenant managers — the same MT machinery the rest of midaz uses. The tenant ID
// is the sole isolation boundary; an empty tenant ID is rejected so a
// misconfigured caller can never read across tenants.
type multiTenantResolver struct {
	pg      PostgresManager
	mongo   MongoManager
	schemas func(configName string) []string
}

// NewMultiTenantResolver builds a TenantResolver backed by lib-commons tenant
// managers. schemas resolves the configured PostgreSQL schema list for a
// datasource config name (env-derived, identical across tenants in
// database-per-tenant mode); a nil schemas func defaults to ["public"].
func NewMultiTenantResolver(pg PostgresManager, mongo MongoManager, schemas func(configName string) []string) TenantResolver {
	if schemas == nil {
		schemas = func(string) []string { return []string{"public"} }
	}

	return &multiTenantResolver{pg: pg, mongo: mongo, schemas: schemas}
}

func (r *multiTenantResolver) IsMultiTenant() bool { return true }

func (r *multiTenantResolver) ResolvePostgres(ctx context.Context, tenantID, configName string) (postgresHandle, error) {
	if err := requireTenant(tenantID); err != nil {
		return postgresHandle{}, err
	}

	db, err := r.pg.GetDB(ctx, tenantID)
	if err != nil {
		return postgresHandle{}, NewEngineUnavailableError("failed to resolve tenant PostgreSQL connection", err)
	}

	return postgresHandle{db: db, schemas: r.schemas(configName)}, nil
}

func (r *multiTenantResolver) ResolveMongo(ctx context.Context, tenantID, _ string) (mongoHandle, error) {
	if err := requireTenant(tenantID); err != nil {
		return mongoHandle{}, err
	}

	db, err := r.mongo.GetDatabaseForTenant(ctx, tenantID)
	if err != nil {
		return mongoHandle{}, NewEngineUnavailableError("failed to resolve tenant MongoDB database", err)
	}

	return mongoHandle{db: db}, nil
}

// requireTenant validates that a tenant ID is present and well-formed before any
// multi-tenant resolution. It reuses the lib-commons tenant-id shape check so
// the reporter's notion of a valid tenant matches the rest of midaz.
func requireTenant(tenantID string) error {
	if tenantID == "" {
		return NewEngineValidationError("tenant id is required for multi-tenant resolution")
	}

	if !tmcore.IsValidTenantID(tenantID) {
		return NewEngineValidationError("tenant id is invalid")
	}

	return nil
}
