// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// estimateBody spells a fee-estimate body whose transaction is balanced and otherwise
// complete, with sendAsset interpolated verbatim so a case can spell the asset key as
// present, absent, or explicitly empty without any other field changing. Decimal values
// are quoted because decimal.Decimal marshals back to a JSON string: a bare number would
// be flagged by the decoder's unknown-field round-trip and never reach the validator.
func estimateBody(sendAsset string) string {
	return `{"packageId":"11111111-1111-1111-1111-111111111111",` +
		`"ledgerId":"22222222-2222-2222-2222-222222222222",` +
		`"transaction":{"send":{` + sendAsset + `"value":"100",` +
		`"source":{"from":[{"accountAlias":"@src","amount":{"asset":"USD","value":"100"}}]},` +
		`"distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"USD","value":"100"}}]}}}}`
}

// TestDecodeValidateBody_EstimateRejectsEmptySendAsset pins the estimate entry point's
// rejection of a fee-estimate body that names no send asset: the `required` tag on the
// estimate's own Send.Asset field must reject it, so such a request never reaches the fee
// engine. Nothing else in the repo pins that tag.
//
// Both spellings of "no asset" are asserted because they are rejected by different rules and
// a caller can send either. An absent key fails `required` (0009); an explicit empty string is
// rejected as an unknown field (0053). Both are 400-class rejections that stop the request
// before the fee engine runs.
func TestDecodeValidateBody_EstimateRejectsEmptySendAsset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sendAsset string
		wantCode  string
	}{
		{
			name:      "absent asset key is a missing required field",
			sendAsset: ``,
			wantCode:  constant.ErrMissingFieldsInRequest.Error(),
		},
		{
			name:      "explicitly empty asset does not survive the round-trip",
			sendAsset: `"asset":"",`,
			wantCode:  constant.ErrUnexpectedFieldsInTheRequest.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var in model.FeeEstimate

			_, err := DecodeValidateBody([]byte(estimateBody(tt.sendAsset)), &in)
			require.Error(t, err, "an estimate body with no send asset must be rejected")

			switch tt.wantCode {
			case constant.ErrMissingFieldsInRequest.Error():
				var known *pkg.ValidationKnownFieldsError
				require.ErrorAs(t, err, &known, "expected the known-field validation class")
				assert.Equal(t, tt.wantCode, known.Code)
				assert.Contains(t, known.Fields, "asset",
					"the rejection must name the offending field so the caller can fix it")
				assert.Contains(t, known.Fields["asset"], "transaction.send.asset",
					"the field message must locate the asset inside the transaction projection")
			case constant.ErrUnexpectedFieldsInTheRequest.Error():
				var unknown pkg.ValidationUnknownFieldsError
				require.ErrorAs(t, err, &unknown, "expected the unknown-field validation class")
				assert.Equal(t, tt.wantCode, unknown.Code)
				assert.Equal(t, pkg.UnknownFields{"transaction": map[string]any{"send": map[string]any{"asset": ""}}},
					unknown.Fields, "the rejection must point the caller at the empty asset")
			default:
				t.Fatalf("unhandled wantCode %q — add an assertion arm for it", tt.wantCode)
			}
		})
	}

	t.Run("asset present passes validation", func(t *testing.T) {
		t.Parallel()

		var in model.FeeEstimate

		_, err := DecodeValidateBody([]byte(estimateBody(`"asset":"USD",`)), &in)
		require.NoError(t, err, "the same body with an asset must pass, proving the asset is the operative rule")
		assert.Equal(t, "USD", in.Transaction.Send.Asset)
	})
}
