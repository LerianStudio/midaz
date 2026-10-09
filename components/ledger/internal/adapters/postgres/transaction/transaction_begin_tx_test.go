// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// noPrimaryDB is a dbresolver.DB whose primary pool list is empty. dbresolver.New
// refuses that shape at construction, so a fake is the only way to reach it.
type noPrimaryDB struct {
	dbresolver.DB
}

func (noPrimaryDB) PrimaryDBs() []*sql.DB { return nil }

func TestBeginTxReturnsPrimaryPoolSQLTx(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name   string
		module string
	}{
		{name: "generic tenant connection"},
		{name: "transaction module connection", module: constant.ModuleTransaction},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			mock.ExpectBegin()
			mock.ExpectRollback()

			resolver := dbresolver.New(dbresolver.WithPrimaryDBs(db))
			ctx := tmcore.ContextWithPG(context.Background(), resolver, scenario.module)
			repo := &TransactionPostgreSQLRepository{connection: newPlaceholderPostgresConnection(), tableName: "transaction"}

			tx, err := repo.BeginTx(ctx)
			require.NoError(t, err)

			sqlTx, ok := tx.(*sql.Tx)
			require.Truef(t, ok, "BeginTx must return the primary pool's *sql.Tx, got %T", tx)
			require.NoError(t, sqlTx.Rollback())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestBeginTxRefusesResolverWithoutPrimaryPool(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithPG(context.Background(), noPrimaryDB{})
	repo := &TransactionPostgreSQLRepository{connection: newPlaceholderPostgresConnection(), tableName: "transaction"}

	tx, err := repo.BeginTx(ctx)
	require.ErrorContains(t, err, "primary pool")
	assert.Nil(t, tx)
}
