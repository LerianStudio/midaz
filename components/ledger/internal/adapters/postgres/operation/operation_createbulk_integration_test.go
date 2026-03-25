//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Helper Types for Bulk Tests
// =============================================================================

// bulkTestInfra holds infrastructure shared across bulk operation tests.
type bulkTestInfra struct {
	container *pgtestutil.ContainerResult
	repo      *OperationPostgreSQLRepository
	ids       testIDs
}

// setupBulkTestInfra creates the test infrastructure for bulk operation tests.
func setupBulkTestInfra(t *testing.T) *bulkTestInfra {
	t.Helper()

	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	return &bulkTestInfra{
		container: container,
		repo:      repo,
		ids:       ids,
	}
}

// createTestOperation creates a single operation entity for testing.
func createTestOperation(t *testing.T, ids testIDs, index int) *Operation {
	t.Helper()

	opID := uuid.Must(libCommons.GenerateUUIDv7())
	now := time.Now().Truncate(time.Microsecond)

	amount := decimal.NewFromInt(int64(100 + index))
	availableBefore := decimal.NewFromInt(10000)
	onHoldBefore := decimal.Zero
	availableAfter := availableBefore.Sub(amount)
	onHoldAfter := decimal.Zero
	versionBefore := int64(1)
	versionAfter := int64(2)

	return &Operation{
		ID:              opID.String(),
		TransactionID:   ids.TransactionID.String(),
		Description:     fmt.Sprintf("Bulk test operation %d", index),
		Type:            "DEBIT",
		AssetCode:       "USD",
		ChartOfAccounts: "1000",
		Amount:          Amount{Value: &amount},
		Balance: Balance{
			Available: &availableBefore,
			OnHold:    &onHoldBefore,
			Version:   &versionBefore,
		},
		BalanceAfter: Balance{
			Available: &availableAfter,
			OnHold:    &onHoldAfter,
			Version:   &versionAfter,
		},
		Status: Status{
			Code: "APPROVED",
		},
		AccountID:       ids.AccountID.String(),
		AccountAlias:    "@bulk-test-account",
		BalanceKey:      "default",
		BalanceID:       ids.BalanceID.String(),
		OrganizationID:  ids.OrgID.String(),
		LedgerID:        ids.LedgerID.String(),
		BalanceAffected: true,
		Direction:       "debit",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// createTestOperationBatch creates a batch of operations for testing.
func createTestOperationBatch(t *testing.T, ids testIDs, count int) []*Operation {
	t.Helper()

	operations := make([]*Operation, count)
	for i := 0; i < count; i++ {
		operations[i] = createTestOperation(t, ids, i)
	}

	return operations
}

// =============================================================================
// CreateBulk Integration Tests
// =============================================================================

// TestIntegration_OperationRepository_CreateBulk_Success tests successful bulk insertion
// of multiple operations.
func TestIntegration_OperationRepository_CreateBulk_Success(t *testing.T) {
	// Arrange
	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	operations := createTestOperationBatch(t, infra.ids, 50)

	// Act
	result, err := infra.repo.CreateBulk(ctx, operations)

	// Assert
	require.NoError(t, err, "CreateBulk should not return error")
	require.NotNil(t, result, "result should not be nil")

	assert.Equal(t, int64(50), result.Attempted, "Attempted should be 50")
	assert.Equal(t, int64(50), result.Inserted, "Inserted should be 50")
	assert.Equal(t, int64(0), result.Ignored, "Ignored should be 0")
	assert.Len(t, result.InsertedIDs, 50, "InsertedIDs should have 50 elements")

	// Verify all operations are retrievable
	for _, op := range operations {
		found, err := infra.repo.Find(ctx, infra.ids.OrgID, infra.ids.LedgerID, infra.ids.TransactionID, uuid.MustParse(op.ID))
		require.NoError(t, err, "should find inserted operation")
		assert.Equal(t, op.ID, found.ID, "ID should match")
		assert.Equal(t, op.Description, found.Description, "Description should match")
	}
}

// TestIntegration_OperationRepository_CreateBulk_DuplicateHandling tests that duplicate
// operations are ignored via ON CONFLICT DO NOTHING.
func TestIntegration_OperationRepository_CreateBulk_DuplicateHandling(t *testing.T) {
	// Arrange
	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	operations := createTestOperationBatch(t, infra.ids, 10)

	// Insert first batch
	result1, err := infra.repo.CreateBulk(ctx, operations)
	require.NoError(t, err)
	assert.Equal(t, int64(10), result1.Inserted)

	// Create new batch with some duplicates (5 existing + 5 new)
	duplicateOps := make([]*Operation, 10)
	copy(duplicateOps[:5], operations[:5])

	for i := 5; i < 10; i++ {
		duplicateOps[i] = createTestOperation(t, infra.ids, 100+i)
	}

	// Act
	result2, err := infra.repo.CreateBulk(ctx, duplicateOps)

	// Assert
	require.NoError(t, err, "CreateBulk with duplicates should not return error")
	assert.Equal(t, int64(10), result2.Attempted, "Attempted should be 10")
	assert.Equal(t, int64(5), result2.Inserted, "Inserted should be 5 (new ones only)")
	assert.Equal(t, int64(5), result2.Ignored, "Ignored should be 5 (duplicates)")
	assert.Len(t, result2.InsertedIDs, 5, "InsertedIDs should have 5 new IDs")
}

// TestIntegration_OperationRepository_CreateBulk_AllDuplicates tests bulk insert where
// all operations are duplicates.
func TestIntegration_OperationRepository_CreateBulk_AllDuplicates(t *testing.T) {
	// Arrange
	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	operations := createTestOperationBatch(t, infra.ids, 5)

	// Insert original batch
	result1, err := infra.repo.CreateBulk(ctx, operations)
	require.NoError(t, err)
	assert.Equal(t, int64(5), result1.Inserted)

	// Act - insert same batch again
	result2, err := infra.repo.CreateBulk(ctx, operations)

	// Assert
	require.NoError(t, err, "CreateBulk with all duplicates should not return error")
	assert.Equal(t, int64(5), result2.Attempted, "Attempted should be 5")
	assert.Equal(t, int64(0), result2.Inserted, "Inserted should be 0 (all duplicates)")
	assert.Equal(t, int64(5), result2.Ignored, "Ignored should be 5")
	assert.Empty(t, result2.InsertedIDs, "InsertedIDs should be empty")
}

// TestIntegration_OperationRepository_CreateBulk_ConcurrentBulks tests that concurrent
// bulk inserts work correctly without deadlocks due to ID sorting.
func TestIntegration_OperationRepository_CreateBulk_ConcurrentBulks(t *testing.T) {
	// Arrange
	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	const numGoroutines = 5
	const opsPerGoroutine = 20

	var wg sync.WaitGroup

	results := make([]*struct {
		result *struct {
			Attempted int64
			Inserted  int64
			Ignored   int64
		}
		err error
	}, numGoroutines)

	// Act - run concurrent bulk inserts
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			operations := createTestOperationBatch(t, infra.ids, opsPerGoroutine)

			result, err := infra.repo.CreateBulk(ctx, operations)
			results[idx] = &struct {
				result *struct {
					Attempted int64
					Inserted  int64
					Ignored   int64
				}
				err error
			}{
				result: &struct {
					Attempted int64
					Inserted  int64
					Ignored   int64
				}{
					Attempted: result.Attempted,
					Inserted:  result.Inserted,
					Ignored:   result.Ignored,
				},
				err: err,
			}
		}(i)
	}

	wg.Wait()

	// Assert - all should succeed without deadlocks
	totalInserted := int64(0)

	for i, r := range results {
		require.NoError(t, r.err, "goroutine %d should not return error", i)
		require.NotNil(t, r.result, "goroutine %d result should not be nil", i)
		assert.Equal(t, int64(opsPerGoroutine), r.result.Attempted, "goroutine %d Attempted should be %d", i, opsPerGoroutine)
		totalInserted += r.result.Inserted
	}

	assert.Equal(t, int64(numGoroutines*opsPerGoroutine), totalInserted, "total inserted should be %d", numGoroutines*opsPerGoroutine)
}

