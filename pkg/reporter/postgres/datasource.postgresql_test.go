// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/lib/pq" // Registers "postgres" driver for sql.Open in tests.
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTableSchema_QualifiedName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		schema     TableSchema
		wantResult string
	}{
		{
			name: "returns qualified name with schema and table",
			schema: TableSchema{
				SchemaName: "sales",
				TableName:  "orders",
			},
			wantResult: "sales.orders",
		},
		{
			name: "returns qualified name for public schema",
			schema: TableSchema{
				SchemaName: "public",
				TableName:  "users",
			},
			wantResult: "public.users",
		},
		{
			name: "handles empty schema name",
			schema: TableSchema{
				SchemaName: "",
				TableName:  "accounts",
			},
			wantResult: "accounts",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.schema.QualifiedName()
			if got != tt.wantResult {
				t.Errorf("QualifiedName() = %q, want %q", got, tt.wantResult)
			}
		})
	}
}

func TestTableSchema_SchemaNameField(t *testing.T) {
	t.Parallel()

	// Test that SchemaName field exists and can be set
	ts := TableSchema{
		SchemaName: "sales",
		TableName:  "orders",
		Columns:    []ColumnInformation{},
	}

	if ts.SchemaName != "sales" {
		t.Errorf("SchemaName = %q, want %q", ts.SchemaName, "sales")
	}

	if ts.TableName != "orders" {
		t.Errorf("TableName = %q, want %q", ts.TableName, "orders")
	}
}

// ---------------------------------------------------------------------------
// Ping — lightweight connectivity probe used by the HealthChecker.
// Replaces the previous "lightweight" GetDatabaseSchema-as-ping that actually
// performed full information_schema scans. PingContext on *sql.DB issues a
// SELECT 1-equivalent under the hood, which is the real lightweight path.
// ---------------------------------------------------------------------------

func TestExternalDataSource_Ping_NilReceiver_ReturnsError(t *testing.T) {
	t.Parallel()

	var ds *ExternalDataSource

	err := ds.Ping(context.Background())
	require.Error(t, err, "nil receiver must not panic and must return error")
	assert.Contains(t, err.Error(), "postgres connection not initialized")
}

func TestExternalDataSource_Ping_NilConnection_ReturnsError(t *testing.T) {
	t.Parallel()

	ds := &ExternalDataSource{connection: nil}

	err := ds.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "postgres connection not initialized")
}

func TestExternalDataSource_Ping_NilConnectionDB_ReturnsError(t *testing.T) {
	t.Parallel()

	ds := &ExternalDataSource{connection: &Connection{ConnectionDB: nil}}

	err := ds.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "postgres connection not initialized")
}

// TestExternalDataSource_Ping_DelegatesToSQLDB verifies that when the
// underlying *sql.DB is set, Ping delegates to PingContext. We use the
// lib/pq driver to construct a *sql.DB without actually dialing — calling
// PingContext on this handle MUST attempt a real network call and fail
// (which is the correct behavior: the wrapper does not swallow errors).
func TestExternalDataSource_Ping_DelegatesToSQLDB(t *testing.T) {
	t.Parallel()

	// Open a *sql.DB pointed at an unreachable address. sql.Open never
	// dials, so this never blocks; only PingContext attempts I/O.
	db, err := sql.Open("postgres", "postgres://nonexistent.invalid:1/none?sslmode=disable&connect_timeout=1")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	ds := &ExternalDataSource{connection: &Connection{ConnectionDB: db}}

	ctx, cancel := context.WithTimeout(context.Background(), 1) // 1ns — already expired
	defer cancel()

	// PingContext on an expired context must surface an error rather than
	// silently succeeding. This pins the contract that Ping does not mask
	// connectivity failures.
	pingErr := ds.Ping(ctx)
	require.Error(t, pingErr, "expired context must yield error")
}
