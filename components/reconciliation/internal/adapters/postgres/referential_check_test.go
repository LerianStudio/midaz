package postgres

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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

	checker := NewReferentialChecker(onboardingDB, transactionDB)
	result, err := checker.Check(context.Background())

	require.NoError(t, err)
	assert.Equal(t, domain.StatusHealthy, result.Status)
	assert.Equal(t, 0, result.OrphanLedgers)
	assert.Equal(t, 0, result.OrphanAssets)
	assert.Equal(t, 0, result.OrphanAccounts)
	assert.Equal(t, 0, result.OrphanOperations)
	assert.Empty(t, result.Orphans)
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

	checker := NewReferentialChecker(onboardingDB, transactionDB)
	result, err := checker.Check(context.Background())

	require.NoError(t, err)
	assert.Equal(t, domain.StatusWarning, result.Status) // 3 orphans < 10 threshold
	assert.Equal(t, 1, result.OrphanLedgers)
	assert.Equal(t, 1, result.OrphanAssets)
	assert.Equal(t, 1, result.OrphanOperations)
	assert.Len(t, result.Orphans, 3)
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
		onboardingRows.AddRow("acc-"+string(rune('a'+i)), "account", "ledger", "ldg-deleted")
	}
	onboardingMock.ExpectQuery(`WITH orphan_ledgers AS`).
		WillReturnRows(onboardingRows)

	transactionRows := sqlmock.NewRows([]string{"entity_id", "entity_type", "reference_type", "reference_id"})
	for i := 0; i < 5; i++ {
		transactionRows.AddRow("op-"+string(rune('a'+i)), "operation", "transaction", "txn-deleted")
	}
	transactionMock.ExpectQuery(`SELECT`).
		WillReturnRows(transactionRows)

	checker := NewReferentialChecker(onboardingDB, transactionDB)
	result, err := checker.Check(context.Background())

	require.NoError(t, err)
	assert.Equal(t, domain.StatusCritical, result.Status) // 13 orphans >= 10
	assert.Equal(t, 8, result.OrphanAccounts)
	assert.Equal(t, 5, result.OrphanOperations)
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

	checker := NewReferentialChecker(onboardingDB, transactionDB)
	result, err := checker.Check(context.Background())

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "referential onboarding check failed")
}