// TestIntegration_OperationRepository_CreateBulk_SortingVerification tests that
// operations are sorted by ID before insertion (deadlock prevention).
func TestIntegration_OperationRepository_CreateBulk_SortingVerification(t *testing.T) {
	// Arrange
	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	// Create operations with specific IDs in reverse order
	operations := make([]*Operation, 5)
	ids := []string{
		"ffffffff-ffff-ffff-ffff-ffffffffffff",
		"cccccccc-cccc-cccc-cccc-cccccccccccc",
		"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		"55555555-5555-5555-5555-555555555555",
		"11111111-1111-1111-1111-111111111111",
	}

	now := time.Now().Truncate(time.Microsecond)
	amount := decimal.NewFromInt(100)
	availableBefore := decimal.NewFromInt(10000)
	onHoldBefore := decimal.Zero
	availableAfter := decimal.NewFromInt(9900)
	onHoldAfter := decimal.Zero
	versionBefore := int64(1)
	versionAfter := int64(2)

	for i, id := range ids {
		operations[i] = &Operation{
			ID:              id,
			TransactionID:   infra.ids.TransactionID.String(),
			Description:     fmt.Sprintf("Sort test operation %d", i),
			Type:            "DEBIT",
			AssetCode:       "USD",
			ChartOfAccounts: "1000",
			Amount:          Amount{Value: &amount},
			Balance: Balance{
				Available: &availableBefore,
				OnHold:    &onHoldBefore,
				Version:   &versionBefore,
			},
			BalanceAfter: Balance{
				Available: &availableAfter,
				OnHold:    &onHoldAfter,
				Version:   &versionAfter,
			},
			Status:          Status{Code: "APPROVED"},
			AccountID:       infra.ids.AccountID.String(),
			AccountAlias:    "@sort-test",
			BalanceKey:      "default",
			BalanceID:       infra.ids.BalanceID.String(),
			OrganizationID:  infra.ids.OrgID.String(),
			LedgerID:        infra.ids.LedgerID.String(),
			BalanceAffected: true,
			Direction:       "debit",
			CreatedAt:       now,
			UpdatedAt:       now,
		}
	}

	// Act
	result, err := infra.repo.CreateBulk(ctx, operations)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(5), result.Inserted)

	// Verify original slice is sorted (CreateBulk sorts in-place)
	sorted := sort.SliceIsSorted(operations, func(i, j int) bool {
		return operations[i].ID < operations[j].ID
	})
	assert.True(t, sorted, "operations slice should be sorted by ID after CreateBulk")
}

