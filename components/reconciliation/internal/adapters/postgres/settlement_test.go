package postgres

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettlementDetector_GetUnsettledCount(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"count"}).AddRow(5)
	mock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(rows)

	detector := NewSettlementDetector(db)
	count, err := detector.GetUnsettledCount(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 5, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSettlementDetector_GetUnsettledCount_Zero(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(rows)

	detector := NewSettlementDetector(db)
	count, err := detector.GetUnsettledCount(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSettlementDetector_GetUnsettledCount_Error(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT`).
		WillReturnError(assert.AnError)

	detector := NewSettlementDetector(db)
	count, err := detector.GetUnsettledCount(context.Background())

	require.Error(t, err)
	assert.Equal(t, 0, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSettlementDetector_GetSettledCount(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"count"}).AddRow(100)
	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs(300). // settlementWaitSeconds
		WillReturnRows(rows)

	detector := NewSettlementDetector(db)
	count, err := detector.GetSettledCount(context.Background(), 300)

	require.NoError(t, err)
	assert.Equal(t, 100, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSettlementDetector_GetSettledCount_Error(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs(300).
		WillReturnError(assert.AnError)

	detector := NewSettlementDetector(db)
	count, err := detector.GetSettledCount(context.Background(), 300)

	require.Error(t, err)
	assert.Equal(t, 0, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}
