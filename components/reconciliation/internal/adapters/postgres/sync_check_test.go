package postgres

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncChecker_Check_NoIssues(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Empty result set - no version mismatches or stale balances
	rows := sqlmock.NewRows([]string{
		"balance_id", "alias", "asset_code", "db_version",
		"max_op_version", "staleness_seconds",
	})
	mock.ExpectQuery(`WITH balance_ops AS`).
		WithArgs(300, 10).
		WillReturnRows(rows)

	checker := NewSyncChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{
		StaleThresholdSeconds: 300,
		MaxResults:            10,
	})

	require.NoError(t, err)
	typedResult := requireSyncResult(t, result)
	assert.Equal(t, domain.StatusHealthy, typedResult.Status)
	assert.Equal(t, 0, typedResult.VersionMismatches)
	assert.Equal(t, 0, typedResult.StaleBalances)
	assert.Empty(t, typedResult.Issues)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncChecker_Check_VersionMismatch(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"balance_id", "alias", "asset_code", "db_version",
		"max_op_version", "staleness_seconds",
	}).AddRow("bal-1", "account1", "USD", int32(5), int32(10), int64(100))

	mock.ExpectQuery(`WITH balance_ops AS`).
		WithArgs(300, 10).
		WillReturnRows(rows)

	checker := NewSyncChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{
		StaleThresholdSeconds: 300,
		MaxResults:            10,
	})

	require.NoError(t, err)
	typedResult := requireSyncResult(t, result)
	assert.Equal(t, domain.StatusWarning, typedResult.Status)
	assert.Equal(t, 1, typedResult.VersionMismatches)
	assert.Equal(t, 0, typedResult.StaleBalances)
	assert.Len(t, typedResult.Issues, 1)
	assert.Equal(t, "bal-1", typedResult.Issues[0].BalanceID)
	assert.Equal(t, int32(5), typedResult.Issues[0].DBVersion)
	assert.Equal(t, int32(10), typedResult.Issues[0].MaxOpVersion)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncChecker_Check_StaleBalance(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Balance version matches but staleness exceeds threshold
	rows := sqlmock.NewRows([]string{
		"balance_id", "alias", "asset_code", "db_version",
		"max_op_version", "staleness_seconds",
	}).AddRow("bal-2", "account2", "EUR", int32(10), int32(10), int64(500))

	mock.ExpectQuery(`WITH balance_ops AS`).
		WithArgs(300, 10).
		WillReturnRows(rows)

	checker := NewSyncChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{
		StaleThresholdSeconds: 300,
		MaxResults:            10,
	})

	require.NoError(t, err)
	typedResult := requireSyncResult(t, result)
	assert.Equal(t, domain.StatusWarning, typedResult.Status)
	assert.Equal(t, 0, typedResult.VersionMismatches)
	assert.Equal(t, 1, typedResult.StaleBalances)
	assert.Len(t, typedResult.Issues, 1)
	assert.Equal(t, "bal-2", typedResult.Issues[0].BalanceID)
	assert.Equal(t, int64(500), typedResult.Issues[0].StalenessSeconds)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncChecker_Check_CriticalIssues(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// 10+ issues triggers CRITICAL status
	rows := sqlmock.NewRows([]string{
		"balance_id", "alias", "asset_code", "db_version",
		"max_op_version", "staleness_seconds",
	})
	for i := 0; i < 12; i++ {
		rows.AddRow("bal-"+string(rune('a'+i)), "account", "USD", int32(i), int32(i+1), int64(100))
	}

	mock.ExpectQuery(`WITH balance_ops AS`).
		WithArgs(300, 20).
		WillReturnRows(rows)

	checker := NewSyncChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{
		StaleThresholdSeconds: 300,
		MaxResults:            20,
	})

	require.NoError(t, err)
	typedResult := requireSyncResult(t, result)
	assert.Equal(t, domain.StatusCritical, typedResult.Status)
	assert.Equal(t, 12, typedResult.VersionMismatches)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncChecker_Check_QueryError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`WITH balance_ops AS`).
		WithArgs(300, 10).
		WillReturnError(assert.AnError)

	checker := NewSyncChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{
		StaleThresholdSeconds: 300,
		MaxResults:            10,
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "sync check query failed")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncChecker_Check_BothVersionMismatchAndStale(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Balance has both version mismatch AND staleness issue
	rows := sqlmock.NewRows([]string{
		"balance_id", "alias", "asset_code", "db_version",
		"max_op_version", "staleness_seconds",
	}).AddRow("bal-both", "account-both", "USD", int32(5), int32(10), int64(500))

	mock.ExpectQuery(`WITH balance_ops AS`).
		WithArgs(300, 10).
		WillReturnRows(rows)

	checker := NewSyncChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{
		StaleThresholdSeconds: 300,
		MaxResults:            10,
	})

	require.NoError(t, err)
	typedResult := requireSyncResult(t, result)
	assert.Equal(t, domain.StatusWarning, typedResult.Status)
	assert.Equal(t, 1, typedResult.VersionMismatches)
	assert.Equal(t, 1, typedResult.StaleBalances)
	assert.Len(t, typedResult.Issues, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func requireSyncResult(t *testing.T, result CheckResult) *domain.SyncCheckResult {
	t.Helper()

	typedResult, ok := result.(*domain.SyncCheckResult)
	require.True(t, ok)

	return typedResult
}
