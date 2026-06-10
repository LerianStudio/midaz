// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"fmt"
	"strings"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/postgres"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
)

const crmDataSourceID = "crm"

// postgresTableSchemas fetches the PostgreSQL table/column schema for a
// datasource. In multi-tenant mode it resolves the tenant-scoped pool through
// the tenant schema source (fail-closed); otherwise it reads the env-pool
// repository. The returned shape is identical in both modes so the callers'
// field-matching and ambiguity detection are unchanged.
func (dp *DirectProvider) postgresTableSchemas(ctx context.Context, dataSourceID string, ds pkg.DataSource) ([]postgres.TableSchema, error) {
	schemas := ds.Schemas
	if len(schemas) == 0 {
		schemas = []string{"public"}
	}

	if dp.tenantSchema != nil {
		return dp.tenantSchema.PostgresSchema(ctx, dataSourceID, schemas)
	}

	if ds.PostgresRepository == nil {
		return nil, fmt.Errorf("postgres repository not initialized for datasource %q", dataSourceID)
	}

	return ds.PostgresRepository.GetDatabaseSchema(ctx, schemas)
}

// mongoSchemaForDetails fetches the MongoDB collection schema for the
// GetDataSourceSchema (details) path. crm uses prefix-grouped discovery
// (logical names like "holders"); an org-scoped datasource uses the org-suffix
// filter; others use plain discovery. In multi-tenant mode the tenant-scoped
// database supplies the snapshot (fail-closed); otherwise the env-pool
// repository does. The crm-vs-org selection is identical in both modes.
func (dp *DirectProvider) mongoSchemaForDetails(ctx context.Context, dataSourceID string, ds pkg.DataSource) ([]mongodb.CollectionSchema, error) {
	if dp.tenantSchema != nil {
		return dp.tenantSchema.MongoSchema(ctx, dataSourceID, ds.MidazOrganizationID)
	}

	if ds.MongoDBRepository == nil {
		return nil, fmt.Errorf("mongodb repository not initialized for datasource %q", dataSourceID)
	}

	switch {
	case dataSourceID == crmDataSourceID:
		return ds.MongoDBRepository.GetDatabaseSchemaForCRM(ctx)
	case ds.MidazOrganizationID != "":
		return ds.MongoDBRepository.GetDatabaseSchemaForOrganization(ctx, ds.MidazOrganizationID)
	default:
		return ds.MongoDBRepository.GetDatabaseSchema(ctx)
	}
}

// mongoSchemaForValidation fetches the MongoDB collection schema for the
// ValidateSchema path. Unlike the details path, validation never uses the crm
// prefix-grouped discovery: it relies on the org-suffix collection-name
// transformation applied by validateMongoDBSchema, so it needs the raw
// per-organization collections. An org-scoped datasource therefore uses the
// org-suffix filter; everything else uses plain discovery. Routing the MT
// source with an empty dataSourceID here keeps it off the crm branch, so the
// snapshot matches the single-tenant validation contract.
func (dp *DirectProvider) mongoSchemaForValidation(ctx context.Context, dataSourceID string, ds pkg.DataSource) ([]mongodb.CollectionSchema, error) {
	if dp.tenantSchema != nil {
		// Pass an empty dataSourceID so the MT source takes the org/plain path,
		// never the crm prefix-grouped path — matching the single-tenant
		// validation selection below.
		return dp.tenantSchema.MongoSchema(ctx, "", ds.MidazOrganizationID)
	}

	if ds.MongoDBRepository == nil {
		return nil, fmt.Errorf("mongodb repository not initialized for datasource %q", dataSourceID)
	}

	if ds.MidazOrganizationID != "" {
		return ds.MongoDBRepository.GetDatabaseSchemaForOrganization(ctx, ds.MidazOrganizationID)
	}

	return ds.MongoDBRepository.GetDatabaseSchema(ctx)
}

// getPostgresSchema retrieves schema from a PostgreSQL datasource repository.
func (dp *DirectProvider) getPostgresSchema(ctx context.Context, dataSourceID string, ds pkg.DataSource) (*DataSourceSchema, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "provider.direct.get_postgres_schema")
	defer span.End()

	tableSchemas, err := dp.postgresTableSchemas(ctx, dataSourceID, ds)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get PostgreSQL schema", err)

		return nil, fmt.Errorf("failed to get PostgreSQL schema for %q: %w", dataSourceID, err)
	}

	tables := make([]SchemaTable, 0, len(tableSchemas))
	for _, ts := range tableSchemas {
		fields := make([]SchemaField, 0, len(ts.Columns))
		for _, col := range ts.Columns {
			fields = append(fields, SchemaField{
				Name: col.Name,
				Type: col.DataType,
			})
		}

		tables = append(tables, SchemaTable{
			Name:   ts.QualifiedName(),
			Schema: ts.SchemaName,
			Fields: fields,
		})
	}

	return &DataSourceSchema{
		DataSourceID: dataSourceID,
		Tables:       tables,
	}, nil
}

// getMongoDBSchema retrieves schema from a MongoDB datasource repository.
func (dp *DirectProvider) getMongoDBSchema(ctx context.Context, dataSourceID string, ds pkg.DataSource) (*DataSourceSchema, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "provider.direct.get_mongodb_schema")
	defer span.End()

	// crm uses prefix-based collection grouping (holders_*, aliases_*)
	// with union schema across all organizations. Other MongoDB datasources
	// use the standard schema discovery. The selection is identical in
	// single-tenant and multi-tenant mode; only the connection source differs.
	collectionSchemas, err := dp.mongoSchemaForDetails(ctx, dataSourceID, ds)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get MongoDB schema", err)

		return nil, fmt.Errorf("failed to get MongoDB schema for %q: %w", dataSourceID, err)
	}

	tables := make([]SchemaTable, 0, len(collectionSchemas))
	for _, cs := range collectionSchemas {
		fields := make([]SchemaField, 0, len(cs.Fields))
		for _, f := range cs.Fields {
			fields = append(fields, SchemaField{
				Name: f.Name,
				Type: f.DataType,
			})
		}

		// For crm, GetDatabaseSchemaForCRM already returns logical
		// names (e.g. "holders"). For org-scoped, strip the suffix.
		displayName := cs.CollectionName
		if ds.MidazOrganizationID != "" && dataSourceID != crmDataSourceID {
			displayName = stripOrgSuffix(cs.CollectionName, ds.MidazOrganizationID)
		}

		tables = append(tables, SchemaTable{
			Name:   displayName,
			Fields: fields,
		})
	}

	return &DataSourceSchema{
		DataSourceID: dataSourceID,
		Tables:       tables,
	}, nil
}

// stripOrgSuffix removes the "_<orgID>" suffix from a collection name.
// For example, "holders_test-org-001" with orgID "test-org-001" returns "holders".
// If the suffix is not present, the original name is returned unchanged.
func stripOrgSuffix(collectionName, orgID string) string {
	suffix := "_" + orgID
	if strings.HasSuffix(collectionName, suffix) {
		return strings.TrimSuffix(collectionName, suffix)
	}

	return collectionName
}

// resolveDisplayName returns a human-readable name for a DataSource.
func (dp *DirectProvider) resolveDisplayName(ds pkg.DataSource) string {
	switch ds.DatabaseType {
	case pkg.PostgreSQLType:
		if ds.DatabaseConfig != nil {
			return ds.DatabaseConfig.DBName
		}

		return ""
	case pkg.MongoDBType:
		return ds.MongoDBName
	default:
		return ""
	}
}
