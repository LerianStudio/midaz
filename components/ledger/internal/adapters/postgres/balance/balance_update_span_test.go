// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// updateBalancesFixture returns a repository and a context whose PostgreSQL handle is a
// sqlmock, plus the two balances a batch updates.
func updateBalancesFixture(t *testing.T) (*BalancePostgreSQLRepository, dbresolver.DB, sqlmock.Sqlmock, []*mmodel.Balance) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	})

	balances := []*mmodel.Balance{
		{ID: uuid.NewString(), Available: decimal.NewFromInt(10), OnHold: decimal.Zero, Version: 2},
		{ID: uuid.NewString(), Available: decimal.NewFromInt(20), OnHold: decimal.Zero, Version: 3},
	}

	resolver := dbresolver.New(dbresolver.WithPrimaryDBs(db), dbresolver.WithReplicaDBs(db))

	return &BalancePostgreSQLRepository{}, resolver, mock, balances
}

// The bulk update is one postgres.update_balances span carrying the batch size; each
// balance's UPDATE is traced by the driver, not by a span of its own.
func TestBalancesUpdateSpans_OneSpanWithTheBatchCount(t *testing.T) {
	repo, pg, mock, balances := updateBalancesFixture(t)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE balance SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE balance SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ctx, recorder := spantest.RecordingContext()
	ctx = tmcore.ContextWithPG(ctx, pg)

	require.NoError(t, repo.BalancesUpdate(ctx, uuid.New(), uuid.New(), balances))

	spantest.AssertNoSpan(t, recorder, "postgres.update_balances.balance", "postgres.update_balance")
	assert.Equal(t, []string{"postgres.update_balances"}, spantest.SpanNames(recorder))

	span := spantest.FindSpan(t, recorder, "postgres.update_balances")
	assert.Contains(t, span.Attributes(), attribute.Int("app.request.balances_count", len(balances)))
	assert.Equal(t, codes.Unset, span.Status().Code)
}

// A failed UPDATE is a technical error on the parent span; the error is returned as before.
func TestBalancesUpdateSpans_FailedUpdateMarksTheParent(t *testing.T) {
	repo, pg, mock, balances := updateBalancesFixture(t)
	failure := errors.New("SQL failure")

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE balance SET").WillReturnError(failure)
	// The pre-existing error handling commits on this path (the loop's err shadows the
	// one the deferred commit/rollback inspects); the test pins today's behavior.
	mock.ExpectCommit()

	ctx, recorder := spantest.RecordingContext()
	ctx = tmcore.ContextWithPG(ctx, pg)

	require.ErrorIs(t, repo.BalancesUpdate(ctx, uuid.New(), uuid.New(), balances), failure)

	assert.Equal(t, []string{"postgres.update_balances"}, spantest.SpanNames(recorder))

	span := spantest.FindSpan(t, recorder, "postgres.update_balances")
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Contains(t, span.Status().Description, "Err on result exec content")
}
