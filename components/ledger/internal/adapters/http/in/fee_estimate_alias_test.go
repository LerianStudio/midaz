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

// recordingFeeService stands in for the fee engine and records whether the estimate reached it and
// which source alias it was asked to price. Whether the engine was called answers the refusal
// rows; which alias it saw answers the rewriting ones, because an estimate priced for a string the
// caller never sent is wrong even when it succeeds.
type recordingFeeService struct {
	called          bool
	seenSourceAlias string
}

func (s *recordingFeeService) EstimateFeeCalculation(_ context.Context, cf *model.FeeEstimate, _, _ uuid.UUID) (*model.FeeEstimateResult, error) {
	s.called = true

	if legs := cf.Transaction.Send.Source.From; len(legs) > 0 {
		s.seenSourceAlias = legs[0].AccountAlias
	}

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
// Every row is checked against the alias the CALLER submitted, not a rewritten one: the decode
// path must hand validation the exact string the body carried, so a refusal is about what the
// caller wrote rather than about what decoding turned it into.
func TestFeeEstimate_RefusesLegAliasNoAccountCanCarry(t *testing.T) {
	t.Parallel()

	refused := []struct {
		alias string
		why   string
	}{
		{"dst->ops", "the fee engine cuts a leg alias at the first arrow, so this would price against dst"},
		{"@payer->fee0->", "spells a movement the fee engine itself mints"},
		{"dst ops", "a space is outside the registered account alias charset"},
		{"dst/ops", "a slash is outside the charset and this is not the external account shape"},
		{"@external/", "the external shape with no asset code names no account"},
		{"@external/brl", "an asset code is uppercase, so no account carries this alias"},
		{"a_b-c.", "a dot is outside the registered account alias charset"},
		{"@a#x", "the composite separator, which keys the funnel's per-entry maps"},
	}

	for _, row := range refused {
		t.Run(row.alias, func(t *testing.T) {
			t.Parallel()

			payload := new(model.FeeEstimate)

			_, err := feehttp.DecodeValidateBody(estimateBodyWithSourceAlias(row.alias), payload)
			require.NoError(t, err, "the row must reach the guard, not be refused at decode")
			require.Equal(t, row.alias, payload.Transaction.Send.Source.From[0].AccountAlias,
				"decoding must not rewrite the submitted alias, or the guard is judging a different string")

			service := &recordingFeeService{}
			handler := &FeeHandler{Service: service}

			_, err = handler.estimateFeeCalculation(context.Background(), uuid.New(), uuid.New(), payload)
			require.Errorf(t, err, "alias %q must be refused: %s", row.alias, row.why)

			var vErr pkg.ValidationError
			require.ErrorAs(t, err, &vErr, "an alias no account can carry is a request-shape error (400)")
			assert.Equal(t, constant.ErrAccountAliasInvalid.Error(), vErr.Code)
			assert.False(t, service.called, "a refused estimate must never reach the fee engine")
			assert.Equal(t, row.alias, payload.Transaction.Send.Source.From[0].AccountAlias,
				"the refused alias must still read as the caller spelled it after the refusal")
		})
	}
}

// TestFeeEstimate_DecodeLeavesTheSubmittedAliasIntact is the rule the refusal rows above depend
// on: a fee body is never rewritten on the way in.
//
// The fee decode path used to strip every character outside its own allow-list, which kept the
// slash and the backslash but removed the colon, the angle bracket and the hash. Two consequences
// made it a money-path defect rather than a cosmetic one. An alias spelled acc:01, which an
// account CAN carry, arrived as acc01 and was priced against a different account, silently and
// with a 200. And an alias spelled dst->ops arrived as dst-ops, which matches the account charset,
// so it slipped past the guard the caller's actual string would have failed.
//
// Validation, not rewriting, is what answers a character a field may not carry.
func TestFeeEstimate_DecodeLeavesTheSubmittedAliasIntact(t *testing.T) {
	t.Parallel()

	for _, alias := range []string{"acc:01", "dst->ops", "@a#x", "@external/BRL", "a;b", "a.b,c"} {
		t.Run(alias, func(t *testing.T) {
			t.Parallel()

			payload := new(model.FeeEstimate)

			_, err := feehttp.DecodeValidateBody(estimateBodyWithSourceAlias(alias), payload)
			require.NoError(t, err)
			assert.Equal(t, alias, payload.Transaction.Send.Source.From[0].AccountAlias,
				"decoding must hand validation the exact alias the body carried")
		})
	}
}

// TestFeeEstimate_LegalColonAliasReachesTheEngineIntact is the defect CodeRabbit named, pinned end
// to end: acc:01 is an alias an account can carry, so the estimate must be priced for it and the
// engine must receive it spelled exactly that way.
func TestFeeEstimate_LegalColonAliasReachesTheEngineIntact(t *testing.T) {
	t.Parallel()

	payload := new(model.FeeEstimate)

	_, err := feehttp.DecodeValidateBody(estimateBodyWithSourceAlias("acc:01"), payload)
	require.NoError(t, err)

	service := &recordingFeeService{}
	handler := &FeeHandler{Service: service}

	_, err = handler.estimateFeeCalculation(context.Background(), uuid.New(), uuid.New(), payload)
	require.NoError(t, err, "a legal alias must not be refused")

	require.True(t, service.called, "the estimate must reach the fee engine")
	assert.Equal(t, "acc:01", service.seenSourceAlias,
		"the engine must be asked to price the alias the caller submitted, not a rewritten one")
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
