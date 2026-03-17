//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"fmt"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createRepository creates an OperationRepository connected to the test database.
func createRepository(t *testing.T, container *pgtestutil.ContainerResult) *OperationPostgreSQLRepository {
	t.Helper()

	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")

	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)

	conn := pgtestutil.CreatePostgresClient(t, connStr, connStr, container.Config.DBName, migrationsPath)

	return NewOperationPostgreSQLRepository(conn)
}

// testIDs holds common test entity IDs for setup convenience.
type testIDs struct {
	OrgID         uuid.UUID
	LedgerID      uuid.UUID
	AccountID     uuid.UUID
	BalanceID     uuid.UUID
	TransactionID uuid.UUID
}

// createTestDependencies creates all required entities for operation tests.
// Note: Organization, Ledger, and Account are in the onboarding database (separate from transaction).
// We use random UUIDs for those since the operation table just stores them as reference IDs.
// Only Transaction and Balance exist in the transaction database.
func createTestDependencies(t *testing.T, container *pgtestutil.ContainerResult) testIDs {
	t.Helper()

	// These entities exist only in onboarding DB - use random UUIDs
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	// Balance exists in transaction DB
	balanceParams := pgtestutil.DefaultBalanceParams()
	balanceParams.Alias = "@test-balance"
	balanceParams.AssetCode = "USD"
	balanceID := pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, balanceParams)

	// Transaction exists in transaction DB
	txID := pgtestutil.CreateTestTransactionWithStatus(t, container.DB, orgID, ledgerID, "APPROVED", decimal.NewFromInt(100), "USD")

	return testIDs{
		OrgID:         orgID,
		LedgerID:      ledgerID,
		AccountID:     accountID,
		BalanceID:     balanceID,
		TransactionID: txID,
	}
}

// ============================================================================
// CreateBatch Tests
// ============================================================================

func TestIntegration_OperationRepository_CreateBatch_EmptySlice(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)

	ctx := context.Background()

	// Act
	inserted, err := repo.CreateBatch(ctx, []*Operation{})

	// Assert
	require.NoError(t, err, "CreateBatch with empty slice should not error")
	assert.Equal(t, int64(0), inserted, "should insert 0 operations")
}

func TestIntegration_OperationRepository_CreateBatch_SingleOperation(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	operation := createTestOperation(ids, "single-batch-op", now)

	// Act
	inserted, err := repo.CreateBatch(ctx, []*Operation{operation})

	// Assert
	require.NoError(t, err, "CreateBatch with single operation should not error")
	assert.Equal(t, int64(1), inserted, "should insert 1 operation")

	// Verify operation was persisted
	found, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, uuid.MustParse(operation.ID))
	require.NoError(t, err)
	assert.Equal(t, operation.ID, found.ID)
}

func TestIntegration_OperationRepository_CreateBatch_MultipleOperations(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create 10 operations
	operations := make([]*Operation, 10)
	for i := 0; i < 10; i++ {
		operations[i] = createTestOperation(ids, fmt.Sprintf("batch-op-%d", i), now)
	}

	// Act
	inserted, err := repo.CreateBatch(ctx, operations)

	// Assert
	require.NoError(t, err, "CreateBatch with multiple operations should not error")
	assert.Equal(t, int64(10), inserted, "should insert 10 operations")

	// Verify all operations were persisted
	for _, op := range operations {
		found, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, uuid.MustParse(op.ID))
		require.NoError(t, err, "operation %s should be found", op.ID)
		assert.Equal(t, op.ID, found.ID)
	}
}

func TestIntegration_OperationRepository_CreateBatch_DuplicateHandling(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create and insert an operation first
	existingOp := createTestOperation(ids, "duplicate-test", now)
	_, err := repo.CreateBatch(ctx, []*Operation{existingOp})
	require.NoError(t, err)

	// Create batch with the same operation (duplicate) plus new ones
	newOp1 := createTestOperation(ids, "new-op-1", now)
	newOp2 := createTestOperation(ids, "new-op-2", now)

	batchWithDuplicate := []*Operation{existingOp, newOp1, newOp2}

	// Act
	inserted, err := repo.CreateBatch(ctx, batchWithDuplicate)

	// Assert
	require.NoError(t, err, "CreateBatch should handle duplicates gracefully")
	assert.Equal(t, int64(2), inserted, "should insert only 2 new operations, skipping duplicate")

	// Verify all operations exist
	for _, op := range batchWithDuplicate {
		found, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, uuid.MustParse(op.ID))
		require.NoError(t, err, "operation %s should be found", op.ID)
		assert.NotNil(t, found)
	}
}

func TestIntegration_OperationRepository_CreateBatch_AllDuplicates(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create and insert operations first
	operations := make([]*Operation, 3)
	for i := 0; i < 3; i++ {
		operations[i] = createTestOperation(ids, fmt.Sprintf("all-dup-%d", i), now)
	}
	_, err := repo.CreateBatch(ctx, operations)
	require.NoError(t, err)

	// Act - try to insert the same operations again
	inserted, err := repo.CreateBatch(ctx, operations)

	// Assert
	require.NoError(t, err, "CreateBatch with all duplicates should not error")
	assert.Equal(t, int64(0), inserted, "should insert 0 operations when all are duplicates")
}

func TestIntegration_OperationRepository_CreateBatch_SortsByID(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create operations with IDs in reverse order (Z-A)
	operations := make([]*Operation, 5)
	for i := 0; i < 5; i++ {
		operations[i] = createTestOperation(ids, fmt.Sprintf("sort-test-%d", i), now)
	}

	// Reverse the slice to simulate out-of-order input
	for i, j := 0, len(operations)-1; i < j; i, j = i+1, j-1 {
		operations[i], operations[j] = operations[j], operations[i]
	}

	// Act - this should sort by ID internally before insert
	inserted, err := repo.CreateBatch(ctx, operations)

	// Assert
	require.NoError(t, err, "CreateBatch should handle out-of-order operations")
	assert.Equal(t, int64(5), inserted, "should insert all 5 operations")
}

func TestIntegration_OperationRepository_CreateBatch_LargeBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large batch test in short mode")
	}

	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create 100 operations (moderate size for CI)
	operations := make([]*Operation, 100)
	for i := 0; i < 100; i++ {
		operations[i] = createTestOperation(ids, fmt.Sprintf("large-batch-%d", i), now)
	}

	// Act
	inserted, err := repo.CreateBatch(ctx, operations)

	// Assert
	require.NoError(t, err, "CreateBatch with 100 operations should not error")
	assert.Equal(t, int64(100), inserted, "should insert all 100 operations")
}

