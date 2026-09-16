// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAtomicTransactionBatchV2Route_PublishesDedicatedOrderedContract(t *testing.T) {
	t.Parallel()

	doc := registerIsolatedV2TransactionContractForTest()
	pathItem, ok := doc.Paths[v2AtomicTransactionBatchPath]
	require.True(t, ok, "the v2 contract must publish the dedicated atomic batch path")
	require.NotNil(t, pathItem.Post, "the atomic batch path must carry a POST operation")

	op := pathItem.Post
	assert.Equal(t, v2AtomicTransactionBatchOperationID, op.OperationID)
	assert.Contains(t, op.Description, "explicit increasing order")
	assert.Contains(t, op.Description, "all-or-none")
	assert.Contains(t, op.Description, "100 postings and 150 balance snapshots")
	assert.Contains(t, op.Description, "no query endpoint")
	assert.EqualValues(t, v2CreateMaxBodyBytes, op.MaxBodyBytes,
		"the batch must use the same decoded-body ceiling as singular v2 creates")
	assert.Equal(t, secTransactionBearer, op.Security,
		"the batch must publish the same bearer security contract as singular transaction creates")

	require.NotNil(t, op.RequestBody)
	assert.True(t, op.RequestBody.Required)
	requestMedia, ok := op.RequestBody.Content[v2CreateBodyContentType]
	require.True(t, ok)
	require.NotNil(t, requestMedia)
	require.NotNil(t, requestMedia.Schema)
	assert.Equal(t, "#/components/schemas/"+v2AtomicTransactionBatchRequestSchemaName, requestMedia.Schema.Ref)

	requestSchema := doc.Components.Schemas.SchemaFromRef(requestMedia.Schema.Ref)
	require.NotNil(t, requestSchema)
	assert.Equal(t, []string{"transactions"}, requestSchema.Required)

	transactions := requestSchema.Properties["transactions"]
	require.NotNil(t, transactions)
	assert.Equal(t, "array", transactions.Type)
	require.NotNil(t, transactions.MinItems)
	require.NotNil(t, transactions.MaxItems)
	assert.Equal(t, 1, *transactions.MinItems)
	assert.Equal(t, atomicTransactionBatchV2AbsoluteMaxSize, *transactions.MaxItems)
	require.NotNil(t, transactions.Items)
	assert.Equal(t, "#/components/schemas/CreateAtomicTransactionBatchV2ItemRequest", transactions.Items.Ref)

	created := op.Responses[createdResponseStatus]
	require.NotNil(t, created)
	responseMedia, ok := created.Content[v2CreateBodyContentType]
	require.True(t, ok)
	require.NotNil(t, responseMedia)
	require.NotNil(t, responseMedia.Schema)
	assert.Equal(t, "#/components/schemas/"+v2AtomicTransactionBatchResponseSchemaName, responseMedia.Schema.Ref)

	responseSchema := doc.Components.Schemas.SchemaFromRef(responseMedia.Schema.Ref)
	require.NotNil(t, responseSchema)
	assert.ElementsMatch(t, []string{"batchId", "transactions"}, responseSchema.Required)
	assert.Equal(t, "uuid", responseSchema.Properties["batchId"].Format)

	responseTransactions := responseSchema.Properties["transactions"]
	require.NotNil(t, responseTransactions)
	assert.Equal(t, "array", responseTransactions.Type)
	require.NotNil(t, responseTransactions.Items)
	assert.Equal(t, "#/components/schemas/AtomicTransactionBatchV2Transaction", responseTransactions.Items.Ref)
	assert.Contains(t, responseTransactions.Description, "increasing logical order")
}

func TestRegisterAtomicTransactionBatchV2Route_DoesNotChangeExistingOperationIDs(t *testing.T) {
	t.Parallel()

	doc := registerIsolatedV2TransactionContractForTest()

	want := map[string]string{
		"/transactions/direct":  "createTransactionDirectV2",
		"/transactions/hold":    "createTransactionHoldV2",
		"/transactions/block":   "createTransactionBlockV2",
		"/transactions/unblock": "createTransactionUnblockV2",
		"/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit": "commitTransactionV2",
		"/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/cancel": "cancelTransactionV2",
		"/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert": "revertTransactionV2",
	}

	for path, operationID := range want {
		pathItem, ok := doc.Paths[path]
		require.Truef(t, ok, "existing path %s must remain published", path)
		require.NotNilf(t, pathItem.Post, "existing path %s must remain a POST", path)
		assert.Equalf(t, operationID, pathItem.Post.OperationID,
			"registering the batch must not rename the existing operation at %s", path)
	}
}

const createdResponseStatus = "201"