// TestIntegration_OperationRepository_CreateBulk_Chunking tests that large batches
// are correctly chunked (operation has 30 columns, chunk size is 1000).
func TestIntegration_OperationRepository_CreateBulk_Chunking(t *testing.T) {
	// Arrange
	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	// Create 2500 operations to test multi-chunk behavior (3 chunks: 1000 + 1000 + 500)
	operations := createTestOperationBatch(t, infra.ids, 2500)

	// Act
	result, err := infra.repo.CreateBulk(ctx, operations)

	// Assert
	require.NoError(t, err, "CreateBulk with large batch should not return error")
	assert.Equal(t, int64(2500), result.Attempted, "Attempted should be 2500")
	assert.Equal(t, int64(2500), result.Inserted, "Inserted should be 2500")
	assert.Equal(t, int64(0), result.Ignored, "Ignored should be 0")
	assert.Len(t, result.InsertedIDs, 2500, "InsertedIDs should have 2500 elements")

	// Verify count in database
	var count int
	err = infra.container.DB.QueryRow(`SELECT COUNT(*) FROM operation WHERE transaction_id = $1`, infra.ids.TransactionID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2500, count, "should have 2500 operations in database")
}

// TestIntegration_OperationRepository_CreateBulk_EmptyInput tests that empty input
// returns early with zero counts.
func TestIntegration_OperationRepository_CreateBulk_EmptyInput(t *testing.T) {
	// Arrange
	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	var operations []*Operation

	// Act
	result, err := infra.repo.CreateBulk(ctx, operations)

	// Assert
	require.NoError(t, err, "CreateBulk with empty input should not return error")
	require.NotNil(t, result, "result should not be nil")
	assert.Equal(t, int64(0), result.Attempted, "Attempted should be 0")
	assert.Equal(t, int64(0), result.Inserted, "Inserted should be 0")
	assert.Equal(t, int64(0), result.Ignored, "Ignored should be 0")
}

// TestIntegration_OperationRepository_CreateBulk_SingleOperation tests that a single
// operation is handled correctly.
func TestIntegration_OperationRepository_CreateBulk_SingleOperation(t *testing.T) {
	// Arrange
	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	operations := createTestOperationBatch(t, infra.ids, 1)

	// Act
	result, err := infra.repo.CreateBulk(ctx, operations)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Attempted)
	assert.Equal(t, int64(1), result.Inserted)
	assert.Equal(t, int64(0), result.Ignored)
	assert.Len(t, result.InsertedIDs, 1)
	assert.Equal(t, operations[0].ID, result.InsertedIDs[0])
}