func TestIntegration_OperationRepository_CreateBatch_ContextCancellation(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	now := time.Now().Truncate(time.Microsecond)

	// Create a cancelled context BEFORE calling CreateBatch
	// This tests the early context check (line 252-254)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	operations := make([]*Operation, 10)
	for i := 0; i < 10; i++ {
		operations[i] = createTestOperation(ids, fmt.Sprintf("cancelled-%d", i), now)
	}

	// Act
	inserted, err := repo.CreateBatch(ctx, operations)

	// Assert - should return context error from early check
	require.Error(t, err, "CreateBatch with cancelled context should error")
	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, int64(0), inserted, "should not insert any operations")
}

func TestIntegration_OperationRepository_CreateBatch_ContextTimeoutDuringChunks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	now := time.Now().Truncate(time.Microsecond)

	// Create >1000 operations to force multiple chunks
	// Use a very short timeout that will expire during chunk processing
	const numOperations = 1500
	operations := make([]*Operation, numOperations)
	for i := 0; i < numOperations; i++ {
		operations[i] = createTestOperation(ids, fmt.Sprintf("timeout-chunk-%d", i), now)
	}

	// Context with very short timeout - should expire during second chunk
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Act - first chunk may succeed, second chunk should hit context timeout
	inserted, err := repo.CreateBatch(ctx, operations)
	// Assert - we expect either success (if fast enough) or context deadline exceeded
	// The key is that this exercises the chunk loop context check
	if err != nil {
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.True(t, inserted >= 0 && inserted < int64(numOperations),
			"should have partial inserts if timeout occurred")
	}
	// If no error, all inserts succeeded within timeout - that's also valid
}

func TestIntegration_OperationRepository_CreateBatch_NilElement(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create a slice with a nil element
	operations := []*Operation{
		createTestOperation(ids, "valid-op-1", now),
		nil, // nil element at index 1
		createTestOperation(ids, "valid-op-2", now),
	}

	// Act
	inserted, err := repo.CreateBatch(ctx, operations)

	// Assert - should return error about nil element
	require.Error(t, err, "CreateBatch with nil element should error")
	assert.Contains(t, err.Error(), "index 1", "error should indicate the nil element index")
	assert.Contains(t, err.Error(), "nil", "error should mention nil")
	assert.Equal(t, int64(0), inserted, "should not insert any operations")
}

func TestIntegration_OperationRepository_CreateBatch_ChunkingBoundary(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chunking boundary test in short mode")
	}

	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create 1001 operations to test chunking (1000 + 1)
	// This exercises the boundary where operations > defaultBatchSize
	const numOperations = 1001
	operations := make([]*Operation, numOperations)
	for i := 0; i < numOperations; i++ {
		operations[i] = createTestOperation(ids, fmt.Sprintf("chunk-boundary-%d", i), now)
	}

	// Act
	inserted, err := repo.CreateBatch(ctx, operations)

	// Assert
	require.NoError(t, err, "CreateBatch with 1001 operations should not error")
	assert.Equal(t, int64(numOperations), inserted, "should insert all 1001 operations")
}

func TestIntegration_OperationRepository_CreateBatch_FallbackPath(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Create operations with invalid transaction_id to force FK violation
	// This triggers the fallback path (batch fails, individual inserts attempted)
	invalidTxID := uuid.Must(uuid.NewV7()).String() // Non-existent transaction

	operations := make([]*Operation, 3)
	for i := 0; i < 3; i++ {
		op := createTestOperation(ids, fmt.Sprintf("fallback-test-%d", i), now)
		op.TransactionID = invalidTxID // This will cause FK violation
		operations[i] = op
	}

	// Act - batch will fail due to FK violation, triggering fallback
	// Fallback will also fail, but it exercises the code path
	inserted, err := repo.CreateBatch(ctx, operations)

	// Assert - we expect 0 inserts due to FK violations
	// The important thing is that the fallback code was exercised
	require.NoError(t, err, "CreateBatch should not return error (fallback handles failures)")
	assert.Equal(t, int64(0), inserted, "should insert 0 operations due to FK violations")
}

func TestIntegration_OperationRepository_CreateBatch_MixedValidAndInvalid(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Mix of valid operations and ones with invalid transaction_id
	// First insert some valid ones, then try batch with mix
	validOp1 := createTestOperation(ids, "mixed-valid-1", now)
	validOp2 := createTestOperation(ids, "mixed-valid-2", now)

	// Insert valid ops first
	inserted, err := repo.CreateBatch(ctx, []*Operation{validOp1, validOp2})
	require.NoError(t, err)
	assert.Equal(t, int64(2), inserted)

	// Now create a batch with duplicates (already inserted) and new valid ones
	newValidOp := createTestOperation(ids, "mixed-new-valid", now)
	operations := []*Operation{validOp1, newValidOp, validOp2} // duplicates + new

	// Act
	inserted, err = repo.CreateBatch(ctx, operations)

	// Assert - should insert only the new one (duplicates skipped via ON CONFLICT)
	require.NoError(t, err)
	assert.Equal(t, int64(1), inserted, "should insert only 1 new operation")
}

func TestIntegration_OperationRepository_CreateBatch_EarlyContextCancellation(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	now := time.Now().Truncate(time.Microsecond)

	// Create a context that is already cancelled before CreateBatch is called
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	operations := []*Operation{
		createTestOperation(ids, "early-cancel-1", now),
		createTestOperation(ids, "early-cancel-2", now),
	}

	// Act - should return immediately due to early context check
	inserted, err := repo.CreateBatch(ctx, operations)

	// Assert
	require.Error(t, err, "CreateBatch should return error for cancelled context")
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int64(0), inserted, "should not insert any operations")
}

