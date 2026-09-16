// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

const (
	batchTestOrganizationID = "00000000-0000-0000-0000-000000000001"
	batchTestLedgerID       = "00000000-0000-0000-0000-000000000002"
	batchTestOtherLedgerID  = "00000000-0000-0000-0000-000000000003"
	batchTestExceptionID    = "00000000-0000-0000-0000-000000000004"
)

func TestDecodeAtomicTransactionBatchV2Wrapper_RequestWideFailures(t *testing.T) {
	t.Parallel()

	validItem := mustMarshalAtomicBatchV2Item(t, validAtomicBatchV2Request("@source", "@destination", batchTestLedgerID))

	tests := []struct {
		name     string
		body     []byte
		maxSize  int
		wantCode error
	}{
		{name: "malformed JSON", body: []byte(`{"transactions":[`), maxSize: 50, wantCode: constant.ErrInvalidRequestBody},
		{name: "non-object wrapper", body: []byte(`[]`), maxSize: 50, wantCode: constant.ErrInvalidRequestBody},
		{name: "null wrapper", body: []byte(`null`), maxSize: 50, wantCode: constant.ErrInvalidRequestBody},
		{name: "missing transactions", body: []byte(`{}`), maxSize: 50, wantCode: constant.ErrTransactionBatchCardinality},
		{name: "null transactions", body: []byte(`{"transactions":null}`), maxSize: 50, wantCode: constant.ErrTransactionBatchCardinality},
		{name: "empty transactions", body: []byte(`{"transactions":[]}`), maxSize: 50, wantCode: constant.ErrTransactionBatchCardinality},
		{name: "transactions has wrong type", body: []byte(`{"transactions":{}}`), maxSize: 50, wantCode: constant.ErrInvalidRequestBody},
		{
			name:     "strict unknown field rejects false zero value",
			body:     []byte(fmt.Sprintf(`{"transactions":[%s],"extra":false}`, validItem)),
			maxSize:  50,
			wantCode: constant.ErrUnexpectedFieldsInTheRequest,
		},
		{
			name:     "configured effective maximum",
			body:     marshalAtomicBatchV2Wrapper(t, validItem, validItem),
			maxSize:  1,
			wantCode: constant.ErrTransactionBatchCardinality,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := decodeAndValidateAtomicTransactionBatchV2(tt.body, tt.maxSize)
			assertAtomicBatchV2Problem(t, err, tt.wantCode.Error())
		})
	}
}

func TestDecodeAtomicTransactionBatchV2Wrapper_EnforcesAbsoluteMaximum(t *testing.T) {
	t.Parallel()

	items := make([]json.RawMessage, atomicTransactionBatchV2AbsoluteMaxSize+1)
	for index := range items {
		items[index] = json.RawMessage(`{}`)
	}

	_, err := decodeAndValidateAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, items...), 51)
	detail := assertAtomicBatchV2Problem(t, err, constant.ErrTransactionBatchCardinality.Error())
	assert.Contains(t, detail.ErrorModel.Detail, "51 items")
	assert.Contains(t, detail.ErrorModel.Detail, "1 and 50 items")
}

func TestDecodeAndValidateAtomicTransactionBatchV2_InputLegLimit(t *testing.T) {
	t.Parallel()

	request := validAtomicBatchV2Request("@source", "@destination", batchTestLedgerID)
	request.Debits = repeatedAtomicBatchV2Legs(500, "@source", batchTestLedgerID)
	request.Credits = repeatedAtomicBatchV2Legs(500, "@destination", batchTestLedgerID)
	exactLimit := mustMarshalAtomicBatchV2Item(t, request)

	result, err := decodeAndValidateAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, exactLimit), 50)
	require.NoError(t, err)
	assert.Equal(t, atomicTransactionBatchV2InputLegLimit, result.inputLegCount)

	request.Credits = append(request.Credits, validAtomicBatchV2Leg("@one-too-many", batchTestLedgerID))
	overLimit := mustMarshalAtomicBatchV2Item(t, request)

	_, err = decodeAndValidateAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, overLimit), 50)
	detail := assertAtomicBatchV2Problem(t, err, constant.ErrTransactionBatchInputLegsLimitExceeded.Error())
	assert.Contains(t, detail.ErrorModel.Detail, "1001 input debit and credit legs")
}

