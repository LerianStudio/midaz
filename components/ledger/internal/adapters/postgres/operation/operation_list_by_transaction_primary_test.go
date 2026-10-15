// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// primaryReadDB records how ListByTransactionPrimary opens its read: the
// read-only transaction is what makes dbresolver target the primary, so a read
// issued on the database handle itself could be served by a lagging replica.
type primaryReadDB struct {
	*mockOperationDB
	beginErr error
	opts     *sql.TxOptions
	tx       *recordingReadTx
}

func (db *primaryReadDB) BeginTx(_ context.Context, opts *sql.TxOptions) (dbresolver.Tx, error) {
	db.opts = opts
	if db.beginErr != nil {
		return nil, db.beginErr
	}

	return db.tx, nil
}

// recordingReadTx fails every query it receives, after recording it, so the test
// observes the routing without fabricating rows.
type recordingReadTx struct {
	dbresolver.Tx
	queried    bool
	rolledBack bool
}

var errQueryReachedTransaction = errors.New("query reached the read-only transaction")

func (tx *recordingReadTx) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	tx.queried = true

	return nil, errQueryReachedTransaction
}

func (tx *recordingReadTx) Rollback() error {
	tx.rolledBack = true

	return nil
}

func (tx *recordingReadTx) Commit() error { return nil }

func TestListByTransactionPrimaryQueriesThroughAReadOnlyTransaction(t *testing.T) {
	t.Parallel()

	db := &primaryReadDB{mockOperationDB: &mockOperationDB{}, tx: &recordingReadTx{}}
	repo := &OperationPostgreSQLRepository{tableName: "operation"}

	_, err := repo.ListByTransactionPrimary(tmcore.ContextWithPG(context.Background(), db), uuid.New(), uuid.New(), uuid.New())

	require.ErrorIs(t, err, errQueryReachedTransaction)
	require.NotNil(t, db.opts)
	assert.True(t, db.opts.ReadOnly, "the read must open a read-only transaction to reach the primary")
	assert.True(t, db.tx.queried, "the query must run inside that transaction")
	assert.Nil(t, db.queryArgs, "the query must not run on the database handle, which a replica may serve")
	assert.True(t, db.tx.rolledBack, "a failed read rolls the transaction back")
}

func TestListByTransactionPrimaryFailsClosedWhenThePrimaryTransactionCannotOpen(t *testing.T) {
	t.Parallel()

	beginErr := errors.New("primary unavailable")
	db := &primaryReadDB{mockOperationDB: &mockOperationDB{}, beginErr: beginErr, tx: &recordingReadTx{}}
	repo := &OperationPostgreSQLRepository{tableName: "operation"}

	operations, err := repo.ListByTransactionPrimary(tmcore.ContextWithPG(context.Background(), db), uuid.New(), uuid.New(), uuid.New())

	require.ErrorIs(t, err, beginErr)
	assert.Nil(t, operations)
	assert.Nil(t, db.queryArgs, "no fallback read on the database handle")
	assert.False(t, db.tx.queried)
}
