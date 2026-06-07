// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/Masterminds/squirrel"
	"go.opentelemetry.io/otel/attribute"
)

// ValidateTableAndFields checks if the specified schema-qualified table exists and validates that all requested fields exist in that table.
func (ds *ExternalDataSource) ValidateTableAndFields(ctx context.Context, schemaName, tableName string, requestedFields []string, schema []TableSchema) ([]string, error) {
	logger := ds.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	_, span := tracer.Start(ctx, "repository.datasource.validate_table_and_fields")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.schema", schemaName),
		attribute.String("app.request.table", tableName),
		attribute.Int("app.request.requested_fields_count", len(requestedFields)),
	)

	logger.Log(ctx, log.LevelDebug, "Validating table and fields", log.String("table", tableName), log.Any("fields", requestedFields))

	var (
		tableFound   bool
		tableColumns []ColumnInformation
	)

	for _, table := range schema {
		if table.SchemaName == schemaName && table.TableName == tableName {
			tableFound = true
			tableColumns = table.Columns

			break
		}
	}

	if !tableFound {
		return nil, fmt.Errorf("table '%s' does not exist in the database", tableName)
	}

	validColumns := make(map[string]bool)
	for _, col := range tableColumns {
		validColumns[col.Name] = true
	}

	if len(requestedFields) == 1 && requestedFields[0] == "*" {
		allFields := make([]string, len(tableColumns))
		for i, col := range tableColumns {
			allFields[i] = col.Name
		}

		return allFields, nil
	}

	var (
		validFields   []string
		invalidFields []string
	)

	for _, field := range requestedFields {
		fieldToCheck := field
		if dotIdx := strings.Index(field, "."); dotIdx != -1 {
			fieldToCheck = field[:dotIdx]
		}

		if validColumns[fieldToCheck] {
			validFields = append(validFields, field)
		} else {
			invalidFields = append(invalidFields, field)
		}
	}

	if len(invalidFields) > 0 {
		return nil, fmt.Errorf("invalid fields for table '%s': %v", tableName, invalidFields)
	}

	if len(validFields) == 0 {
		return nil, fmt.Errorf("no valid fields specified for table '%s'", tableName)
	}

	logger.Log(ctx, log.LevelDebug, "Successfully validated table and fields", log.String("table", tableName), log.Any("fields", validFields))

	return validFields, nil
}

// buildDynamicFilters applies filter criteria to the query builder based on valid columns.
func buildDynamicFilters(queryBuilder squirrel.SelectBuilder, schema []TableSchema, schemaName, table string, filter map[string][]any) squirrel.SelectBuilder {
	validColumns := resolveValidColumns(schema, schemaName, table)

	for field, values := range filter {
		if validColumns[field] && len(values) > 0 {
			queryBuilder = applyFilter(queryBuilder, field, values)
		}
	}

	return queryBuilder
}

// resolveValidColumns finds the matching schema-qualified table in the schema list and returns its column names.
func resolveValidColumns(schema []TableSchema, schemaName, table string) map[string]bool {
	var tableColumns []ColumnInformation

	for _, t := range schema {
		if t.SchemaName == schemaName && t.TableName == table {
			tableColumns = t.Columns
			break
		}
	}

	validColumns := make(map[string]bool)
	for _, col := range tableColumns {
		validColumns[col.Name] = true
	}

	return validColumns
}

// applyFilter adds a WHERE condition for a field with multiple possible values.
func applyFilter(queryBuilder squirrel.SelectBuilder, fieldName string, values []any) squirrel.SelectBuilder {
	if len(values) == 0 {
		return queryBuilder
	}

	placeholder := squirrel.Placeholders(len(values))

	return queryBuilder.Where(fieldName+" IN ("+placeholder+")", values...)
}

// QueryWithAdvancedFilters executes a SELECT SQL query with advanced FilterCondition support.
func (ds *ExternalDataSource) QueryWithAdvancedFilters(ctx context.Context, schema []TableSchema, schemaName string, table string, fields []string, filter map[string]model.FilterCondition) ([]map[string]any, error) {
	logger := ds.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.datasource.query_with_advanced_filters")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.schema", schemaName),
		attribute.String("app.request.table", table),
		attribute.Int("app.request.requested_fields_count", len(fields)),
		attribute.Int("app.request.filter_conditions_count", len(filter)),
	)

	qualifiedTable := qualifyTableName(schemaName, table)
	logger.Log(ctx, log.LevelDebug, "Querying table with advanced filters", log.String("table", qualifiedTable), log.Any("fields", fields))

	queriedFields, err := ds.ValidateTableAndFields(ctx, schemaName, table, fields, schema)
	if err != nil {
		return nil, err
	}

	selectFields := transformFieldsForSelect(queriedFields)

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	queryBuilder := psql.Select(selectFields...).From(qualifiedTable)

	queryBuilder, err = ds.buildAdvancedFilters(queryBuilder, schema, schemaName, table, filter)
	if err != nil {
		return nil, fmt.Errorf("error building advanced filters: %w", err)
	}

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("error generating SQL: %w", err)
	}

	logger.Log(ctx, log.LevelDebug, "Executing PostgreSQL advanced-filter query",
		log.String("table", qualifyTableName(schemaName, table)),
		log.Int("field_count", len(fields)),
		log.Int("filter_count", len(filter)),
		log.Int("arg_count", len(args)),
	)

	queryCtx, cancel := context.WithTimeout(ctx, constant.QueryTimeoutSlow)
	defer cancel()

	rows, err := ds.connection.ConnectionDB.QueryContext(queryCtx, query, args...)
	if err != nil {
		if queryCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("advanced filter query timeout after %v: %w", constant.QueryTimeoutSlow, err)
		}

		return nil, fmt.Errorf("error executing query: %w", err)
	}
	defer rows.Close()

	return scanRows(queryCtx, rows, logger)
}

