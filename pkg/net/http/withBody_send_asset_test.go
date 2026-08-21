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

// TestDecodeAndValidate_TransactionRejectsEmptySendAsset pins the v1 transaction-create
// entry point's rejection of a body that names no send asset, proving the request never
// reaches applyFees — and therefore never reaches the fee engine — without one.
//
// The only thing rejecting it is the `required` tag on Send.Asset, reached through the
// `required,dive` tag on CreateTransactionInput.Send. Nothing else in the repo pins that
// tag. ValidateSendSourceAndDistribute, which runs later on the built Transaction, copies
// the asset through without checking it (see
// TestValidateSendSourceAndDistribute_DoesNotRejectEmptyAsset), so this layer and the fee
// engine's own guard are the whole defense.
//
// Both spellings of "no asset" are asserted because they are rejected by different rules and
// a caller can send either. An absent key fails `required` (0009). An explicit empty string
// fails the unknown-field round-trip first (0053): Send.Asset is `omitempty`, so an empty
// value does not survive the re-marshal and the field looks unrecognized.
func TestDecodeAndValidate_TransactionRejectsEmptySendAsset(t *testing.T) {
	t.Parallel()

	t.Run("absent asset key is a missing required field", func(t *testing.T) {
		t.Parallel()

		var in mtransaction.CreateTransactionInput

		_, err := DecodeAndValidate([]byte(sendAssetBody(``)), &in)
		require.Error(t, err, "a create body with no send asset must be rejected")

		var known *pkg.ValidationKnownFieldsError
		require.ErrorAs(t, err, &known, "expected the known-field validation class")
		assert.Equal(t, constant.ErrMissingFieldsInRequest.Error(), known.Code)
		assert.Contains(t, known.Fields, "asset",
			"the rejection must name the offending field so the caller can fix it")
		assert.Contains(t, known.Fields["asset"], "send.asset",
			"the field message must locate the asset inside the send block")
	})

	t.Run("explicitly empty asset does not survive the round-trip", func(t *testing.T) {
		t.Parallel()

		var in mtransaction.CreateTransactionInput

		_, err := DecodeAndValidate([]byte(sendAssetBody(`"asset":"",`)), &in)
		require.Error(t, err, "a create body with an empty send asset must be rejected")

		var unknown pkg.ValidationUnknownFieldsError
		require.ErrorAs(t, err, &unknown, "expected the unknown-field validation class")
		assert.Equal(t, constant.ErrUnexpectedFieldsInTheRequest.Error(), unknown.Code)
		assert.Equal(t, pkg.UnknownFields{"send": map[string]any{"asset": ""}}, unknown.Fields,
			"the rejection must point the caller at the empty asset")
	})

	t.Run("asset present passes validation", func(t *testing.T) {
		t.Parallel()

		var in mtransaction.CreateTransactionInput

		_, err := DecodeAndValidate([]byte(sendAssetBody(`"asset":"USD",`)), &in)
		require.NoError(t, err, "the same body with an asset must pass, proving the asset is the operative rule")
		assert.Equal(t, "USD", in.BuildTransaction().Send.Asset,
			"the validated asset must reach the Transaction that applyFees runs over")
	})
}
