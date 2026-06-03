// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"

	"github.com/LerianStudio/reporter/pkg/model"

	"github.com/LerianStudio/lib-observability/log"
)

// Repository defines an interface for querying data from a specified table and fields.
//
//go:generate mockgen --destination=datasource.postgresql.mock.go --package=postgres --copyright_file=../../COPYRIGHT . Repository
type Repository interface {
	Query(ctx context.Context, schema []TableSchema, schemaName string, table string, fields []string, filter map[string][]any) ([]map[string]any, error)
	QueryWithAdvancedFilters(ctx context.Context, schema []TableSchema, schemaName string, table string, fields []string, filter map[string]model.FilterCondition) ([]map[string]any, error)
	GetDatabaseSchema(ctx context.Context, schemas []string) ([]TableSchema, error)
	CloseConnection() error

	// Ping verifies connectivity with a minimal query (SELECT 1-equivalent
	// via *sql.DB.PingContext). Used by health checks to avoid the cost of
	// GetDatabaseSchema, which performs a full information_schema scan.
	Ping(ctx context.Context) error
}

// qualifyTableName returns a qualified table name with schema if provided.
func qualifyTableName(schemaName, tableName string) string {
	if schemaName == "" {
		return tableName
	}

	return fmt.Sprintf(`"%s"."%s"`, schemaName, tableName)
}

// TableSchema represents the structure of a database table.
type TableSchema struct {
	SchemaName string              `json:"schema_name"`
	TableName  string              `json:"table_name"`
	Columns    []ColumnInformation `json:"columns"`
}

// QualifiedName returns the fully qualified table name in the format "schema.table".
// When the schema is empty, only the table name is returned.
func (t TableSchema) QualifiedName() string {
	if t.SchemaName == "" {
		return t.TableName
	}

	return fmt.Sprintf("%s.%s", t.SchemaName, t.TableName)
}

// ColumnInformation contains the details of a database column.
type ColumnInformation struct {
	Name         string `json:"name"`
	DataType     string `json:"data_type"`
	IsNullable   bool   `json:"is_nullable"`
	IsPrimaryKey bool   `json:"is_primary_key"`
}

// ExternalDataSource provides an interface for interacting with a PostgreSQL database connection.
type ExternalDataSource struct {
	connection *Connection
}

// Compile-time interface satisfaction check.
var _ Repository = (*ExternalDataSource)(nil)

// NewDataSourceRepository creates a new ExternalDataSource instance using the provided postgres.Connection.
func NewDataSourceRepository(pc *Connection) (*ExternalDataSource, error) {
	if pc == nil {
		return nil, fmt.Errorf("postgres connection must not be nil")
	}

	c := &ExternalDataSource{connection: pc}

	_, err := c.connection.GetDB()
	if err != nil {
		pc.Logger.Log(context.Background(), log.LevelError, "Failed to establish PostgreSQL connection", log.Err(err))
		return nil, fmt.Errorf("failed to establish PostgreSQL connection: %w", err)
	}

	return c, nil
}

// Ping verifies connectivity with a minimal query against the underlying
// *sql.DB. The stdlib PingContext issues a SELECT 1-equivalent and is the
// canonical lightweight reachability probe — replacing the previous
// GetDatabaseSchema-as-ping implementation that misleadingly performed a
// full information_schema scan on every health-check pass.
//
// Returns an error (rather than panicking) when the receiver, the
// connection wrapper, or the underlying *sql.DB are nil. This matches the
// nil-safety contract that the HealthChecker relies on: a nil-safe Ping
// lets the checker mark the datasource Unavailable instead of crashing the
// process during a stale-connection window.
func (ds *ExternalDataSource) Ping(ctx context.Context) error {
	if ds == nil || ds.connection == nil || ds.connection.ConnectionDB == nil {
		return fmt.Errorf("postgres connection not initialized")
	}

	return ds.connection.ConnectionDB.PingContext(ctx)
}

// CloseConnection closes the connection with PostgreSQL.
func (ds *ExternalDataSource) CloseConnection() error {
	if ds.connection.ConnectionDB != nil {
		ds.connection.Logger.Log(context.Background(), log.LevelInfo, "Closing connection to PostgreSQL...")

		if err := ds.connection.ConnectionDB.Close(); err != nil {
			ds.connection.Logger.Log(context.Background(), log.LevelError, "Error closing PostgreSQL connection", log.Err(err))
			return err
		}

		ds.connection.Connected = false
		ds.connection.ConnectionDB = nil
		ds.connection.Logger.Log(context.Background(), log.LevelInfo, "PostgreSQL connection closed successfully.")
	}

	return nil
}
