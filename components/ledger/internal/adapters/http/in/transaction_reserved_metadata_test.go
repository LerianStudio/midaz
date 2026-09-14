// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	nethttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// reservedMetadataJSON is one metadata object carrying the key the ledger reserves, spelled from
// the constant the fee engine writes so a rename cannot leave these tests passing on a dead name.
const reservedMetadataJSON = `"metadata":{"` + constant.MetadataKeyFeeLeg + `":"true"}`

// v1Body builds a /v1 create body, optionally carrying metadata on the single source leg and on
// the transaction envelope. Both slots take a complete leading-comma metadata fragment or the
// empty string, which is what lets one builder serve a refused row and its accepted twin.
func v1Body(legMetadata, transactionMetadata string) string {
	body := `{"send":{"asset":"BRL","value":"100",` +
		`"source":{"from":[{"accountAlias":"@src","amount":{"asset":"BRL","value":"100"}` + legMetadata + `}]},` +
		`"distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"BRL","value":"100"}}]}}`

	return body + transactionMetadata + `}`
}

// inflowBody builds a /v1 inflow create body carrying the given transaction metadata fragment.
func inflowBody(transactionMetadata string) string {
	return `{"send":{"asset":"BRL","value":"100","distribute":{"to":[` +
		`{"accountAlias":"@dst","amount":{"asset":"BRL","value":"100"}}]}}` +
		transactionMetadata + `}`
}

// outflowBody builds a /v1 outflow create body carrying the given transaction metadata fragment.
func outflowBody(transactionMetadata string) string {
	return `{"send":{"asset":"BRL","value":"100","source":{"from":[` +
		`{"accountAlias":"@src","amount":{"asset":"BRL","value":"100"}}]}}` +
		transactionMetadata + `}`
}

// v2Body builds a /v2/transactions/direct create body carrying the given transaction metadata
// fragment.
func v2Body(transactionMetadata string) string {
	return `{"asset":"BRL","amount":"100",` +
		`"debits":[{"alias":"@src",` + scopeJSON + `,"amount":"100"}],` +
		`"credits":[{"alias":"@dst",` + scopeJSON + `,"amount":"100"}]` +
		transactionMetadata + `}`
}

// TestReservedFeeMetadataKeyIsRefused pins the promise the published operation contract makes:
// the fee mark is the ledger's word, so a caller that writes it itself is refused rather than
// silently stripped. A silent strip would be indistinguishable, on the wire, from a caller whose
// key was accepted, and the contract would still be unprovable.
//
// Every body a caller can put operation metadata in is covered: the /v2 create, the /v1 create
// (both the transaction envelope and an individual leg, which is the one that actually reaches an
// operation), the /v1 inflow and outflow bodies, and the operation metadata update, which can add
// the mark to an operation the ledger already wrote.
func TestReservedFeeMetadataKeyIsRefused(t *testing.T) {
	t.Parallel()

	refused := []struct {
		name string
		body string
		into any
	}{
		{
			name: "v2 create, transaction metadata",
			body: v2Body(`,` + reservedMetadataJSON),
			into: &CreateTransactionV2Request{},
		},
		{
			name: "v1 create, leg metadata",
			body: v1Body(`,`+reservedMetadataJSON, ``),
			into: &CreateTransactionRequest{},
		},
		{
			name: "v1 create, transaction metadata",
			body: v1Body(``, `,`+reservedMetadataJSON),
			into: &CreateTransactionRequest{},
		},
		{
			name: "v1 inflow, transaction metadata",
			body: inflowBody(`,` + reservedMetadataJSON),
			into: &CreateTransactionInflowRequestBody{},
		},
		{
			name: "v1 outflow, transaction metadata",
			body: outflowBody(`,` + reservedMetadataJSON),
			into: &CreateTransactionOutflowRequestBody{},
		},
		{
			name: "operation metadata update",
			body: `{"description":"reconciled",` + reservedMetadataJSON + `}`,
			into: &operation.UpdateOperationInput{},
		},
	}

	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := nethttp.DecodeAndValidate([]byte(tc.body), tc.into)
			require.Error(t, err, "a caller-supplied fee mark must be refused, never accepted or stripped")
			assert.Contains(t, err.Error(), constant.MetadataKeyFeeLeg,
				"the refusal must name the reserved key, so the caller knows which key to drop")
		})
	}

	accepted := []struct {
		name string
		body string
		into any
	}{
		{
			name: "v2 create, ordinary caller metadata",
			body: v2Body(`,"metadata":{"invoice":"INV-12345"}`),
			into: &CreateTransactionV2Request{},
		},
		{
			name: "v1 create, ordinary leg metadata",
			body: v1Body(`,"metadata":{"invoice":"INV-12345"}`, ``),
			into: &CreateTransactionRequest{},
		},
		{
			// The refused inflow and outflow rows above are only proof of the reserved-key rule
			// if the same body without the reserved key is accepted, so each carries its twin.
			name: "v1 inflow, ordinary caller metadata",
			body: inflowBody(`,"metadata":{"invoice":"INV-12345"}`),
			into: &CreateTransactionInflowRequestBody{},
		},
		{
			name: "v1 outflow, ordinary caller metadata",
			body: outflowBody(`,"metadata":{"invoice":"INV-12345"}`),
			into: &CreateTransactionOutflowRequestBody{},
		},
		{
			name: "operation metadata update, ordinary caller metadata",
			body: `{"description":"reconciled","metadata":{"reference":"INV-12345"}}`,
			into: &operation.UpdateOperationInput{},
		},
	}

	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := nethttp.DecodeAndValidate([]byte(tc.body), tc.into)
			require.NoError(t, err, "the refusal must be scoped to the reserved key and nothing else")
		})
	}
}

