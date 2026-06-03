// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateFieldsInSchemaPostgres_AllFieldsExist(t *testing.T) {
	t.Parallel()

	schema := TableSchema{
		SchemaName: "public",
		TableName:  "users",
		Columns: []ColumnInformation{
			{Name: "id", DataType: "uuid"},
			{Name: "name", DataType: "varchar"},
			{Name: "email", DataType: "varchar"},
			{Name: "created_at", DataType: "timestamp"},
		},
	}

	expectedFields := []string{"id", "name", "email"}
	var count int32 = 0

	missing := ValidateFieldsInSchemaPostgres(expectedFields, schema, &count)

	assert.Empty(t, missing)
	assert.Equal(t, int32(3), count)
}

func TestValidateFieldsInSchemaPostgres_SomeMissing(t *testing.T) {
	t.Parallel()

	schema := TableSchema{
		SchemaName: "public",
		TableName:  "users",
		Columns: []ColumnInformation{
			{Name: "id", DataType: "uuid"},
			{Name: "name", DataType: "varchar"},
		},
	}

	expectedFields := []string{"id", "name", "email", "phone"}
	var count int32 = 0

	missing := ValidateFieldsInSchemaPostgres(expectedFields, schema, &count)

	assert.Len(t, missing, 2)
	assert.Contains(t, missing, "email")
	assert.Contains(t, missing, "phone")
	assert.Equal(t, int32(4), count)
}

func TestValidateFieldsInSchemaPostgres_AllMissing(t *testing.T) {
	t.Parallel()

	schema := TableSchema{
		SchemaName: "public",
		TableName:  "users",
		Columns: []ColumnInformation{
			{Name: "id", DataType: "uuid"},
		},
	}

	expectedFields := []string{"foo", "bar", "baz"}
	var count int32 = 0

	missing := ValidateFieldsInSchemaPostgres(expectedFields, schema, &count)

	assert.Len(t, missing, 3)
	assert.Equal(t, int32(3), count)
}

func TestValidateFieldsInSchemaPostgres_EmptyFields(t *testing.T) {
	t.Parallel()

	schema := TableSchema{
		SchemaName: "public",
		TableName:  "users",
		Columns: []ColumnInformation{
			{Name: "id", DataType: "uuid"},
		},
	}

	var count int32 = 0

	missing := ValidateFieldsInSchemaPostgres([]string{}, schema, &count)

	assert.Empty(t, missing)
	assert.Equal(t, int32(0), count)
}

func TestValidateFieldsInSchemaPostgres_EmptySchema(t *testing.T) {
	t.Parallel()

	schema := TableSchema{
		SchemaName: "public",
		TableName:  "empty_table",
		Columns:    []ColumnInformation{},
	}

	expectedFields := []string{"id", "name"}
	var count int32 = 0

	missing := ValidateFieldsInSchemaPostgres(expectedFields, schema, &count)

	assert.Len(t, missing, 2)
	assert.Equal(t, int32(2), count)
}

func TestValidateFieldsInSchemaPostgres_CaseInsensitive(t *testing.T) {
	t.Parallel()

	schema := TableSchema{
		SchemaName: "public",
		TableName:  "users",
		Columns: []ColumnInformation{
			{Name: "ID", DataType: "uuid"},
			{Name: "Name", DataType: "varchar"},
			{Name: "EMAIL", DataType: "varchar"},
		},
	}

	// Lowercase expected fields should match uppercase schema columns
	expectedFields := []string{"id", "name", "email"}
	var count int32 = 0

	missing := ValidateFieldsInSchemaPostgres(expectedFields, schema, &count)

	assert.Empty(t, missing)
	assert.Equal(t, int32(3), count)
}

func TestValidateFieldsInSchemaPostgres_CountAccumulation(t *testing.T) {
	t.Parallel()

	schema := TableSchema{
		SchemaName: "public",
		TableName:  "users",
		Columns: []ColumnInformation{
			{Name: "id", DataType: "uuid"},
		},
	}

	var count int32 = 10 // Start with existing count

	_ = ValidateFieldsInSchemaPostgres([]string{"id", "name"}, schema, &count)

	assert.Equal(t, int32(12), count) // 10 + 2
}

