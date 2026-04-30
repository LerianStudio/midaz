//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"sync"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TEST HELPERS - CreateBulk Integration Tests
// =============================================================================

// bulkTestInfra holds infrastructure for bulk operation integration tests.
type bulkTestInfra struct {
	pgContainer *pgtestutil.ContainerResult
	repo        *TransactionPostgreSQLRepository
	orgID       uuid.UUID
	ledgerID    uuid.UUID
}

// setupBulkTestInfra sets up the test infrastructure for bulk integration testing.
func setupBulkTestInfra(t *testing.T) *bulkTestInfra {
	t.Helper()

	// Setup PostgreSQL container
	pgContainer := pgtestutil.SetupContainer(t)

	// Create lib-commons PostgreSQL connection
	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")
	connStr := pgtestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)

	conn := pgtestutil.CreatePostgresClient(t, connStr, connStr, pgContainer.Config.DBName, migrationsPath)

	// Create repository
	repo := NewTransactionPostgreSQLRepository(conn)

	// Use fake UUIDs for external entities (no FK constraints between components)
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	return &bulkTestInfra{
		pgContainer: pgContainer,
		repo:        repo,
		orgID:       orgID,
		ledgerID:    ledgerID,
	}
}

// createBulkTestTransactions creates n test transactions with proper IDs.
func (infra *bulkTestInfra) createBulkTestTransactions(t *testing.T, count int) []*Transaction {
	t.Helper()

	transactions := make([]*Transaction, count)
	now := time.Now().UTC()
	amount := decimal.NewFromInt(1000)

	for i := 0; i < count; i++ {
		transactions[i] = &Transaction{
			ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
			Description:    "Bulk test transaction",
			Status:         Status{Code: "PENDING"},
			Amount:         &amount,
			AssetCode:      "USD",
			LedgerID:       infra.ledgerID.String(),
			OrganizationID: infra.orgID.String(),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}

	return transactions
}

// =============================================================================
// INTEGRATION TESTS - CreateBulk
// =============================================================================

// TestIntegration_TransactionCreateBulk_Success tests successful bulk insert of 50 transactions.
func TestIntegration_TransactionCreateBulk_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	// Create 50 transactions (matches PRD target bulk size)
	transactions := infra.createBulkTestTransactions(t, 50)

	// Act
	result, err := infra.repo.CreateBulk(ctx, transactions)

	// Assert
	require.NoError(t, err, "CreateBulk should not return error")
	require.NotNil(t, result, "result should not be nil")

	assert.Equal(t, int64(50), result.Attempted, "Attempted should be 50")
	assert.Equal(t, int64(50), result.Inserted, "Inserted should be 50")
	assert.Equal(t, int64(0), result.Ignored, "Ignored should be 0 for fresh inserts")

	// Verify invariant: Attempted = Inserted + Ignored
	assert.Equal(t, result.Attempted, result.Inserted+result.Ignored, "invariant should hold")

	// Verify InsertedIDs are returned
	assert.Len(t, result.InsertedIDs, 50, "InsertedIDs should contain 50 IDs")

	// Verify transactions exist in database
	for _, tx := range transactions {
		found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, uuid.MustParse(tx.ID))
		require.NoError(t, err, "transaction should be findable")
		assert.Equal(t, tx.ID, found.ID)
		assert.Equal(t, "PENDING", found.Status.Code)
	}

	t.Log("Integration test passed: bulk insert of 50 transactions verified")
}

// TestIntegration_TransactionCreateBulk_DuplicateHandling tests idempotency with duplicate transactions.
func TestIntegration_TransactionCreateBulk_DuplicateHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	// Create and insert 20 transactions
	originalTransactions := infra.createBulkTestTransactions(t, 20)

	result1, err := infra.repo.CreateBulk(ctx, originalTransactions)
	require.NoError(t, err)
	assert.Equal(t, int64(20), result1.Inserted)

	// Create a mix of existing and new transactions
	newTransactions := infra.createBulkTestTransactions(t, 10)

	// Combine: 10 duplicates (from original) + 10 new = 20 total
	mixedTransactions := make([]*Transaction, 20)
	copy(mixedTransactions[:10], originalTransactions[:10]) // 10 duplicates
	copy(mixedTransactions[10:], newTransactions)           // 10 new

	// Act - insert mixed batch
	result2, err := infra.repo.CreateBulk(ctx, mixedTransactions)

	// Assert
	require.NoError(t, err, "CreateBulk should not error on duplicates")
	require.NotNil(t, result2)

	assert.Equal(t, int64(20), result2.Attempted, "Attempted should be 20")
	assert.Equal(t, int64(10), result2.Inserted, "Inserted should be 10 (new only)")
	assert.Equal(t, int64(10), result2.Ignored, "Ignored should be 10 (duplicates)")

	// Verify invariant
	assert.Equal(t, result2.Attempted, result2.Inserted+result2.Ignored, "invariant should hold")

	// Verify InsertedIDs contains only new transactions
	assert.Len(t, result2.InsertedIDs, 10, "InsertedIDs should contain only newly inserted IDs")

	// Verify original transactions are unchanged
	for _, tx := range originalTransactions[:10] {
		found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, uuid.MustParse(tx.ID))
		require.NoError(t, err)
		assert.Equal(t, tx.Description, found.Description, "original should be unchanged")
	}

	t.Log("Integration test passed: duplicate handling with correct ignored count verified")
}