// createTestOperation is a helper to create test Operation entities
func createTestOperation(ids testIDs, description string, now time.Time) *Operation {
	amount := decimal.NewFromInt(100)
	availableBefore := decimal.NewFromInt(1000)
	onHoldBefore := decimal.Zero
	availableAfter := decimal.NewFromInt(900)
	onHoldAfter := decimal.Zero
	versionBefore := int64(1)
	versionAfter := int64(2)

	return &Operation{
		ID:              uuid.Must(libCommons.GenerateUUIDv7()).String(),
		TransactionID:   ids.TransactionID.String(),
		Description:     description,
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
		AccountAlias:    "@test-account",
		BalanceKey:      "default",
		BalanceID:       ids.BalanceID.String(),
		OrganizationID:  ids.OrgID.String(),
		LedgerID:        ids.LedgerID.String(),
		BalanceAffected: true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// ============================================================================
// Create Tests
// ============================================================================

func TestIntegration_OperationRepository_Create_Success(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	amount := decimal.NewFromInt(100)
	availableBefore := decimal.NewFromInt(1000)
	onHoldBefore := decimal.Zero
	availableAfter := decimal.NewFromInt(900)
	onHoldAfter := decimal.Zero
	versionBefore := int64(1)
	versionAfter := int64(2)
	statusDesc := "Operation approved"

	operation := &Operation{
		ID:              uuid.Must(libCommons.GenerateUUIDv7()).String(),
		TransactionID:   ids.TransactionID.String(),
		Description:     "Test debit operation",
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
			Code:        "APPROVED",
			Description: &statusDesc,
		},
		AccountID:       ids.AccountID.String(),
		AccountAlias:    "@test-account",
		BalanceKey:      "default",
		BalanceID:       ids.BalanceID.String(),
		OrganizationID:  ids.OrgID.String(),
		LedgerID:        ids.LedgerID.String(),
		BalanceAffected: true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Act
	created, err := repo.Create(ctx, operation)

	// Assert
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, created, "created operation should not be nil")

	assert.Equal(t, operation.ID, created.ID, "ID should match")
	assert.Equal(t, operation.TransactionID, created.TransactionID, "TransactionID should match")
	assert.Equal(t, operation.Description, created.Description, "Description should match")
	assert.Equal(t, operation.Type, created.Type, "Type should match")
	assert.Equal(t, operation.AssetCode, created.AssetCode, "AssetCode should match")
	assert.True(t, created.Amount.Value.Equal(amount), "Amount should match")
	assert.Equal(t, "APPROVED", created.Status.Code, "Status code should match")
	assert.True(t, created.BalanceAffected, "BalanceAffected should be true")
}

func TestIntegration_OperationRepository_Create_GeneratesIDWhenEmpty(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	amount := decimal.NewFromInt(50)
	availableBefore := decimal.NewFromInt(1000)
	onHoldBefore := decimal.Zero
	availableAfter := decimal.NewFromInt(1050)
	onHoldAfter := decimal.Zero
	versionBefore := int64(1)
	versionAfter := int64(2)

	operation := &Operation{
		ID:            "", // Empty ID - should be generated
		TransactionID: ids.TransactionID.String(),
		Type:          "CREDIT",
		AssetCode:     "USD",
		Amount:        Amount{Value: &amount},
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
		AccountID:       ids.AccountID.String(),
		AccountAlias:    "@test-account",
		BalanceID:       ids.BalanceID.String(),
		OrganizationID:  ids.OrgID.String(),
		LedgerID:        ids.LedgerID.String(),
		BalanceAffected: true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Act
	created, err := repo.Create(ctx, operation)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.NotEmpty(t, created.ID, "ID should be generated when empty")
	assert.Len(t, created.ID, 36, "Generated ID should be a valid UUID string")
}

// ============================================================================
// Find Tests
// ============================================================================

func TestIntegration_OperationRepository_Find_ReturnsOperation(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	// Create operation using fixture
	opParams := pgtestutil.OperationParams{
		TransactionID:         ids.TransactionID,
		Description:           "Find test operation",
		Type:                  "DEBIT",
		AccountID:             ids.AccountID,
		AccountAlias:          "@test-account",
		BalanceID:             ids.BalanceID,
		AssetCode:             "USD",
		Amount:                decimal.NewFromInt(100),
		AvailableBalance:      decimal.NewFromInt(1000),
		OnHoldBalance:         decimal.Zero,
		AvailableBalanceAfter: decimal.NewFromInt(900),
		OnHoldBalanceAfter:    decimal.Zero,
		BalanceVersionBefore:  1,
		BalanceVersionAfter:   2,
		Status:                "APPROVED",
		BalanceAffected:       true,
	}
	opID := pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)

	ctx := context.Background()

	// Act
	found, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, opID)

	// Assert
	require.NoError(t, err, "Find should not return error for existing operation")
	require.NotNil(t, found, "operation should not be nil")

	assert.Equal(t, opID.String(), found.ID, "ID should match")
	assert.Equal(t, ids.TransactionID.String(), found.TransactionID, "TransactionID should match")
	assert.Equal(t, "Find test operation", found.Description, "Description should match")
	assert.Equal(t, "DEBIT", found.Type, "Type should match")
	assert.Equal(t, "USD", found.AssetCode, "AssetCode should match")
	assert.Equal(t, ids.AccountID.String(), found.AccountID, "AccountID should match")
	assert.Equal(t, "@test-account", found.AccountAlias, "AccountAlias should match")
	assert.True(t, found.Amount.Value.Equal(decimal.NewFromInt(100)), "Amount should match")
	assert.Equal(t, "APPROVED", found.Status.Code, "Status should match")
	assert.True(t, found.BalanceAffected, "BalanceAffected should be true")
}

func TestIntegration_OperationRepository_Find_ReturnsEntityNotFoundError(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	nonExistentID := uuid.Must(libCommons.GenerateUUIDv7())

	ctx := context.Background()

	// Act
	found, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, nonExistentID)

	// Assert
	require.Error(t, err, "Find should return error for non-existent operation")
	assert.Nil(t, found, "operation should be nil")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
	assert.Equal(t, constant.ErrEntityNotFound.Error(), entityNotFoundErr.Code, "error code should be ErrEntityNotFound")
	assert.Equal(t, "Operation", entityNotFoundErr.EntityType, "entity type should be Operation")
}

func TestIntegration_OperationRepository_Find_IgnoresDeletedOperation(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	// Create deleted operation
	deletedAt := time.Now().Add(-1 * time.Hour)
	opParams := pgtestutil.OperationParams{
		TransactionID:   ids.TransactionID,
		Type:            "DEBIT",
		AccountID:       ids.AccountID,
		AccountAlias:    "@test-account",
		BalanceID:       ids.BalanceID,
		AssetCode:       "USD",
		Amount:          decimal.NewFromInt(100),
		Status:          "APPROVED",
		BalanceAffected: true,
		DeletedAt:       &deletedAt,
	}
	opID := pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)

	ctx := context.Background()

	// Act
	found, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, opID)

	// Assert
	require.Error(t, err, "Find should return error for deleted operation")
	assert.Nil(t, found, "deleted operation should not be returned")
}

