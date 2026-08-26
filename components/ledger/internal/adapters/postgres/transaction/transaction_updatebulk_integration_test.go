//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// =============================================================================
// INTEGRATION TESTS - UpdateBulk
// =============================================================================
//
// These cover what the mock-based UpdateBulk suite in transaction_bulk_test.go
// structurally cannot: the result triple is decided by the DATABASE. Attempted is
// the repository's own count, but Updated is RowsAffected from a statement whose
// WHERE clause carries "t.status != v.new_status", so Unchanged — Attempted minus
// Updated — is a value only a real server produces. A mock returns whatever
// rowsAffected the test handed it, which makes every accounting assertion over it
// a restatement of the fixture.
//
// The infrastructure comes from setupBulkTestInfra in
// transaction_createbulk_integration_test.go; seeding goes through CreateBulk
// because rows must exist before they can be updated.

// seedForUpdate inserts count transactions at PENDING and returns them, so an
// UpdateBulk case starts from rows that are known to be present and known to carry a
// status different from the one it will write.
func (infra *bulkTestInfra) seedForUpdate(t *testing.T, ctx context.Context, count int) []*Transaction {
	t.Helper()

	transactions := infra.createBulkTestTransactions(t, count)

	result, err := infra.repo.CreateBulk(ctx, transactions)
	require.NoError(t, err, "seeding via CreateBulk must succeed")
	require.Equal(t, int64(count), result.Inserted, "every seeded row must be inserted")

	return transactions
}

// restatus returns a copy of transactions carrying the given status code, which is
// what UpdateBulk writes. The seeded rows are left untouched so a case can assert
// against the values it seeded.
func restatus(transactions []*Transaction, code string) []*Transaction {
	updates := make([]*Transaction, len(transactions))

	for i, tx := range transactions {
		clone := *tx
		clone.Status = Status{Code: code}
		updates[i] = &clone
	}

	return updates
}

// TestIntegration_TransactionUpdateBulk_ChunkBoundary updates exactly one full chunk.
// The boundary is the interesting size because updateBulkInternal's loop advances by
// updateBulkChunkSize and its end index is min(i+size, len): an off-by-one there
// either drops the last row of the chunk or issues a second empty statement, and both
// show up in the result triple.
func TestIntegration_TransactionUpdateBulk_ChunkBoundary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	seeded := infra.seedForUpdate(t, ctx, updateBulkChunkSize)

	result, err := infra.repo.UpdateBulk(ctx, restatus(seeded, cn.APPROVED))

	require.NoError(t, err, "UpdateBulk must handle exactly one full chunk")
	require.NotNil(t, result)

	assert.Equal(t, int64(updateBulkChunkSize), result.Attempted)
	assert.Equal(t, int64(updateBulkChunkSize), result.Updated,
		"every row carried a different status, so every row must be affected")
	assert.Equal(t, int64(0), result.Unchanged)

	// The count is the database's; confirm it also wrote what it counted, at both
	// ends of the chunk.
	for _, idx := range []int{0, updateBulkChunkSize - 1} {
		found, findErr := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, uuid.MustParse(seeded[idx].ID))
		require.NoErrorf(t, findErr, "transaction at index %d must be findable", idx)
		assert.Equalf(t, cn.APPROVED, found.Status.Code, "transaction at index %d must carry the new status", idx)
	}
}

// TestIntegration_TransactionUpdateBulk_MultipleChunks updates across three chunks,
// the smallest count that exercises a first chunk, a middle one and a short tail.
func TestIntegration_TransactionUpdateBulk_MultipleChunks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	const count = updateBulkChunkSize*2 + 1

	seeded := infra.seedForUpdate(t, ctx, count)

	result, err := infra.repo.UpdateBulk(ctx, restatus(seeded, cn.APPROVED))

	require.NoError(t, err, "UpdateBulk must handle chunking without error")
	require.NotNil(t, result)

	assert.Equal(t, int64(count), result.Attempted, "every input must be attempted across chunks")
	assert.Equal(t, int64(count), result.Updated, "chunk results must accumulate, not overwrite")
	assert.Equal(t, int64(0), result.Unchanged)

	// Spot check either side of both chunk boundaries and the tail. UpdateBulk sorts
	// its input by ID, so the seeded order is not the applied order — reading each row
	// back by ID is what makes the check independent of that.
	checkIndices := []int{
		0,
		updateBulkChunkSize - 1,
		updateBulkChunkSize,
		updateBulkChunkSize*2 - 1,
		updateBulkChunkSize * 2,
		count - 1,
	}

	for _, idx := range checkIndices {
		found, findErr := infra.repo.Find(ctx, infra.orgID, infra.ledgerID, uuid.MustParse(seeded[idx].ID))
		require.NoErrorf(t, findErr, "transaction at index %d must be findable", idx)
		assert.Equalf(t, cn.APPROVED, found.Status.Code, "transaction at index %d must carry the new status", idx)
	}
}

// TestIntegration_TransactionUpdateBulk_UnchangedRowsAreNotCounted pins the half of
// the result triple the mock suite cannot reach. updateTransactionChunk's WHERE
// carries "t.status != v.new_status", so a row already at the target status is
// matched and skipped by the server: it lands in Unchanged, not Updated. The mixed
// case additionally proves the two counts are derived per statement rather than from
// the submitted length.
func TestIntegration_TransactionUpdateBulk_UnchangedRowsAreNotCounted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupBulkTestInfra(t)
	ctx := context.Background()

	const count = 100

	seeded := infra.seedForUpdate(t, ctx, count)

	// Rewriting PENDING over PENDING matches every row and changes none.
	noop, err := infra.repo.UpdateBulk(ctx, restatus(seeded, cn.PENDING))
	require.NoError(t, err)
	require.NotNil(t, noop)

	assert.Equal(t, int64(count), noop.Attempted)
	assert.Equal(t, int64(0), noop.Updated, "a row already at the target status must not be counted as updated")
	assert.Equal(t, int64(count), noop.Unchanged)

	// Half to APPROVED, half left at PENDING, submitted as one batch.
	const changed = count / 2

	updates := append(
		restatus(seeded[:changed], cn.APPROVED),
		restatus(seeded[changed:], cn.PENDING)...,
	)

	mixed, err := infra.repo.UpdateBulk(ctx, updates)
	require.NoError(t, err)
	require.NotNil(t, mixed)

	assert.Equal(t, int64(count), mixed.Attempted)
	assert.Equal(t, int64(changed), mixed.Updated)
	assert.Equal(t, int64(count-changed), mixed.Unchanged)

	assert.Equal(t, mixed.Attempted, mixed.Updated+mixed.Unchanged,
		"Attempted must equal Updated + Unchanged")
}
