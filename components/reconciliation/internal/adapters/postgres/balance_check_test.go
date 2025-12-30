package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

func TestBalanceChecker_Check_NoDiscrepancies(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Setup summary query expectation
	summaryRows := sqlmock.NewRows([]string{"total_balances", "discrepancies", "total_discrepancy"}).
		AddRow(100, 0, 0)
	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(0)).
		WillReturnRows(summaryRows)

	checker := NewBalanceChecker(db)
	result, err := checker.Check(context.Background(), 0, 10)

	require.NoError(t, err)
	assert.Equal(t, domain.StatusHealthy, result.Status)
	assert.Equal(t, 100, result.TotalBalances)
	assert.Equal(t, 0, result.BalancesWithDiscrepancy)
	assert.Empty(t, result.Discrepancies)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceChecker_Check_WithDiscrepancies(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Setup summary query expectation - 5 discrepancies (WARNING status)
	summaryRows := sqlmock.NewRows([]string{"total_balances", "discrepancies", "total_discrepancy"}).
		AddRow(100, 5, 500)
	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(0)).
		WillReturnRows(summaryRows)

	// Setup detail query expectation
	detailRows := sqlmock.NewRows([]string{
		"balance_id", "account_id", "alias", "asset_code",
		"current_balance", "expected_balance", "discrepancy",
		"operation_count", "updated_at",
	}).
		AddRow("bal-1", "acc-1", "account1", "USD", int64(1000), int64(900), int64(100), int64(10), time.Now())

	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(0), 10).
		WillReturnRows(detailRows)

	checker := NewBalanceChecker(db)
	result, err := checker.Check(context.Background(), 0, 10)

	require.NoError(t, err)
	assert.Equal(t, domain.StatusWarning, result.Status)
	assert.Equal(t, 100, result.TotalBalances)
	assert.Equal(t, 5, result.BalancesWithDiscrepancy)
	assert.Len(t, result.Discrepancies, 1)
	assert.Equal(t, "bal-1", result.Discrepancies[0].BalanceID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceChecker_Check_CriticalDiscrepancies(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Setup summary query expectation - 15 discrepancies (CRITICAL status)
	summaryRows := sqlmock.NewRows([]string{"total_balances", "discrepancies", "total_discrepancy"}).
		AddRow(100, 15, 1500)
	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(0)).
		WillReturnRows(summaryRows)

	// Setup detail query expectation (empty for simplicity)
	detailRows := sqlmock.NewRows([]string{
		"balance_id", "account_id", "alias", "asset_code",
		"current_balance", "expected_balance", "discrepancy",
		"operation_count", "updated_at",
	})
	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(0), 10).
		WillReturnRows(detailRows)

	checker := NewBalanceChecker(db)
	result, err := checker.Check(context.Background(), 0, 10)

	require.NoError(t, err)
	assert.Equal(t, domain.StatusCritical, result.Status)
	assert.Equal(t, 15, result.BalancesWithDiscrepancy)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceChecker_Check_WithThreshold(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// With threshold of 100, minor discrepancies are ignored
	summaryRows := sqlmock.NewRows([]string{"total_balances", "discrepancies", "total_discrepancy"}).
		AddRow(50, 0, 0)
	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(100)).
		WillReturnRows(summaryRows)

	checker := NewBalanceChecker(db)
	result, err := checker.Check(context.Background(), 100, 10)

	require.NoError(t, err)
	assert.Equal(t, domain.StatusHealthy, result.Status)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceChecker_Check_QueryError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(0)).
		WillReturnError(assert.AnError)

	checker := NewBalanceChecker(db)
	result, err := checker.Check(context.Background(), 0, 10)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "balance summary query failed")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceChecker_Check_DiscrepancyPercentage(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// 10 out of 100 = 10%
	summaryRows := sqlmock.NewRows([]string{"total_balances", "discrepancies", "total_discrepancy"}).
		AddRow(100, 10, 1000)
	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(0)).
		WillReturnRows(summaryRows)

	detailRows := sqlmock.NewRows([]string{
		"balance_id", "account_id", "alias", "asset_code",
		"current_balance", "expected_balance", "discrepancy",
		"operation_count", "updated_at",
	})
	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(0), 10).
		WillReturnRows(detailRows)

	checker := NewBalanceChecker(db)
	result, err := checker.Check(context.Background(), 0, 10)

	require.NoError(t, err)
	assert.Equal(t, 10.0, result.DiscrepancyPercentage)
	assert.NoError(t, mock.ExpectationsWereMet())
}
