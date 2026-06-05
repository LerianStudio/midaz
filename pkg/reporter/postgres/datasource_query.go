// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/Masterminds/squirrel"
	"go.opentelemetry.io/otel/attribute"
)

// Query executes a SELECT SQL query on the specified table with the given fields and filter criteria.
func (ds *ExternalDataSource) Query(ctx context.Context, schema []TableSchema, schemaName string, table string, fields []string, filter map[string][]any) ([]map[string]any, error) {
	logger := ds.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.datasource.query")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.schema", schemaName),
		attribute.String("app.request.table", table),
		attribute.Int("app.request.field_count", len(fields)),
		attribute.Int("app.request.filter_count", len(filter)),
	)

	qualifiedTable := qualifyTableName(schemaName, table)
	logger.Log(ctx, log.LevelInfo, "Querying PostgreSQL table",
		log.String("table", qualifiedTable),
		log.Int("field_count", len(fields)),
		log.Int("filter_count", len(filter)),
	)

	queriedFields, err := ds.ValidateTableAndFields(ctx, schemaName, table, fields, schema)
	if err != nil {
		return nil, err
	}

	selectFields := transformFieldsForSelect(queriedFields)
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	queryBuilder := psql.Select(selectFields...).From(qualifiedTable)
	queryBuilder = buildDynamicFilters(queryBuilder, schema, schemaName, table, filter)

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("error generating SQL: %w", err)
	}

	logger.Log(ctx, log.LevelInfo, "Executing PostgreSQL query",
		log.String("table", qualifiedTable),
		log.Int("arg_count", len(args)),
	)

	queryCtx, cancel := context.WithTimeout(ctx, constant.QueryTimeoutMedium)
	defer cancel()

	rows, err := ds.connection.ConnectionDB.QueryContext(queryCtx, query, args...)
	if err != nil {
		if queryCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("query execution timeout after %v: %w", constant.QueryTimeoutMedium, err)
		}

		return nil, fmt.Errorf("error executing query: %w", err)
	}
	defer rows.Close()

	return scanRows(queryCtx, rows, logger)
}

// scanRows processes the query rows and creates the resulting slice of maps.
func scanRows(ctx context.Context, rows *sql.Rows, logger log.Logger) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("error getting column names: %w", err)
	}

	values := make([]any, len(columns))

	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}

	var result []map[string]any

	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}

		result = append(result, createRowMap(ctx, columns, values, logger))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return result, nil
}

// createRowMap maps column names to their respective values.
func createRowMap(ctx context.Context, columns []string, values []any, logger log.Logger) map[string]any {
	rowMap := make(map[string]any)
	for i, column := range columns {
		rowMap[column] = parseJSONBField(ctx, values[i], logger)
	}

	return rowMap
}

// parseJSONBField unmarshals any field that might be a JSONB type.
func parseJSONBField(ctx context.Context, value any, logger log.Logger) any {
	if value == nil {
		return nil
	}

	if byteData, ok := value.([]uint8); ok {
		var jsonMap map[string]any
		if err := json.Unmarshal(byteData, &jsonMap); err == nil {
			return jsonMap
		}

		var jsonArray []any
		if err := json.Unmarshal(byteData, &jsonArray); err == nil {
			return jsonArray
		}

		var jsonString string
		if err := json.Unmarshal(byteData, &jsonString); err == nil {
			return jsonString
		}

		logger.Log(ctx, log.LevelWarn, "Failed to unmarshal potential JSONB data", log.String("value", string(byteData)))
	}

	return value
}

// extractRootColumn extracts the root column name from a potentially nested JSONB field path.
func extractRootColumn(field string) string {
	if dotIdx := strings.Index(field, "."); dotIdx != -1 {
		return field[:dotIdx]
	}

	return field
}

// transformFieldsForSelect converts a list of fields to SQL-safe column names.
func transformFieldsForSelect(fields []string) []string {
	seen := make(map[string]bool)

	var result []string

	for _, field := range fields {
		rootColumn := extractRootColumn(field)
		if !seen[rootColumn] {
			seen[rootColumn] = true
			result = append(result, rootColumn)
		}
	}

	return result
}
