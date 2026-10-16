// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package completion

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
)

// tracedTransactionStore opens the span the real transaction repository opens around
// its bulk insert, recording its own failure on it, so the test sees the nesting.
type tracedTransactionStore struct {
	transactionStoreStub
}

func (store tracedTransactionStore) CreateBulkTx(ctx context.Context, tx repository.DBExecutor, transactions []*transaction.Transaction) (*repository.BulkInsertResult, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_bulk_transactions_tx")
	defer span.End()

	result, err := store.transactionStoreStub.CreateBulkTx(ctx, tx, transactions)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to insert transaction chunk", err)
	}

	return result, err
}

// tracedOperationStore is the operation repository counterpart of tracedTransactionStore.
type tracedOperationStore struct {
	operationStoreStub
}

func (store tracedOperationStore) CreateBulkTx(ctx context.Context, tx repository.DBExecutor, operations []*operation.Operation) (*repository.BulkInsertResult, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_bulk_operations_tx")
	defer span.End()

	result, err := store.operationStoreStub.CreateBulkTx(ctx, tx, operations)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to insert operation chunk", err)
	}

	return result, err
}

func newTracedStoreTest(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	})

	return NewStore(tracedTransactionStore{transactionStoreStub{db: db}}, tracedOperationStore{}), mock
}

// assertChildrenOf asserts that every named span is a direct child of parent.
func assertChildrenOf(t *testing.T, recorder *tracetest.SpanRecorder, parent sdktrace.ReadOnlySpan, children ...string) {
	t.Helper()

	for _, name := range children {
		child := spantest.FindSpan(t, recorder, name)
		assert.Equal(t, parent.SpanContext().SpanID(), child.Parent().SpanID(), "%s must be a child of %s", name, parent.Name())
	}
}

// expectCreatedTransaction queues the SQL of a first-time persistence of the frozen record.
func expectCreatedTransaction(mock sqlmock.Sqlmock) {
	expectTransaction(mock, "", true, true)
	mock.ExpectExec("INSERT recovery_transaction").WillReturnResult(sqlmock.NewResult(0, 1))
	expectTransaction(mock, constant.APPROVED, true, true)
	expectOperations(mock, 1)
}

// Scenario: Persistência da projeção tem span de domínio. Persist delegates to
// PersistWithOutcome without opening a span of its own.
func TestCompletionSpans_PersistHasOneDomainSpanAboveTheBulkInserts(t *testing.T) {
	store, mock := newTracedStoreTest(t)
	mock.ExpectBegin()
	expectCreatedTransaction(mock)
	mock.ExpectCommit()

	ctx, recorder := spantest.RecordingContext()

	require.NoError(t, store.Persist(ctx, frozenStoreRecord()))

	assert.Equal(t, 1, spantest.CountSpans(recorder, "postgres.persist_transaction"), "Persist opens no second span")

	parent := spantest.FindSpan(t, recorder, "postgres.persist_transaction")
	assert.Equal(t, codes.Unset, parent.Status().Code)
	assertChildrenOf(t, recorder, parent, "postgres.create_bulk_transactions_tx", "postgres.create_bulk_operations_tx")
}

// Scenario: Persistência da projeção tem span de domínio (bulk).
func TestCompletionSpans_PersistBulkHasOneDomainSpanAboveTheBulkInserts(t *testing.T) {
	store, mock := newTracedStoreTest(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM transaction WHERE id IN \(\$1\) ORDER BY id FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"})).RowsWillBeClosed()
	expectCreatedTransaction(mock)
	mock.ExpectCommit()

	ctx, recorder := spantest.RecordingContext()

	outcomes, err := store.PersistBulkWithOutcome(ctx, []command.TransactionWriteSet{frozenStoreRecord()})
	require.NoError(t, err)
	require.Len(t, outcomes, 1)

	parent := spantest.FindSpan(t, recorder, "postgres.persist_transactions_bulk")
	assert.Equal(t, codes.Unset, parent.Status().Code)
	assertChildrenOf(t, recorder, parent, "postgres.create_bulk_transactions_tx", "postgres.create_bulk_operations_tx")
}

// A failure at the completion level (here the commit) marks the domain span.
func TestCompletionSpans_OwnLevelFailureIsRecordedOnTheDomainSpan(t *testing.T) {
	store, mock := newTracedStoreTest(t)
	failure := errors.New("SQL failure")
	mock.ExpectBegin()
	expectCreatedTransaction(mock)
	mock.ExpectCommit().WillReturnError(failure)

	ctx, recorder := spantest.RecordingContext()

	_, err := store.PersistWithOutcome(ctx, frozenStoreRecord())
	require.ErrorIs(t, err, failure)

	parent := spantest.FindSpan(t, recorder, "postgres.persist_transaction")
	assert.Equal(t, codes.Error, parent.Status().Code)
	assert.Contains(t, parent.Status().Description, "commit recovery persistence")
}

// A bulk insert failure is recorded once, on the repository span that owns it.
func TestCompletionSpans_RepositoryFailureIsNotRecordedTwice(t *testing.T) {
	store, mock := newTracedStoreTest(t)
	failure := errors.New("SQL failure")
	mock.ExpectBegin()
	expectTransaction(mock, "", true, true)
	mock.ExpectExec("INSERT recovery_transaction").WillReturnResult(sqlmock.NewResult(0, 1))
	expectTransaction(mock, constant.APPROVED, true, true)
	mock.ExpectExec("INSERT recovery_operations").WillReturnError(failure)
	mock.ExpectRollback()

	ctx, recorder := spantest.RecordingContext()

	_, err := store.PersistWithOutcome(ctx, frozenStoreRecord())
	require.ErrorIs(t, err, failure)

	assert.Equal(t, codes.Error, spantest.FindSpan(t, recorder, "postgres.create_bulk_operations_tx").Status().Code)

	parent := spantest.FindSpan(t, recorder, "postgres.persist_transaction")
	assert.Equal(t, codes.Unset, parent.Status().Code, "the repository already recorded this failure")
	assert.Empty(t, parent.Events())
}

// A caller cancellation is an event on the domain span with the status unset; the error
// returned to the caller is unchanged.
func TestCompletionSpans_CallerCancellationIsNotAnError(t *testing.T) {
	store, _ := newTracedStoreTest(t)

	ctx, recorder := spantest.RecordingContext()
	ctx, cancel := context.WithCancel(ctx)
	cancel()

	_, err := store.PersistWithOutcome(ctx, frozenStoreRecord())
	require.ErrorIs(t, err, context.Canceled)

	parent := spantest.FindSpan(t, recorder, "postgres.persist_transaction")
	assert.Equal(t, codes.Unset, parent.Status().Code)

	_, abandoned := spantest.FindEvent(parent, "Persistence abandoned by caller")
	assert.True(t, abandoned)
}
