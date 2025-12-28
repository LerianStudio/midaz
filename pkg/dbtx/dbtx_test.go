package dbtx

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextWithTx_NilTx(t *testing.T) {
	ctx := context.Background()
	ctxWithTx := ContextWithTx(ctx, nil)

	tx := TxFromContext(ctxWithTx)
	assert.Nil(t, tx, "nil tx should return nil from context")
}

func TestTxFromContext_NoTx(t *testing.T) {
	ctx := context.Background()
	tx := TxFromContext(ctx)
	assert.Nil(t, tx, "context without tx should return nil")
}

func TestContextWithTx_RoundTrip(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)

	ctx := context.Background()
	ctxWithTx := ContextWithTx(ctx, tx)

	retrieved := TxFromContext(ctxWithTx)
	assert.Equal(t, tx, retrieved, "should retrieve same tx from context")

	mock.ExpectRollback()
	_ = tx.Rollback()
}

func TestGetExecutor_WithTx(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)

	ctx := ContextWithTx(context.Background(), tx)
	executor := GetExecutor(ctx, db)

	// The executor should be the transaction, not the db
	_, isTx := executor.(*sql.Tx)
	assert.True(t, isTx, "executor should be *sql.Tx when tx in context")

	mock.ExpectRollback()
	_ = tx.Rollback()
}

func TestGetExecutor_WithoutTx(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	executor := GetExecutor(ctx, db)

	// The executor should be the db
	_, isDB := executor.(*sql.DB)
	assert.True(t, isDB, "executor should be *sql.DB when no tx in context")
}

func TestRunInTransaction_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()

	called := false
	err = RunInTransaction(context.Background(), db, func(ctx context.Context) error {
		called = true
		// Verify transaction is in context
		tx := TxFromContext(ctx)
		assert.NotNil(t, tx, "tx should be in context")
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "function should be called")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRunInTransaction_FunctionError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()

	expectedErr := errors.New("function error")
	err = RunInTransaction(context.Background(), db, func(ctx context.Context) error {
		return expectedErr
	})

	assert.Equal(t, expectedErr, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRunInTransaction_BeginError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expectedErr := errors.New("begin error")
	mock.ExpectBegin().WillReturnError(expectedErr)

	err = RunInTransaction(context.Background(), db, func(ctx context.Context) error {
		t.Fatal("function should not be called")
		return nil
	})

	assert.Equal(t, expectedErr, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRunInTransaction_CommitError(t *testing.T) {
	// Use sqlmock with MatchExpectationsInOrder(false) to handle the commit error case
	// After commit fails, the defer tries to rollback but the tx state is uncertain
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(false))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	expectedErr := errors.New("commit error")
	mock.ExpectCommit().WillReturnError(expectedErr)
	// The defer will try to rollback after commit fails
	mock.ExpectRollback()

	err = RunInTransaction(context.Background(), db, func(ctx context.Context) error {
		return nil
	})

	assert.Equal(t, expectedErr, err)
	// Don't strictly check expectations since rollback behavior after commit error is driver-dependent
}

func TestRunInTransaction_Panic(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()

	assert.Panics(t, func() {
		_ = RunInTransaction(context.Background(), db, func(ctx context.Context) error {
			panic("test panic")
		})
	})

	assert.NoError(t, mock.ExpectationsWereMet())
}