// buildAdvancedFilters applies FilterCondition criteria to the query builder.
func (ds *ExternalDataSource) buildAdvancedFilters(queryBuilder squirrel.SelectBuilder, schema []TableSchema, schemaName, table string, filter map[string]model.FilterCondition) (squirrel.SelectBuilder, error) {
	validColumns := resolveValidColumns(schema, schemaName, table)

	for field, condition := range filter {
		if isFilterConditionEmpty(condition) {
			continue
		}

		if !validColumns[field] {
			return queryBuilder, fmt.Errorf("unknown filter field '%s' for table '%s'", field, table)
		}

		if err := validateFilterCondition(field, condition); err != nil {
			return queryBuilder, err
		}

		queryBuilder = ds.applyAdvancedFilter(queryBuilder, field, condition)
	}

	return queryBuilder, nil
}

// applyAdvancedFilter applies a single FilterCondition to the query builder.
func (ds *ExternalDataSource) applyAdvancedFilter(queryBuilder squirrel.SelectBuilder, field string, condition model.FilterCondition) squirrel.SelectBuilder {
	if len(condition.Equals) > 0 {
		if len(condition.Equals) == 1 {
			queryBuilder = queryBuilder.Where(squirrel.Eq{field: condition.Equals[0]})
		} else {
			queryBuilder = queryBuilder.Where(squirrel.Eq{field: condition.Equals})
		}
	}

	if len(condition.GreaterThan) > 0 {
		queryBuilder = queryBuilder.Where(squirrel.Gt{field: condition.GreaterThan[0]})
	}

	if len(condition.GreaterOrEqual) > 0 {
		queryBuilder = queryBuilder.Where(squirrel.GtOrEq{field: condition.GreaterOrEqual[0]})
	}

	if len(condition.LessThan) > 0 {
		queryBuilder = queryBuilder.Where(squirrel.Lt{field: condition.LessThan[0]})
	}

	if len(condition.LessOrEqual) > 0 {
		queryBuilder = queryBuilder.Where(squirrel.LtOrEq{field: condition.LessOrEqual[0]})
	}

	if len(condition.Between) == constant.BetweenOperatorValues {
		startValue := condition.Between[0]

		endValue := condition.Between[1]
		if isDateField(field) && isDateString(startValue) && isDateString(endValue) {
			if endStr, ok := endValue.(string); ok && len(endStr) == constant.DateOnlyStringLength {
				endValue = endStr + "T23:59:59.999Z"
			}
		}

		queryBuilder = queryBuilder.Where(squirrel.GtOrEq{field: startValue}).Where(squirrel.LtOrEq{field: endValue})
	}

	if len(condition.In) > 0 {
		queryBuilder = queryBuilder.Where(squirrel.Eq{field: condition.In})
	}

	if len(condition.NotIn) > 0 {
		queryBuilder = queryBuilder.Where(squirrel.NotEq{field: condition.NotIn})
	}

	return queryBuilder
}

func isFilterConditionEmpty(condition model.FilterCondition) bool {
	return len(condition.Equals) == 0 &&
		len(condition.GreaterThan) == 0 &&
		len(condition.GreaterOrEqual) == 0 &&
		len(condition.LessThan) == 0 &&
		len(condition.LessOrEqual) == 0 &&
		len(condition.Between) == 0 &&
		len(condition.In) == 0 &&
		len(condition.NotIn) == 0
}

func validateFilterCondition(fieldName string, condition model.FilterCondition) error {
	if len(condition.Between) > 0 && len(condition.Between) != 2 {
		return fmt.Errorf("between operator for field '%s' must have exactly 2 values, got %d", fieldName, len(condition.Between))
	}

	singleValueOps := map[string][]any{"gt": condition.GreaterThan, "gte": condition.GreaterOrEqual, "lt": condition.LessThan, "lte": condition.LessOrEqual}
	for opName, values := range singleValueOps {
		if len(values) > 0 && len(values) != 1 {
			return fmt.Errorf("%s operator for field '%s' must have exactly 1 value, got %d", opName, fieldName, len(values))
		}
	}

	if isLikelyUUIDField(fieldName) {
		if err := validateUUIDFieldValues(fieldName, condition); err != nil {
			return err
		}
	}

	return nil
}