// ============================================================================
// FindByAccount Tests
// ============================================================================

func TestIntegration_OperationRepository_FindByAccount_ReturnsOperation(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	opParams := pgtestutil.OperationParams{
		TransactionID:   ids.TransactionID,
		Description:     "FindByAccount test",
		Type:            "CREDIT",
		AccountID:       ids.AccountID,
		AccountAlias:    "@test-account",
		BalanceID:       ids.BalanceID,
		AssetCode:       "USD",
		Amount:          decimal.NewFromInt(200),
		Status:          "APPROVED",
		BalanceAffected: true,
	}
	opID := pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)

	ctx := context.Background()

	// Act
	found, err := repo.FindByAccount(ctx, ids.OrgID, ids.LedgerID, ids.AccountID, opID)

	// Assert
	require.NoError(t, err, "FindByAccount should not return error")
	require.NotNil(t, found)

	assert.Equal(t, opID.String(), found.ID)
	assert.Equal(t, ids.AccountID.String(), found.AccountID)
	assert.Equal(t, "FindByAccount test", found.Description)
}

func TestIntegration_OperationRepository_FindByAccount_ReturnsEntityNotFoundError(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	nonExistentID := uuid.Must(libCommons.GenerateUUIDv7())

	ctx := context.Background()

	// Act
	found, err := repo.FindByAccount(ctx, ids.OrgID, ids.LedgerID, ids.AccountID, nonExistentID)

	// Assert
	require.Error(t, err, "FindByAccount should return error for non-existent operation")
	assert.Nil(t, found)

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr)
}

func TestIntegration_OperationRepository_FindByAccount_WrongAccountReturnsError(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	// Create operation for ids.AccountID
	opParams := pgtestutil.OperationParams{
		TransactionID:   ids.TransactionID,
		Type:            "DEBIT",
		AccountID:       ids.AccountID,
		AccountAlias:    "@test-account",
		BalanceID:       ids.BalanceID,
		AssetCode:       "USD",
		Amount:          decimal.NewFromInt(100),
		Status:          "APPROVED",
		BalanceAffected: true,
	}
	opID := pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)

	// Try to find with different account
	differentAccountID := uuid.Must(libCommons.GenerateUUIDv7())

	ctx := context.Background()

	// Act
	found, err := repo.FindByAccount(ctx, ids.OrgID, ids.LedgerID, differentAccountID, opID)

	// Assert
	require.Error(t, err, "FindByAccount should return error when account doesn't match")
	assert.Nil(t, found)
}

// ============================================================================
// FindAll Tests (by Transaction)
// ============================================================================

func TestIntegration_OperationRepository_FindAll_ReturnsOperations(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	// Create multiple operations for the same transaction
	for i := 0; i < 3; i++ {
		opParams := pgtestutil.OperationParams{
			TransactionID:   ids.TransactionID,
			Description:     fmt.Sprintf("FindAll test %d", i),
			Type:            "DEBIT",
			AccountID:       ids.AccountID,
			AccountAlias:    "@test-account",
			BalanceID:       ids.BalanceID,
			AssetCode:       "USD",
			Amount:          decimal.NewFromInt(int64(100 + i*10)),
			Status:          "APPROVED",
			BalanceAffected: true,
		}
		pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)
	}

	ctx := context.Background()

	// Act
	operations, cur, err := repo.FindAll(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, defaultPagination())

	// Assert
	require.NoError(t, err, "FindAll should not return error")
	assert.Len(t, operations, 3, "should return 3 operations")
	assert.Empty(t, cur.Next, "should not have next cursor with only 3 items")
}

func TestIntegration_OperationRepository_FindAll_EmptyForNonExistentTransaction(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	nonExistentTxID := uuid.Must(libCommons.GenerateUUIDv7())

	ctx := context.Background()

	// Act
	operations, _, err := repo.FindAll(ctx, ids.OrgID, ids.LedgerID, nonExistentTxID, defaultPagination())

	// Assert
	require.NoError(t, err, "should not error for empty result")
	assert.Empty(t, operations, "should return empty slice")
}