func TestDecodeAndValidateAtomicTransactionBatchV2_PreservesItemOrder(t *testing.T) {
	t.Parallel()

	first := mustMarshalAtomicBatchV2Item(t, validAtomicBatchV2Request("@first-source", "@first-destination", batchTestLedgerID))
	second := mustMarshalAtomicBatchV2Item(t, validAtomicBatchV2Request("@second-source", "@second-destination", batchTestLedgerID))

	result, err := decodeAndValidateAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, first, second), 50)
	require.NoError(t, err)
	require.Len(t, result.items, 2)
	assert.Equal(t, "@first-source", result.items[0].request.Debits[0].Alias)
	assert.Equal(t, "@second-source", result.items[1].request.Debits[0].Alias)
	assert.Equal(t, batchTestOrganizationID, result.scope.OrganizationID)
	assert.Equal(t, batchTestLedgerID, result.scope.LedgerID)
	assert.Equal(t, 4, result.inputLegCount)
}

func TestDecodeAndValidateAtomicTransactionBatchV2_AggregatesOrderedItemDiagnostics(t *testing.T) {
	t.Parallel()

	first := []byte(`{
		"asset":"USD",
		"amount":"0",
		"debits":[{"alias":"bad alias","organizationId":"00000000-0000-0000-0000-000000000001","ledgerId":"00000000-0000-0000-0000-000000000002","amount":"-1"}],
		"credits":[{"alias":"@destination","organizationId":"00000000-0000-0000-0000-000000000001","ledgerId":"00000000-0000-0000-0000-000000000002","amount":"1"}]
	}`)
	second := []byte(`{
		"amount":"1",
		"zeta":"unexpected",
		"alpha":"unexpected",
		"debits":[{"alias":"@source","organizationId":"00000000-0000-0000-0000-000000000001","ledgerId":"00000000-0000-0000-0000-000000000002","amount":"1"}],
		"credits":[{"alias":"@destination","organizationId":"00000000-0000-0000-0000-000000000001","ledgerId":"00000000-0000-0000-0000-000000000002","amount":"1"}]
	}`)
	third := []byte(`{
		"asset":"USD",
		"amount":"1",
		"debits":[{"alias":123,"organizationId":"00000000-0000-0000-0000-000000000001","ledgerId":"00000000-0000-0000-0000-000000000002","amount":"1"}],
		"credits":[{"alias":"@destination","organizationId":"00000000-0000-0000-0000-000000000001","ledgerId":"00000000-0000-0000-0000-000000000002","amount":"1"}]
	}`)

	_, err := decodeAndValidateAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, first, second, third), 50)
	detail := assertAtomicBatchV2Problem(t, err, constant.ErrInvalidTransactionNonPositiveValue.Error())
	assert.Equal(t, http.StatusUnprocessableEntity, detail.Status)

	locations := atomicBatchV2ProblemLocations(detail)
	assert.Equal(t, []string{
		"body.transactions[0].amount",
		"body.transactions[0].debits[0].alias",
		"body.transactions[0].debits[0].amount",
		"body.transactions[1].alpha",
		"body.transactions[1].asset",
		"body.transactions[1].zeta",
		"body.transactions[2].debits[0].alias",
	}, locations)
}

func TestDecodeAndValidateAtomicTransactionBatchV2_ReportsMalformedItemLocation(t *testing.T) {
	t.Parallel()

	malformedItem := []byte(`{
		"asset":"USD",
		"amount":"1",
		"debits":[{"alias":123,"organizationId":"00000000-0000-0000-0000-000000000001","ledgerId":"00000000-0000-0000-0000-000000000002","amount":"1"}],
		"credits":[{"alias":"@destination","organizationId":"00000000-0000-0000-0000-000000000001","ledgerId":"00000000-0000-0000-0000-000000000002","amount":"1"}]
	}`)

	_, err := decodeAndValidateAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, malformedItem), 50)
	detail := assertAtomicBatchV2Problem(t, err, constant.ErrInvalidRequestBody.Error())
	assert.Equal(t, http.StatusBadRequest, detail.Status)
	assert.Equal(t, []string{
		"body.transactions[0].debits[0].alias",
	}, atomicBatchV2ProblemLocations(detail))
}

func TestDecodeAndValidateAtomicTransactionBatchV2_ReportsFirstSharedScopeDifference(t *testing.T) {
	t.Parallel()

	first := mustMarshalAtomicBatchV2Item(t, validAtomicBatchV2Request("@source", "@destination", batchTestLedgerID))
	second := mustMarshalAtomicBatchV2Item(t, validAtomicBatchV2Request("@source-two", "@destination-two", batchTestOtherLedgerID))
	third := mustMarshalAtomicBatchV2Item(t, validAtomicBatchV2Request("@source-three", "@destination-three", batchTestOtherLedgerID))

	_, err := decodeAndValidateAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, first, second, third), 50)
	detail := assertAtomicBatchV2Problem(t, err, constant.ErrTransactionScopeMismatch.Error())
	assert.Equal(t, []string{
		"body.transactions[1].debits[0].ledgerId",
	}, atomicBatchV2ProblemLocations(detail))
}

