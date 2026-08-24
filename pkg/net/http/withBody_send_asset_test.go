// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// Fixed scope for the v2 leg arrays. The scope is not what these cases exercise; it only has
// to be a valid, agreeing pair so the asset stays the operative rule.
const (
	sendAssetOrgID    = "00000000-0000-0000-0000-000000000001"
	sendAssetLedgerID = "00000000-0000-0000-0000-000000000002"
)

// sendAssetBody spells a v1 JSON-create body that is balanced and otherwise complete, with
// sendAsset interpolated verbatim so a case can spell the asset key as present, absent, or
// explicitly empty without any other field changing. Decimal values are quoted because
// decimal.Decimal marshals back to a JSON string: a bare number would be flagged by the
// unknown-field round-trip and never reach the validator.
func sendAssetBody(sendAsset string) string {
	return `{"send":{` + sendAsset + `"value":"100",` +
		`"source":{"from":[{"accountAlias":"@src","amount":{"asset":"USD","value":"100"}}]},` +
		`"distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"USD","value":"100"}}]}}}`
}

// inflowSendAssetBody spells an inflow-create body, which carries a distribute block and no
// source: SendInflow has no Source field, so lending it one would be an unknown field rather
// than the asset case the test is after. Same quoting rule as sendAssetBody.
func inflowSendAssetBody(sendAsset string) string {
	return `{"send":{` + sendAsset + `"value":"100",` +
		`"distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"USD","value":"100"}}]}}}`
}

// outflowSendAssetBody spells an outflow-create body, the mirror of the inflow one: SendOutflow
// carries a source block and has no Distribute field. Same quoting rule as sendAssetBody.
func outflowSendAssetBody(sendAsset string) string {
	return `{"send":{` + sendAsset + `"value":"100",` +
		`"source":{"from":[{"accountAlias":"@src","amount":{"asset":"USD","value":"100"}}]}}}`
}

// v2AssetBody spells a v2-create body. The asset is interpolated at the TOP level, not inside a
// send block, because CreateTransactionV2Input carries it there. Each leg names its own scope
// and an explicit amount so the two sides sum to the request total.
func v2AssetBody(asset string) string {
	leg := func(alias string) string {
		return `{"alias":"` + alias + `","organizationId":"` + sendAssetOrgID +
			`","ledgerId":"` + sendAssetLedgerID + `","amount":"100"}`
	}

	return `{` + asset + `"amount":"100",` +
		`"debits":[` + leg("@src") + `],` +
		`"credits":[` + leg("@dst") + `]}`
}

// assetOutcome is how DecodeAndValidate answers one asset spelling.
type assetOutcome int

const (
	// assetAccepted means the body validated and the asset reached the built transaction.
	assetAccepted assetOutcome = iota
	// assetMissingRequiredField means the `required` tag rejected it (0009).
	assetMissingRequiredField
	// assetUnknownField means the unknown-field round-trip rejected it first (0053).
	assetUnknownField
)

// sendAssetDecoder runs one input type through DecodeAndValidate and, on success, returns the
// asset that type carries forward to the transaction applyFees runs over. Errors raised by the
// build/translate step are not the subject here and fail the case outright.
type sendAssetDecoder func(t *testing.T, body string) (string, error)

func decodeV1SendAsset(t *testing.T, body string) (string, error) {
	t.Helper()

	var in mtransaction.CreateTransactionInput
	if _, err := DecodeAndValidate([]byte(body), &in); err != nil {
		return "", err
	}

	return in.BuildTransaction().Send.Asset, nil
}

func decodeInflowSendAsset(t *testing.T, body string) (string, error) {
	t.Helper()

	var in mtransaction.CreateTransactionInflowInput
	if _, err := DecodeAndValidate([]byte(body), &in); err != nil {
		return "", err
	}

	return in.BuildInflowEntry().Send.Asset, nil
}

func decodeOutflowSendAsset(t *testing.T, body string) (string, error) {
	t.Helper()

	var in mtransaction.CreateTransactionOutflowInput
	if _, err := DecodeAndValidate([]byte(body), &in); err != nil {
		return "", err
	}

	return in.BuildOutflowEntry().Send.Asset, nil
}

func decodeV2Asset(t *testing.T, body string) (string, error) {
	t.Helper()

	var in mtransaction.CreateTransactionV2Input
	if _, err := DecodeAndValidate([]byte(body), &in); err != nil {
		return "", err
	}

	tx, _, err := in.Translate(false)
	require.NoError(t, err, "the v2 body must translate; the asset is the only rule under test")

	return tx.Send.Asset, nil
}

