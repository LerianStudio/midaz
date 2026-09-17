// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeAndValidateRevisedAtomicTransactionBatchV2_CanonicalizesPhysicalPermutations(t *testing.T) {
	t.Parallel()

	first := validAtomicBatchV2Request("@first-source", "@first-destination", batchTestLedgerID)
	first.Metadata = map[string]any{"large": 9007199254740993, "fraction": 0.125}
	second := validAtomicBatchV2Request("@second-source", "@second-destination", batchTestLedgerID)

	firstItem := revisedAtomicBatchV2Item(t, first, "direct", 1)
	secondItem := revisedAtomicBatchV2Item(t, second, "hold", 2)

	physicalOrder, err := decodeAndValidateRevisedAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, secondItem, firstItem), 50)
	require.NoError(t, err)
	logicalOrder, err := decodeAndValidateRevisedAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, firstItem, secondItem), 50)
	require.NoError(t, err)

	assert.Equal(t, logicalOrder.canonicalRequest, physicalOrder.canonicalRequest)
	assert.Equal(t, logicalOrder.requestFingerprint, physicalOrder.requestFingerprint)
	assert.Equal(t, "midaz.atomic-transaction-batch.request.v2\x00", revisedAtomicTransactionBatchV2RequestFingerprintDomain)
}

func TestDecodeAndValidateRevisedAtomicTransactionBatchV2_CanonicalIdentityIncludesItemSemantics(t *testing.T) {
	t.Parallel()

	first := validAtomicBatchV2Request("@first-source", "@first-destination", batchTestLedgerID)
	first.Debits = append(first.Debits, validAtomicBatchV2Leg("@first-extra", batchTestLedgerID))
	second := validAtomicBatchV2Request("@second-source", "@second-destination", batchTestLedgerID)
	second.Credits = append(second.Credits, validAtomicBatchV2Leg("@second-extra", batchTestLedgerID))

	baseline := revisedAtomicBatchV2Decode(
		t,
		revisedAtomicBatchV2Item(t, first, "direct", 1),
		revisedAtomicBatchV2Item(t, second, "hold", 2),
	)

	changedAction := revisedAtomicBatchV2Decode(
		t,
		revisedAtomicBatchV2Item(t, first, "hold", 1),
		revisedAtomicBatchV2Item(t, second, "hold", 2),
	)
	changedOrderAssociation := revisedAtomicBatchV2Decode(
		t,
		revisedAtomicBatchV2Item(t, first, "direct", 2),
		revisedAtomicBatchV2Item(t, second, "hold", 1),
	)
	changedContent := first
	changedContent.Amount = "2"
	changedDebitOrder := first
	changedDebitOrder.Debits = slices.Clone(first.Debits)
	changedDebitOrder.Debits[0], changedDebitOrder.Debits[1] = changedDebitOrder.Debits[1], changedDebitOrder.Debits[0]
	changedCreditOrder := second
	changedCreditOrder.Credits = slices.Clone(second.Credits)
	changedCreditOrder.Credits[0], changedCreditOrder.Credits[1] = changedCreditOrder.Credits[1], changedCreditOrder.Credits[0]

	assert.NotEqual(t, baseline.requestFingerprint, changedAction.requestFingerprint)
	assert.NotEqual(t, baseline.requestFingerprint, changedOrderAssociation.requestFingerprint)
	assert.NotEqual(t, baseline.requestFingerprint, revisedAtomicBatchV2Decode(
		t,
		revisedAtomicBatchV2Item(t, changedContent, "direct", 1),
		revisedAtomicBatchV2Item(t, second, "hold", 2),
	).requestFingerprint)
	assert.NotEqual(t, baseline.requestFingerprint, revisedAtomicBatchV2Decode(
		t,
		revisedAtomicBatchV2Item(t, changedDebitOrder, "direct", 1),
		revisedAtomicBatchV2Item(t, second, "hold", 2),
	).requestFingerprint)
	assert.NotEqual(t, baseline.requestFingerprint, revisedAtomicBatchV2Decode(
		t,
		revisedAtomicBatchV2Item(t, first, "direct", 1),
		revisedAtomicBatchV2Item(t, changedCreditOrder, "hold", 2),
	).requestFingerprint)
}

func TestDecodeAndValidateRevisedAtomicTransactionBatchV2_RejectsInvalidRequestBeforeCanonicalization(t *testing.T) {
	t.Parallel()

	request := validAtomicBatchV2Request("@source", "@destination", batchTestLedgerID)
	item := revisedAtomicBatchV2ItemWithout(t, request, "order", nil)

	result, err := decodeAndValidateRevisedAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, item), 50)
	require.Error(t, err)
	assert.Empty(t, result.canonicalRequest)
	assert.Empty(t, result.requestFingerprint)
}

func revisedAtomicBatchV2Decode(t *testing.T, items ...json.RawMessage) revisedDecodedAtomicTransactionBatchV2 {
	t.Helper()

	result, err := decodeAndValidateRevisedAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, items...), 50)
	require.NoError(t, err)

	return result
}
