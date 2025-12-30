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

func TestOrphanChecker_Check_NoOrphans(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	summaryRows := sqlmock.NewRows([]string{"orphan_transactions", "partial_transactions"}).
		AddRow(0, 0)
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(summaryRows)

	checker := NewOrphanChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireOrphanResult(t, result)
	assert.Equal(t, domain.StatusHealthy, typedResult.Status)
	assert.Equal(t, 0, typedResult.OrphanTransactions)
	assert.Equal(t, 0, typedResult.PartialTransactions)
	assert.Empty(t, typedResult.Orphans)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrphanChecker_Check_WithOrphans(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Orphan transactions are CRITICAL
	summaryRows := sqlmock.NewRows([]string{"orphan_transactions", "partial_transactions"}).
		AddRow(2, 1)
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(summaryRows)

	createdAt := time.Now()
	detailRows := sqlmock.NewRows([]string{
		"transaction_id", "organization_id", "ledger_id",
		"status", "amount", "asset_code", "created_at", "operation_count",
	}).
		AddRow("txn-1", "org-1", "ldg-1", "APPROVED", int64(1000), "USD", createdAt, int32(0)).
		AddRow("txn-2", "org-1", "ldg-1", "APPROVED", int64(500), "USD", createdAt, int32(0)).
		AddRow("txn-3", "org-1", "ldg-1", "APPROVED", int64(200), "USD", createdAt, int32(1))

	mock.ExpectQuery(`SELECT`).
		WithArgs(10).
		WillReturnRows(detailRows)

	checker := NewOrphanChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireOrphanResult(t, result)
	assert.Equal(t, domain.StatusCritical, typedResult.Status)
	assert.Equal(t, 2, typedResult.OrphanTransactions)
	assert.Equal(t, 1, typedResult.PartialTransactions)
	assert.Len(t, typedResult.Orphans, 3)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrphanChecker_Check_OnlyPartial(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Partial only (1 operation) is WARNING
	summaryRows := sqlmock.NewRows([]string{"orphan_transactions", "partial_transactions"}).
		AddRow(0, 3)
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(summaryRows)

	createdAt := time.Now()
	detailRows := sqlmock.NewRows([]string{
		"transaction_id", "organization_id", "ledger_id",
		"status", "amount", "asset_code", "created_at", "operation_count",
	}).
		AddRow("txn-1", "org-1", "ldg-1", "APPROVED", int64(1000), "USD", createdAt, int32(1))

	mock.ExpectQuery(`SELECT`).
		WithArgs(10).
		WillReturnRows(detailRows)

	checker := NewOrphanChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireOrphanResult(t, result)
	assert.Equal(t, domain.StatusWarning, typedResult.Status)
	assert.Equal(t, 0, typedResult.OrphanTransactions)
	assert.Equal(t, 3, typedResult.PartialTransactions)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrphanChecker_Check_QueryError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT`).
		WillReturnError(assert.AnError)

	checker := NewOrphanChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: 10})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "orphan summary query failed")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func requireOrphanResult(t *testing.T, result CheckResult) *domain.OrphanCheckResult {
	t.Helper()

	typedResult, ok := result.(*domain.OrphanCheckResult)
	require.True(t, ok)

	return typedResult
}