// TestDecodeAndValidate_TransactionRejectsEmptySendAsset pins the transaction-create entry
// points' rejection of a body that names no asset, so such a request never reaches the fee
// engine without one.
//
// Four input types are covered because each carries its OWN `required` tag on its own asset
// field, and all four reach the fee engine: CreateTransactionInput.Send.Asset (v1 JSON),
// SendInflow.Asset, SendOutflow.Asset, and CreateTransactionV2Input.Asset. Dropping any one
// of those tags has to fail here.
//
// Both spellings of "no asset" are asserted per type because different rules reject them and a
// caller can send either. An absent key fails `required` (0009). An explicit empty string is
// rejected as an unknown field (0053) for the v1, inflow and outflow asset fields, and fails
// `required` (0009) for CreateTransactionV2Input.Asset.
func TestDecodeAndValidate_TransactionRejectsEmptySendAsset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// decode is the input type under test.
		decode sendAssetDecoder
		// body is that type's create body with the asset spelled as this case wants it.
		body string
		// want is the outcome DecodeAndValidate is pinned to.
		want assetOutcome
		// wantFieldPath is the location the 0009 message must name for the caller.
		wantFieldPath string
		// wantUnknown is the exact 0053 field map.
		wantUnknown pkg.UnknownFields
	}{
		{
			name:          "v1 json: absent asset key is a missing required field",
			decode:        decodeV1SendAsset,
			body:          sendAssetBody(``),
			want:          assetMissingRequiredField,
			wantFieldPath: "send.asset",
		},
		{
			name:        "v1 json: explicitly empty asset does not survive the round-trip",
			decode:      decodeV1SendAsset,
			body:        sendAssetBody(`"asset":"",`),
			want:        assetUnknownField,
			wantUnknown: pkg.UnknownFields{"send": map[string]any{"asset": ""}},
		},
		{
			name:   "v1 json: asset present passes validation",
			decode: decodeV1SendAsset,
			body:   sendAssetBody(`"asset":"USD",`),
			want:   assetAccepted,
		},
		{
			name:          "inflow: absent asset key is a missing required field",
			decode:        decodeInflowSendAsset,
			body:          inflowSendAssetBody(``),
			want:          assetMissingRequiredField,
			wantFieldPath: "send.asset",
		},
		{
			name:        "inflow: explicitly empty asset does not survive the round-trip",
			decode:      decodeInflowSendAsset,
			body:        inflowSendAssetBody(`"asset":"",`),
			want:        assetUnknownField,
			wantUnknown: pkg.UnknownFields{"send": map[string]any{"asset": ""}},
		},
		{
			name:   "inflow: asset present passes validation",
			decode: decodeInflowSendAsset,
			body:   inflowSendAssetBody(`"asset":"USD",`),
			want:   assetAccepted,
		},
		{
			name:          "outflow: absent asset key is a missing required field",
			decode:        decodeOutflowSendAsset,
			body:          outflowSendAssetBody(``),
			want:          assetMissingRequiredField,
			wantFieldPath: "send.asset",
		},
		{
			name:        "outflow: explicitly empty asset does not survive the round-trip",
			decode:      decodeOutflowSendAsset,
			body:        outflowSendAssetBody(`"asset":"",`),
			want:        assetUnknownField,
			wantUnknown: pkg.UnknownFields{"send": map[string]any{"asset": ""}},
		},
		{
			name:   "outflow: asset present passes validation",
			decode: decodeOutflowSendAsset,
			body:   outflowSendAssetBody(`"asset":"USD",`),
			want:   assetAccepted,
		},
		{
			name:          "v2: absent asset key is a missing required field",
			decode:        decodeV2Asset,
			body:          v2AssetBody(``),
			want:          assetMissingRequiredField,
			wantFieldPath: "asset",
		},
		{
			name:          "v2: explicitly empty asset is a missing required field, not an unknown one",
			decode:        decodeV2Asset,
			body:          v2AssetBody(`"asset":"",`),
			want:          assetMissingRequiredField,
			wantFieldPath: "asset",
		},
		{
			name:   "v2: asset present passes validation",
			decode: decodeV2Asset,
			body:   v2AssetBody(`"asset":"USD",`),
			want:   assetAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			asset, err := tt.decode(t, tt.body)

			switch tt.want {
			case assetAccepted:
				require.NoError(t, err,
					"the same body with an asset must pass, proving the asset is the operative rule")
				assert.Equal(t, "USD", asset,
					"the validated asset must reach the Transaction that applyFees runs over")

			case assetMissingRequiredField:
				require.Error(t, err, "a create body with no asset must be rejected")

				var known *pkg.ValidationKnownFieldsError
				require.ErrorAs(t, err, &known, "expected the known-field validation class")
				assert.Equal(t, constant.ErrMissingFieldsInRequest.Error(), known.Code)
				assert.Contains(t, known.Fields, "asset",
					"the rejection must name the offending field so the caller can fix it")
				assert.Contains(t, known.Fields["asset"], tt.wantFieldPath,
					"the field message must locate the asset in the body")

			case assetUnknownField:
				require.Error(t, err, "a create body with an empty asset must be rejected")

				var unknown pkg.ValidationUnknownFieldsError
				require.ErrorAs(t, err, &unknown, "expected the unknown-field validation class")
				assert.Equal(t, constant.ErrUnexpectedFieldsInTheRequest.Error(), unknown.Code)
				assert.Equal(t, tt.wantUnknown, unknown.Fields,
					"the rejection must point the caller at the empty asset")

			default:
				t.Fatalf("unhandled assetOutcome %v — add an assertion arm for it", tt.want)
			}
		})
	}
}
