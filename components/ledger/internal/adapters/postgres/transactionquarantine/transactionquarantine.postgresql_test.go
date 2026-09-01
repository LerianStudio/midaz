// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transactionquarantine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewQuarantinePostgreSQLRepository_Defaults verifies the single-tenant
// constructor leaves requireTenant false and sets the canonical table name.
func TestNewQuarantinePostgreSQLRepository_Defaults(t *testing.T) {
	t.Parallel()

	r := NewQuarantinePostgreSQLRepository(nil)

	require.NotNil(t, r)
	assert.Equal(t, "transaction_backup_quarantine", r.tableName)
	assert.False(t, r.requireTenant, "single-tenant constructor must leave requireTenant false")
}

// TestNewQuarantinePostgreSQLRepository_MultiTenant verifies the multi-tenant
// variant sets requireTenant so getDB fails closed without a tenant connection.
func TestNewQuarantinePostgreSQLRepository_MultiTenant(t *testing.T) {
	t.Parallel()

	r := NewQuarantinePostgreSQLRepository(nil, true)

	require.NotNil(t, r)
	assert.True(t, r.requireTenant, "multi-tenant constructor must set requireTenant")
}

// TestGetDB_FailsClosedWhenTenantRequired verifies that in multi-tenant mode
// getDB returns an error rather than falling back to a static connection when
// no tenant connection is present in context. A poison financial record must
// never be written to the wrong tenant database.
func TestGetDB_FailsClosedWhenTenantRequired(t *testing.T) {
	t.Parallel()

	r := NewQuarantinePostgreSQLRepository(nil, true)

	db, err := r.getDB(context.Background())

	require.Error(t, err, "getDB must fail closed when requireTenant and no tenant in context")
	assert.Nil(t, db)
}

// TestGetDB_NoConnectionSingleTenant verifies single-tenant mode returns a clear
// error when no connection was wired (rather than panicking on a nil resolver).
func TestGetDB_NoConnectionSingleTenant(t *testing.T) {
	t.Parallel()

	r := NewQuarantinePostgreSQLRepository(nil)

	db, err := r.getDB(context.Background())

	require.Error(t, err, "getDB must error when no connection is available")
	assert.Nil(t, db)
}
