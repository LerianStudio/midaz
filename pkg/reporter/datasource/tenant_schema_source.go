// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/multitenant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/postgres"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/lib/pq"
	libMongo "go.mongodb.org/mongo-driver/v2/mongo"
)

// TenantPostgresManager is the per-tenant PostgreSQL resolution seam the
// DirectProvider needs in multi-tenant mode. It is satisfied by lib-commons
// tenant-manager/postgres.Manager. Declaring the narrow interface here (rather
// than importing the concrete Manager) keeps the schema source unit-testable
// and inverts the dependency onto the lib-commons machinery the rest of midaz
// uses. It mirrors the worker's engine.PostgresManager seam.
type TenantPostgresManager interface {
	// GetDB returns the resolved per-tenant dbresolver.DB for the tenant. The
	// returned value exposes QueryContext, which is all schema discovery needs.
	GetDB(ctx context.Context, tenantID string) (dbresolver.DB, error)
}

// TenantMongoManager is the per-tenant MongoDB resolution seam the
// DirectProvider needs in multi-tenant mode. It is satisfied by lib-commons
// tenant-manager/mongo.Manager. It mirrors the worker's engine.MongoManager
// seam.
type TenantMongoManager interface {
	// GetDatabaseForTenant returns the per-tenant database resolved from the
	// tenant config.
	GetDatabaseForTenant(ctx context.Context, tenantID string) (*libMongo.Database, error)
}

// schemaSource is the per-tenant schema-snapshot seam the DirectProvider
// dispatches to in multi-tenant mode. It is implemented by tenantSchemaSource
// (backed by the lib-commons tenant managers) in production; declaring it as an
// interface lets provider-level tests substitute a recorder that observes the
// exact (dataSourceID, organizationID) routing — the CRM-vs-org asymmetry and
// the fail-closed contract — without a live database.
type schemaSource interface {
	// PostgresSchema returns the tenant-scoped base-table schema for the named
	// datasource across the given schemas, failing closed on resolution errors.
	PostgresSchema(ctx context.Context, configName string, schemas []string) ([]postgres.TableSchema, error)
	// MongoSchema returns the tenant-scoped collection schema. A dataSourceID of
	// crmDataSourceID selects CRM prefix-grouped discovery; a non-empty
	// organizationID selects org-suffix discovery; otherwise plain discovery.
	MongoSchema(ctx context.Context, dataSourceID, organizationID string) ([]mongodb.CollectionSchema, error)
}

// tenantSchemaSource resolves a tenant-scoped schema view for a datasource by
// reaching the per-tenant connection pool through the lib-commons tenant
// managers — the same MT machinery the worker's extraction path uses. It feeds
// the DirectProvider's existing validation, field-matching, schema-ambiguity,
// and CRM logic from a per-tenant snapshot instead of the global, env-built
// SafeDataSources pool, which is single-tenant only.
//
// CRITICAL ISOLATION INVARIANT (third-rail): the tenant ID is the SOLE
// isolation boundary. It is read from the request context via lib-commons and
// is NEVER substituted with a placeholder/default under multi-tenancy. Every
// seam fails CLOSED: a missing/malformed tenant, a manager resolution error, or
// a nil resolved connection returns an error rather than falling back to a
// shared pool or another tenant's pool. Client ownership of their data is a
// first principle — a cross-tenant schema read is a security defect.
type tenantSchemaSource struct {
	pg     TenantPostgresManager
	mongo  TenantMongoManager
	logger log.Logger
}

// compile-time check: tenantSchemaSource satisfies the schemaSource seam.
var _ schemaSource = (*tenantSchemaSource)(nil)

// newTenantSchemaSource builds the multi-tenant schema source backed by the
// lib-commons tenant managers.
func newTenantSchemaSource(pg TenantPostgresManager, mongo TenantMongoManager, logger log.Logger) *tenantSchemaSource {
	return &tenantSchemaSource{pg: pg, mongo: mongo, logger: logger}
}

// resolveTenant reads and validates the tenant ID from context. It never
// invents identity: an empty or malformed tenant fails closed so a misconfigured
// caller can never read across tenants or fall back to a shared pool.
func resolveTenant(ctx context.Context) (string, error) {
	tenantID := tmcore.GetTenantIDContext(ctx)
	if err := multitenant.ValidateTenantID(tenantID); err != nil {
		return "", fmt.Errorf("multi-tenant schema discovery: %w", err)
	}

	return tenantID, nil
}

// PostgresSchema resolves the tenant's PostgreSQL connection and returns the
// table/column schema across the configured schemas, in the same
// []postgres.TableSchema shape the DirectProvider's PostgreSQL validation and
// ambiguity detection already consume. It introspects the same base-table set
// the env-pool repository exposes (see discoverPostgresSchema), run over the
// tenant-scoped pool.
func (s *tenantSchemaSource) PostgresSchema(ctx context.Context, configName string, schemas []string) ([]postgres.TableSchema, error) {
	tenantID, err := resolveTenant(ctx)
	if err != nil {
		return nil, err
	}

	db, err := s.pg.GetDB(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve tenant PostgreSQL connection for %q: %w", configName, err)
	}

	// Nil-guard the resolved connection: a manager that returns a nil handle
	// without an error would otherwise nil-deref on the first QueryContext,
	// bypassing the fail-closed contract. Catch it at the seam boundary.
	if db == nil {
		return nil, fmt.Errorf("tenant PostgreSQL manager returned a nil connection for tenant scope")
	}

	if len(schemas) == 0 {
		schemas = []string{"public"}
	}

	return discoverPostgresSchema(ctx, db, schemas)
}

