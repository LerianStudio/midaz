// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	libBackoff "github.com/LerianStudio/lib-commons/v6/commons/backoff"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	servicesMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// newRetryService builds a ReservationService whose transient-retry backoff is a
// deterministic recording no-op (no wall-clock sleep). The resolver/repo/audit
// mocks are wired but unused: these tests exercise inTx directly with a caller
// closure, so the retry seam is isolated from the reservation business logic.
func newRetryService(t *testing.T) (*ReservationService, *pgdbMocks.MockTxBeginner, *pgdbMocks.MockTx, *int) {
	t.Helper()

	ctrl := gomock.NewController(t)

	conn := pgdbMocks.NewMockTxBeginner(ctrl)
	tx := pgdbMocks.NewMockTx(ctrl)
	resolver := servicesMocks.NewMockLimitResolver(ctrl)
	repo := servicesMocks.NewMockReservationRepository(ctrl)
	audit := servicesMocks.NewMockReservationAuditWriter(ctrl)

	svc, err := NewReservationService(conn, resolver, repo, audit, testutil.NewMockClock(testutil.FixedTime()))
	require.NoError(t, err)

	sleeps := 0
	svc.retrySleep = func(context.Context, time.Duration) error {
		sleeps++

		return nil
	}

	return svc, conn, tx, &sleeps
}

func TestReservationService_inTx_RetriesTransientThenSucceeds(t *testing.T) {
	t.Parallel()

	svc, conn, tx, sleeps := newRetryService(t)
	span := trace.SpanFromContext(context.Background())

	// A fresh BeginTx per attempt: the whole transaction re-runs on retry.
	conn.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil).Times(2)
	tx.EXPECT().Rollback().Return(nil).Times(1) // attempt 1 deadlocked -> rollback
	tx.EXPECT().Commit().Return(nil).Times(1)   // attempt 2 succeeded -> commit

	calls := 0
	fn := func(pgdb.DB) error {
		calls++
		if calls == 1 {
			return &pgconn.PgError{Code: "40P01"} // deadlock_detected
		}

		return nil
	}

	err := svc.inTx(context.Background(), span, fn)
	require.NoError(t, err)
	assert.Equal(t, 2, calls, "the closure must re-run on the fresh transaction")
	assert.Equal(t, 1, *sleeps, "exactly one backoff between the two attempts")
}

func TestReservationService_inTx_NonTransientDoesNotRetry(t *testing.T) {
	t.Parallel()

	cases := map[string]error{
		"pg unique violation": &pgconn.PgError{Code: "23505"},
		"plain error":         errors.New("boom"),
	}

	for name, injErr := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc, conn, tx, sleeps := newRetryService(t)
			span := trace.SpanFromContext(context.Background())

			conn.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil).Times(1)
			tx.EXPECT().Rollback().Return(nil).Times(1)

			calls := 0
			fn := func(pgdb.DB) error {
				calls++

				return injErr
			}

			err := svc.inTx(context.Background(), span, fn)
			require.ErrorIs(t, err, injErr)
			assert.Equal(t, 1, calls, "a non-transient error must propagate on the first occurrence")
			assert.Equal(t, 0, *sleeps, "no backoff for a non-transient error")
		})
	}
}

func TestReservationService_inTx_ExhaustsRetriesAndReturnsLastError(t *testing.T) {
	t.Parallel()

	svc, conn, tx, sleeps := newRetryService(t)
	span := trace.SpanFromContext(context.Background())

	conn.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil).Times(reserveMaxAttempts)
	tx.EXPECT().Rollback().Return(nil).Times(reserveMaxAttempts)

	deadlock := &pgconn.PgError{Code: "40P01"}

	calls := 0
	fn := func(pgdb.DB) error {
		calls++

		return deadlock
	}

	err := svc.inTx(context.Background(), span, fn)
	require.Error(t, err)

	var pgErr *pgconn.PgError

	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, "40P01", pgErr.SQLState())
	assert.Equal(t, reserveMaxAttempts, calls, "every bounded attempt runs the closure")
	assert.Equal(t, reserveMaxAttempts-1, *sleeps, "no backoff after the final attempt")
}

func TestReservationService_inTx_ContextCancellationStopsRetries(t *testing.T) {
	t.Parallel()

	svc, conn, tx, _ := newRetryService(t)
	span := trace.SpanFromContext(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	svc.retrySleep = func(c context.Context, _ time.Duration) error {
		cancel()

		return c.Err()
	}

	conn.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil).Times(1)
	tx.EXPECT().Rollback().Return(nil).Times(1)

	calls := 0
	fn := func(pgdb.DB) error {
		calls++

		return &pgconn.PgError{Code: "40001"} // serialization_failure
	}

	err := svc.inTx(ctx, span, fn)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls, "cancellation during backoff prevents a second attempt")
}

func TestReservationService_inTx_PreCanceledContextReturnsImmediately(t *testing.T) {
	t.Parallel()

	svc, conn, _, _ := newRetryService(t)
	span := trace.SpanFromContext(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// No BeginTx expected: a cancelled context short-circuits before the first attempt.
	_ = conn

	called := false
	err := svc.inTx(ctx, span, func(pgdb.DB) error {
		called = true

		return nil
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, called, "the closure must not run under a cancelled context")
}

func TestIsTransientDBError(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		err  error
		want bool
	}{
		"serialization_failure": {&pgconn.PgError{Code: "40001"}, true},
		"deadlock_detected":     {&pgconn.PgError{Code: "40P01"}, true},
		"lock_not_available":    {&pgconn.PgError{Code: "55P03"}, true},
		"unique_violation":      {&pgconn.PgError{Code: "23505"}, false},
		"plain_error":           {errors.New("boom"), false},
		"nil_error":             {nil, false},
		"wrapped_transient":     {fmt.Errorf("begin: %w", &pgconn.PgError{Code: "40P01"}), true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isTransientDBError(tc.err))
		})
	}
}

func TestPgSQLState(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "40P01", pgSQLState(&pgconn.PgError{Code: "40P01"}))
	assert.Equal(t, "", pgSQLState(errors.New("not a pg error")))
}

func TestBackoffDelay_ExponentialWithJitter_WithinWindow(t *testing.T) {
	t.Parallel()

	// inTx passes attempt-1 (0-based) to libBackoff.ExponentialWithJitter, so the
	// wait for retry attempt n stays in [0, reserveRetryBaseBackoff*2^(n-1)) — the
	// same full-jitter window the deleted hand-rolled helper produced.
	for attempt := 1; attempt <= reserveMaxAttempts; attempt++ {
		window := reserveRetryBaseBackoff << (attempt - 1)

		d := libBackoff.ExponentialWithJitter(reserveRetryBaseBackoff, attempt-1)
		assert.GreaterOrEqual(t, d, time.Duration(0))
		assert.Less(t, d, window, "delay must stay under the exponential window for the attempt")
	}
}
