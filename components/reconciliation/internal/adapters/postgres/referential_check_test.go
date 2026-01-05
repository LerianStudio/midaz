package postgres

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

func TestReferentialChecker_Check_NoOrphans(t *testing.T) {
	t.Parallel()

	onboardingDB, onboardingMock, err := sqlmock.New()
	require.NoError(t, err)
	defer onboardingDB.Close()

	transactionDB, transactionMock, err := sqlmock.New()
	require.NoError(t, err)
	defer transactionDB.Close()

	// Onboarding query returns no orphans
	onboardingRows := sqlmock.NewRows([]string{"entity_id", "entity_type", "reference_type", "reference_id"})
	onboardingMock.ExpectQuery(`WITH orphan_ledgers AS`).
		WillReturnRows(onboardingRows)

	// Transaction query returns no orphans
	transactionRows := sqlmock.NewRows([]string{"entity_id", "entity_type", "reference_type", "reference_id"})
	transactionMock.ExpectQuery(`SELECT`).
		WillReturnRows(transactionRows)
	transactionMock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	checker := NewReferentialChecker(onboardingDB, transactionDB, nil)
	result, err := checker.Check(context.Background(), CheckerConfig{})

	require.NoError(t, err)
	typedResult := requireReferentialResult(t, result)
	assert.Equal(t, domain.StatusHealthy, typedResult.Status)
	assert.Equal(t, 0, typedResult.OrphanLedgers)
	assert.Equal(t, 0, typedResult.OrphanAssets)
	assert.Equal(t, 0, typedResult.OrphanAccounts)
	assert.Equal(t, 0, typedResult.OrphanOperations)
	assert.Empty(t, typedResult.Orphans)
	assert.NoError(t, onboardingMock.ExpectationsWereMet())
	assert.NoError(t, transactionMock.ExpectationsWereMet())
}

func TestReferentialChecker_Check_WithOrphans(t *testing.T) {
	t.Parallel()

	onboardingDB, onboardingMock, err := sqlmock.New()
	require.NoError(t, err)
	defer onboardingDB.Close()

	transactionDB, transactionMock, err := sqlmock.New()
	require.NoError(t, err)
	defer transactionDB.Close()

	// Onboarding returns orphan ledger and asset
	onboardingRows := sqlmock.NewRows([]string{"entity_id", "entity_type", "reference_type", "reference_id"}).
		AddRow("ldg-1", "ledger", "organization", "org-deleted").
		AddRow("ast-1", "asset", "ledger", "ldg-deleted")
	onboardingMock.ExpectQuery(`WITH orphan_ledgers AS`).
		WillReturnRows(onboardingRows)

	// Transaction returns orphan operation
	transactionRows := sqlmock.NewRows([]string{"entity_id", "entity_type", "reference_type", "reference_id"}).
		AddRow("op-1", "operation", "transaction", "txn-deleted")
	transactionMock.ExpectQuery(`SELECT`).
		WillReturnRows(transactionRows)
	transactionMock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	checker := NewReferentialChecker(onboardingDB, transactionDB, nil)
	result, err := checker.Check(context.Background(), CheckerConfig{})

	require.NoError(t, err)
	typedResult := requireReferentialResult(t, result)
	assert.Equal(t, domain.StatusWarning, typedResult.Status) // 3 orphans < 10 threshold
	assert.Equal(t, 1, typedResult.OrphanLedgers)
	assert.Equal(t, 1, typedResult.OrphanAssets)
	assert.Equal(t, 1, typedResult.OrphanOperations)
	assert.Len(t, typedResult.Orphans, 3)
	assert.NoError(t, onboardingMock.ExpectationsWereMet())
	assert.NoError(t, transactionMock.ExpectationsWereMet())
}

