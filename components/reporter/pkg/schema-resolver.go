// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"fmt"
	"strings"

	"github.com/LerianStudio/reporter/pkg/postgres"
)

// SchemaResolver resolves database schema names for table references.
// It handles both explicit schema references (database:schema.table) and
// implicit references (database.table) with autodiscovery.
type SchemaResolver struct {
	// registry maps database names to their table schemas
	registry map[string][]postgres.TableSchema
}

// NewSchemaResolver creates a new SchemaResolver instance.
func NewSchemaResolver() *SchemaResolver {
	return &SchemaResolver{
		registry: make(map[string][]postgres.TableSchema),
	}
}

// RegisterDatabase registers the table schemas for a database.
// This should be called during initialization with the discovered schema.
func (r *SchemaResolver) RegisterDatabase(database string, tables []postgres.TableSchema) {
	r.registry[database] = tables
}

// ResolveSchema resolves the schema name for a table reference.
//
// If explicitSchema is provided, it validates that the table exists in that schema.
// If explicitSchema is empty, it performs autodiscovery:
//   - If table exists in exactly one schema, returns that schema
//   - If table exists in multiple schemas including "public", returns "public"
//   - If table exists in multiple schemas without "public", returns SchemaAmbiguityError
//   - If table doesn't exist in any schema, returns an error
func (r *SchemaResolver) ResolveSchema(database, explicitSchema, table string) (string, error) {
	tables, ok := r.registry[database]
	if !ok {
		return "", fmt.Errorf("database '%s' not registered", database)
	}

	// If explicit schema is provided, validate it
	if explicitSchema != "" {
		for _, t := range tables {
			if t.SchemaName == explicitSchema && t.TableName == table {
				return explicitSchema, nil
			}
		}

		return "", fmt.Errorf("table '%s' not found in schema '%s' of database '%s'", table, explicitSchema, database)
	}

	// Autodiscovery: find all schemas containing this table
	schemasWithTable := r.findSchemasWithTable(tables, table)

	switch len(schemasWithTable) {
	case 0:
		return "", fmt.Errorf("table '%s' not found in any configured schema of database '%s'", table, database)
	case 1:
		return schemasWithTable[0], nil
	default:
		// Multiple schemas have this table - check for public
		for _, schema := range schemasWithTable {
			if schema == "public" {
				return "public", nil
			}
		}
		// No public schema - return ambiguity error
		return "", &SchemaAmbiguityError{
			Database: database,
			Table:    table,
			Schemas:  schemasWithTable,
		}
	}
}

// findSchemasWithTable returns all schema names that contain the given table.
func (r *SchemaResolver) findSchemasWithTable(tables []postgres.TableSchema, tableName string) []string {
	schemaSet := make(map[string]bool)

	for _, t := range tables {
		if t.TableName == tableName {
			schemaSet[t.SchemaName] = true
		}
	}

	schemas := make([]string, 0, len(schemaSet))
	for schema := range schemaSet {
		schemas = append(schemas, schema)
	}

	return schemas
}

// SchemaAmbiguityError is returned when a table exists in multiple schemas
// and no explicit schema was provided.
type SchemaAmbiguityError struct {
	Database string
	Table    string
	Schemas  []string
}

// Error implements the error interface with actionable suggestions.
func (e *SchemaAmbiguityError) Error() string {
	suggestions := make([]string, 0, len(e.Schemas))
	for _, schema := range e.Schemas {
		suggestions = append(suggestions, fmt.Sprintf("  {{ %s:%s.%s }}", e.Database, schema, e.Table))
	}

	return fmt.Sprintf(
		"ambiguous table reference: '%s.%s' exists in multiple schemas: [%s]\n"+
			"Please use explicit schema syntax:\n%s",
		e.Database,
		e.Table,
		strings.Join(e.Schemas, ", "),
		strings.Join(suggestions, "\n"),
	)
}
