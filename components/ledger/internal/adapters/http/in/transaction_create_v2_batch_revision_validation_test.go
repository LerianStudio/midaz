// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestDecodeAndValidateRevisedAtomicTransactionBatchV2_OrdersItemsLogically(t *testing.T) {
	t.Parallel()

	third := revisedAtomicBatchV2Item(t, validAtomicBatchV2Request("@third-source", "@third-destination", batchTestLedgerID), "direct", 3)
	first := revisedAtomicBatchV2Item(t, validAtomicBatchV2Request("@first-source", "@first-destination", batchTestLedgerID), "hold", 1)
	second := revisedAtomicBatchV2Item(t, validAtomicBatchV2Request("@second-source", "@second-destination", batchTestLedgerID), "direct", 2)

	result, err := decodeAndValidateRevisedAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, third, first, second), 50)
	require.NoError(t, err)
	require.Len(t, result.items, 3)
	assert.Equal(t, []int{1, 2, 3}, []int{result.items[0].order, result.items[1].order, result.items[2].order})
	assert.Equal(t, []int{1, 2, 0}, []int{result.items[0].originalIndex, result.items[1].originalIndex, result.items[2].originalIndex})
	assert.Equal(t, []atomicTransactionBatchV2Action{
		atomicTransactionBatchV2ActionHold,
		atomicTransactionBatchV2ActionDirect,
		atomicTransactionBatchV2ActionDirect,
	}, []atomicTransactionBatchV2Action{result.items[0].action, result.items[1].action, result.items[2].action})
}

func TestDecodeAndValidateRevisedAtomicTransactionBatchV2_PreservesBalanceKeyPerLeg(t *testing.T) {
	t.Parallel()

	request := validAtomicBatchV2Request("@source", "@destination", batchTestLedgerID)
	request.Debits[0].BalanceKey = "food"
	request.Credits[0].BalanceKey = "settlement"
	item := revisedAtomicBatchV2Item(t, request, "direct", 1)

	result, err := decodeAndValidateRevisedAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, item), 50)
	require.NoError(t, err)
	require.Len(t, result.items, 1)
	require.Len(t, result.items[0].normalized.transaction.Send.Source.From, 1)
	require.Len(t, result.items[0].normalized.transaction.Send.Distribute.To, 1)
	assert.Equal(t, "food", result.items[0].normalized.transaction.Send.Source.From[0].BalanceKey)
	assert.Equal(t, "settlement", result.items[0].normalized.transaction.Send.Distribute.To[0].BalanceKey)
}

func TestDecodeAndValidateRevisedAtomicTransactionBatchV2_AggregatesStructuralErrors(t *testing.T) {
	t.Parallel()

	duplicateOne := revisedAtomicBatchV2Item(t, validAtomicBatchV2Request("@first-source", "@first-destination", batchTestLedgerID), "direct", 1)
	missingAction := revisedAtomicBatchV2ItemWithout(t, validAtomicBatchV2Request("@second-source", "@second-destination", batchTestLedgerID), "action", 2)
	duplicateTwo := revisedAtomicBatchV2Item(t, validAtomicBatchV2Request("@third-source", "@third-destination", batchTestLedgerID), "hold", 1)
	invalidOrder := revisedAtomicBatchV2Item(t, validAtomicBatchV2Request("@fourth-source", "@fourth-destination", batchTestLedgerID), "direct", 1.5)

	_, err := decodeAndValidateRevisedAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, duplicateOne, missingAction, duplicateTwo, invalidOrder), 50)
	detail := assertAtomicBatchV2Problem(t, err, constant.ErrTransactionBatchStructuralValidation.Error())
	assert.Equal(t, "Invalid Transaction Batch", detail.Title)
	assert.Equal(t, "One or more transactions in the batch failed structural validation. Check errors for details.", detail.ErrorModel.Detail)
	assert.Equal(t, []string{
		"body.transactions[0].order",
		"body.transactions[1].action",
		"body.transactions[2].order",
		"body.transactions[3].order",
	}, atomicBatchV2ProblemLocations(detail))
	assert.Contains(t, detail.Errors[0].Message, "Transaction order 1:")
	assert.Contains(t, detail.Errors[1].Message, "Transaction order 2:")
	assert.Contains(t, detail.Errors[3].Message, "Transaction at index 3:")
}