// TestIntegration_OperationRepository_CreateBulk_InsertedIDsTracking tests that
// InsertedIDs correctly tracks which operations were actually inserted.
func TestIntegration_OperationRepository_CreateBulk_InsertedIDsTracking(t *testing.T) {
	// Arrange
	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	// Insert initial batch
	firstBatch := createTestOperationBatch(t, infra.ids, 3)
	_, err := infra.repo.CreateBulk(ctx, firstBatch)
	require.NoError(t, err)

	// Create mixed batch: 2 duplicates + 3 new
	mixedBatch := make([]*Operation, 5)
	copy(mixedBatch[:2], firstBatch[:2])

	for i := 2; i < 5; i++ {
		mixedBatch[i] = createTestOperation(t, infra.ids, 200+i)
	}

	// Act
	result, err := infra.repo.CreateBulk(ctx, mixedBatch)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(5), result.Attempted)
	assert.Equal(t, int64(3), result.Inserted)
	assert.Equal(t, int64(2), result.Ignored)
	assert.Len(t, result.InsertedIDs, 3)

	// Verify InsertedIDs contains only the new operations
	insertedIDSet := make(map[string]bool)
	for _, id := range result.InsertedIDs {
		insertedIDSet[id] = true
	}

	for i := 2; i < 5; i++ {
		assert.True(t, insertedIDSet[mixedBatch[i].ID], "new operation ID should be in InsertedIDs")
	}

	for i := 0; i < 2; i++ {
		assert.False(t, insertedIDSet[mixedBatch[i].ID], "duplicate operation ID should NOT be in InsertedIDs")
	}
}

// TestIntegration_OperationRepository_CreateBulk_ContextCancellation tests that
// context cancellation is handled correctly between chunks.
func TestIntegration_OperationRepository_CreateBulk_ContextCancellation(t *testing.T) {
	// Arrange
	infra := setupBulkTestInfra(t)

	// Create enough operations to span multiple chunks
	operations := createTestOperationBatch(t, infra.ids, 1500)

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	result, err := infra.repo.CreateBulk(ctx, operations)

	// Assert
	require.Error(t, err, "should return error for cancelled context")
	assert.ErrorIs(t, err, context.Canceled, "error should be context.Canceled")

	// Partial results may be available
	if result != nil {
		assert.Equal(t, int64(1500), result.Attempted, "Attempted should reflect total")
		// Inserted may be 0 or partial depending on timing
	}
}

// TestIntegration_OperationRepository_CreateBulk_OperationWithAllFields tests that
// all operation fields are correctly persisted.
func TestIntegration_OperationRepository_CreateBulk_OperationWithAllFields(t *testing.T) {
	// Arrange
	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	opID := uuid.Must(libCommons.GenerateUUIDv7())
	now := time.Now().Truncate(time.Microsecond)

	amount := decimal.NewFromInt(12345)
	availableBefore := decimal.NewFromInt(100000)
	onHoldBefore := decimal.NewFromInt(5000)
	availableAfter := decimal.NewFromInt(87655)
	onHoldAfter := decimal.NewFromInt(5000)
	versionBefore := int64(10)
	versionAfter := int64(11)
	statusDesc := "Approved by system"

	operation := &Operation{
		ID:              opID.String(),
		TransactionID:   infra.ids.TransactionID.String(),
		Description:     "Full fields test operation",
		Type:            "CREDIT",
		AssetCode:       "BRL",
		ChartOfAccounts: "2000",
		Amount:          Amount{Value: &amount},
		Balance: Balance{
			Available: &availableBefore,
			OnHold:    &onHoldBefore,
			Version:   &versionBefore,
		},
		BalanceAfter: Balance{
			Available: &availableAfter,
			OnHold:    &onHoldAfter,
			Version:   &versionAfter,
		},
		Status: Status{
			Code:        "APPROVED",
			Description: &statusDesc,
		},
		AccountID:       infra.ids.AccountID.String(),
		AccountAlias:    "@full-fields-test",
		BalanceKey:      "premium",
		BalanceID:       infra.ids.BalanceID.String(),
		OrganizationID:  infra.ids.OrgID.String(),
		LedgerID:        infra.ids.LedgerID.String(),
		Route:           "legacy-route-value",
		BalanceAffected: true,
		Direction:       "credit",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	operations := []*Operation{operation}

	// Act
	result, err := infra.repo.CreateBulk(ctx, operations)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Inserted)

	// Verify all fields via Find
	found, err := infra.repo.Find(ctx, infra.ids.OrgID, infra.ids.LedgerID, infra.ids.TransactionID, opID)
	require.NoError(t, err)

	assert.Equal(t, opID.String(), found.ID)
	assert.Equal(t, infra.ids.TransactionID.String(), found.TransactionID)
	assert.Equal(t, "Full fields test operation", found.Description)
	assert.Equal(t, "CREDIT", found.Type)
	assert.Equal(t, "BRL", found.AssetCode)
	assert.Equal(t, "2000", found.ChartOfAccounts)
	assert.True(t, found.Amount.Value.Equal(amount))
	assert.True(t, found.Balance.Available.Equal(availableBefore))
	assert.True(t, found.Balance.OnHold.Equal(onHoldBefore))
	assert.Equal(t, versionBefore, *found.Balance.Version)
	assert.True(t, found.BalanceAfter.Available.Equal(availableAfter))
	assert.True(t, found.BalanceAfter.OnHold.Equal(onHoldAfter))
	assert.Equal(t, versionAfter, *found.BalanceAfter.Version)
	assert.Equal(t, "APPROVED", found.Status.Code)
	assert.Equal(t, statusDesc, *found.Status.Description)
	assert.Equal(t, infra.ids.AccountID.String(), found.AccountID)
	assert.Equal(t, "@full-fields-test", found.AccountAlias)
	assert.Equal(t, "premium", found.BalanceKey)
	assert.Equal(t, infra.ids.BalanceID.String(), found.BalanceID)
	assert.True(t, found.BalanceAffected)
	assert.Equal(t, "credit", found.Direction)
	// RouteID, RouteCode, RouteDescription are FK-constrained, tested separately
}

