// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/testutil/spantest"
)

const createBulkOperationsTxSpan = "postgres.create_bulk_operations_tx"

// Scenario: Projeção assíncrona exporta um span por operação de repositório. Two chunks
// are inserted under the single create_bulk_operations_tx span, with no chunk span.
func TestOperationCreateBulkTxSpans_ChunksOpenNoSpan(t *testing.T) {
	t.Parallel()

	const chunkSize = 1000

	operations := generateTestOperations(chunkSize + 1)
	mockDB := &mockOperationDBSequence{resultsPerCall: []mockCallResult{
		{rowsAffected: chunkSize},
		{rowsAffected: 1},
	}}

	ctx, recorder := spantest.RecordingContext()

	result, err := (&OperationPostgreSQLRepository{tableName: "operation"}).CreateBulkTx(ctx, mockDB, operations)
	require.NoError(t, err)
	require.Equal(t, int64(chunkSize+1), result.Inserted)

	spantest.AssertNoSpan(t, recorder, "postgres.insert_operation_chunk")
	assert.Equal(t, []string{createBulkOperationsTxSpan}, spantest.SpanNames(recorder))
	assert.Equal(t, codes.Unset, spantest.FindSpan(t, recorder, createBulkOperationsTxSpan).Status().Code)
}

// A failing chunk is recorded once, with the failing step, on the create_bulk_*_tx span.
func TestOperationCreateBulkTxSpans_ChunkFailureIsRecordedOnceOnTheBulkSpan(t *testing.T) {
	t.Parallel()

	dbErr := errors.New("connection refused")
	mockDB := &mockOperationDBSequence{resultsPerCall: []mockCallResult{{err: dbErr}}}

	ctx, recorder := spantest.RecordingContext()

	_, err := (&OperationPostgreSQLRepository{tableName: "operation"}).CreateBulkTx(ctx, mockDB, generateTestOperations(3))
	require.ErrorIs(t, err, dbErr)

	spantest.AssertNoSpan(t, recorder, "postgres.insert_operation_chunk")

	span := spantest.FindSpan(t, recorder, createBulkOperationsTxSpan)
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