func TestValidateFieldsInSchemaPostgres_NestedJSONBFields(t *testing.T) {
	t.Parallel()

	// Schema with a JSONB column called "fee_charge"
	schema := TableSchema{
		SchemaName: "payment",
		TableName:  "transfers",
		Columns: []ColumnInformation{
			{Name: "id", DataType: "uuid"},
			{Name: "amount", DataType: "numeric"},
			{Name: "status", DataType: "varchar"},
			{Name: "fee_charge", DataType: "jsonb"},
			{Name: "metadata", DataType: "jsonb"},
		},
	}

	// Test nested JSONB field paths like "fee_charge.totalAmount"
	// These should validate successfully if the root column exists
	expectedFields := []string{
		"id",
		"amount",
		"fee_charge.totalAmount", // Nested JSONB path
		"fee_charge.currency",    // Another nested path
		"metadata.fees.amount",   // Deeply nested path
		"status",
	}
	var count int32 = 0

	missing := ValidateFieldsInSchemaPostgres(expectedFields, schema, &count)

	assert.Empty(t, missing, "Nested JSONB field paths should not be reported as missing")
	assert.Equal(t, int32(6), count)
}

func TestValidateFieldsInSchemaPostgres_NestedFieldMissingRootColumn(t *testing.T) {
	t.Parallel()

	schema := TableSchema{
		SchemaName: "payment",
		TableName:  "transfers",
		Columns: []ColumnInformation{
			{Name: "id", DataType: "uuid"},
			{Name: "amount", DataType: "numeric"},
		},
	}

	// "nonexistent.field" should be reported as missing because "nonexistent" column doesn't exist
	expectedFields := []string{"id", "nonexistent.field", "another.nested.path"}
	var count int32 = 0

	missing := ValidateFieldsInSchemaPostgres(expectedFields, schema, &count)

	assert.Len(t, missing, 2)
	assert.Contains(t, missing, "nonexistent.field")
	assert.Contains(t, missing, "another.nested.path")
}

func TestValidateFieldsInSchemaPostgres_MixedSimpleAndNested(t *testing.T) {
	t.Parallel()

	schema := TableSchema{
		SchemaName: "public",
		TableName:  "orders",
		Columns: []ColumnInformation{
			{Name: "id", DataType: "uuid"},
			{Name: "customer", DataType: "jsonb"},
			{Name: "total", DataType: "numeric"},
		},
	}

	// Mix of simple fields, valid nested fields, and invalid nested fields
	expectedFields := []string{
		"id",                    // Simple - exists
		"customer.name",         // Nested - root exists
		"customer.address.city", // Deeply nested - root exists
		"total",                 // Simple - exists
		"invalid.path",          // Nested - root doesn't exist
	}
	var count int32 = 0

	missing := ValidateFieldsInSchemaPostgres(expectedFields, schema, &count)

	assert.Len(t, missing, 1)
	assert.Contains(t, missing, "invalid.path")
}

func TestValidateFieldsInSchemaPostgres_DottedPathOnNonJSONBColumn(t *testing.T) {
	t.Parallel()

	// Test that dotted paths are validated based on root column existence only.
	// Note: The current implementation does NOT check if the column type is JSONB.
	// It only validates that the root column exists, regardless of type.
	// This is a design decision - the database query will fail at runtime if
	// the column doesn't support nested access.
	schema := TableSchema{
		SchemaName: "public",
		TableName:  "users",
		Columns: []ColumnInformation{
			{Name: "id", DataType: "uuid"},
			{Name: "name", DataType: "varchar"},   // Not JSONB
			{Name: "settings", DataType: "jsonb"}, // JSONB
			{Name: "created_at", DataType: "timestamp"},
		},
	}

	expectedFields := []string{
		"id",                // Simple - exists
		"name.first",        // Dotted path on varchar - root column exists (passes validation)
		"settings.theme",    // Dotted path on jsonb - root column exists
		"created_at.year",   // Dotted path on timestamp - root column exists
		"nonexistent.field", // Dotted path where root doesn't exist - should be missing
	}
	var count int32 = 0

	missing := ValidateFieldsInSchemaPostgres(expectedFields, schema, &count)

	// Current behavior: Only validates root column existence, not type
	// Only "nonexistent.field" should be missing because "nonexistent" column doesn't exist
	assert.Len(t, missing, 1, "Only dotted paths with non-existent root columns should be missing")
	assert.Contains(t, missing, "nonexistent.field", "Dotted path with non-existent root should be missing")
	assert.Equal(t, int32(5), count)
}

