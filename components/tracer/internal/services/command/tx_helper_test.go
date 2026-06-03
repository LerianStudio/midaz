// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "tracer/internal/adapters/postgres/db"
	pgdbMocks "tracer/internal/adapters/postgres/db/mocks"
)

// TestExecuteInTx_NilTxBeginner verifies that executeInTx returns
// pgdb.ErrNilConnection when the txBeginner is nil, rather than panicking on
// the BeginTx call. pgdb.NewTxBeginnerAdapter returns nil for a nil
// dbresolver.DB, so callers can land here if bootstrap wiring is incomplete
// or tests omit the txBeginner dependency.
func TestExecuteInTx_NilTxBeginner(t *testing.T) {
	t.Parallel()

	fnCalled := false

	err := executeInTx(context.Background(), nil, func(_ pgdb.DB) error {
		fnCalled = true
		return nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, pgdb.ErrNilConnection, "nil txBeginner must surface as pgdb.ErrNilConnection")
	assert.False(t, fnCalled, "callback must not be invoked when txBeginner is nil")
}

// TestExecuteInTx_Success verifies that executeInTx commits the transaction
// and returns nil when the callback succeeds.
func TestExecuteInTx_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	// BeginTx succeeds, fn runs with the returned tx, Commit succeeds.
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil).Times(1)
	mockTx.EXPECT().Commit().Return(nil).Times(1)

	var received pgdb.DB

	err := executeInTx(context.Background(), mockTxBeginner, func(db pgdb.DB) error {
		received = db
		return nil
	})

	require.NoError(t, err)
	assert.Same(t, mockTx, received, "callback must receive the tx returned by BeginTx")
}

// TestExecuteInTx_FnError_Rollback verifies that executeInTx rolls back
// the transaction and propagates the callback error when fn returns an error.
func TestExecuteInTx_FnError_Rollback(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	fnErr := errors.New("business failure")

	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil).Times(1)
	// On fn error: Rollback MUST be called, Commit MUST NOT be called.
	mockTx.EXPECT().Rollback().Return(nil).Times(1)

	err := executeInTx(context.Background(), mockTxBeginner, func(_ pgdb.DB) error {
		return fnErr
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, fnErr, "callback error must be propagated unchanged")
}

// TestExecuteInTx_BeginError verifies that executeInTx returns a wrapped
// error (no callback invocation, no Commit/Rollback) when BeginTx fails.
func TestExecuteInTx_BeginError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	beginErr := errors.New("connection refused")

	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, beginErr).Times(1)

	fnCalled := false

	err := executeInTx(context.Background(), mockTxBeginner, func(_ pgdb.DB) error {
		fnCalled = true
		return nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, beginErr, "BeginTx error must be wrapped with %%w")
	assert.False(t, fnCalled, "callback must not be invoked when BeginTx fails")
}

// TestExecuteInTx_BeginReturnsNilTxWithoutError covers the defensive branch in
// executeInTx where BeginTx returns (nil, nil) — a driver contract violation
// that would otherwise lead to a nil-pointer panic on the first Commit/Rollback
// call. executeInTx must surface an explicit error and must not invoke the
// callback when the transaction handle is nil.
func TestExecuteInTx_BeginReturnsNilTxWithoutError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, nil).Times(1)

	fnCalled := false
	err := executeInTx(context.Background(), mockTxBeginner, func(_ pgdb.DB) error {
		fnCalled = true
		return nil
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "BeginTx returned nil transaction without error")
	assert.False(t, fnCalled, "callback must not be invoked when tx is nil")
}

// TestExecuteInTx_CommitError verifies that executeInTx returns a wrapped
// error when Commit fails after a successful callback.
func TestExecuteInTx_CommitError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	commitErr := errors.New("commit failed")

	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil).Times(1)
	mockTx.EXPECT().Commit().Return(commitErr).Times(1)
	// Commit failure is a rollback-eligible path: defer-based cleanup must
	// invoke Rollback to release any held locks. Rollback after a failed
	// Commit is a no-op for *sql.Tx but we still expect the call.
	mockTx.EXPECT().Rollback().Return(nil).Times(1)

	err := executeInTx(context.Background(), mockTxBeginner, func(_ pgdb.DB) error {
		return nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, commitErr, "Commit error must be wrapped with %%w")
}

// TestExecuteInTx_PanicInFn_Rollback verifies that when fn panics, executeInTx
// still rolls back the transaction (via its deferred cleanup) and converts
// the panic into a returned error rather than re-raising it. CLAUDE.md forbids
// panic propagation in production code, so the helper must never re-raise.
func TestExecuteInTx_PanicInFn_Rollback(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(mockTx, nil).Times(1)
	// Panic in fn: Commit MUST NOT be called, Rollback MUST be called exactly once.
	mockTx.EXPECT().Rollback().Return(nil).Times(1)

	err := executeInTx(context.Background(), mockTxBeginner, func(_ pgdb.DB) error {
		panic("simulated panic inside fn")
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "transaction callback panicked")
	require.Contains(t, err.Error(), "simulated panic inside fn")
}
