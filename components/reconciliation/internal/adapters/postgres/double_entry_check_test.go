package postgres

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

func TestDoubleEntryChecker_Check_AllBalanced(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	summaryRows := sqlmock.NewRows([]string{"total_transactions", "unbalanced", "no_operations"}).
		AddRow(500, 0, 0)
	mock.ExpectQuery(`WITH transaction_balance AS`).
		WillReturnRows(summaryRows)

	checker := NewDoubleEntryChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireDoubleEntryResult(t, result)
	assert.Equal(t, domain.StatusHealthy, typedResult.Status)
	assert.Equal(t, 500, typedResult.TotalTransactions)
	assert.Equal(t, 0, typedResult.UnbalancedTransactions)
	assert.Equal(t, 0, typedResult.TransactionsNoOperations)
	assert.Empty(t, typedResult.Imbalances)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDoubleEntryChecker_Check_WithUnbalanced(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Any unbalanced transaction is CRITICAL
	summaryRows := sqlmock.NewRows([]string{"total_transactions", "unbalanced", "no_operations"}).
		AddRow(100, 2, 0)
	mock.ExpectQuery(`WITH transaction_balance AS`).
		WillReturnRows(summaryRows)

	detailRows := sqlmock.NewRows([]string{
		"transaction_id", "status", "asset_code",
		"total_credits", "total_debits", "imbalance", "operation_count",
	}).
		AddRow("txn-1", "APPROVED", "USD", int64(1000), int64(900), int64(100), int64(2)).
		AddRow("txn-2", "APPROVED", "EUR", int64(500), int64(600), int64(-100), int64(2))

	mock.ExpectQuery(`WITH transaction_balance AS`).
		WithArgs(10).
		WillReturnRows(detailRows)

	checker := NewDoubleEntryChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireDoubleEntryResult(t, result)
	assert.Equal(t, domain.StatusCritical, typedResult.Status)
	assert.Equal(t, 2, typedResult.UnbalancedTransactions)
	assert.Len(t, typedResult.Imbalances, 2)
	assert.Equal(t, "txn-1", typedResult.Imbalances[0].TransactionID)
	assert.Equal(t, int64(100), typedResult.Imbalances[0].Imbalance)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDoubleEntryChecker_Check_QueryError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`WITH transaction_balance AS`).
		WillReturnError(assert.AnError)

	checker := NewDoubleEntryChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: 10})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "double-entry summary query failed")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDoubleEntryChecker_Check_UnbalancedPercentage(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// 5 out of 100 = 5%
	summaryRows := sqlmock.NewRows([]string{"total_transactions", "unbalanced", "no_operations"}).
		AddRow(100, 5, 0)
	mock.ExpectQuery(`WITH transaction_balance AS`).
		WillReturnRows(summaryRows)

	detailRows := sqlmock.NewRows([]string{
		"transaction_id", "status", "asset_code",
		"total_credits", "total_debits", "imbalance", "operation_count",
	})
	mock.ExpectQuery(`WITH transaction_balance AS`).
		WithArgs(10).
		WillReturnRows(detailRows)

	checker := NewDoubleEntryChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireDoubleEntryResult(t, result)
	assert.Equal(t, 5.0, typedResult.UnbalancedPercentage)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDoubleEntryChecker_Check_ZeroTransactions(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	summaryRows := sqlmock.NewRows([]string{"total_transactions", "unbalanced", "no_operations"}).
		AddRow(0, 0, 0)
	mock.ExpectQuery(`WITH transaction_balance AS`).
		WillReturnRows(summaryRows)

	checker := NewDoubleEntryChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireDoubleEntryResult(t, result)
	assert.Equal(t, domain.StatusHealthy, typedResult.Status)
	assert.Equal(t, 0, typedResult.TotalTransactions)
	assert.Equal(t, 0.0, typedResult.UnbalancedPercentage) // Avoid division by zero
	assert.NoError(t, mock.ExpectationsWereMet())
}

func requireDoubleEntryResult(t *testing.T, result CheckResult) *domain.DoubleEntryCheckResult {
	t.Helper()

	typedResult, ok := result.(*domain.DoubleEntryCheckResult)
	require.Truef(
		t,
		ok,
		"expected result to be *domain.DoubleEntryCheckResult, got %T: %#v",
		result,
		result,
	)

	return typedResult
}