// TestIntegration_TransactionCreateBulk_AllDuplicates tests inserting all duplicates.
func TestIntegration_TransactionCreateBulk_AllDuplicates(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	// Create and insert transactions
	transactions := infra.createBulkTestTransactions(t, 25)

	result1, err := infra.repo.CreateBulk(ctx, transactions)
	require.NoError(t, err)
	assert.Equal(t, int64(25), result1.Inserted)

	// Act - try to insert the same transactions again
	result2, err := infra.repo.CreateBulk(ctx, transactions)

	// Assert
	require.NoError(t, err, "CreateBulk should not error on all duplicates")
	require.NotNil(t, result2)

	assert.Equal(t, int64(25), result2.Attempted)
	assert.Equal(t, int64(0), result2.Inserted, "no new rows should be inserted")
	assert.Equal(t, int64(25), result2.Ignored, "all should be ignored as duplicates")
	assert.Empty(t, result2.InsertedIDs, "no InsertedIDs when all duplicates")

	t.Log("Integration test passed: all duplicates correctly ignored")
}

// TestIntegration_TransactionCreateBulk_ConcurrentBulks tests concurrent bulk inserts without deadlocks.
func TestIntegration_TransactionCreateBulk_ConcurrentBulks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	numGoroutines := 5
	transactionsPerGoroutine := 20

	type bulkResult struct {
		goroutineID int
		result      *struct {
			attempted int64
			inserted  int64
		}
		err error
	}

	results := make(chan bulkResult, numGoroutines)

	// Launch concurrent bulk inserts
	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			// Each goroutine inserts unique transactions
			transactions := infra.createBulkTestTransactions(t, transactionsPerGoroutine)

			res, err := infra.repo.CreateBulk(ctx, transactions)

			br := bulkResult{
				goroutineID: goroutineID,
				err:         err,
			}
			if res != nil {
				br.result = &struct {
					attempted int64
					inserted  int64
				}{
					attempted: res.Attempted,
					inserted:  res.Inserted,
				}
			}

			results <- br
		}(g)
	}

	// Wait for all goroutines and close channel
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and analyze results
	var successCount, errorCount int
	var totalInserted int64

	for r := range results {
		if r.err != nil {
			errorCount++
			t.Errorf("Goroutine %d failed: %v", r.goroutineID, r.err)
		} else {
			successCount++
			totalInserted += r.result.inserted
		}
	}

	// Assert
	assert.Equal(t, numGoroutines, successCount, "all goroutines should succeed")
	assert.Equal(t, 0, errorCount, "no errors should occur")

	expectedTotal := int64(numGoroutines * transactionsPerGoroutine)
	assert.Equal(t, expectedTotal, totalInserted, "total inserted should match expected")

	t.Logf("Integration test passed: %d concurrent bulk inserts completed without deadlocks", numGoroutines)
}