func TestDecodeAndValidateAtomicTransactionBatchV2_RejectsFirstRepeatedException(t *testing.T) {
	t.Parallel()

	firstRequest := validAtomicBatchV2Request("@source", "@destination", batchTestLedgerID)
	firstRequest.AccountBlockExceptionID = stringPointer(batchTestExceptionID)
	secondRequest := validAtomicBatchV2Request("@source-two", "@destination-two", batchTestLedgerID)
	secondRequest.AccountBlockExceptionID = stringPointer(batchTestExceptionID)
	thirdRequest := validAtomicBatchV2Request("@source-three", "@destination-three", batchTestLedgerID)
	thirdRequest.AccountBlockExceptionID = stringPointer(batchTestExceptionID)

	_, err := decodeAndValidateAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(
		t,
		mustMarshalAtomicBatchV2Item(t, firstRequest),
		mustMarshalAtomicBatchV2Item(t, secondRequest),
		mustMarshalAtomicBatchV2Item(t, thirdRequest),
	), 50)
	detail := assertAtomicBatchV2Problem(t, err, constant.ErrAccountBlockExceptionInvalid.Error())
	assert.Equal(t, []string{
		"body.transactions[1].accountBlockExceptionId",
	}, atomicBatchV2ProblemLocations(detail))
}

func TestDecodeAndValidateAtomicTransactionBatchV2_TruncatesStructuralDiagnostics(t *testing.T) {
	t.Parallel()

	var item map[string]any
	require.NoError(t, json.Unmarshal(mustMarshalAtomicBatchV2Item(
		t,
		validAtomicBatchV2Request("@source", "@destination", batchTestLedgerID),
	), &item))
	for index := 0; index <= pkg.MaxFieldErrors; index++ {
		item[fmt.Sprintf("unexpected%03d", index)] = "value"
	}
	rawItem, err := json.Marshal(item)
	require.NoError(t, err)

	_, err = decodeAndValidateAtomicTransactionBatchV2(marshalAtomicBatchV2Wrapper(t, rawItem), 50)
	detail := assertAtomicBatchV2Problem(t, err, constant.ErrUnexpectedFieldsInTheRequest.Error())
	require.Len(t, detail.Errors, pkg.MaxFieldErrors)
	assert.Equal(t, "body.transactions[0].unexpected000", detail.Errors[0].Location)
	assert.Equal(t, "body.transactions[0].unexpected098", detail.Errors[pkg.MaxFieldErrors-2].Location)
	assert.Equal(t, pkg.FieldErrorTruncationLocation, detail.Errors[pkg.MaxFieldErrors-1].Location)
	assert.Equal(t, pkg.FieldErrorTruncationMessage, detail.Errors[pkg.MaxFieldErrors-1].Message)
}

func validAtomicBatchV2Request(source, destination, ledgerID string) CreateTransactionV2Request {
	return CreateTransactionV2Request{
		Asset:   "USD",
		Amount:  "1",
		Debits:  []TransactionV2LegRequest{validAtomicBatchV2Leg(source, ledgerID)},
		Credits: []TransactionV2LegRequest{validAtomicBatchV2Leg(destination, ledgerID)},
	}
}

func validAtomicBatchV2Leg(alias, ledgerID string) TransactionV2LegRequest {
	return TransactionV2LegRequest{
		Alias:          alias,
		OrganizationID: batchTestOrganizationID,
		LedgerID:       ledgerID,
		Amount:         "1",
	}
}

func repeatedAtomicBatchV2Legs(count int, alias, ledgerID string) []TransactionV2LegRequest {
	legs := make([]TransactionV2LegRequest, count)
	for index := range legs {
		legs[index] = validAtomicBatchV2Leg(alias, ledgerID)
	}

	return legs
}

func mustMarshalAtomicBatchV2Item(t *testing.T, request CreateTransactionV2Request) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(request)
	require.NoError(t, err)

	return raw
}

func marshalAtomicBatchV2Wrapper(t *testing.T, items ...json.RawMessage) []byte {
	t.Helper()

	raw, err := json.Marshal(map[string]any{"transactions": items})
	require.NoError(t, err)

	return raw
}

func assertAtomicBatchV2Problem(t *testing.T, err error, code string) pkgHTTP.Detail {
	t.Helper()
	require.Error(t, err)

	rendered := pkgHTTP.HumaProblem(err)
	detail, ok := rendered.(*pkgHTTP.Detail)
	require.True(t, ok)
	assert.Equal(t, code, detail.Code)

	return *detail
}

func atomicBatchV2ProblemLocations(detail pkgHTTP.Detail) []string {
	locations := make([]string, 0, len(detail.Errors))
	for _, field := range detail.Errors {
		locations = append(locations, field.Location)
	}

	return locations
}

func stringPointer(value string) *string {
	return &value
}
