package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"github.com/shopspring/decimal"
)

func TestBalanceChecker_Check_NoDiscrepancies(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Setup summary query expectation
	summaryRows := sqlmock.NewRows([]string{"total_balances", "discrepancies", "total_discrepancy", "on_hold_discrepancies", "total_on_hold_discrepancy", "negative_available", "negative_on_hold"}).
		AddRow(100, 0, "0", 0, "0", 0, 0)
	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(0)).
		WillReturnRows(summaryRows)

	checker := NewBalanceChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{
		DiscrepancyThreshold: 0,
		MaxResults:           10,
	})

	require.NoError(t, err)
	typedResult := requireBalanceResult(t, result)
	assert.Equal(t, domain.StatusHealthy, typedResult.Status)
	assert.Equal(t, 100, typedResult.TotalBalances)
	assert.Equal(t, 0, typedResult.BalancesWithDiscrepancy)
	assert.Equal(t, 0, typedResult.OnHoldWithDiscrepancy)
	assert.True(t, decimal.NewFromInt(0).Equal(typedResult.TotalOnHoldDiscrepancy))
	assert.Equal(t, 0, typedResult.NegativeAvailable)
	assert.Equal(t, 0, typedResult.NegativeOnHold)
	assert.Empty(t, typedResult.Discrepancies)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceChecker_Check_WithDiscrepancies(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Setup summary query expectation - 5 discrepancies (WARNING status)
	summaryRows := sqlmock.NewRows([]string{"total_balances", "discrepancies", "total_discrepancy", "on_hold_discrepancies", "total_on_hold_discrepancy", "negative_available", "negative_on_hold"}).
		AddRow(100, 5, "500", 0, "0", 0, 0)
	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(0)).
		WillReturnRows(summaryRows)

	// Setup detail query expectation
	detailRows := sqlmock.NewRows([]string{
		"balance_id", "account_id", "alias", "asset_code",
		"current_balance", "expected_balance", "discrepancy",
		"operation_count", "updated_at",
	}).
		AddRow("bal-1", "acc-1", "account1", "USD", "1000", "900", "100", int64(10), time.Now())

	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(0), 10).
		WillReturnRows(detailRows)

	checker := NewBalanceChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{
		DiscrepancyThreshold: 0,
		MaxResults:           10,
	})

	require.NoError(t, err)
	typedResult := requireBalanceResult(t, result)
	assert.Equal(t, domain.StatusWarning, typedResult.Status)
	assert.Equal(t, 100, typedResult.TotalBalances)
	assert.Equal(t, 5, typedResult.BalancesWithDiscrepancy)
	assert.Equal(t, 0, typedResult.OnHoldWithDiscrepancy)
	assert.True(t, decimal.NewFromInt(0).Equal(typedResult.TotalOnHoldDiscrepancy))
	assert.Equal(t, 0, typedResult.NegativeAvailable)
	assert.Equal(t, 0, typedResult.NegativeOnHold)
	assert.Len(t, typedResult.Discrepancies, 1)
	assert.Equal(t, "bal-1", typedResult.Discrepancies[0].BalanceID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceChecker_Check_CriticalDiscrepancies(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Setup summary query expectation - 15 discrepancies (CRITICAL status)
	summaryRows := sqlmock.NewRows([]string{"total_balances", "discrepancies", "total_discrepancy", "on_hold_discrepancies", "total_on_hold_discrepancy", "negative_available", "negative_on_hold"}).
		AddRow(100, 15, "1500", 0, "0", 0, 0)
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
	result, err := checker.Check(context.Background(), CheckerConfig{
		DiscrepancyThreshold: 0,
		MaxResults:           10,
	})

	require.NoError(t, err)
	typedResult := requireBalanceResult(t, result)
	assert.Equal(t, domain.StatusCritical, typedResult.Status)
	assert.Equal(t, 15, typedResult.BalancesWithDiscrepancy)
	assert.Equal(t, 0, typedResult.OnHoldWithDiscrepancy)
	assert.True(t, decimal.NewFromInt(0).Equal(typedResult.TotalOnHoldDiscrepancy))
	assert.Equal(t, 0, typedResult.NegativeAvailable)
	assert.Equal(t, 0, typedResult.NegativeOnHold)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceChecker_Check_WithThreshold(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// With threshold of 100, minor discrepancies are ignored
	summaryRows := sqlmock.NewRows([]string{"total_balances", "discrepancies", "total_discrepancy", "on_hold_discrepancies", "total_on_hold_discrepancy", "negative_available", "negative_on_hold"}).
		AddRow(50, 0, "0", 0, "0", 0, 0)
	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(100)).
		WillReturnRows(summaryRows)

	checker := NewBalanceChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{
		DiscrepancyThreshold: 100,
		MaxResults:           10,
	})

	require.NoError(t, err)
	typedResult := requireBalanceResult(t, result)
	assert.Equal(t, domain.StatusHealthy, typedResult.Status)
	assert.Equal(t, 0, typedResult.OnHoldWithDiscrepancy)
	assert.True(t, decimal.NewFromInt(0).Equal(typedResult.TotalOnHoldDiscrepancy))
	assert.Equal(t, 0, typedResult.NegativeAvailable)
	assert.Equal(t, 0, typedResult.NegativeOnHold)
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
	result, err := checker.Check(context.Background(), CheckerConfig{
		DiscrepancyThreshold: 0,
		MaxResults:           10,
	})

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
	summaryRows := sqlmock.NewRows([]string{"total_balances", "discrepancies", "total_discrepancy", "on_hold_discrepancies", "total_on_hold_discrepancy", "negative_available", "negative_on_hold"}).
		AddRow(100, 10, "1000", 0, "0", 0, 0)
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
	result, err := checker.Check(context.Background(), CheckerConfig{
		DiscrepancyThreshold: 0,
		MaxResults:           10,
	})

	require.NoError(t, err)
	typedResult := requireBalanceResult(t, result)
	assert.Equal(t, 10.0, typedResult.DiscrepancyPercentage)
	assert.True(t, decimal.NewFromInt(1000).Equal(typedResult.TotalAbsoluteDiscrepancy))
	assert.Equal(t, 0, typedResult.OnHoldWithDiscrepancy)
	assert.True(t, decimal.NewFromInt(0).Equal(typedResult.TotalOnHoldDiscrepancy))
	assert.Equal(t, 0, typedResult.NegativeAvailable)
	assert.Equal(t, 0, typedResult.NegativeOnHold)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceChecker_Check_ParsesOnHoldAndNegativeColumns(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Non-zero values for the newly-added summary columns.
	// Note: any negative balance forces CRITICAL status and triggers a follow-up query for details.
	summaryRows := sqlmock.NewRows([]string{
		"total_balances", "discrepancies", "total_discrepancy",
		"on_hold_discrepancies", "total_on_hold_discrepancy",
		"negative_available", "negative_on_hold",
	}).AddRow(10, 0, "0", 2, "25.50", 1, 2)
	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(0)).
		WillReturnRows(summaryRows)

	// On-hold detail query (can be empty; we're asserting the summary parsing here).
	onHoldDetailRows := sqlmock.NewRows([]string{
		"balance_id", "account_id", "alias", "asset_code",
		"current_on_hold", "expected_on_hold", "discrepancy",
		"operation_count", "updated_at",
	})
	mock.ExpectQuery(`WITH balance_calc AS`).
		WithArgs(int64(0), 10).
		WillReturnRows(onHoldDetailRows)

	// Negative balances detail query.
	negativeRows := sqlmock.NewRows([]string{
		"id", "account_id", "alias", "asset_code", "available", "on_hold",
	}).
		AddRow("bal-neg-1", "acc-1", "a1", "USD", "-1", "0").
		AddRow("bal-neg-2", "acc-2", "a2", "USD", "0", "-2")
	mock.ExpectQuery(`SELECT\s+id::text,`).
		WithArgs(10).
		WillReturnRows(negativeRows)

	checker := NewBalanceChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{
		DiscrepancyThreshold: 0,
		MaxResults:           10,
	})

	require.NoError(t, err)
	typedResult := requireBalanceResult(t, result)
	assert.Equal(t, 10, typedResult.TotalBalances)
	assert.Equal(t, 0, typedResult.BalancesWithDiscrepancy)
	assert.Equal(t, 2, typedResult.OnHoldWithDiscrepancy)
	assert.True(t, decimal.RequireFromString("25.50").Equal(typedResult.TotalOnHoldDiscrepancy))
	assert.Equal(t, 1, typedResult.NegativeAvailable)
	assert.Equal(t, 2, typedResult.NegativeOnHold)
	assert.Len(t, typedResult.NegativeBalances, 2)
	assert.Equal(t, domain.StatusCritical, typedResult.Status)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func requireBalanceResult(t *testing.T, result CheckResult) *domain.BalanceCheckResult {
	t.Helper()

	typedResult, ok := result.(*domain.BalanceCheckResult)
	require.True(t, ok)

	return typedResult
}
