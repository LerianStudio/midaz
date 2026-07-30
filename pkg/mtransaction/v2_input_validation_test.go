// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction_test

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	nethttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// requireKnownFieldsError asserts err is the known-field validation class carrying wantCode,
// and hands back its per-field map. The generic top-level message is shared by every rejection
// in the class, so the field map is the only part that identifies WHICH rule fired.
func requireKnownFieldsError(t *testing.T, err error, wantCode error) pkg.FieldValidations {
	t.Helper()

	var known *pkg.ValidationKnownFieldsError
	require.ErrorAs(t, err, &known, "expected the known-field validation class")
	require.Equal(t, wantCode.Error(), known.Code)

	return known.Fields
}

// legArrayBody spells a body whose source array holds count legs, each with an explicit
// amount, so a test can push the array past the published per-side cap without any other
// field changing.
func legArrayBody(count int) string {
	legs := make([]string, 0, count)

	for i := range count {
		legs = append(legs, `{"account":"@src`+strconv.Itoa(i)+`","amount":"1"}`)
	}

	return `{"asset":"BRL","amount":"` + strconv.Itoa(count) + `",` +
		`"sources":[` + strings.Join(legs, ",") + `],` +
		`"destinations":[{"account":"@dstA","amount":"` + strconv.Itoa(count) + `"}]}`
}

// TestV2LegInput_NoRemainingExpression locks the v2 leg down to the two value expressions
// the surface publishes. A `remaining` leg resolves during validation but contributes no
// operation row and no balance movement, committing an unbalanced transaction, so the v2
// surface must not offer the expression at all: with no field to decode into, a client that
// sends one gets the decoder's unknown-field rejection.
func TestV2LegInput_NoRemainingExpression(t *testing.T) {
	t.Parallel()

	t.Run("struct carries no remaining field", func(t *testing.T) {
		t.Parallel()

		_, ok := reflect.TypeFor[mtransaction.V2LegInput]().FieldByName("Remaining")
		assert.False(t, ok, "V2LegInput must not publish a remaining expression")
	})

	t.Run("remaining on a leg is an unknown field", func(t *testing.T) {
		t.Parallel()

		body := `{"asset":"BRL","amount":"100",` +
			`"sources":[{"account":"@srcA","amount":"60"},{"account":"@srcB","remaining":true}],` +
			`"destinations":[{"account":"@dstA","amount":"100"}]}`

		var in mtransaction.CreateTransactionV2Input

		_, err := nethttp.DecodeAndValidate([]byte(body), &in)
		require.Error(t, err, "a leg spelling `remaining` must be rejected")

		var unknown pkg.ValidationUnknownFieldsError
		require.ErrorAs(t, err, &unknown, "an expression the leg does not publish is an unknown field")
		assert.Equal(t, constant.ErrUnexpectedFieldsInTheRequest.Error(), unknown.Code)
		assert.Equal(t, pkg.UnknownFields{"sources": []any{map[string]any{"remaining": true}}}, unknown.Fields,
			"the rejection must point the caller at the unsupported expression")
	})
}

