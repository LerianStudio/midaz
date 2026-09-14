// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// recordingFeeService stands in for the fee engine and records whether the estimate reached it.
// Whether the engine was called is the whole assertion: an estimate refused on input must never
// price anything.
type recordingFeeService struct{ called bool }

func (s *recordingFeeService) EstimateFeeCalculation(_ context.Context, _ *model.FeeEstimate, _, _ uuid.UUID) (*model.FeeEstimateResult, error) {
	s.called = true

	return &model.FeeEstimateResult{}, nil
}

// estimateBodyWithSourceAlias spells a complete fee-estimate body whose single debit leg carries
// the given alias, so a row travels the real decode path rather than a struct built in Go.
func estimateBodyWithSourceAlias(alias string) []byte {
	return []byte(`{"packageId":"11111111-1111-1111-1111-111111111111","transaction":{"send":` +
		`{"asset":"BRL","value":"100",` +
		`"source":{"from":[{"accountAlias":"` + alias + `","amount":{"asset":"BRL","value":"100"}}]},` +
		`"distribute":{"to":[{"accountAlias":"@person2","amount":{"asset":"BRL","value":"100"}}]}}}}`)
}

// TestFeeEstimate_RefusesLegAliasNoAccountCanCarry holds the fee estimate to the same alias rule
// as the v2 create. An estimate is a price quoted for the accounts the body names, so quoting one
// for an alias that can never resolve is the create-side defect moved one step earlier: the
// caller is told what a transaction would cost without being told the transaction cannot exist.
//
// KNOWN GAP, deliberately not asserted here. The fee decode path sanitizes every string field
// before this guard runs, stripping any character outside its own allow-list, so an alias spelled
// dst->ops arrives as dst-ops and an alias spelled acc:01 arrives as acc01. Those two never reach
// this rule as the caller spelled them, so this guard cannot refuse them, and the aliases below
// are exactly the ones that survive the sanitizer intact. The sanitizer rewriting a legal alias
// onto a different account is a separate defect on the fee surface, reported rather than closed
// here, because changing that allow-list is a decision about the whole fee input surface.
func TestFeeEstimate_RefusesLegAliasNoAccountCanCarry(t *testing.T) {
	t.Parallel()

	refused := []struct {
		alias string
		why   string
	}{
		{"dst ops", "a space is outside the registered account alias charset"},
		{"dst/ops", "a slash is outside the charset and this is not the external account shape"},
		{"@external/", "the external shape with no asset code names no account"},
		{"@external/brl", "an asset code is uppercase, so no account carries this alias"},
		{"a_b-c.", "a dot is outside the registered account alias charset"},
	}

	for _, row := range refused {
		t.Run(row.alias, func(t *testing.T) {
			t.Parallel()

			payload := new(model.FeeEstimate)

			_, err := feehttp.DecodeValidateBody(estimateBodyWithSourceAlias(row.alias), payload)
			require.NoError(t, err, "the row must reach the guard, not be refused at decode")
			require.Equal(t, row.alias, payload.Transaction.Send.Source.From[0].AccountAlias,
				"the sanitizer must leave this alias intact, or the row is testing a different string")

			service := &recordingFeeService{}
			handler := &FeeHandler{Service: service}

			_, err = handler.estimateFeeCalculation(context.Background(), uuid.New(), uuid.New(), payload)
			require.Errorf(t, err, "alias %q must be refused: %s", row.alias, row.why)

			var vErr pkg.ValidationError
			require.ErrorAs(t, err, &vErr, "an alias no account can carry is a request-shape error (400)")
			assert.Equal(t, constant.ErrAccountAliasInvalid.Error(), vErr.Code)
			assert.False(t, service.called, "a refused estimate must never reach the fee engine")
		})
	}
}

// TestFeeEstimate_AcceptsLegAliasAnAccountCanCarry is the other half of the guard: refusing what
// no account can carry must not cost the aliases every account does carry. The credit leg carries
// the same sweep as the debit leg through the shared body builder's counterpart below.
func TestFeeEstimate_AcceptsLegAliasAnAccountCanCarry(t *testing.T) {
	t.Parallel()

	for _, alias := range []string{"payer", "@merchant", "@external/BRL", "a_b-c"} {
		t.Run(alias, func(t *testing.T) {
			t.Parallel()

			payload := new(model.FeeEstimate)

			_, err := feehttp.DecodeValidateBody(estimateBodyWithSourceAlias(alias), payload)
			require.NoError(t, err)

			service := &recordingFeeService{}
			handler := &FeeHandler{Service: service}

			_, err = handler.estimateFeeCalculation(context.Background(), uuid.New(), uuid.New(), payload)
			require.NoErrorf(t, err, "alias %q must be accepted", alias)
			assert.True(t, service.called, "an accepted estimate must reach the fee engine")
		})
	}
}

// TestFeeEstimate_RefusesCreditLegAlias pins that the rule covers the credit side too, so a
// refusal is not something a caller can route around by moving the alias to the other leg array.
func TestFeeEstimate_RefusesCreditLegAlias(t *testing.T) {
	t.Parallel()

	body := []byte(`{"packageId":"11111111-1111-1111-1111-111111111111","transaction":{"send":` +
		`{"asset":"BRL","value":"100",` +
		`"source":{"from":[{"accountAlias":"@payer","amount":{"asset":"BRL","value":"100"}}]},` +
		`"distribute":{"to":[{"accountAlias":"dst/ops","amount":{"asset":"BRL","value":"100"}}]}}}}`)

	payload := new(model.FeeEstimate)

	_, err := feehttp.DecodeValidateBody(body, payload)
	require.NoError(t, err)

	service := &recordingFeeService{}
	handler := &FeeHandler{Service: service}

	_, err = handler.estimateFeeCalculation(context.Background(), uuid.New(), uuid.New(), payload)
	require.Error(t, err, "a credit leg alias no account can carry must be refused")

	var vErr pkg.ValidationError
	require.ErrorAs(t, err, &vErr)
	assert.Equal(t, constant.ErrAccountAliasInvalid.Error(), vErr.Code)
	assert.False(t, service.called, "a refused estimate must never reach the fee engine")
}