// MongoSchema resolves the tenant's MongoDB database and returns its collection
// schema, in the same []mongodb.CollectionSchema shape the DirectProvider's
// MongoDB validation and CRM/org-scoped logic already consume. It reuses the
// reporter's existing schema-discovery methods unchanged (including the CRM
// prefix-grouping and org-suffix filtering) by building a repository over the
// tenant-scoped database.
//
// crmDataSourceID and a non-empty organizationID select the same specialized
// discovery the single-tenant path uses, preserving the CRM contract exactly.
func (s *tenantSchemaSource) MongoSchema(ctx context.Context, dataSourceID, organizationID string) ([]mongodb.CollectionSchema, error) {
	tenantID, err := resolveTenant(ctx)
	if err != nil {
		return nil, err
	}

	db, err := s.mongo.GetDatabaseForTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve tenant MongoDB database for %q: %w", dataSourceID, err)
	}

	if db == nil {
		return nil, fmt.Errorf("tenant MongoDB manager returned a nil database for tenant scope")
	}

	repo, err := mongodb.NewDataSourceRepositoryFromDatabase(db, s.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to build tenant MongoDB schema repository for %q: %w", dataSourceID, err)
	}

	switch {
	case dataSourceID == crmDataSourceID:
		return repo.GetDatabaseSchemaForCRM(ctx)
	case organizationID != "":
		return repo.GetDatabaseSchemaForOrganization(ctx, organizationID)
	default:
		return repo.GetDatabaseSchema(ctx)
	}
}

// discoverPostgresSchema runs the information_schema introspection over a
// tenant-resolved read surface and assembles []postgres.TableSchema. It mirrors
// the env-pool repository's GetDatabaseSchema: it restricts the relation set to
// base tables (table_type = 'BASE TABLE') via a join on information_schema.tables,
// so views and other column-bearing relations are excluded exactly as the
// single-tenant path excludes them. Without that filter, a view shared across
// schemas could land in MissingTables in one mode and resolve in the other, and
// detectSchemaAmbiguity could flag ambiguity differently — the same template
// would then pass under multi-tenancy and fail single-tenant against the same
// database. Primary-key flags are omitted because schema validation only consults
// table and column names, never the IsPrimaryKey flag.
func discoverPostgresSchema(ctx context.Context, db dbresolver.DB, schemas []string) ([]postgres.TableSchema, error) {
	const q = `
		SELECT c.table_schema, c.table_name, c.column_name, c.data_type,
		       CASE WHEN c.is_nullable = 'YES' THEN true ELSE false END as is_nullable
		FROM information_schema.columns c
		JOIN information_schema.tables t
			ON t.table_schema = c.table_schema
			AND t.table_name = c.table_name
		WHERE c.table_schema = ANY($1)
			AND t.table_type = 'BASE TABLE'
		ORDER BY c.table_schema, c.table_name, c.ordinal_position
	`

	schemaCtx, cancel := context.WithTimeout(ctx, constant.SchemaDiscoveryTimeout)
	defer cancel()

	rows, err := db.QueryContext(schemaCtx, q, pq.Array(schemas))
	if err != nil {
		if schemaCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("tenant schema discovery timeout after %v: %w", constant.SchemaDiscoveryTimeout, err)
		}

		return nil, fmt.Errorf("error querying tenant schema: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Preserve first-seen table order so output is deterministic for the same
	// ordinal-position-ordered result set.
	order := make([]string, 0)
	byTable := make(map[string]*postgres.TableSchema)

	for rows.Next() {
		var schemaName, tableName, columnName, dataType string

		var isNullable bool

		if err := rows.Scan(&schemaName, &tableName, &columnName, &dataType, &isNullable); err != nil {
			return nil, fmt.Errorf("error scanning tenant schema row: %w", err)
		}

		key := schemaName + "." + tableName

		ts, ok := byTable[key]
		if !ok {
			ts = &postgres.TableSchema{SchemaName: schemaName, TableName: tableName}
			byTable[key] = ts
			order = append(order, key)
		}

		ts.Columns = append(ts.Columns, postgres.ColumnInformation{
			Name:       columnName,
			DataType:   dataType,
			IsNullable: isNullable,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tenant schema rows: %w", err)
	}

	result := make([]postgres.TableSchema, 0, len(order))
	for _, key := range order {
		result = append(result, *byTable[key])
	}

	return result, nil
}

// compile-time check: the stdlib *sql.DB also satisfies the QueryContext shape
// dbresolver.DB exposes, documenting that discoverPostgresSchema is connection-
// provenance agnostic.
var _ interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
} = (*sql.DB)(nil)
