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

func TestDLQChecker_Check_NoEntries(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	summaryRows := sqlmock.NewRows([]string{"total", "transaction_count", "operation_count"}).
		AddRow(0, 0, 0)
	mock.ExpectQuery(`FROM metadata_outbox`).
		WillReturnRows(summaryRows)

	checker := NewDLQChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireDLQResult(t, result)
	assert.Equal(t, int64(0), typedResult.Total)
	assert.Equal(t, domain.StatusHealthy, typedResult.Status)
	assert.Empty(t, typedResult.Entries)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDLQChecker_Check_WithEntries(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	summaryRows := sqlmock.NewRows([]string{"total", "transaction_count", "operation_count"}).
		AddRow(3, 2, 1)
	mock.ExpectQuery(`FROM metadata_outbox`).
		WillReturnRows(summaryRows)

	detailRows := sqlmock.NewRows([]string{
		"id", "entity_id", "entity_type", "retry_count", "last_error", "created_at",
	}).AddRow("id-1", "entity-1", "Transaction", 10, "boom", time.Now())

	mock.ExpectQuery(`SELECT`).
		WithArgs(10).
		WillReturnRows(detailRows)

	checker := NewDLQChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: 10})

	require.NoError(t, err)
	typedResult := requireDLQResult(t, result)
	assert.Equal(t, int64(3), typedResult.Total)
	assert.Equal(t, int64(2), typedResult.TransactionEntries)
	assert.Equal(t, int64(1), typedResult.OperationEntries)
	assert.Equal(t, domain.StatusWarning, typedResult.Status)
	require.Len(t, typedResult.Entries, 1)
	assert.Equal(t, "id-1", typedResult.Entries[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDLQChecker_Check_Critical(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	summaryRows := sqlmock.NewRows([]string{"total", "transaction_count", "operation_count"}).
		AddRow(25, 15, 10)
	mock.ExpectQuery(`FROM metadata_outbox`).
		WillReturnRows(summaryRows)

	checker := NewDLQChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: 0})

	require.NoError(t, err)
	typedResult := requireDLQResult(t, result)
	assert.Equal(t, domain.StatusCritical, typedResult.Status)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDLQChecker_Check_NegativeLimit(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// If limit is negative, it should be clamped to 0 and must not execute the detail query.
	summaryRows := sqlmock.NewRows([]string{"total", "transaction_count", "operation_count"}).
		AddRow(3, 2, 1)
	mock.ExpectQuery(`FROM metadata_outbox`).
		WillReturnRows(summaryRows)

	checker := NewDLQChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: -1})

	require.NoError(t, err)
	typedResult := requireDLQResult(t, result)
	assert.Equal(t, int64(3), typedResult.Total)
	assert.Empty(t, typedResult.Entries)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDLQChecker_Check_QueryError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`FROM metadata_outbox`).
		WillReturnError(assert.AnError)

	checker := NewDLQChecker(db)
	result, err := checker.Check(context.Background(), CheckerConfig{MaxResults: 10})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDetermineDLQStatus_ThresholdIsExclusive(t *testing.T) {
	t.Parallel()

	assert.Equal(t, domain.StatusHealthy, DetermineStatus(0, StatusThresholds{
		WarningThreshold:          dlqWarningThreshold,
		WarningThresholdExclusive: true,
	}))
	assert.Equal(t, domain.StatusWarning, DetermineStatus(dlqWarningThreshold-1, StatusThresholds{
		WarningThreshold:          dlqWarningThreshold,
		WarningThresholdExclusive: true,
	}))
	assert.Equal(t, domain.StatusCritical, DetermineStatus(dlqWarningThreshold, StatusThresholds{
		WarningThreshold:          dlqWarningThreshold,
		WarningThresholdExclusive: true,
	}))
	assert.Equal(t, domain.StatusCritical, DetermineStatus(dlqWarningThreshold+1, StatusThresholds{
		WarningThreshold:          dlqWarningThreshold,
		WarningThresholdExclusive: true,
	}))
}

func requireDLQResult(t *testing.T, result CheckResult) *domain.DLQCheckResult {
	t.Helper()

	typedResult, ok := result.(*domain.DLQCheckResult)
	require.True(t, ok)

	return typedResult
}
