// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package partitionstate

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func newReaderWithSQLMock(t *testing.T, ttl time.Duration) (*Reader, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	r := NewReader(db, ttl, nil)

	return r, mock, func() { _ = db.Close() }
}

func TestReader_PhaseFreshFetch(t *testing.T) {
	r, mock, cleanup := newReaderWithSQLMock(t, time.Minute)
	defer cleanup()

	mock.ExpectQuery("SELECT phase FROM partition_migration_state").
		WillReturnRows(sqlmock.NewRows([]string{"phase"}).AddRow(string(PhaseDualWrite)))

	p, err := r.Phase(context.Background())
	require.NoError(t, err)
	require.Equal(t, PhaseDualWrite, p)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReader_PhaseCachedWithinTTL(t *testing.T) {
	r, mock, cleanup := newReaderWithSQLMock(t, time.Minute)
	defer cleanup()

	mock.ExpectQuery("SELECT phase FROM partition_migration_state").
		WillReturnRows(sqlmock.NewRows([]string{"phase"}).AddRow(string(PhaseDualWrite)))

	_, err := r.Phase(context.Background())
	require.NoError(t, err)

	// Second call must not hit the DB (no additional expectation set).
	p, err := r.Phase(context.Background())
	require.NoError(t, err)
	require.Equal(t, PhaseDualWrite, p)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReader_PhaseRefreshesAfterTTL(t *testing.T) {
	r, mock, cleanup := newReaderWithSQLMock(t, time.Millisecond)
	defer cleanup()

	mock.ExpectQuery("SELECT phase").
		WillReturnRows(sqlmock.NewRows([]string{"phase"}).AddRow(string(PhaseLegacyOnly)))
	mock.ExpectQuery("SELECT phase").
		WillReturnRows(sqlmock.NewRows([]string{"phase"}).AddRow(string(PhasePartitioned)))

	p, err := r.Phase(context.Background())
	require.NoError(t, err)
	require.Equal(t, PhaseLegacyOnly, p)

	time.Sleep(5 * time.Millisecond)

	p, err = r.Phase(context.Background())
	require.NoError(t, err)
	require.Equal(t, PhasePartitioned, p)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReader_UnknownPhaseIsRejected(t *testing.T) {
	r, mock, cleanup := newReaderWithSQLMock(t, time.Minute)
	defer cleanup()

	mock.ExpectQuery("SELECT phase").
		WillReturnRows(sqlmock.NewRows([]string{"phase"}).AddRow("bogus"))

	p, err := r.Phase(context.Background())
	require.ErrorIs(t, err, ErrUnknownPhase)
	require.Equal(t, PhaseLegacyOnly, p) // safe default on unknown value
}

func TestReader_DBFailureWithCacheReturnsStale(t *testing.T) {
	r, mock, cleanup := newReaderWithSQLMock(t, time.Millisecond)
	defer cleanup()

	mock.ExpectQuery("SELECT phase").
		WillReturnRows(sqlmock.NewRows([]string{"phase"}).AddRow(string(PhaseDualWrite)))
	mock.ExpectQuery("SELECT phase").
		WillReturnError(errors.New("db down")) //nolint:err113 // test-only error fixture for go-sqlmock failure injection.

	_, err := r.Phase(context.Background())
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)

	p, err := r.Phase(context.Background())
	require.Error(t, err) // wrapped error is returned
	require.Equal(t, PhaseDualWrite, p)
}

func TestReader_DBFailureNoCacheFallsBackLegacy(t *testing.T) {
	r, mock, cleanup := newReaderWithSQLMock(t, time.Minute)
	defer cleanup()

	mock.ExpectQuery("SELECT phase").
		WillReturnError(sql.ErrConnDone)

	p, err := r.Phase(context.Background())
	require.Error(t, err)
	require.Equal(t, PhaseLegacyOnly, p)
}

func TestReader_InvalidateForcesRefresh(t *testing.T) {
	r, mock, cleanup := newReaderWithSQLMock(t, time.Hour)
	defer cleanup()

	mock.ExpectQuery("SELECT phase").
		WillReturnRows(sqlmock.NewRows([]string{"phase"}).AddRow(string(PhaseLegacyOnly)))
	mock.ExpectQuery("SELECT phase").
		WillReturnRows(sqlmock.NewRows([]string{"phase"}).AddRow(string(PhaseDualWrite)))

	_, err := r.Phase(context.Background())
	require.NoError(t, err)

	r.Invalidate()

	p, err := r.Phase(context.Background())
	require.NoError(t, err)
	require.Equal(t, PhaseDualWrite, p)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStaticReader(t *testing.T) {
	s := StaticReader{P: PhasePartitioned}
	p, err := s.Phase(context.Background())
	require.NoError(t, err)
	require.Equal(t, PhasePartitioned, p)
	s.Invalidate() // no-op, must not panic
}