func TestIntegration_OperationRepository_FindAll_Pagination(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	// Create 7 operations
	for i := 0; i < 7; i++ {
		opParams := pgtestutil.OperationParams{
			TransactionID:   ids.TransactionID,
			Description:     fmt.Sprintf("Pagination test %d", i),
			Type:            "DEBIT",
			AccountID:       ids.AccountID,
			AccountAlias:    "@test-account",
			BalanceID:       ids.BalanceID,
			AssetCode:       "USD",
			Amount:          decimal.NewFromInt(int64(100 + i*10)),
			Status:          "APPROVED",
			BalanceAffected: true,
		}
		pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)
	}

	ctx := context.Background()

	// Page 1: limit=3
	page1Filter := http.Pagination{
		Limit:     3,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page1, cur1, err := repo.FindAll(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, page1Filter)

	require.NoError(t, err)
	assert.Len(t, page1, 3, "page 1 should have 3 items")
	assert.NotEmpty(t, cur1.Next, "page 1 should have next cursor")

	// Page 2: using next cursor
	page2Filter := http.Pagination{
		Limit:     3,
		SortOrder: "DESC",
		Cursor:    cur1.Next,
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page2, cur2, err := repo.FindAll(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, page2Filter)

	require.NoError(t, err)
	assert.Len(t, page2, 3, "page 2 should have 3 items")
	assert.NotEmpty(t, cur2.Prev, "page 2 should have prev cursor")

	// Page 3: last page with 1 item
	page3Filter := http.Pagination{
		Limit:     3,
		SortOrder: "DESC",
		Cursor:    cur2.Next,
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page3, cur3, err := repo.FindAll(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, page3Filter)

	require.NoError(t, err)
	assert.Len(t, page3, 1, "page 3 should have 1 item")
	assert.Empty(t, cur3.Next, "page 3 should not have next cursor")
	assert.NotEmpty(t, cur3.Prev, "page 3 should have prev cursor")
}

func TestIntegration_OperationRepository_FindAll_FiltersByDateRange(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	// Create an operation (created today)
	opParams := pgtestutil.OperationParams{
		TransactionID:   ids.TransactionID,
		Type:            "DEBIT",
		AccountID:       ids.AccountID,
		AccountAlias:    "@test-account",
		BalanceID:       ids.BalanceID,
		AssetCode:       "USD",
		Amount:          decimal.NewFromInt(100),
		Status:          "APPROVED",
		BalanceAffected: true,
	}
	pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)

	ctx := context.Background()

	// Act 1: Query with past-only window (should return 0 items)
	pastFilter := http.Pagination{
		Limit:     10,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(0, 0, -10),
		EndDate:   time.Now().AddDate(0, 0, -9),
	}
	opsPast, _, err := repo.FindAll(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, pastFilter)
	require.NoError(t, err)
	assert.Empty(t, opsPast, "past-only window should return 0 items")

	// Act 2: Query with today's window (should return 1 item)
	todayFilter := http.Pagination{
		Limit:     10,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(0, 0, -1),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	opsToday, _, err := repo.FindAll(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, todayFilter)
	require.NoError(t, err)
	assert.Len(t, opsToday, 1, "today's window should return 1 item")
}

// ============================================================================
// FindAllByAccount Tests
// ============================================================================

func TestIntegration_OperationRepository_FindAllByAccount_ReturnsOperations(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	// Create multiple operations for the same account
	for i := 0; i < 3; i++ {
		// Create a new transaction for each operation
		txID := pgtestutil.CreateTestTransactionWithStatus(t, container.DB, ids.OrgID, ids.LedgerID, "APPROVED", decimal.NewFromInt(100), "USD")
		opParams := pgtestutil.OperationParams{
			TransactionID:   txID,
			Description:     fmt.Sprintf("FindAllByAccount test %d", i),
			Type:            "DEBIT",
			AccountID:       ids.AccountID,
			AccountAlias:    "@test-account",
			BalanceID:       ids.BalanceID,
			AssetCode:       "USD",
			Amount:          decimal.NewFromInt(int64(100 + i*10)),
			Status:          "APPROVED",
			BalanceAffected: true,
		}
		pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)
	}

	ctx := context.Background()

	// Act
	operations, cur, err := repo.FindAllByAccount(ctx, ids.OrgID, ids.LedgerID, ids.AccountID, nil, defaultPagination())

	// Assert
	require.NoError(t, err, "FindAllByAccount should not return error")
	assert.Len(t, operations, 3, "should return 3 operations")
	assert.Empty(t, cur.Next, "should not have next cursor with only 3 items")
}

func TestIntegration_OperationRepository_FindAllByAccount_FiltersByOperationType(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	// Create DEBIT operations
	for i := 0; i < 2; i++ {
		txID := pgtestutil.CreateTestTransactionWithStatus(t, container.DB, ids.OrgID, ids.LedgerID, "APPROVED", decimal.NewFromInt(100), "USD")
		opParams := pgtestutil.OperationParams{
			TransactionID:   txID,
			Type:            "DEBIT",
			AccountID:       ids.AccountID,
			AccountAlias:    "@test-account",
			BalanceID:       ids.BalanceID,
			AssetCode:       "USD",
			Amount:          decimal.NewFromInt(100),
			Status:          "APPROVED",
			BalanceAffected: true,
		}
		pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)
	}

	// Create CREDIT operations
	for i := 0; i < 3; i++ {
		txID := pgtestutil.CreateTestTransactionWithStatus(t, container.DB, ids.OrgID, ids.LedgerID, "APPROVED", decimal.NewFromInt(100), "USD")
		opParams := pgtestutil.OperationParams{
			TransactionID:   txID,
			Type:            "CREDIT",
			AccountID:       ids.AccountID,
			AccountAlias:    "@test-account",
			BalanceID:       ids.BalanceID,
			AssetCode:       "USD",
			Amount:          decimal.NewFromInt(100),
			Status:          "APPROVED",
			BalanceAffected: true,
		}
		pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)
	}

	ctx := context.Background()

	// Act - filter by DEBIT type
	debitType := "DEBIT"
	debitOps, _, err := repo.FindAllByAccount(ctx, ids.OrgID, ids.LedgerID, ids.AccountID, &debitType, defaultPagination())

	require.NoError(t, err)
	assert.Len(t, debitOps, 2, "should return only DEBIT operations")
	for _, op := range debitOps {
		assert.Equal(t, "DEBIT", op.Type, "all operations should be DEBIT type")
	}

	// Act - filter by CREDIT type
	creditType := "CREDIT"
	creditOps, _, err := repo.FindAllByAccount(ctx, ids.OrgID, ids.LedgerID, ids.AccountID, &creditType, defaultPagination())

	require.NoError(t, err)
	assert.Len(t, creditOps, 3, "should return only CREDIT operations")
	for _, op := range creditOps {
		assert.Equal(t, "CREDIT", op.Type, "all operations should be CREDIT type")
	}
}

func TestIntegration_OperationRepository_FindAllByAccount_EmptyForNonExistentAccount(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	nonExistentAccountID := uuid.Must(libCommons.GenerateUUIDv7())

	ctx := context.Background()

	// Act
	operations, _, err := repo.FindAllByAccount(ctx, ids.OrgID, ids.LedgerID, nonExistentAccountID, nil, defaultPagination())

	// Assert
	require.NoError(t, err, "should not error for empty result")
	assert.Empty(t, operations, "should return empty slice")
}