func TestReferentialChecker_Check_CriticalOrphans(t *testing.T) {
	t.Parallel()

	onboardingDB, onboardingMock, err := sqlmock.New()
	require.NoError(t, err)
	defer onboardingDB.Close()

	transactionDB, transactionMock, err := sqlmock.New()
	require.NoError(t, err)
	defer transactionDB.Close()

	// Create 10+ orphans for CRITICAL status
	onboardingRows := sqlmock.NewRows([]string{"entity_id", "entity_type", "reference_type", "reference_id"})
	for i := 0; i < 8; i++ {
		onboardingRows.AddRow(fmt.Sprintf("acc-%c", 'a'+i), "account", "ledger", "ldg-deleted")
	}
	onboardingMock.ExpectQuery(`WITH orphan_ledgers AS`).
		WillReturnRows(onboardingRows)

	transactionRows := sqlmock.NewRows([]string{"entity_id", "entity_type", "reference_type", "reference_id"})
	for i := 0; i < 5; i++ {
		transactionRows.AddRow(fmt.Sprintf("op-%d", i), "operation", "transaction", "txn-deleted")
	}
	transactionMock.ExpectQuery(`SELECT`).
		WillReturnRows(transactionRows)
	transactionMock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	checker := NewReferentialChecker(onboardingDB, transactionDB, nil)
	result, err := checker.Check(context.Background(), CheckerConfig{})

	require.NoError(t, err)
	typedResult := requireReferentialResult(t, result)
	assert.Equal(t, domain.StatusCritical, typedResult.Status) // 13 orphans >= 10
	assert.Equal(t, 8, typedResult.OrphanAccounts)
	assert.Equal(t, 5, typedResult.OrphanOperations)
	assert.NoError(t, onboardingMock.ExpectationsWereMet())
	assert.NoError(t, transactionMock.ExpectationsWereMet())
}

func TestReferentialChecker_Check_UnknownEntityType_IsCountedAndLogged(t *testing.T) {
	t.Parallel()

	onboardingDB, onboardingMock, err := sqlmock.New()
	require.NoError(t, err)
	defer onboardingDB.Close()

	transactionDB, transactionMock, err := sqlmock.New()
	require.NoError(t, err)
	defer transactionDB.Close()

	onboardingRows := sqlmock.NewRows([]string{"entity_id", "entity_type", "reference_type", "reference_id"}).
		AddRow("weird-1", "new_entity_type", "ledger", "ldg-123")
	onboardingMock.ExpectQuery(`WITH orphan_ledgers AS`).
		WillReturnRows(onboardingRows)

	// Transaction query returns no orphans
	transactionRows := sqlmock.NewRows([]string{"entity_id", "entity_type", "reference_type", "reference_id"})
	transactionMock.ExpectQuery(`SELECT`).
		WillReturnRows(transactionRows)
	transactionMock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Use real logger (logging output is verified by seeing no panic and proper count)
	logger := libZap.InitializeLogger()
	checker := NewReferentialChecker(onboardingDB, transactionDB, logger)
	result, err := checker.Check(context.Background(), CheckerConfig{})

	require.NoError(t, err)
	typedResult := requireReferentialResult(t, result)

	// Verify unknown entity type is properly counted
	assert.Equal(t, 1, typedResult.OrphanUnknown)
	assert.Len(t, typedResult.Orphans, 1)
	assert.Equal(t, "new_entity_type", typedResult.Orphans[0].EntityType)
	assert.Equal(t, "weird-1", typedResult.Orphans[0].EntityID)

	assert.NoError(t, onboardingMock.ExpectationsWereMet())
	assert.NoError(t, transactionMock.ExpectationsWereMet())
}

func TestReferentialChecker_Check_OnboardingQueryError(t *testing.T) {
	t.Parallel()

	onboardingDB, onboardingMock, err := sqlmock.New()
	require.NoError(t, err)
	defer onboardingDB.Close()

	transactionDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer transactionDB.Close()

	onboardingMock.ExpectQuery(`WITH orphan_ledgers AS`).
		WillReturnError(assert.AnError)

	checker := NewReferentialChecker(onboardingDB, transactionDB, nil)
	result, err := checker.Check(context.Background(), CheckerConfig{})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "referential onboarding check failed")
}

func requireReferentialResult(t *testing.T, result CheckResult) *domain.ReferentialCheckResult {
	t.Helper()

	typedResult, ok := result.(*domain.ReferentialCheckResult)
	require.True(t, ok)

	return typedResult
}