// TestReservedFeeMetadataKeyOnTheFeeValidator covers the second body validator in this binary.
// The fee routes decode through their own validator instance, and a rule registered on only one
// of the two does not merely go unenforced there: go-playground panics on a tag it does not know,
// so the fee estimate would answer every request with a 500. The estimate body embeds the whole
// canonical transaction, which is how the per-leg metadata rule reaches it.
func TestReservedFeeMetadataKeyOnTheFeeValidator(t *testing.T) {
	t.Parallel()

	estimateBody := func(legMetadata string) []byte {
		return []byte(`{"ledgerId":"` + testLedgerID + `","transaction":{"send":{"asset":"BRL","value":"100",` +
			`"source":{"from":[{"accountAlias":"@src","amount":{"asset":"BRL","value":"100"}` + legMetadata + `}]},` +
			`"distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"BRL","value":"100"}}]}}}}`)
	}

	t.Run("ordinary caller metadata is accepted", func(t *testing.T) {
		t.Parallel()

		_, err := feehttp.DecodeValidateBody(estimateBody(`,"metadata":{"invoice":"INV-12345"}`), new(model.FeeCalculate))
		require.NoError(t, err)
	})

	t.Run("the reserved fee mark is refused", func(t *testing.T) {
		t.Parallel()

		_, err := feehttp.DecodeValidateBody(estimateBody(`,`+reservedMetadataJSON), new(model.FeeCalculate))
		require.Error(t, err)
		assert.Contains(t, err.Error(), constant.MetadataKeyFeeLeg)
	})
}

// TestReservedFeeMetadataKeyErrorCode pins the business error the refusal carries, so a client can
// branch on the code rather than on the prose.
func TestReservedFeeMetadataKeyErrorCode(t *testing.T) {
	t.Parallel()

	body := `{"description":"reconciled","metadata":{"` + constant.MetadataKeyFeeLeg + `":"true"}}`

	_, err := nethttp.DecodeAndValidate([]byte(body), &operation.UpdateOperationInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrReservedMetadataKey.Error())
}

// reservedTransactionFeeKeys are the three transaction-level keys the fee engine writes. They are
// spelled literally, not through a constant, on purpose: these strings are the published wire
// contract, and a test that spelled them from whatever the guard spells them from would stay green
// through a rename that broke every client.
var reservedTransactionFeeKeys = []string{"feeApplied", "packageAppliedID", "feeExemption"}