// TestIntegration_TransactionCreateBulk_SortingVerification tests that transactions are sorted by ID.
func TestIntegration_TransactionCreateBulk_SortingVerification(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	// Create transactions with specific IDs in reverse order
	transactions := make([]*Transaction, 3)
	now := time.Now().UTC()
	amount := decimal.NewFromInt(1000)

	// IDs in descending order (will be sorted ascending)
	transactions[0] = &Transaction{
		ID:             "ffffffff-ffff-ffff-ffff-ffffffffffff",
		Description:    "Transaction C (highest ID)",
		Status:         Status{Code: "PENDING"},
		Amount:         &amount,
		AssetCode:      "USD",
		LedgerID:       infra.ledgerID.String(),
		OrganizationID: infra.orgID.String(),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	transactions[1] = &Transaction{
		ID:             "00000000-0000-0000-0000-000000000001",
		Description:    "Transaction A (lowest ID)",
		Status:         Status{Code: "PENDING"},
		Amount:         &amount,
		AssetCode:      "USD",
		LedgerID:       infra.ledgerID.String(),
		OrganizationID: infra.orgID.String(),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	transactions[2] = &Transaction{
		ID:             "88888888-8888-8888-8888-888888888888",
		Description:    "Transaction B (middle ID)",
		Status:         Status{Code: "PENDING"},
		Amount:         &amount,
		AssetCode:      "USD",
		LedgerID:       infra.ledgerID.String(),
		OrganizationID: infra.orgID.String(),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Verify initial order
	assert.Equal(t, "ffffffff-ffff-ffff-ffff-ffffffffffff", transactions[0].ID, "initial order should have highest ID first")

	// Act
	result, err := infra.repo.CreateBulk(ctx, transactions)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.Inserted)

	// Verify the slice was sorted in-place (ascending by ID)
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", transactions[0].ID, "first should be lowest ID after sort")
	assert.Equal(t, "88888888-8888-8888-8888-888888888888", transactions[1].ID, "second should be middle ID after sort")
	assert.Equal(t, "ffffffff-ffff-ffff-ffff-ffffffffffff", transactions[2].ID, "third should be highest ID after sort")

	// Verify all transactions were inserted correctly
	for _, tx := range transactions {
		found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, uuid.MustParse(tx.ID))
		require.NoError(t, err)
		assert.Equal(t, tx.Description, found.Description)
	}

	t.Log("Integration test passed: sorting by ID verified")
}

// TestIntegration_TransactionCreateBulk_Chunking tests bulk insert with >1000 rows (chunking).
func TestIntegration_TransactionCreateBulk_Chunking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	// Create 2500 transactions to trigger multiple chunks (chunk size is 1000)
	transactions := infra.createBulkTestTransactions(t, 2500)

	// Act
	result, err := infra.repo.CreateBulk(ctx, transactions)

	// Assert
	require.NoError(t, err, "CreateBulk should handle chunking without error")
	require.NotNil(t, result)

	assert.Equal(t, int64(2500), result.Attempted)
	assert.Equal(t, int64(2500), result.Inserted)
	assert.Equal(t, int64(0), result.Ignored)
	assert.Len(t, result.InsertedIDs, 2500)

	// Spot check some transactions from different chunks
	checkIndices := []int{0, 500, 999, 1000, 1500, 2000, 2499}
	for _, idx := range checkIndices {
		tx := transactions[idx]
		found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, uuid.MustParse(tx.ID))
		require.NoError(t, err, "transaction at index %d should be findable", idx)
		assert.Equal(t, tx.ID, found.ID)
	}

	t.Log("Integration test passed: chunking with 2500 transactions verified")
}

// TestIntegration_TransactionCreateBulk_EmptyInput tests handling of empty input.
func TestIntegration_TransactionCreateBulk_EmptyInput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	// Act
	result, err := infra.repo.CreateBulk(ctx, []*Transaction{})

	// Assert
	require.NoError(t, err, "empty input should not error")
	require.NotNil(t, result)

	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, int64(0), result.Inserted)
	assert.Equal(t, int64(0), result.Ignored)
	assert.Empty(t, result.InsertedIDs)

	t.Log("Integration test passed: empty input handled correctly")
}

// TestIntegration_TransactionCreateBulk_SingleTransaction tests bulk insert with single transaction.
func TestIntegration_TransactionCreateBulk_SingleTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	// Create single transaction
	transactions := infra.createBulkTestTransactions(t, 1)

	// Act
	result, err := infra.repo.CreateBulk(ctx, transactions)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int64(1), result.Attempted)
	assert.Equal(t, int64(1), result.Inserted)
	assert.Equal(t, int64(0), result.Ignored)
	assert.Len(t, result.InsertedIDs, 1)
	assert.Equal(t, transactions[0].ID, result.InsertedIDs[0])

	// Verify transaction exists
	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, uuid.MustParse(transactions[0].ID))
	require.NoError(t, err)
	assert.Equal(t, transactions[0].ID, found.ID)

	t.Log("Integration test passed: single transaction bulk insert verified")
}