// TestIntegration_OperationRepository_CreateBulk_DifferentOperationTypes tests
// bulk insert with mixed DEBIT and CREDIT operations.
func TestIntegration_OperationRepository_CreateBulk_DifferentOperationTypes(t *testing.T) {
	// Arrange
	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Microsecond)

	operations := make([]*Operation, 10)

	for i := 0; i < 10; i++ {
		opID := uuid.Must(libCommons.GenerateUUIDv7())
		amount := decimal.NewFromInt(int64(100 + i*10))
		availableBefore := decimal.NewFromInt(10000)
		onHoldBefore := decimal.Zero
		versionBefore := int64(1)
		versionAfter := int64(2)

		var opType, direction string

		var availableAfter decimal.Decimal

		if i%2 == 0 {
			opType = "DEBIT"
			direction = "debit"
			availableAfter = availableBefore.Sub(amount)
		} else {
			opType = "CREDIT"
			direction = "credit"
			availableAfter = availableBefore.Add(amount)
		}

		operations[i] = &Operation{
			ID:              opID.String(),
			TransactionID:   infra.ids.TransactionID.String(),
			Description:     fmt.Sprintf("%s operation %d", opType, i),
			Type:            opType,
			AssetCode:       "USD",
			ChartOfAccounts: "1000",
			Amount:          Amount{Value: &amount},
			Balance: Balance{
				Available: &availableBefore,
				OnHold:    &onHoldBefore,
				Version:   &versionBefore,
			},
			BalanceAfter: Balance{
				Available: &availableAfter,
				OnHold:    &onHoldBefore,
				Version:   &versionAfter,
			},
			Status:          Status{Code: "APPROVED"},
			AccountID:       infra.ids.AccountID.String(),
			AccountAlias:    "@mixed-types",
			BalanceKey:      "default",
			BalanceID:       infra.ids.BalanceID.String(),
			OrganizationID:  infra.ids.OrgID.String(),
			LedgerID:        infra.ids.LedgerID.String(),
			BalanceAffected: true,
			Direction:       direction,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
	}

	// Act
	result, err := infra.repo.CreateBulk(ctx, operations)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(10), result.Inserted)

	// Verify types are correct
	var debitCount, creditCount int

	err = infra.container.DB.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE type = 'DEBIT'),
			COUNT(*) FILTER (WHERE type = 'CREDIT')
		FROM operation
		WHERE transaction_id = $1`, infra.ids.TransactionID).Scan(&debitCount, &creditCount)
	require.NoError(t, err)

	assert.Equal(t, 5, debitCount, "should have 5 DEBIT operations")
	assert.Equal(t, 5, creditCount, "should have 5 CREDIT operations")
}
