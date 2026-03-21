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
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

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
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

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
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

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
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

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
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

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
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

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
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

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

	// Create 2001 transactions to trigger chunking
	transactions := generateTestTransactions(2001)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mockDB := &bulkMockDB{rowsAffected: 1}
	ctx = tmcore.ContextWithModulePGConnection(ctx, "transaction", mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	result, err := repo.UpdateBulk(ctx, transactions)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	// Partial result with Attempted set
	assert.Equal(t, int64(2001), result.Attempted)
}

func TestUpdateBulk_StatusTransition_PendingToApproved(t *testing.T) {
	t.Parallel()

	mockDB := &bulkMockDB{rowsAffected: 1}
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

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
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

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
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create 5 transactions - should be processed in a single batch (under chunk size of 1000)
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

	// Verify query uses UPDATE...FROM (VALUES...) pattern
	assert.Contains(t, mockDB.capturedQuery, "UPDATE transaction t")
	assert.Contains(t, mockDB.capturedQuery, "FROM (VALUES")
	assert.Contains(t, mockDB.capturedQuery, "t.status != v.new_status")
	assert.Contains(t, mockDB.capturedQuery, "t.deleted_at IS NULL")

	// Verify parameter count: 5 transactions * 4 parameters each = 20 parameters
	assert.Equal(t, 20, len(mockDB.capturedArgs), "should have 4 parameters per transaction")
}

func TestUpdateBulk_BatchedQuery_ParameterStructure(t *testing.T) {
	t.Parallel()

	mockDB := &updateBulkQueryCaptureMock{
		bulkMockDB: bulkMockDB{rowsAffected: 2},
	}
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

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
	// Each transaction contributes: id, status, status_description, updated_at
	require.Equal(t, 8, len(mockDB.capturedArgs), "should have 8 parameters for 2 transactions")

	// First transaction parameters (indices 0-3)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", mockDB.capturedArgs[0])
	assert.Equal(t, "APPROVED", mockDB.capturedArgs[1])
	assert.NotNil(t, mockDB.capturedArgs[2]) // status_description pointer
	// mockDB.capturedArgs[3] is updated_at timestamp

	// Second transaction parameters (indices 4-7)
	assert.Equal(t, "22222222-2222-2222-2222-222222222222", mockDB.capturedArgs[4])
	assert.Equal(t, "CANCELED", mockDB.capturedArgs[5])
	assert.NotNil(t, mockDB.capturedArgs[6]) // status_description pointer
	// mockDB.capturedArgs[7] is updated_at timestamp
}

func TestUpdateBulk_BatchedQuery_MultipleChunks(t *testing.T) {
	t.Parallel()

	// Use a sequence mock to handle multiple chunks
	sequenceMock := &bulkMockDBSequence{
		resultsPerCall: []bulkMockCallResult{
			{rowsAffected: 1000}, // First chunk
			{rowsAffected: 1000}, // Second chunk
			{rowsAffected: 1},    // Third chunk (1 transaction)
		},
	}
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", sequenceMock)

	repo := &TransactionPostgreSQLRepository{
		connection:    nil,
		tableName:     "transaction",
		requireTenant: false,
	}

	// Create 2001 transactions to trigger 3 chunks (1000 + 1000 + 1)
	transactions := generateTestTransactions(2001)

	result, err := repo.UpdateBulk(ctx, transactions)

	require.NoError(t, err)
	assert.Equal(t, int64(2001), result.Attempted)
	assert.Equal(t, int64(2001), result.Updated)
	assert.Equal(t, int64(0), result.Unchanged)

	// Verify 3 ExecContext calls (one per chunk)
	assert.Equal(t, 3, sequenceMock.callCount, "should make 3 ExecContext calls for 3 chunks")
}

func TestUpdateBulk_BatchedQuery_EmptyChunk(t *testing.T) {
	t.Parallel()

	mockDB := &updateBulkQueryCaptureMock{
		bulkMockDB: bulkMockDB{rowsAffected: 0},
	}
	ctx := tmcore.ContextWithModulePGConnection(context.Background(), "transaction", mockDB)

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

	// Verify that 1000 rows * 4 columns stays under PostgreSQL's 65,535 limit
	const chunkSize = 1000
	const columnCount = 4 // id, status, status_description, updated_at
	const postgresLimit = 65535

	parametersPerChunk := chunkSize * columnCount
	assert.Less(t, parametersPerChunk, postgresLimit,
		"parameters per chunk (%d) should be less than PostgreSQL limit (%d)",
		parametersPerChunk, postgresLimit)
}