// TestCreateTransactionV2Input_PerSideSpelling covers the per-side spelling rule: each side
// chooses the scalar field or the leg array independently, so one payer to many payees is a
// valid request. Only a side that spells itself BOTH ways, or NEITHER way, is rejected.
func TestCreateTransactionV2Input_PerSideSpelling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantErr    bool
		wantFromNo int
		wantToNo   int
	}{
		{
			name:       "scalar source with array destinations",
			body:       `{"asset":"BRL","amount":"100","from":"@srcA","destinations":[{"account":"@dstA","amount":"60"},{"account":"@dstB","amount":"40"}]}`,
			wantFromNo: 1,
			wantToNo:   2,
		},
		{
			name:       "array sources with scalar destination",
			body:       `{"asset":"BRL","amount":"100","sources":[{"account":"@srcA","amount":"60"},{"account":"@srcB","amount":"40"}],"to":"@dstA"}`,
			wantFromNo: 2,
			wantToNo:   1,
		},
		{
			name:       "both sides scalar",
			body:       `{"asset":"BRL","amount":"100","from":"@srcA","to":"@dstA"}`,
			wantFromNo: 1,
			wantToNo:   1,
		},
		{
			name:       "both sides arrays",
			body:       `{"asset":"BRL","amount":"100","sources":[{"account":"@srcA","amount":"100"}],"destinations":[{"account":"@dstA","amount":"100"}]}`,
			wantFromNo: 1,
			wantToNo:   1,
		},
		{
			name:    "source side spelled both ways",
			body:    `{"asset":"BRL","amount":"100","from":"@srcA","sources":[{"account":"@srcB","amount":"100"}],"to":"@dstA"}`,
			wantErr: true,
		},
		{
			name:    "destination side spelled both ways",
			body:    `{"asset":"BRL","amount":"100","from":"@srcA","to":"@dstA","destinations":[{"account":"@dstB","amount":"100"}]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(tt.body), &in)
			require.NoError(t, err, "the body must clear decode and struct validation")

			got, err := in.Translate(false)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err, "a side may choose its spelling independently of the other side")
			assert.Len(t, got.Send.Source.From, tt.wantFromNo)
			assert.Len(t, got.Send.Distribute.To, tt.wantToNo)
		})
	}
}

// TestCreateTransactionV2Input_LegArrayCap locks the published per-side leg cap. The cap is
// what keeps the request off the create funnel's quadratic leg passes: the byte limit alone
// admits tens of thousands of legs.
func TestCreateTransactionV2Input_LegArrayCap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		legs    int
		wantErr bool
	}{
		{name: "at the cap", legs: 500},
		{name: "one past the cap", legs: 501, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(legArrayBody(tt.legs)), &in)
			if tt.wantErr {
				require.Error(t, err, "%d legs must be rejected", tt.legs)

				fields := requireKnownFieldsError(t, err, constant.ErrBadRequest)
				assert.Equal(t, "sources must contain at maximum 500 items", fields["sources"],
					"the rejection must name the offending side and the cap")

				return
			}

			require.NoError(t, err, "%d legs must be accepted", tt.legs)
		})
	}
}

// TestV2ShareInput_PercentageBounds locks the share bounds. A zero or negative percentage
// clears struct validation today and reaches the funnel, where the leg silently produces no
// operation row (zero) or an inverted movement (negative) while the transaction commits.
func TestV2ShareInput_PercentageBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		share     string
		wantErr   bool
		wantCode  error
		wantField string
		wantRule  string
	}{
		{name: "positive percentage", share: `{"percentage":100}`},
		{name: "percentage with percentage-of-percentage", share: `{"percentage":60,"percentageOfPercentage":50}`},
		{name: "percentage-of-percentage at the upper bound", share: `{"percentage":60,"percentageOfPercentage":100}`},
		{
			// Zero and omitted are indistinguishable on an int64, so both land on
			// `required`: the leg would produce no operation row while the transaction
			// still commits.
			name: "zero percentage", share: `{"percentage":0}`,
			wantErr: true, wantCode: constant.ErrMissingFieldsInRequest,
			wantField: "percentage", wantRule: "is a required field",
		},
		{
			name: "omitted percentage", share: `{}`,
			wantErr: true, wantCode: constant.ErrMissingFieldsInRequest,
			wantField: "percentage", wantRule: "is a required field",
		},
		{
			// A negative share inverts the leg: a source leg becomes a credit.
			name: "negative percentage", share: `{"percentage":-50}`,
			wantErr: true, wantCode: constant.ErrBadRequest,
			wantField: "percentage", wantRule: "must be greater than 0",
		},
		{
			name: "negative percentage-of-percentage", share: `{"percentage":60,"percentageOfPercentage":-1}`,
			wantErr: true, wantCode: constant.ErrBadRequest,
			wantField: "percentageOfPercentage", wantRule: "must be 0 or greater",
		},
		{
			name: "percentage-of-percentage over 100", share: `{"percentage":60,"percentageOfPercentage":101}`,
			wantErr: true, wantCode: constant.ErrBadRequest,
			wantField: "percentageOfPercentage", wantRule: "must be 100 or less",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := `{"asset":"BRL","amount":"100",` +
				`"sources":[{"account":"@srcA","share":` + tt.share + `}],` +
				`"destinations":[{"account":"@dstA","amount":"100"}]}`

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(body), &in)
			if tt.wantErr {
				require.Error(t, err, "share %s must be rejected", tt.share)

				fields := requireKnownFieldsError(t, err, tt.wantCode)
				require.Containsf(t, fields, tt.wantField,
					"the rejection must key on the offending share field")
				assert.Contains(t, fields[tt.wantField], tt.wantRule,
					"the rejection must state the violated bound")

				return
			}

			require.NoError(t, err, "share %s must be accepted", tt.share)
		})
	}
}

// TestV2LegInput_AccountRequiredByTag proves the leg account obligation is a struct tag, not
// an imperative check inside the translator: the tag path runs at the decode boundary and
// names the offending leg by index, which a translator-side check cannot do.
func TestV2LegInput_AccountRequiredByTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantPath string
	}{
		{
			name: "second source leg without an account",
			body: `{"asset":"BRL","amount":"100",` +
				`"sources":[{"account":"@srcA","amount":"60"},{"amount":"40"}],` +
				`"destinations":[{"account":"@dstA","amount":"100"}]}`,
			wantPath: "sources[1].account is a required field",
		},
		{
			name: "second destination leg without an account",
			body: `{"asset":"BRL","amount":"100",` +
				`"sources":[{"account":"@srcA","amount":"100"}],` +
				`"destinations":[{"account":"@dstA","amount":"60"},{"amount":"40"}]}`,
			wantPath: "destinations[1].account is a required field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(tt.body), &in)
			require.Error(t, err, "a leg without an account must be rejected at decode")

			fields := requireKnownFieldsError(t, err, constant.ErrMissingFieldsInRequest)
			assert.Equal(t, tt.wantPath, fields["account"],
				"the rejection must name the offending leg by index")
		})
	}
}

// TestCreateTransactionV2Input_EmptyScalarSideIsAKnownField pins the reason `from` and `to`
// carry no json `omitempty`: with it, an explicitly empty `"from": ""` vanishes from the
// re-marshal the decoder diffs against the submitted body and comes back as an UNKNOWN
// field, telling the client to remove a field the contract accepts.
func TestCreateTransactionV2Input_EmptyScalarSideIsAKnownField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "explicit empty from",
			body: `{"asset":"BRL","amount":"100","from":"","destinations":[{"account":"@dstA","amount":"100"}]}`,
		},
		{
			name: "explicit empty to",
			body: `{"asset":"BRL","amount":"100","sources":[{"account":"@srcA","amount":"100"}],"to":""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(tt.body), &in)
			require.NoError(t, err, "an explicitly empty scalar side must stay a known field")
		})
	}
}

