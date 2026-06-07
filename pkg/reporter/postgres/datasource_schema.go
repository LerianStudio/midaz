// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
)

// tableInfo holds schema and table name for internal processing.
type tableInfo struct {
	schemaName string
	tableName  string
}

// GetDatabaseSchema retrieves all tables and their column details from the specified schemas.
func (ds *ExternalDataSource) GetDatabaseSchema(ctx context.Context, schemas []string) ([]TableSchema, error) {
	logger := ds.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.datasource.get_database_schema")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))
	logger.Log(ctx, log.LevelDebug, "Retrieving database schema information", log.Any("schemas", schemas))

	schemaCtx, cancel := context.WithTimeout(ctx, constant.SchemaDiscoveryTimeout)
	defer cancel()

	tables, err := ds.queryTables(schemaCtx, schemas)
	if err != nil {
		return nil, err
	}

	primaryKeys, err := ds.queryPrimaryKeys(schemaCtx, schemas)
	if err != nil {
		return nil, err
	}

	result, err := ds.buildTableSchemas(schemaCtx, tables, primaryKeys)
	if err != nil {
		return nil, err
	}

	logger.Log(ctx, log.LevelDebug, "Retrieved schema", log.Int("table_count", len(result)), log.Int("schema_count", len(schemas)))

	return result, nil
}

// queryTables retrieves all user tables from the specified schemas.
func (ds *ExternalDataSource) queryTables(ctx context.Context, schemas []string) ([]tableInfo, error) {
	const tableQuery = `
		SELECT table_schema, table_name
		FROM information_schema.tables
		WHERE table_schema = ANY($1)
		AND table_type = 'BASE TABLE'
		ORDER BY table_schema, table_name
	`

	rows, err := ds.connection.ConnectionDB.QueryContext(ctx, tableQuery, pq.Array(schemas))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("schema discovery timeout after %v while querying tables: %w", constant.SchemaDiscoveryTimeout, err)
		}

		return nil, fmt.Errorf("error querying tables: %w", err)
	}
	defer rows.Close()

	var tables []tableInfo

	for rows.Next() {
		var info tableInfo
		if err := rows.Scan(&info.schemaName, &info.tableName); err != nil {
			return nil, fmt.Errorf("error scanning table name: %w", err)
		}

		tables = append(tables, info)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tables: %w", err)
	}

	return tables, nil
}

// queryPrimaryKeys retrieves primary key information for all tables in the specified schemas.
func (ds *ExternalDataSource) queryPrimaryKeys(ctx context.Context, schemas []string) (map[string]map[string]bool, error) {
	const pkQuery = `
		SELECT tc.table_schema, tc.table_name, kc.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kc
			ON kc.table_name = tc.table_name
			AND kc.table_schema = tc.table_schema
			AND kc.constraint_name = tc.constraint_name
		WHERE tc.constraint_type = 'PRIMARY KEY'
		AND tc.table_schema = ANY($1)
	`

	pkRows, err := ds.connection.ConnectionDB.QueryContext(ctx, pkQuery, pq.Array(schemas))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("schema discovery timeout after %v while querying primary keys: %w", constant.SchemaDiscoveryTimeout, err)
		}

		return nil, fmt.Errorf("error querying primary keys: %w", err)
	}
	defer pkRows.Close()

	primaryKeys := make(map[string]map[string]bool)

	for pkRows.Next() {
		var schemaName, tableName, columnName string
		if err := pkRows.Scan(&schemaName, &tableName, &columnName); err != nil {
			return nil, fmt.Errorf("error scanning primary key info: %w", err)
		}

		key := schemaName + "." + tableName
		if primaryKeys[key] == nil {
			primaryKeys[key] = make(map[string]bool)
		}

		primaryKeys[key][columnName] = true
	}

	if err := pkRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating primary keys: %w", err)
	}

	return primaryKeys, nil
}

// buildTableSchemas builds the complete schema information for all tables,
// issuing one column query per table to retrieve full column metadata.
func (ds *ExternalDataSource) buildTableSchemas(ctx context.Context, tables []tableInfo, primaryKeys map[string]map[string]bool) ([]TableSchema, error) {
	result := make([]TableSchema, 0, len(tables))
	for _, tbl := range tables {
		columns, err := ds.queryTableColumns(ctx, tbl, primaryKeys)
		if err != nil {
			return nil, err
		}

		result = append(result, TableSchema{SchemaName: tbl.schemaName, TableName: tbl.tableName, Columns: columns})
	}

	return result, nil
}

// queryTableColumns retrieves column information for a specific table.
func (ds *ExternalDataSource) queryTableColumns(ctx context.Context, tbl tableInfo, primaryKeys map[string]map[string]bool) ([]ColumnInformation, error) {
	const columnQuery = `
		SELECT column_name, data_type,
		       CASE WHEN is_nullable = 'YES' THEN true ELSE false END as is_nullable
		FROM information_schema.columns
		WHERE table_schema = $1
		AND table_name = $2
		ORDER BY ordinal_position
	`

	colRows, err := ds.connection.ConnectionDB.QueryContext(ctx, columnQuery, tbl.schemaName, tbl.tableName)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("schema discovery timeout after %v while querying columns for table %s.%s: %w", constant.SchemaDiscoveryTimeout, tbl.schemaName, tbl.tableName, err)
		}

		return nil, fmt.Errorf("error querying columns for table %s.%s: %w", tbl.schemaName, tbl.tableName, err)
	}
	defer colRows.Close()

	var columns []ColumnInformation

	pkKey := tbl.schemaName + "." + tbl.tableName

	for colRows.Next() {
		var col ColumnInformation
		if err := colRows.Scan(&col.Name, &col.DataType, &col.IsNullable); err != nil {
			return nil, fmt.Errorf("error scanning column info: %w", err)
		}

		if pkCols, exists := primaryKeys[pkKey]; exists {
			col.IsPrimaryKey = pkCols[col.Name]
		}

		columns = append(columns, col)
	}

	if err := colRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating columns: %w", err)
	}

	return columns, nil
}
