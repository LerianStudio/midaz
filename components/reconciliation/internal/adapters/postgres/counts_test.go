package postgres

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntityCounter_GetOnboardingCounts(t *testing.T) {
	t.Parallel()

	onboardingDB, onboardingMock, err := sqlmock.New()
	require.NoError(t, err)
	defer onboardingDB.Close()

	transactionDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer transactionDB.Close()

	rows := sqlmock.NewRows([]string{"organizations", "ledgers", "assets", "accounts", "portfolios"}).
		AddRow(int64(10), int64(50), int64(100), int64(500), int64(25))
	onboardingMock.ExpectQuery(`SELECT`).
		WillReturnRows(rows)

	counter := NewEntityCounter(onboardingDB, transactionDB)
	counts, err := counter.GetOnboardingCounts(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(10), counts.Organizations)
	assert.Equal(t, int64(50), counts.Ledgers)
	assert.Equal(t, int64(100), counts.Assets)
	assert.Equal(t, int64(500), counts.Accounts)
	assert.Equal(t, int64(25), counts.Portfolios)
	assert.NoError(t, onboardingMock.ExpectationsWereMet())
}

func TestEntityCounter_GetOnboardingCounts_Error(t *testing.T) {
	t.Parallel()

	onboardingDB, onboardingMock, err := sqlmock.New()
	require.NoError(t, err)
	defer onboardingDB.Close()

	transactionDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer transactionDB.Close()

	onboardingMock.ExpectQuery(`SELECT`).
		WillReturnError(assert.AnError)

	counter := NewEntityCounter(onboardingDB, transactionDB)
	counts, err := counter.GetOnboardingCounts(context.Background())

	require.Error(t, err)
	assert.NotNil(t, counts) // Still returns struct, just empty
	assert.NoError(t, onboardingMock.ExpectationsWereMet())
}

func TestEntityCounter_GetTransactionCounts(t *testing.T) {
	t.Parallel()

	onboardingDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer onboardingDB.Close()

	transactionDB, transactionMock, err := sqlmock.New()
	require.NoError(t, err)
	defer transactionDB.Close()

	rows := sqlmock.NewRows([]string{"transactions", "operations", "balances"}).
		AddRow(int64(1000), int64(5000), int64(2000))
	transactionMock.ExpectQuery(`SELECT`).
		WillReturnRows(rows)

	counter := NewEntityCounter(onboardingDB, transactionDB)
	counts, err := counter.GetTransactionCounts(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(1000), counts.Transactions)
	assert.Equal(t, int64(5000), counts.Operations)
	assert.Equal(t, int64(2000), counts.Balances)
	assert.NoError(t, transactionMock.ExpectationsWereMet())
}

func TestEntityCounter_GetTransactionCounts_Error(t *testing.T) {
	t.Parallel()

	onboardingDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer onboardingDB.Close()

	transactionDB, transactionMock, err := sqlmock.New()
	require.NoError(t, err)
	defer transactionDB.Close()

	transactionMock.ExpectQuery(`SELECT`).
		WillReturnError(assert.AnError)

	counter := NewEntityCounter(onboardingDB, transactionDB)
	counts, err := counter.GetTransactionCounts(context.Background())

	require.Error(t, err)
	assert.NotNil(t, counts)
	assert.NoError(t, transactionMock.ExpectationsWereMet())
}
