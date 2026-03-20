// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
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

	// Create mock that returns different results per call
	mockDB := &bulkMockDBSequence{
		resultsPerCall: []bulkMockCallResult{
			{rowsAffected: 1}, // First transaction updated
			{rowsAffected: 0}, // Second transaction unchanged
			{rowsAffected: 1}, // Third transaction updated
		},
	}
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

	mockDB := &bulkMockDB{rowsAffected: 1}

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