// TestIntegration_TransactionCreateBulk_InsertedIDsTracking tests that InsertedIDs correctly tracks inserted rows.
func TestIntegration_TransactionCreateBulk_InsertedIDsTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	// Create and insert some transactions
	existingTransactions := infra.createBulkTestTransactions(t, 5)
	_, err := infra.repo.CreateBulk(ctx, existingTransactions)
	require.NoError(t, err)

	// Create mixed batch
	newTransactions := infra.createBulkTestTransactions(t, 5)
	mixedTransactions := make([]*Transaction, 10)
	copy(mixedTransactions[:5], existingTransactions) // duplicates
	copy(mixedTransactions[5:], newTransactions)      // new

	// Act
	result, err := infra.repo.CreateBulk(ctx, mixedTransactions)

	// Assert
	require.NoError(t, err)
	assert.Len(t, result.InsertedIDs, 5, "InsertedIDs should only contain newly inserted")

	// Verify InsertedIDs contains only new transaction IDs
	newIDSet := make(map[string]bool)
	for _, tx := range newTransactions {
		newIDSet[tx.ID] = true
	}

	for _, insertedID := range result.InsertedIDs {
		assert.True(t, newIDSet[insertedID], "InsertedID %s should be a new transaction", insertedID)
	}

	// Verify existing IDs are NOT in InsertedIDs
	for _, tx := range existingTransactions {
		found := false
		for _, insertedID := range result.InsertedIDs {
			if insertedID == tx.ID {
				found = true
				break
			}
		}
		assert.False(t, found, "existing ID %s should NOT be in InsertedIDs", tx.ID)
	}

	t.Log("Integration test passed: InsertedIDs tracking verified")
}

// TestIntegration_TransactionCreateBulk_ContextCancellation tests handling of context cancellation.
func TestIntegration_TransactionCreateBulk_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)

	// Create transactions
	transactions := infra.createBulkTestTransactions(t, 100)

	// Cancel context before calling CreateBulk
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	result, err := infra.repo.CreateBulk(ctx, transactions)

	// Assert
	require.Error(t, err, "should error on cancelled context")
	assert.ErrorIs(t, err, context.Canceled)

	// Partial result should be returned
	require.NotNil(t, result)
	assert.Equal(t, int64(100), result.Attempted)
	assert.Equal(t, int64(0), result.Inserted, "no rows should be inserted on immediate cancellation")

	t.Log("Integration test passed: context cancellation handled correctly")
}

// TestIntegration_TransactionCreateBulk_TransactionWithAllFields tests bulk insert with all fields populated.
func TestIntegration_TransactionCreateBulk_TransactionWithAllFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	now := time.Now().UTC()
	amount := decimal.NewFromInt(5000)
	statusDesc := "Pending approval"
	parentID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	// Create first transaction (parent)
	parentTx := &Transaction{
		ID:             parentID,
		Description:    "Parent transaction",
		Status:         Status{Code: "APPROVED", Description: &statusDesc},
		Amount:         &amount,
		AssetCode:      "BRL",
		LedgerID:       infra.ledgerID.String(),
		OrganizationID: infra.orgID.String(),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	_, err := infra.repo.CreateBulk(ctx, []*Transaction{parentTx})
	require.NoError(t, err)

	// Create child transaction with all fields
	childTx := &Transaction{
		ID:                       uuid.Must(libCommons.GenerateUUIDv7()).String(),
		ParentTransactionID:      &parentID,
		Description:              "Child transaction with all fields",
		Status:                   Status{Code: "PENDING", Description: &statusDesc},
		Amount:                   &amount,
		AssetCode:                "BRL",
		ChartOfAccountsGroupName: "revenue",
		LedgerID:                 infra.ledgerID.String(),
		OrganizationID:           infra.orgID.String(),
		Route:                    "from(@user1)->to(@user2)",
		CreatedAt:                now,
		UpdatedAt:                now,
	}

	// Act
	result, err := infra.repo.CreateBulk(ctx, []*Transaction{childTx})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Inserted)

	// Verify all fields persisted correctly
	found, err := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, uuid.MustParse(childTx.ID))
	require.NoError(t, err)

	assert.Equal(t, childTx.Description, found.Description)
	assert.Equal(t, "PENDING", found.Status.Code)
	require.NotNil(t, found.Status.Description)
	assert.Equal(t, statusDesc, *found.Status.Description)
	assert.Equal(t, "BRL", found.AssetCode)
	assert.Equal(t, "revenue", found.ChartOfAccountsGroupName)
	assert.Equal(t, "from(@user1)->to(@user2)", found.Route)
	// RouteID is FK-constrained, not tested here
	require.NotNil(t, found.ParentTransactionID)
	assert.Equal(t, parentID, *found.ParentTransactionID)

	t.Log("Integration test passed: transaction with all fields verified")
}
