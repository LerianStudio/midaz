// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateBulk_EmptyInput(t *testing.T) {
	t.Parallel()

	repo := &TransactionPostgreSQLRepository{
		connection: nil, // Will return empty result before DB call
		tableName:  "transaction",
	}

	result, err := repo.UpdateBulk(context.Background(), []*Transaction{})

	require.NoError(t, err, "empty input should not error")
	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, int64(0), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)
}

func TestUpdateBulk_NilInput(t *testing.T) {
	t.Parallel()

	repo := &TransactionPostgreSQLRepository{
		connection: nil,
		tableName:  "transaction",
	}

	result, err := repo.UpdateBulk(context.Background(), nil)

	require.NoError(t, err, "nil input should be treated as empty")
	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, int64(0), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)
}

func TestUpdateBulk_NilElementInSlice(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	transactions := []*Transaction{
		generateTestTransaction(""),
		nil, // nil element
		generateTestTransaction(""),
	}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.Error(t, err, "should error on nil element")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil transaction at index 1")
}

func TestUpdateBulk_NilElementAtStart(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	transactions := []*Transaction{
		nil, // nil at index 0
		generateTestTransaction(""),
	}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil transaction at index 0")
}

func TestUpdateBulk_SingleTransaction_AllUpdated(t *testing.T) {
	t.Parallel()

	// With batched update, a single transaction results in a single ExecContext call
	// rowsAffected = 1 means the transaction was updated
	mockDB := &bulkMockDB{rowsAffected: 1}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	transactions := []*Transaction{
		generateTestTransaction(""),
	}
	transactions[0].Status = Status{Code: "APPROVED"}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Attempted)
	assert.Equal(t, int64(1), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)
}

func TestUpdateBulk_SingleTransaction_Unchanged(t *testing.T) {
	t.Parallel()

	// rowsAffected = 0 means status already matches (no update needed)
	mockDB := &bulkMockDB{rowsAffected: 0}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	transactions := []*Transaction{
		generateTestTransaction(""),
	}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Attempted)
	assert.Equal(t, int64(0), result.Updated)
	assert.Equal(t, int64(1), result.Unchanged)
}

func TestUpdateBulk_MultipleTransactions_MixedResults(t *testing.T) {
	t.Parallel()

	// With batched update, all transactions in a chunk are updated in a single ExecContext call.
	// The rowsAffected reflects how many rows were actually updated (status changed).
	// For 3 transactions where 2 have status changes and 1 doesn't, rowsAffected = 2.
	mockDB := &bulkMockDB{rowsAffected: 2}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	transactions := generateTestTransactions(3)

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(3), result.Attempted)
	assert.Equal(t, int64(2), result.Updated)
	assert.Equal(t, int64(1), result.Unchanged)
}

func TestUpdateBulk_SortsByID(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{rowsAffected: 1}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create transactions with IDs in reverse order
	transactions := []*Transaction{
		generateTestTransaction("zzz00000-0000-0000-0000-000000000003"),
		generateTestTransaction("aaa00000-0000-0000-0000-000000000001"),
		generateTestTransaction("mmm00000-0000-0000-0000-000000000002"),
	}

	_, err := repo.UpdateBulk(ctx, transactions)
	require.NoError(t, err)

	// Verify transactions were sorted in-place
	assert.Equal(t, "aaa00000-0000-0000-0000-000000000001", transactions[0].ID)
	assert.Equal(t, "mmm00000-0000-0000-0000-000000000002", transactions[1].ID)
	assert.Equal(t, "zzz00000-0000-0000-0000-000000000003", transactions[2].ID)
}

func TestUpdateBulk_DatabaseError(t *testing.T) {
	t.Parallel()

	dbErr := errors.New("database connection lost")
	mockDB := &bulkMockDB{execErr: dbErr}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	transactions := generateTestTransactions(2)

	result, err := repo.UpdateBulk(ctx, transactions)

	require.Error(t, err)
	assert.Equal(t, dbErr, err)
	// Partial result should be returned
	assert.Equal(t, int64(2), result.Attempted)
	assert.Equal(t, int64(0), result.Updated)
}