func TestColumnInformation_Struct(t *testing.T) {
	t.Parallel()

	col := ColumnInformation{
		Name:         "user_id",
		DataType:     "uuid",
		IsNullable:   false,
		IsPrimaryKey: true,
	}

	assert.Equal(t, "user_id", col.Name)
	assert.Equal(t, "uuid", col.DataType)
	assert.False(t, col.IsNullable)
	assert.True(t, col.IsPrimaryKey)
}

func TestConnection_Struct(t *testing.T) {
	t.Parallel()

	conn := Connection{
		ConnectionString:   "postgres://user:pass@localhost:5432/db",
		DBName:             "testdb",
		Connected:          false,
		MaxOpenConnections: 10,
		MaxIdleConnections: 5,
	}

	assert.Equal(t, "postgres://user:pass@localhost:5432/db", conn.ConnectionString)
	assert.Equal(t, "testdb", conn.DBName)
	assert.False(t, conn.Connected)
	assert.Equal(t, 10, conn.MaxOpenConnections)
	assert.Equal(t, 5, conn.MaxIdleConnections)
	assert.Nil(t, conn.ConnectionDB)
}

func TestTableSchema_Struct(t *testing.T) {
	t.Parallel()

	columns := []ColumnInformation{
		{Name: "id", DataType: "uuid", IsPrimaryKey: true},
		{Name: "name", DataType: "varchar", IsNullable: true},
	}

	schema := TableSchema{
		SchemaName: "public",
		TableName:  "users",
		Columns:    columns,
	}

	assert.Equal(t, "public", schema.SchemaName)
	assert.Equal(t, "users", schema.TableName)
	assert.Len(t, schema.Columns, 2)
	assert.Equal(t, "id", schema.Columns[0].Name)
	assert.Equal(t, "name", schema.Columns[1].Name)
}

func TestExtractRootColumn_SimpleField(t *testing.T) {
	t.Parallel()

	// Simple fields should be returned as-is
	assert.Equal(t, "id", extractRootColumn("id"))
	assert.Equal(t, "amount", extractRootColumn("amount"))
	assert.Equal(t, "created_at", extractRootColumn("created_at"))
}

func TestExtractRootColumn_NestedField(t *testing.T) {
	t.Parallel()

	// Nested fields should return only the root column
	assert.Equal(t, "fee_charge", extractRootColumn("fee_charge.totalAmount"))
	assert.Equal(t, "metadata", extractRootColumn("metadata.version"))
	assert.Equal(t, "data", extractRootColumn("data.user.name"))
	assert.Equal(t, "config", extractRootColumn("config.settings.theme.color"))
}

func TestTransformFieldsForSelect(t *testing.T) {
	t.Parallel()

	fields := []string{
		"id",
		"amount",
		"fee_charge.totalAmount",
		"metadata.user.name",
		"status",
	}

	result := transformFieldsForSelect(fields)

	// Should extract root columns and deduplicate
	assert.Len(t, result, 5)
	assert.Contains(t, result, "id")
	assert.Contains(t, result, "amount")
	assert.Contains(t, result, "fee_charge")
	assert.Contains(t, result, "metadata")
	assert.Contains(t, result, "status")
}

func TestTransformFieldsForSelect_Deduplication(t *testing.T) {
	t.Parallel()

	// Multiple nested fields from the same root column should result in single column
	fields := []string{
		"id",
		"fee_charge.totalAmount",
		"fee_charge.currency",
		"fee_charge.breakdown.tax",
		"metadata.version",
	}

	result := transformFieldsForSelect(fields)

	// fee_charge should appear only once
	assert.Len(t, result, 3)
	assert.Contains(t, result, "id")
	assert.Contains(t, result, "fee_charge")
	assert.Contains(t, result, "metadata")
}

func TestTransformFieldsForSelect_EmptySlice(t *testing.T) {
	t.Parallel()

	result := transformFieldsForSelect([]string{})
	assert.Empty(t, result)
}

func TestTransformFieldsForSelect_AllSimple(t *testing.T) {
	t.Parallel()

	fields := []string{"id", "name", "email"}
	result := transformFieldsForSelect(fields)

	assert.Equal(t, fields, result)
}