func TestDecodeAndValidateRevisedAtomicTransactionBatchV2_RejectsInvalidActionAndOrderValues(t *testing.T) {
	t.Parallel()

	request := validAtomicBatchV2Request("@source", "@destination", batchTestLedgerID)
	tests := []struct {
		name            string
		action          any
		order           any
		wantLocation    string
		messageFragment string
	}{
		{name: "unsupported action", action: "commit", order: 1, wantLocation: "body.transactions[0].action", messageFragment: "Transaction order 1:"},
		{name: "null action", action: nil, order: 1, wantLocation: "body.transactions[0].action", messageFragment: "Transaction order 1:"},
		{name: "string order", action: "direct", order: "1", wantLocation: "body.transactions[0].order", messageFragment: "Transaction at index 0:"},
		{name: "null order", action: "direct", order: nil, wantLocation: "body.transactions[0].order", messageFragment: "Transaction at index 0:"},
		{name: "fractional order", action: "direct", order: 1.5, wantLocation: "body.transactions[0].order", messageFragment: "Transaction at index 0:"},
		{name: "zero order", action: "direct", order: 0, wantLocation: "body.transactions[0].order", messageFragment: "Transaction at index 0:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			item := revisedAtomicBatchV2ItemWithAction(t, request, tt.action, tt.order)
			_, err := decodeAndValidateRevisedAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, item), 50)
			detail := assertAtomicBatchV2Problem(t, err, constant.ErrTransactionBatchStructuralValidation.Error())
			require.NotEmpty(t, detail.Errors)
			assert.Equal(t, tt.wantLocation, detail.Errors[0].Location)
			assert.Contains(t, detail.Errors[0].Message, tt.messageFragment)
		})
	}
}

func TestDecodeAndValidateRevisedAtomicTransactionBatchV2_RejectsOrderGapAtOriginalLocation(t *testing.T) {
	t.Parallel()

	first := revisedAtomicBatchV2Item(t, validAtomicBatchV2Request("@first-source", "@first-destination", batchTestLedgerID), "direct", 1)
	second := revisedAtomicBatchV2Item(t, validAtomicBatchV2Request("@second-source", "@second-destination", batchTestLedgerID), "direct", 2)
	fourth := revisedAtomicBatchV2Item(t, validAtomicBatchV2Request("@fourth-source", "@fourth-destination", batchTestLedgerID), "direct", 4)

	_, err := decodeAndValidateRevisedAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, first, second, fourth), 50)
	detail := assertAtomicBatchV2Problem(t, err, constant.ErrTransactionBatchStructuralValidation.Error())
	require.Len(t, detail.Errors, 1)
	assert.Equal(t, "body.transactions[2].order", detail.Errors[0].Location)
	assert.Contains(t, detail.Errors[0].Message, "Transaction at index 2:")
}

func TestDecodeAndValidateRevisedAtomicTransactionBatchV2_PreservesOriginalPathAndOrdersDiagnostics(t *testing.T) {
	t.Parallel()

	second := revisedAtomicBatchV2Item(t, validAtomicBatchV2Request("@second-source", "@second-destination", batchTestLedgerID), "direct", 2)
	firstRequest := validAtomicBatchV2Request("@first-source", "@first-destination", batchTestLedgerID)
	firstRequest.Amount = "0"
	first := revisedAtomicBatchV2Item(t, firstRequest, "hold", 1)

	_, err := decodeAndValidateRevisedAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, second, first), 50)
	detail := assertAtomicBatchV2Problem(t, err, constant.ErrTransactionBatchStructuralValidation.Error())
	require.NotEmpty(t, detail.Errors)
	assert.Equal(t, "body.transactions[1].amount", detail.Errors[0].Location)
	assert.Contains(t, detail.Errors[0].Message, "Transaction order 1:")
}

func TestDecodeAndValidateRevisedAtomicTransactionBatchV2_TruncatesDiagnostics(t *testing.T) {
	t.Parallel()

	item := revisedAtomicBatchV2Item(t, validAtomicBatchV2Request("@source", "@destination", batchTestLedgerID), "direct", 1)
	var document map[string]any
	require.NoError(t, json.Unmarshal(item, &document))
	for index := 0; index <= pkg.MaxFieldErrors; index++ {
		document[fmt.Sprintf("unexpected%03d", index)] = "value"
	}
	raw, err := json.Marshal(document)
	require.NoError(t, err)

	_, err = decodeAndValidateRevisedAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, raw), 50)
	detail := assertAtomicBatchV2Problem(t, err, constant.ErrTransactionBatchStructuralValidation.Error())
	require.Len(t, detail.Errors, pkg.MaxFieldErrors)
	assert.Equal(t, pkg.FieldErrorTruncationLocation, detail.Errors[pkg.MaxFieldErrors-1].Location)
	assert.Equal(t, pkg.FieldErrorTruncationMessage, detail.Errors[pkg.MaxFieldErrors-1].Message)
}

func revisedAtomicBatchV2Item(t *testing.T, request CreateTransactionV2Request, action string, order any) json.RawMessage {
	t.Helper()

	return revisedAtomicBatchV2ItemWithAction(t, request, action, order)
}

func revisedAtomicBatchV2ItemWithAction(t *testing.T, request CreateTransactionV2Request, action, order any) json.RawMessage {
	t.Helper()

	var document map[string]any
	require.NoError(t, json.Unmarshal(mustMarshalAtomicBatchV2Item(t, request), &document))
	document["action"] = action
	document["order"] = order

	raw, err := json.Marshal(document)
	require.NoError(t, err)

	return raw
}

func revisedAtomicBatchV2ItemWithout(t *testing.T, request CreateTransactionV2Request, field string, order any) json.RawMessage {
	t.Helper()

	var document map[string]any
	require.NoError(t, json.Unmarshal(mustMarshalAtomicBatchV2Item(t, request), &document))
	document["order"] = order
	delete(document, field)

	raw, err := json.Marshal(document)
	require.NoError(t, err)

	return raw
}
