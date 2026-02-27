// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPlaceholderPostgresConnection creates a *libPostgres.PostgresConnection
// with minimal fields set. This simulates a multi-tenant bootstrap mode where
// the static connection is a placeholder and the real database comes from the
// per-request tenant context.
func newPlaceholderPostgresConnection() *libPostgres.PostgresConnection {
	return &libPostgres.PostgresConnection{
		Logger: &libLog.NoneLogger{},
	}
}

// =============================================================================
// getDB Tests -- Multi-Tenant Path
// =============================================================================

func TestGetDB_ReturnsTenantDB_WhenContextHasModulePostgres(t *testing.T) {
	t.Parallel()

	// Use two distinct mockDB instances so assert.Same proves identity,
	// not just type. The second sub-case verifies that the specific tenant DB
	// injected into context is the one returned -- not some other instance.
	tenantDBAlpha := &mockDB{}
	tenantDBBeta := &mockDB{}

	tests := []struct {
		name     string
		tenantDB dbresolver.DB
	}{
		{
			name:     "tenant_pg_connection_present_in_context",
			tenantDB: tenantDBAlpha,
		},
		{
			name:     "returns_exact_tenant_db_instance_not_any_mock",
			tenantDB: tenantDBBeta,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			repo := &TransactionPostgreSQLRepository{
				connection: newPlaceholderPostgresConnection(),
				tableName:  "transaction",
			}
			ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", tt.tenantDB)

			// Act
			db, err := repo.getDB(ctx)

			// Assert
			require.NoError(t, err, "getDB should not return error when tenant DB is in context")
			require.NotNil(t, db, "returned database must not be nil")
			assert.Same(t, tt.tenantDB, db, "must return the exact tenant DB from context")
		})
	}
}

// =============================================================================
// getDB Tests -- Single-Tenant Fallback Path
// =============================================================================

func TestGetDB_FallsBackToStaticConnection_WhenNoTenantContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ctx  context.Context
	}{
		{
			name: "plain_background_context_no_tenant",
			ctx:  context.Background(),
		},
		{
			name: "context_with_unrelated_values",
			ctx:  context.WithValue(context.Background(), struct{}{}, "unrelated"),
		},
		{
			name: "context_with_tenant_db_for_different_module",
			ctx:  tmcore.ContextWithModulePGConnection(context.Background(), "onboarding", &mockDB{}),
		},
		{
			name: "context_with_tenant_db_for_empty_module_name",
			ctx:  tmcore.ContextWithModulePGConnection(context.Background(), "", &mockDB{}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange -- placeholder connection without initialized DB pools.
			// GetDB will attempt to connect and fail since there are no real servers.
			repo := &TransactionPostgreSQLRepository{
				connection: newPlaceholderPostgresConnection(),
				tableName:  "transaction",
			}

			// Act
			db, err := repo.getDB(tt.ctx)

			// Assert -- the static connection path runs but fails because there is
			// no live PostgreSQL server. This proves the fallback path was taken.
			require.Error(t, err, "getDB should return error when static connection has no live PostgreSQL")
			assert.Nil(t, db, "database should be nil when static connection fails")
		})
	}
}

// =============================================================================
// mockDB -- minimal dbresolver.DB implementation for unit testing
// =============================================================================

// mockDB is a no-op implementation of dbresolver.DB for testing.
// It satisfies the interface so we can use assert.Same to verify identity.
type mockDB struct {
	dbresolver.DB
}