func TestIntegration_OperationRepository_FindAllByAccount_Pagination(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	// Create 7 operations for same account
	for i := 0; i < 7; i++ {
		txID := pgtestutil.CreateTestTransactionWithStatus(t, container.DB, ids.OrgID, ids.LedgerID, "APPROVED", decimal.NewFromInt(100), "USD")
		opParams := pgtestutil.OperationParams{
			TransactionID:   txID,
			Description:     fmt.Sprintf("Pagination test %d", i),
			Type:            "DEBIT",
			AccountID:       ids.AccountID,
			AccountAlias:    "@test-account",
			BalanceID:       ids.BalanceID,
			AssetCode:       "USD",
			Amount:          decimal.NewFromInt(int64(100 + i*10)),
			Status:          "APPROVED",
			BalanceAffected: true,
		}
		pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)
	}

	ctx := context.Background()

	// Page 1
	page1Filter := http.Pagination{
		Limit:     3,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page1, cur1, err := repo.FindAllByAccount(ctx, ids.OrgID, ids.LedgerID, ids.AccountID, nil, page1Filter)

	require.NoError(t, err)
	assert.Len(t, page1, 3)
	assert.NotEmpty(t, cur1.Next)

	// Page 2
	page2Filter := http.Pagination{
		Limit:     3,
		SortOrder: "DESC",
		Cursor:    cur1.Next,
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page2, cur2, err := repo.FindAllByAccount(ctx, ids.OrgID, ids.LedgerID, ids.AccountID, nil, page2Filter)

	require.NoError(t, err)
	assert.Len(t, page2, 3)
	assert.NotEmpty(t, cur2.Prev)

	// Page 3
	page3Filter := http.Pagination{
		Limit:     3,
		SortOrder: "DESC",
		Cursor:    cur2.Next,
		StartDate: time.Now().AddDate(-1, 0, 0),
		EndDate:   time.Now().AddDate(0, 0, 1),
	}
	page3, cur3, err := repo.FindAllByAccount(ctx, ids.OrgID, ids.LedgerID, ids.AccountID, nil, page3Filter)

	require.NoError(t, err)
	assert.Len(t, page3, 1)
	assert.Empty(t, cur3.Next)
}

// ============================================================================
// ListByIDs Tests
// ============================================================================

func TestIntegration_OperationRepository_ListByIDs_ReturnsMatchingOperations(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	// Create 5 operations
	opIDs := make([]uuid.UUID, 5)
	for i := 0; i < 5; i++ {
		opParams := pgtestutil.OperationParams{
			TransactionID:   ids.TransactionID,
			Description:     fmt.Sprintf("ListByIDs test %d", i),
			Type:            "DEBIT",
			AccountID:       ids.AccountID,
			AccountAlias:    "@test-account",
			BalanceID:       ids.BalanceID,
			AssetCode:       "USD",
			Amount:          decimal.NewFromInt(int64(100 + i*10)),
			Status:          "APPROVED",
			BalanceAffected: true,
		}
		opIDs[i] = pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)
	}

	ctx := context.Background()

	// Request only 3 of the 5 operations
	requestedIDs := []uuid.UUID{opIDs[0], opIDs[2], opIDs[4]}

	// Act
	operations, err := repo.ListByIDs(ctx, ids.OrgID, ids.LedgerID, requestedIDs)

	// Assert
	require.NoError(t, err, "ListByIDs should not return error")
	assert.Len(t, operations, 3, "should return exactly 3 operations")

	// Verify the correct operations were returned
	returnedIDs := make([]string, len(operations))
	for i, op := range operations {
		returnedIDs[i] = op.ID
	}
	for _, reqID := range requestedIDs {
		assert.Contains(t, returnedIDs, reqID.String(), "requested ID should be in results")
	}
}

func TestIntegration_OperationRepository_ListByIDs_EmptyForNonExistentIDs(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	nonExistentIDs := []uuid.UUID{uuid.Must(libCommons.GenerateUUIDv7()), uuid.Must(libCommons.GenerateUUIDv7())}

	ctx := context.Background()

	// Act
	operations, err := repo.ListByIDs(ctx, ids.OrgID, ids.LedgerID, nonExistentIDs)

	// Assert
	require.NoError(t, err, "should not error for non-existent IDs")
	assert.Empty(t, operations, "should return empty slice")
}

func TestIntegration_OperationRepository_ListByIDs_IgnoresDeletedOperations(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	// Create 2 active operations
	activeIDs := make([]uuid.UUID, 2)
	for i := 0; i < 2; i++ {
		opParams := pgtestutil.OperationParams{
			TransactionID:   ids.TransactionID,
			Type:            "DEBIT",
			AccountID:       ids.AccountID,
			AccountAlias:    "@test-account",
			BalanceID:       ids.BalanceID,
			AssetCode:       "USD",
			Amount:          decimal.NewFromInt(100),
			Status:          "APPROVED",
			BalanceAffected: true,
		}
		activeIDs[i] = pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)
	}

	// Create 1 deleted operation
	deletedAt := time.Now().Add(-1 * time.Hour)
	deletedParams := pgtestutil.OperationParams{
		TransactionID:   ids.TransactionID,
		Type:            "DEBIT",
		AccountID:       ids.AccountID,
		AccountAlias:    "@test-account",
		BalanceID:       ids.BalanceID,
		AssetCode:       "USD",
		Amount:          decimal.NewFromInt(100),
		Status:          "APPROVED",
		BalanceAffected: true,
		DeletedAt:       &deletedAt,
	}
	deletedID := pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, deletedParams)

	ctx := context.Background()

	// Request all 3 IDs
	allIDs := append(activeIDs, deletedID)

	// Act
	operations, err := repo.ListByIDs(ctx, ids.OrgID, ids.LedgerID, allIDs)

	// Assert
	require.NoError(t, err)
	assert.Len(t, operations, 2, "should return only active operations")
}

// ============================================================================
// Update Tests
// ============================================================================

func TestIntegration_OperationRepository_Update_Success(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	opParams := pgtestutil.OperationParams{
		TransactionID:   ids.TransactionID,
		Description:     "Original description",
		Type:            "DEBIT",
		AccountID:       ids.AccountID,
		AccountAlias:    "@test-account",
		BalanceID:       ids.BalanceID,
		AssetCode:       "USD",
		Amount:          decimal.NewFromInt(100),
		Status:          "APPROVED",
		BalanceAffected: true,
	}
	opID := pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)

	ctx := context.Background()

	// Get original to compare updated_at
	original, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, opID)
	require.NoError(t, err)
	originalUpdatedAt := original.UpdatedAt.Truncate(time.Microsecond)

	// Act
	updateInput := &Operation{
		Description: "Updated description",
	}
	updated, err := repo.Update(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, opID, updateInput)

	// Assert
	require.NoError(t, err, "Update should not return error")
	require.NotNil(t, updated, "updated operation should not be nil")

	assert.Equal(t, "Updated description", updated.Description, "description should be updated")
	assert.False(t, updated.UpdatedAt.Truncate(time.Microsecond).Equal(originalUpdatedAt), "updated_at should be changed")

	// Verify via Find
	found, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, opID)
	require.NoError(t, err)
	assert.Equal(t, "Updated description", found.Description)
}

