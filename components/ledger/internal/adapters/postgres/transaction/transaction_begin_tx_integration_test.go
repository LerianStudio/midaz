//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"database/sql"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/require"
)

// createBeginTxProbe adds a scratch table to db so a write made through the
// repository's transaction can be checked for visibility from outside it.
func createBeginTxProbe(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.ExecContext(context.Background(), `CREATE TABLE IF NOT EXISTS begin_tx_probe (id TEXT PRIMARY KEY)`)
	require.NoError(t, err)
}

func countBeginTxProbe(t *testing.T, db *sql.DB, id string) int {
	t.Helper()

	var count int

	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT count(*) FROM begin_tx_probe WHERE id = $1`, id).Scan(&count))

	return count
}

func beginSQLTx(t *testing.T, repo *TransactionPostgreSQLRepository, ctx context.Context) *sql.Tx {
	t.Helper()

	tx, err := repo.BeginTx(ctx)
	require.NoError(t, err)

	sqlTx, ok := tx.(*sql.Tx)
	require.Truef(t, ok, "BeginTx must return the primary pool's *sql.Tx, got %T", tx)

	return sqlTx
}

// TestIntegration_TransactionRepo_BeginTx_ReturnsPrimarySQLTx proves the
// transaction is a concrete *sql.Tx on the pool getDB resolves, in both the
// static and the tenant-context mode, and that it commits and rolls back there.
func TestIntegration_TransactionRepo_BeginTx_ReturnsPrimarySQLTx(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupIntegrationInfra(t)
	tenantContainer, tenantDB := setupTenantContainer(t)
	createBeginTxProbe(t, infra.pgContainer.DB)
	createBeginTxProbe(t, tenantContainer.DB)

	for _, scenario := range []struct {
		name  string
		ctx   context.Context
		db    *sql.DB
		other *sql.DB
	}{
		{name: "static connection", ctx: context.Background(), db: infra.pgContainer.DB, other: tenantContainer.DB},
		{name: "tenant connection from context", ctx: tmcore.ContextWithPG(context.Background(), tenantDB), db: tenantContainer.DB, other: infra.pgContainer.DB},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			committedID := t.Name() + ":commit"
			tx := beginSQLTx(t, infra.repo, scenario.ctx)

			_, err := tx.ExecContext(scenario.ctx, `INSERT INTO begin_tx_probe (id) VALUES ($1)`, committedID)
			require.NoError(t, err)
			require.Equal(t, 0, countBeginTxProbe(t, scenario.db, committedID), "an uncommitted write must not be visible outside the transaction")
			require.NoError(t, tx.Commit())
			require.Equal(t, 1, countBeginTxProbe(t, scenario.db, committedID))
			require.Equal(t, 0, countBeginTxProbe(t, scenario.other, committedID), "the transaction must run on the resolved pool only")

			rolledBackID := t.Name() + ":rollback"
			tx = beginSQLTx(t, infra.repo, scenario.ctx)

			_, err = tx.ExecContext(scenario.ctx, `INSERT INTO begin_tx_probe (id) VALUES ($1)`, rolledBackID)
			require.NoError(t, err)
			require.NoError(t, tx.Rollback())
			require.Equal(t, 0, countBeginTxProbe(t, scenario.db, rolledBackID))
		})
	}
}
