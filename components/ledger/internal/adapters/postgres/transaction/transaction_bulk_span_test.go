// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
)

const createBulkTransactionsTxSpan = "postgres.create_bulk_transactions_tx"

// Scenario: Projeção assíncrona exporta um span por operação de repositório. Two chunks
// are inserted under the single create_bulk_transactions_tx span, with no chunk span.
func TestCreateBulkTxSpans_ChunksOpenNoSpan(t *testing.T) {
	t.Parallel()

	transactions := generateTestTransactions(createBulkChunkSize + 1)
	mockDB := &bulkMockDBSequence{resultsPerCall: []bulkMockCallResult{
		{rowsAffected: int64(createBulkChunkSize)},
		{rowsAffected: 1},
	}}

	ctx, recorder := spantest.RecordingContext()

	result, err := (&TransactionPostgreSQLRepository{tableName: "transaction"}).CreateBulkTx(ctx, mockDB, transactions)
	require.NoError(t, err)
	require.Equal(t, int64(createBulkChunkSize+1), result.Inserted)

	spantest.AssertNoSpan(t, recorder, "postgres.insert_transaction_chunk")
	assert.Equal(t, []string{createBulkTransactionsTxSpan}, spantest.SpanNames(recorder))
	assert.Equal(t, codes.Unset, spantest.FindSpan(t, recorder, createBulkTransactionsTxSpan).Status().Code)
}

// A failing chunk is recorded once, with the failing step, on the create_bulk_*_tx span.
func TestCreateBulkTxSpans_ChunkFailureIsRecordedOnceOnTheBulkSpan(t *testing.T) {
	t.Parallel()

	dbErr := errors.New("connection refused")
	mockDB := &bulkMockDBSequence{resultsPerCall: []bulkMockCallResult{{err: dbErr}}}

	ctx, recorder := spantest.RecordingContext()

	_, err := (&TransactionPostgreSQLRepository{tableName: "transaction"}).CreateBulkTx(ctx, mockDB, generateTestTransactions(3))
	require.ErrorIs(t, err, dbErr)

	spantest.AssertNoSpan(t, recorder, "postgres.insert_transaction_chunk")

	span := spantest.FindSpan(t, recorder, createBulkTransactionsTxSpan)
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Contains(t, span.Status().Description, "Failed to execute bulk insert")

	exceptions := 0

	for _, event := range span.Events() {
		if event.Name == "exception" {
			exceptions++
		}
	}

	assert.Equal(t, 1, exceptions, "the chunk failure must be recorded exactly once")
}