func TestIntegration_OperationRepository_Update_ReturnsEntityNotFoundError(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	nonExistentID := uuid.Must(libCommons.GenerateUUIDv7())

	ctx := context.Background()

	// Act
	updateInput := &Operation{
		Description: "New description",
	}
	updated, err := repo.Update(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, nonExistentID, updateInput)

	// Assert
	require.Error(t, err, "Update should return error for non-existent operation")
	assert.Nil(t, updated, "updated operation should be nil on error")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
	assert.Equal(t, constant.ErrEntityNotFound.Error(), entityNotFoundErr.Code)
	assert.Equal(t, "Operation", entityNotFoundErr.EntityType)
}

func TestIntegration_OperationRepository_Update_IgnoresDeletedOperation(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	deletedAt := time.Now().Add(-1 * time.Hour)
	opParams := pgtestutil.OperationParams{
		TransactionID:   ids.TransactionID,
		Description:     "Deleted operation",
		Type:            "DEBIT",
		AccountID:       ids.AccountID,
		AccountAlias:    "@test-account",
		BalanceID:       ids.BalanceID,
		AssetCode:       "USD",
		Amount:          decimal.NewFromInt(100),
		Status:          "APPROVED",
		BalanceAffected: true,
		DeletedAt:       &deletedAt,
	}
	opID := pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)

	ctx := context.Background()

	// Act
	updateInput := &Operation{
		Description: "Should not update",
	}
	updated, err := repo.Update(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, opID, updateInput)

	// Assert
	require.Error(t, err, "Update should return error for deleted operation")
	assert.Nil(t, updated)
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestIntegration_OperationRepository_Delete_SoftDeletesOperation(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	opParams := pgtestutil.OperationParams{
		TransactionID:   ids.TransactionID,
		Description:     "To be deleted",
		Type:            "DEBIT",
		AccountID:       ids.AccountID,
		AccountAlias:    "@test-account",
		BalanceID:       ids.BalanceID,
		AssetCode:       "USD",
		Amount:          decimal.NewFromInt(100),
		Status:          "APPROVED",
		BalanceAffected: true,
	}
	opID := pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, ids.OrgID, ids.LedgerID, opID)

	// Assert
	require.NoError(t, err, "Delete should not return error")

	// Verify deleted_at is set in DB
	var deletedAt *time.Time
	err = container.DB.QueryRow(`SELECT deleted_at FROM operation WHERE id = $1`, opID).Scan(&deletedAt)
	require.NoError(t, err, "should be able to query operation directly")
	require.NotNil(t, deletedAt, "deleted_at should be set")

	// Operation should not be findable via repository
	found, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, opID)
	require.Error(t, err, "Find should return error after delete")
	assert.Nil(t, found, "deleted operation should not be returned")
}

func TestIntegration_OperationRepository_Delete_ReturnsEntityNotFoundError(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	nonExistentID := uuid.Must(libCommons.GenerateUUIDv7())

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, ids.OrgID, ids.LedgerID, nonExistentID)

	// Assert
	require.Error(t, err, "Delete should return error for non-existent operation")

	var entityNotFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundErr, "error should be EntityNotFoundError")
}

func TestIntegration_OperationRepository_Delete_AlreadyDeletedReturnsError(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	deletedAt := time.Now().Add(-1 * time.Hour)
	opParams := pgtestutil.OperationParams{
		TransactionID:   ids.TransactionID,
		Type:            "DEBIT",
		AccountID:       ids.AccountID,
		AccountAlias:    "@test-account",
		BalanceID:       ids.BalanceID,
		AssetCode:       "USD",
		Amount:          decimal.NewFromInt(100),
		Status:          "APPROVED",
		BalanceAffected: true,
		DeletedAt:       &deletedAt,
	}
	opID := pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)

	ctx := context.Background()

	// Act
	err := repo.Delete(ctx, ids.OrgID, ids.LedgerID, opID)

	// Assert
	require.Error(t, err, "Delete should return error for already deleted operation")
}

// ============================================================================
// Schema Default Values Tests (Migration Backwards Compatibility)
// ============================================================================

func TestIntegration_OperationRepository_SchemaDefaults(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()

	tests := []struct {
		name       string
		insertSQL  string
		argsFunc   func(id uuid.UUID, now time.Time) []any
		assertFunc func(t *testing.T, op *Operation)
	}{
		{
			name: "balance_key defaults to 'default'",
			insertSQL: `
				INSERT INTO operation (id, transaction_id, description, type, asset_code, amount, available_balance, on_hold_balance,
					available_balance_after, on_hold_balance_after, balance_version_before, balance_version_after,
					status, account_id, account_alias, balance_id, chart_of_accounts, organization_id, ledger_id,
					balance_affected, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)`,
			argsFunc: func(id uuid.UUID, now time.Time) []any {
				return []any{
					id, ids.TransactionID, "Test operation", "DEBIT", "USD", 100,
					1000, 0, 900, 0, 1, 2,
					"APPROVED", ids.AccountID, "@test-account", ids.BalanceID, "1000",
					ids.OrgID, ids.LedgerID, true, now, now,
				}
			},
			assertFunc: func(t *testing.T, op *Operation) {
				assert.Equal(t, "default", op.BalanceKey, "balance_key should default to 'default'")
			},
		},
		{
			name: "balance_affected defaults to true",
			insertSQL: `
				INSERT INTO operation (id, transaction_id, description, type, asset_code, amount, available_balance, on_hold_balance,
					available_balance_after, on_hold_balance_after, balance_version_before, balance_version_after,
					status, account_id, account_alias, balance_id, balance_key, chart_of_accounts, organization_id, ledger_id,
					created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)`,
			argsFunc: func(id uuid.UUID, now time.Time) []any {
				return []any{
					id, ids.TransactionID, "Test operation", "DEBIT", "USD", 100,
					1000, 0, 900, 0, 1, 2,
					"APPROVED", ids.AccountID, "@test-account", ids.BalanceID, "default", "1000",
					ids.OrgID, ids.LedgerID, now, now,
				}
			},
			assertFunc: func(t *testing.T, op *Operation) {
				assert.True(t, op.BalanceAffected, "balance_affected should default to true")
			},
		},
		{
			name: "updated_at defaults to now()",
			insertSQL: `
				INSERT INTO operation (id, transaction_id, description, type, asset_code, amount, available_balance, on_hold_balance,
					available_balance_after, on_hold_balance_after, balance_version_before, balance_version_after,
					status, account_id, account_alias, balance_id, balance_key, chart_of_accounts, organization_id, ledger_id,
					balance_affected, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)`,
			argsFunc: func(id uuid.UUID, now time.Time) []any {
				return []any{
					id, ids.TransactionID, "Test operation", "DEBIT", "USD", 100,
					1000, 0, 900, 0, 1, 2,
					"APPROVED", ids.AccountID, "@test-account", ids.BalanceID, "default", "1000",
					ids.OrgID, ids.LedgerID, true, now,
				}
			},
			assertFunc: func(t *testing.T, op *Operation) {
				now := time.Now()
				assert.True(t, op.UpdatedAt.After(now.Add(-5*time.Second)), "updated_at should be recent")
				assert.True(t, op.UpdatedAt.Before(now.Add(5*time.Second)), "updated_at should not be in future")
			},
		},
		{
			name: "nullable fields accept NULL gracefully",
			insertSQL: `
				INSERT INTO operation (id, transaction_id, description, type, asset_code, amount, available_balance, on_hold_balance,
					available_balance_after, on_hold_balance_after, balance_version_before, balance_version_after,
					status, status_description, account_id, account_alias, balance_id, balance_key, chart_of_accounts,
					organization_id, ledger_id, balance_affected, route, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NULL, $14, $15, $16, $17, $18, $19, $20, $21, NULL, $22, $23)`,
			argsFunc: func(id uuid.UUID, now time.Time) []any {
				return []any{
					id, ids.TransactionID, "Test operation", "DEBIT", "USD", 100,
					1000, 0, 900, 0, 1, 2,
					"APPROVED", ids.AccountID, "@test-account", ids.BalanceID, "default", "1000",
					ids.OrgID, ids.LedgerID, true, now, now,
				}
			},
			assertFunc: func(t *testing.T, op *Operation) {
				assert.Nil(t, op.Status.Description, "status_description should be nil when NULL")
				assert.Empty(t, op.Route, "route should be empty when NULL")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opID := uuid.Must(libCommons.GenerateUUIDv7())
			now := time.Now().Truncate(time.Microsecond)

			args := tt.argsFunc(opID, now)
			_, err := container.DB.Exec(tt.insertSQL, args...)
			require.NoError(t, err, "raw insert should succeed")

			op, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, opID)
			require.NoError(t, err, "Find should succeed")

			tt.assertFunc(t, op)
		})
	}
}