// TestV2LegInput_AccountRejectsConcatMarker locks the narrow alias guard on the leg account.
//
// A leg alias is rewritten in place into the composite "index#alias#balanceKey" form before the
// funnel keys its per-leg map on it, and the "already composite?" test is "starts with digits
// followed by #". A client that submits an alias already in that shape therefore keeps its own
// spelling, and two legs can be forged onto ONE map key — last write wins, and the difference
// is minted. `#` is the whole vector, so rejecting that single character closes it.
//
// The guard is deliberately NOT the registered `invalidaliascharacters` charset: that charset
// also excludes `/`, which would reject `@external/<ASSET>` — the alias every ledger's external
// account carries, and the only way to spell funding or withdrawal on the v2 surface, which
// publishes no inflow/outflow action. The accepted cases below are that regression, locked.
func TestV2LegInput_AccountRejectsConcatMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		account string
		wantErr bool
	}{
		{
			name:    "forged composite alias is rejected",
			account: "0#@a#default",
			wantErr: true,
		},
		{
			name:    "any hash in the alias is rejected",
			account: "@a#b",
			wantErr: true,
		},
		{
			// The alias of the ledger's external account. It contains `/`, which the
			// registered alias charset excludes — applying that charset here would 400 every
			// deposit and withdrawal spelled in the leg-array form.
			name:    "external account alias is accepted",
			account: "@external/USD",
		},
		{
			name:    "plain alias is accepted",
			account: "@alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := arrayV2Input(
				[]mtransaction.V2LegInput{{Account: tt.account, Amount: "1000"}},
				[]mtransaction.V2LegInput{{Account: "@person2", Amount: "1000"}},
			)

			got, err := in.Translate(false)
			if tt.wantErr {
				require.Error(t, err, "a leg alias carrying `#` must be rejected")

				var vErr pkg.ValidationError
				require.ErrorAs(t, err, &vErr, "an invalid alias character is a request-shape error (400)")
				assert.Equal(t, constant.ErrAccountAliasInvalid.Error(), vErr.Code)
				assert.True(t, got.IsEmpty(), "the error path must not leak a populated transaction")

				return
			}

			require.NoError(t, err, "alias %q must be accepted", tt.account)
			require.Len(t, got.Send.Source.From, 1)
			assert.Equal(t, tt.account, got.Send.Source.From[0].AccountAlias,
				"an accepted alias must reach the canonical leg unchanged")
		})
	}
}

// TestV2LegInput_ConcatMarkerRejectedOnBothSides asserts the guard covers destination legs too.
// A guard applied to one side only passes every source-side case while leaving the credit side
// forgeable.
func TestV2LegInput_ConcatMarkerRejectedOnBothSides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input mtransaction.CreateTransactionV2Input
	}{
		{
			name: "source leg",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "0#@a#default", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Account: "@person2", Amount: "1000"}},
			),
		},
		{
			name: "destination leg",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "@person1", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Account: "0#@a#default", Amount: "1000"}},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.input.Translate(false)
			require.Error(t, err)

			var vErr pkg.ValidationError
			require.ErrorAs(t, err, &vErr)
			assert.Equal(t, constant.ErrAccountAliasInvalid.Error(), vErr.Code)
		})
	}
}