func TestUpdateBulkTx_NilExecutor(t *testing.T) {
	t.Parallel()

	repo := &TransactionPostgreSQLRepository{
		connection: nil,
		tableName:  "transaction",
	}

	transactions := generateTestTransactions(1)

	result, err := repo.UpdateBulkTx(context.Background(), nil, transactions)

	require.Error(t, err)
	assert.Equal(t, repository.ErrNilDBExecutor, err)
	assert.Nil(t, result)
}

func TestUpdateBulkTx_EmptyInput(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{}

	repo := &TransactionPostgreSQLRepository{
		connection: nil,
		tableName:  "transaction",
	}

	result, err := repo.UpdateBulkTx(context.Background(), mockDB, []*Transaction{})

	require.NoError(t, err)
	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, int64(0), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)
}

func TestUpdateBulkTx_Success(t *testing.T) {
	t.Parallel()

	// With batched update, all transactions are updated in a single ExecContext call.
	// rowsAffected should equal the number of transactions that were actually updated.
	mockDB := &bulkMockDB{rowsAffected: 2}

	repo := &TransactionPostgreSQLRepository{
		connection: nil,
		tableName:  "transaction",
	}

	transactions := generateTestTransactions(2)

	result, err := repo.UpdateBulkTx(context.Background(), mockDB, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Attempted)
	assert.Equal(t, int64(2), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)
}

func TestUpdateBulk_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Create 1001 transactions to trigger chunking (500 + 500 + 1 = 3 chunks)
	transactions := generateTestTransactions(1001)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mockDB := &bulkMockDB{rowsAffected: 1}
	ctx = tmcore.ContextWithTenantPGConnection(ctx, mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	// When context is cancelled before any chunk is submitted,
	// Attempted reflects only rows actually submitted (0 in this case)
	assert.Equal(t, int64(0), result.Attempted)
}

func TestUpdateBulk_StatusTransition_PendingToApproved(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{rowsAffected: 1}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create transaction with status transition
	tx := generateTestTransaction("")
	tx.Status = Status{Code: "APPROVED", Description: strPtr("Transaction approved")}

	transactions := []*Transaction{tx}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Attempted)
	assert.Equal(t, int64(1), result.Updated)
}

func TestUpdateBulk_StatusTransition_PendingToCanceled(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{rowsAffected: 1}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create transaction with status transition
	tx := generateTestTransaction("")
	tx.Status = Status{Code: "CANCELED", Description: strPtr("Transaction canceled by user")}

	transactions := []*Transaction{tx}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Attempted)
	assert.Equal(t, int64(1), result.Updated)
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}

// updateBulkQueryCaptureMock captures the query and args for verification
type updateBulkQueryCaptureMock struct {
	bulkMockDB
	capturedQuery string
	capturedArgs  []any
	callCount     int
}

func (m *updateBulkQueryCaptureMock) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	m.capturedQuery = query
	m.capturedArgs = args
	m.callCount++

	return &bulkMockResult{rowsAffected: m.rowsAffected}, nil
}

