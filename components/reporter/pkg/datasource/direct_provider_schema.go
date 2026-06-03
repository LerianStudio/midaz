// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"fmt"
	"strings"

	"github.com/LerianStudio/reporter/pkg"
	"github.com/LerianStudio/reporter/pkg/ctxutil"
	"github.com/LerianStudio/reporter/pkg/mongodb"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
)

const pluginCRMDataSourceID = "plugin_crm"

// getPostgresSchema retrieves schema from a PostgreSQL datasource repository.
func (dp *DirectProvider) getPostgresSchema(ctx context.Context, dataSourceID string, ds pkg.DataSource) (*DataSourceSchema, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "provider.direct.get_postgres_schema")
	defer span.End()

	if ds.PostgresRepository == nil {
		err := fmt.Errorf("postgres repository not initialized for datasource %q", dataSourceID)
		libOpentelemetry.HandleSpanError(span, "Nil PostgreSQL repository", err)

		return nil, err
	}

	schemas := ds.Schemas
	if len(schemas) == 0 {
		schemas = []string{"public"}
	}

	tableSchemas, err := ds.PostgresRepository.GetDatabaseSchema(ctx, schemas)
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

	if ds.MongoDBRepository == nil {
		err := fmt.Errorf("mongodb repository not initialized for datasource %q", dataSourceID)
		libOpentelemetry.HandleSpanError(span, "Nil MongoDB repository", err)

		return nil, err
	}

	// plugin_crm uses prefix-based collection grouping (holders_*, aliases_*)
	// with union schema across all organizations. Other MongoDB datasources
	// use the standard schema discovery.
	var collectionSchemas []mongodb.CollectionSchema

	var err error

	if dataSourceID == pluginCRMDataSourceID {
		collectionSchemas, err = ds.MongoDBRepository.GetDatabaseSchemaForPluginCRM(ctx)
	} else if ds.MidazOrganizationID != "" {
		collectionSchemas, err = ds.MongoDBRepository.GetDatabaseSchemaForOrganization(ctx, ds.MidazOrganizationID)
	} else {
		collectionSchemas, err = ds.MongoDBRepository.GetDatabaseSchema(ctx)
	}

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

		// For plugin_crm, GetDatabaseSchemaForPluginCRM already returns logical
		// names (e.g. "holders"). For org-scoped, strip the suffix.
		displayName := cs.CollectionName
		if ds.MidazOrganizationID != "" && dataSourceID != pluginCRMDataSourceID {
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