// TestIntegration_OperationRepository_NewColumnMigration_BackwardsCompatible tests that
// existing rows without new columns still work correctly after a migration adds new columns.
func TestIntegration_OperationRepository_NewColumnMigration_BackwardsCompatible(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()

	// Insert operation with only the minimum required columns
	// This simulates a row that existed before a new column was added
	opID := uuid.Must(libCommons.GenerateUUIDv7())
	now := time.Now().Truncate(time.Microsecond)

	_, err := container.DB.Exec(`
		INSERT INTO operation (
			id, transaction_id, description, type, asset_code, amount,
			available_balance, on_hold_balance, available_balance_after, on_hold_balance_after,
			balance_version_before, balance_version_after,
			status, account_id, account_alias, balance_id, balance_key, chart_of_accounts,
			organization_id, ledger_id, balance_affected, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12,
			$13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22, $23
		)`,
		opID, ids.TransactionID, "Migration test", "DEBIT", "USD", decimal.NewFromInt(100),
		decimal.NewFromInt(1000), decimal.Zero, decimal.NewFromInt(900), decimal.Zero,
		1, 2,
		"APPROVED", ids.AccountID, "@test-account", ids.BalanceID, "default", "1000",
		ids.OrgID, ids.LedgerID, true, now, now,
	)
	require.NoError(t, err, "minimal insert should succeed")

	// Act - Repository should be able to read the row
	op, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, opID)

	// Assert
	require.NoError(t, err, "Find should succeed for minimal row")
	require.NotNil(t, op)

	// Verify core fields are read correctly
	assert.Equal(t, opID.String(), op.ID)
	assert.Equal(t, "DEBIT", op.Type)
	assert.Equal(t, "USD", op.AssetCode)
	assert.True(t, op.Amount.Value.Equal(decimal.NewFromInt(100)))

	// Verify nullable/optional fields have safe defaults
	assert.Nil(t, op.Status.Description, "status_description should be nil")
	assert.Empty(t, op.Route, "route should be empty")
}

// TestIntegration_OperationRepository_DecimalPrecision_Preserved tests that
// large decimal values are preserved through the repository layer.
func TestIntegration_OperationRepository_DecimalPrecision_Preserved(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()

	// Large precision values
	largeAmount, _ := decimal.NewFromString("123456789012345678901234567890.123456789012345678901234567890")
	largeAvailable, _ := decimal.NewFromString("987654321098765432109876543210.987654321098765432109876543210")

	opID := uuid.Must(libCommons.GenerateUUIDv7())
	now := time.Now().Truncate(time.Microsecond)

	_, err := container.DB.Exec(`
		INSERT INTO operation (
			id, transaction_id, description, type, asset_code, amount,
			available_balance, on_hold_balance, available_balance_after, on_hold_balance_after,
			balance_version_before, balance_version_after,
			status, account_id, account_alias, balance_id, balance_key, chart_of_accounts,
			organization_id, ledger_id, balance_affected, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12,
			$13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22, $23
		)`,
		opID, ids.TransactionID, "Large precision test", "DEBIT", "USD", largeAmount,
		largeAvailable, decimal.Zero, largeAvailable.Sub(largeAmount), decimal.Zero,
		1, 2,
		"APPROVED", ids.AccountID, "@test-account", ids.BalanceID, "default", "1000",
		ids.OrgID, ids.LedgerID, true, now, now,
	)
	require.NoError(t, err, "insert with large decimals should succeed")

	// Act
	op, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, opID)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, op)

	assert.True(t, op.Amount.Value.Equal(largeAmount), "amount should preserve large precision")
	assert.True(t, op.Balance.Available.Equal(largeAvailable), "available_balance should preserve large precision")
}

// ============================================================================
// Helpers
// ============================================================================

func defaultPagination() http.Pagination {
	return http.Pagination{
		Limit:     10,
		SortOrder: "DESC",
		StartDate: time.Now().AddDate(-1, 0, 0), // 1 year ago
		EndDate:   time.Now().AddDate(0, 0, 1),  // 1 day ahead
	}
}