func TestUpdateBulk_BatchedQuery_SingleExecPerChunk(t *testing.T) {
	t.Parallel()

	mockDB := &updateBulkQueryCaptureMock{
		bulkMockDB: bulkMockDB{rowsAffected: 5},
	}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create 5 transactions - should be processed in a single batch (under chunk size of 500)
	transactions := generateTestTransactions(5)
	for i, tx := range transactions {
		tx.Status = Status{Code: "APPROVED", Description: strPtr("Approved " + string(rune('A'+i)))}
	}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(5), result.Attempted)
	assert.Equal(t, int64(5), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)

	// Verify single ExecContext call for all 5 transactions (batched update)
	assert.Equal(t, 1, mockDB.callCount, "batched update should make exactly 1 ExecContext call for chunk")

	// Verify query uses UPDATE...FROM (VALUES...) pattern with org/ledger scoping
	assert.Contains(t, mockDB.capturedQuery, "UPDATE transaction t")
	assert.Contains(t, mockDB.capturedQuery, "FROM (VALUES")
	assert.Contains(t, mockDB.capturedQuery, "t.status != v.new_status")
	assert.Contains(t, mockDB.capturedQuery, "t.deleted_at IS NULL")
	assert.Contains(t, mockDB.capturedQuery, "t.organization_id = v.organization_id")
	assert.Contains(t, mockDB.capturedQuery, "t.ledger_id = v.ledger_id")

	// Verify parameter count: 5 transactions * 6 parameters each = 30 parameters
	// Parameters: id, organization_id, ledger_id, status, status_description, updated_at
	assert.Equal(t, 30, len(mockDB.capturedArgs), "should have 6 parameters per transaction")
}

func TestUpdateBulk_BatchedQuery_ParameterStructure(t *testing.T) {
	t.Parallel()

	mockDB := &updateBulkQueryCaptureMock{
		bulkMockDB: bulkMockDB{rowsAffected: 2},
	}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create 2 transactions with specific statuses
	tx1 := generateTestTransaction("11111111-1111-1111-1111-111111111111")
	tx1.Status = Status{Code: "APPROVED", Description: strPtr("First approved")}

	tx2 := generateTestTransaction("22222222-2222-2222-2222-222222222222")
	tx2.Status = Status{Code: "CANCELED", Description: strPtr("Second canceled")}

	transactions := []*Transaction{tx1, tx2}

	_, err := repo.UpdateBulk(ctx, transactions)
	require.NoError(t, err)

	// Verify parameters are in correct order (sorted by ID)
	// After sorting: tx1 (11111111...) comes before tx2 (22222222...)
	// Each transaction contributes: id, organization_id, ledger_id, status, status_description, updated_at
	require.Equal(t, 12, len(mockDB.capturedArgs), "should have 12 parameters for 2 transactions (6 each)")

	// First transaction parameters (indices 0-5)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", mockDB.capturedArgs[0]) // id
	assert.NotEmpty(t, mockDB.capturedArgs[1])                                      // organization_id
	assert.NotEmpty(t, mockDB.capturedArgs[2])                                      // ledger_id
	assert.Equal(t, "APPROVED", mockDB.capturedArgs[3])                             // status
	assert.NotNil(t, mockDB.capturedArgs[4])                                        // status_description pointer
	// mockDB.capturedArgs[5] is updated_at timestamp

	// Second transaction parameters (indices 6-11)
	assert.Equal(t, "22222222-2222-2222-2222-222222222222", mockDB.capturedArgs[6]) // id
	assert.NotEmpty(t, mockDB.capturedArgs[7])                                      // organization_id
	assert.NotEmpty(t, mockDB.capturedArgs[8])                                      // ledger_id
	assert.Equal(t, "CANCELED", mockDB.capturedArgs[9])                             // status
	assert.NotNil(t, mockDB.capturedArgs[10])                                       // status_description pointer
	// mockDB.capturedArgs[11] is updated_at timestamp
}

func TestUpdateBulk_BatchedQuery_MultipleChunks(t *testing.T) {
	t.Parallel()

	// Use a sequence mock to handle multiple chunks
	// With chunk size of 500 and 1001 transactions, we get 3 chunks: 500 + 500 + 1
	sequenceMock := &bulkMockDBSequence{
		resultsPerCall: []bulkMockCallResult{
			{rowsAffected: 500}, // First chunk
			{rowsAffected: 500}, // Second chunk
			{rowsAffected: 1},   // Third chunk (1 transaction)
		},
	}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), sequenceMock)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create 1001 transactions to trigger 3 chunks (500 + 500 + 1)
	transactions := generateTestTransactions(1001)

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(1001), result.Attempted)
	assert.Equal(t, int64(1001), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)

	// Verify 3 ExecContext calls (one per chunk)
	assert.Equal(t, 3, sequenceMock.callCount, "should make 3 ExecContext calls for 3 chunks")
}