// TestReservedFeeTransactionMetadataKeysAreRefused pins the reservation on the three fee keys the
// ledger writes on a transaction: feeApplied, packageAppliedID and feeExemption.
//
// Each one is the ledger's own statement about a fee charge. A caller that writes feeApplied on a
// transaction that paid no fee puts a statement in the audit trail the ledger never made, and no
// reader downstream, a report or a reconciliation, can tell it from one the ledger wrote. So a
// body carrying any of the three is refused before any calculation runs, exactly like the
// operation-level fee mark.
//
// The guard is a map-key rule, so its reach is every metadata field tagged with it rather than the
// transaction level alone: the rows below cover the four create bodies at the transaction level,
// an individual /v1 leg, and both metadata updates, which could otherwise stamp the claim onto a
// record the ledger had already written.
func TestReservedFeeTransactionMetadataKeysAreRefused(t *testing.T) {
	t.Parallel()

	for _, key := range reservedTransactionFeeKeys {
		metadata := `,"metadata":{"` + key + `":"true"}`

		surfaces := []struct {
			name string
			body string
			into any
		}{
			{
				name: "v2 create, transaction metadata",
				body: v2Body(metadata),
				into: &CreateTransactionV2Request{},
			},
			{
				name: "v1 create, transaction metadata",
				body: v1Body(``, metadata),
				into: &CreateTransactionRequest{},
			},
			{
				name: "v1 create, leg metadata",
				body: v1Body(metadata, ``),
				into: &CreateTransactionRequest{},
			},
			{
				name: "v1 inflow, transaction metadata",
				body: inflowBody(metadata),
				into: &CreateTransactionInflowRequestBody{},
			},
			{
				name: "v1 outflow, transaction metadata",
				body: outflowBody(metadata),
				into: &CreateTransactionOutflowRequestBody{},
			},
			{
				name: "transaction metadata update",
				body: `{"description":"reconciled"` + metadata + `}`,
				into: &transaction.UpdateTransactionInput{},
			},
			{
				name: "operation metadata update",
				body: `{"description":"reconciled"` + metadata + `}`,
				into: &operation.UpdateOperationInput{},
			},
		}

		for _, tc := range surfaces {
			t.Run(key+", "+tc.name, func(t *testing.T) {
				t.Parallel()

				_, err := nethttp.DecodeAndValidate([]byte(tc.body), tc.into)
				require.Error(t, err,
					"a caller-supplied fee statement must be refused, never accepted or stripped")
				assert.Contains(t, err.Error(), key,
					"the refusal must name the reserved key, so the caller knows which key to drop")
				assert.Contains(t, err.Error(), constant.ErrReservedMetadataKey.Error(),
					"the refusal must carry the reserved-key business error, so a client branches on the code")
			})
		}
	}
}

// TestReservedFeeTransactionMetadataKeysScope keeps the refusal narrow. Without these rows a guard
// that refused every metadata key would pass the table above, and the three reservations would be
// indistinguishable from a ban on caller metadata.
func TestReservedFeeTransactionMetadataKeysScope(t *testing.T) {
	t.Parallel()

	accepted := []struct {
		name string
		body string
		into any
	}{
		{
			name: "v2 create, a key that merely starts with a reserved one",
			body: v2Body(`,"metadata":{"feeAppliedByUs":"true"}`),
			into: &CreateTransactionV2Request{},
		},
		{
			name: "v1 create, an ordinary transaction key",
			body: v1Body(``, `,"metadata":{"invoice":"INV-12345"}`),
			into: &CreateTransactionRequest{},
		},
		{
			name: "transaction metadata update, an ordinary key",
			body: `{"description":"reconciled","metadata":{"reference":"INV-12345"}}`,
			into: &transaction.UpdateTransactionInput{},
		},
	}

	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := nethttp.DecodeAndValidate([]byte(tc.body), tc.into)
			require.NoError(t, err, "the refusal must be scoped to the reserved keys and nothing else")
		})
	}
}