func TestUpdateBulk_BatchedQuery_EmptyChunk(t *testing.T) {
	t.Parallel()

	mockDB := &updateBulkQueryCaptureMock{
		bulkMockDB: bulkMockDB{rowsAffected: 0},
	}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Empty input should not call ExecContext
	result, err := repo.UpdateBulk(ctx, []*Transaction{})

	require.NoError(t, err)
	assert.Equal(t, int64(0), result.Attempted)
	assert.Equal(t, 0, mockDB.callCount, "should not call ExecContext for empty input")
}

func TestUpdateBulk_BatchedQuery_ParameterLimitCalculation(t *testing.T) {
	t.Parallel()

	// Verify that 500 rows * 6 columns stays under PostgreSQL's 65,535 limit
	// Columns: id, organization_id, ledger_id, status, status_description, updated_at
	const chunkSize = 500
	const columnCount = 6
	const postgresLimit = 65535

	parametersPerChunk := chunkSize * columnCount
	assert.Less(t, parametersPerChunk, postgresLimit,
		"parameters per chunk (%d) should be less than PostgreSQL limit (%d)",
		parametersPerChunk, postgresLimit)
}

func TestUpdateBulk_CrossTenantIsolation(t *testing.T) {
	t.Parallel()

	// This test verifies that the bulk update WHERE clause correctly includes
	// organization_id and ledger_id, preventing cross-tenant updates.
	// When the org/ledger IDs in the VALUES don't match the database record,
	// the update should affect 0 rows (no security breach).

	// Mock returns 0 rows affected - simulating org/ledger mismatch
	mockDB := &updateBulkQueryCaptureMock{
		bulkMockDB: bulkMockDB{rowsAffected: 0}, // No rows updated due to WHERE mismatch
	}
	ctx := tmcore.ContextWithTenantPGConnection(context.Background(), mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create transaction with specific org/ledger
	tx := generateTestTransaction("")
	tx.OrganizationID = "org-attempt-cross-tenant"
	tx.LedgerID = "ledger-attempt-cross-tenant"
	tx.Status = Status{Code: "APPROVED", Description: strPtr("Attempted cross-tenant update")}

	transactions := []*Transaction{tx}

	result, err := repo.UpdateBulk(ctx, transactions)

	// Should NOT return error (query executed successfully, just matched 0 rows)
	require.NoError(t, err)

	// CRITICAL: Verify result shows 0 updates (cross-tenant blocked by WHERE clause)
	assert.Equal(t, int64(1), result.Attempted, "Should attempt 1 transaction")
	assert.Equal(t, int64(0), result.Updated, "Should update 0 rows due to org/ledger mismatch in WHERE")
	assert.Equal(t, int64(1), result.Unchanged, "Transaction should be marked as unchanged")

	// Verify the query contains the tenant isolation conditions
	assert.Contains(t, mockDB.capturedQuery, "t.organization_id = v.organization_id",
		"Query MUST include organization_id in WHERE clause for tenant isolation")
	assert.Contains(t, mockDB.capturedQuery, "t.ledger_id = v.ledger_id",
		"Query MUST include ledger_id in WHERE clause for tenant isolation")

	// Verify org/ledger IDs are in the parameters
	// Parameters order: id, organization_id, ledger_id, status, status_description, updated_at
	require.GreaterOrEqual(t, len(mockDB.capturedArgs), 6, "Should have at least 6 parameters")
	assert.Equal(t, "org-attempt-cross-tenant", mockDB.capturedArgs[1],
		"organization_id should be in parameters")
	assert.Equal(t, "ledger-attempt-cross-tenant", mockDB.capturedArgs[2],
		"ledger_id should be in parameters")
}
