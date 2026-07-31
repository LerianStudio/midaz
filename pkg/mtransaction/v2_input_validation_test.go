// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction_test

import (
	"context"
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

// shareLegsAt spells a source array whose leg at index carries the given share expression and
// whose preceding legs carry a valid one, so a share rule can be aimed at any index.
func shareLegsAt(index int, share string) string {
	legs := make([]string, 0, index+1)

	for i := range index {
		legs = append(legs, `{"account":"@filler`+strconv.Itoa(i)+`","share":{"percentage":1}}`)
	}

	return strings.Join(append(legs, `{"account":"@srcA","share":`+share+`}`), ",")
}

// TestV2LegInput_NoRemainingExpression locks the v2 leg down to the two value expressions
// the surface publishes. A `remaining` leg resolves during validation but contributes no
// operation row and no balance movement, committing an unbalanced transaction, so the v2
// surface must not offer the expression at all: with no field to decode into, a client that
// sends one gets the decoder's unknown-field rejection.
//
// Rejecting the submitted expression is the whole guarantee, so it is asserted directly. A
// reflective "the struct has no Remaining field" assertion would restate the same rule one
// layer earlier and fail for the same reason, which makes it a change detector rather than a
// second guarantee.
func TestV2LegInput_NoRemainingExpression(t *testing.T) {
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

// TestCreateTransactionV2Input_LegArrayCap locks the published per-side leg cap. It is the only
// bound on the leg count: the request-body byte limit alone admits tens of thousands of legs,
// each carrying its own downstream cost.
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
//
// Only PercentageOfPercentage carries an upper bound. Percentage has none: it is a factor the
// narrowing field can scale back down, so a value above 100 is not on its own unbalanceable
// (see TestV2ShareInput_NarrowedPercentageAboveOneHundredBalances). Whether a side's legs sum
// to the transaction total is a whole-body property, which a per-field bound cannot decide.
//
// atIndex places the offending share on a leg other than the first, which is what pins that
// the per-leg tags are evaluated at EVERY index rather than only at the head of the array.
// The rendered field message carries no index — that is a property of the shared decoder, not
// of these tags — so the index case asserts that the tag fires, not what it prints.
func TestV2ShareInput_PercentageBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		share     string
		atIndex   int
		wantErr   bool
		wantCode  error
		wantField string
		wantRule  string
	}{
		{name: "positive percentage", share: `{"percentage":100}`},
		{name: "percentage with percentage-of-percentage", share: `{"percentage":60,"percentageOfPercentage":50}`},
		{name: "percentage-of-percentage at the upper bound", share: `{"percentage":60,"percentageOfPercentage":100}`},
		{name: "percentage above 100 carries no field bound", share: `{"percentage":101}`},
		{name: "percentage above 100 on the second leg", share: `{"percentage":101}`, atIndex: 1},
		{
			name: "negative percentage on the second leg", share: `{"percentage":-50}`, atIndex: 1,
			wantErr: true, wantCode: constant.ErrBadRequest,
			wantField: "percentage", wantRule: "must be greater than 0",
		},
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
				`"sources":[` + shareLegsAt(tt.atIndex, tt.share) + `],` +
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

// TestV2ShareInput_NarrowedPercentageAboveOneHundredBalances pins why Percentage carries no
// upper bound. The resolver computes total x (percentage/100) x (percentageOfPercentage/100),
// so PercentageOfPercentage scales Percentage back down: a leg reading 150% narrowed to 50%
// resolves to 75% of the total, and paired with a 50%-narrowed-to-50% sibling the side sums to
// exactly the total. A per-field cap on Percentage answers that balanced body 400 at decode.
//
// The genuinely unbalanceable spelling is covered too: it decodes, and the funnel's whole-body
// total check is what rejects it. That is the layer that can actually see the sum.
func TestV2ShareInput_NarrowedPercentageAboveOneHundredBalances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sourceLegs  string
		wantBalance bool
	}{
		{
			// 100 x 1.50 x 0.50 = 75 and 100 x 0.50 x 0.50 = 25, summing to the total.
			name: "narrowed shares above and below 100 sum to the total",
			sourceLegs: `{"account":"@srcA","share":{"percentage":150,"percentageOfPercentage":50}},` +
				`{"account":"@srcB","share":{"percentage":50,"percentageOfPercentage":50}}`,
			wantBalance: true,
		},
		{
			// 100 x 2.00 = 200 against a 100 total: no sibling narrows it back down.
			name:       "an unnarrowed share above 100 cannot balance",
			sourceLegs: `{"account":"@srcA","share":{"percentage":200}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := `{"asset":"BRL","amount":"100",` +
				`"sources":[` + tt.sourceLegs + `],` +
				`"destinations":[{"account":"@dstA","amount":"100"}]}`

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(body), &in)
			require.NoError(t, err, "a share expression is a whole-body property, not a request-shape violation")

			transaction, err := in.Translate(false)
			require.NoError(t, err, "Translate carries share expressions forward without resolving them")

			ctx := context.Background()

			responses, err := mtransaction.ValidateSendSourceAndDistribute(ctx, transaction, constant.CREATED)

			if !tt.wantBalance {
				require.Error(t, err, "a side that cannot sum to the total must be rejected by the funnel")
				assert.True(t, pkg.IsBusinessError(err),
					"an unbalanceable body is a business rejection, not a technical failure")

				return
			}

			require.NoError(t, err, "a side whose narrowed shares sum to the total must be accepted")
			require.NotNil(t, responses)
			assert.Equal(t, "100", responses.Total.String(),
				"the resolved side must sum to the transaction total")
		})
	}
}

// TestV2LegInput_AccountRequired proves the leg account obligation is enforced twice, and that
// the two guards are complementary rather than redundant. This case exercises the struct tag,
// which is the guard every HTTP caller meets: it fires at the decode boundary, before Translate
// runs. The imperative sibling in buildLeg is what covers a caller that builds the input in Go
// and never runs it through the decoder (TestCreateTransactionV2Input_Translate's
// "leg without an account is rejected").
//
// Both name the offending entry by index. The tag does so inside the rendered message, which
// the shared decoder builds from the validator's full field namespace; the map KEY stays the
// bare leaf name.
func TestV2LegInput_AccountRequired(t *testing.T) {
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

// v2AliasPositions enumerates the four aliases the v2 surface accepts: one per side, per
// spelling. Each entry plants the caller's alias in ONE of them and fills the other side with
// a valid value, and reads the alias back off the leg the planted side produced. Sweeping the
// alias rules over all four is what pins that a rule covers the whole surface: a guard on the
// leg arrays alone passes every leg case while leaving both scalar fields forgeable.
var v2AliasPositions = []struct {
	name  string
	build func(alias string) mtransaction.CreateTransactionV2Input
	read  func(mtransaction.Transaction) string
}{
	{
		name: "source leg",
		build: func(alias string) mtransaction.CreateTransactionV2Input {
			return arrayV2Input(
				[]mtransaction.V2LegInput{{Account: alias, Amount: "1000"}},
				[]mtransaction.V2LegInput{{Account: "@person2", Amount: "1000"}},
			)
		},
		read: func(tr mtransaction.Transaction) string { return tr.Send.Source.From[0].AccountAlias },
	},
	{
		name: "destination leg",
		build: func(alias string) mtransaction.CreateTransactionV2Input {
			return arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "@person1", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Account: alias, Amount: "1000"}},
			)
		},
		read: func(tr mtransaction.Transaction) string { return tr.Send.Distribute.To[0].AccountAlias },
	},
	{
		name: "scalar from",
		build: func(alias string) mtransaction.CreateTransactionV2Input {
			return scalarV2Input(alias, "@person2")
		},
		read: func(tr mtransaction.Transaction) string { return tr.Send.Source.From[0].AccountAlias },
	},
	{
		name: "scalar to",
		build: func(alias string) mtransaction.CreateTransactionV2Input {
			return scalarV2Input("@person1", alias)
		},
		read: func(tr mtransaction.Transaction) string { return tr.Send.Distribute.To[0].AccountAlias },
	},
}

// scalarV2Input returns a valid scalar-form CreateTransactionV2Input with the caller's two
// aliases and no leg arrays, so a test can aim the alias rules at the scalar spelling.
func scalarV2Input(from, to string) mtransaction.CreateTransactionV2Input {
	in := validV2Input()
	in.From = from
	in.To = to

	return in
}

// TestV2Alias_RejectsConcatMarkerOnEveryPosition locks the narrow alias guard across all four
// aliases the surface accepts.
//
// An alias is rewritten in place into the composite "index#alias#balanceKey" form before
// downstream code keys its per-entry maps on it, and the "already composite?" test is "starts
// with digits followed by the separator" — see TestIsConcatedAlias. An alias submitted in that
// shape therefore keeps the client's own spelling and reaches those maps unmutated, where it can
// collide with another entry's key or match none of them. Either way an entry is lost, and a
// transaction that loses one side's entry moves value in one direction only. The separator is the
// whole vector, so rejecting that single character closes it.
//
// The scalar spelling matters as much as the arrays: nothing else checks it, since the scalar
// alias has no per-leg tag behind it.
func TestV2Alias_RejectsConcatMarkerOnEveryPosition(t *testing.T) {
	t.Parallel()

	forged := []string{
		// The exact shape the composite test recognises.
		"0#@a#default",
		// Any `#` at all: the funnel's split keys on the separator wherever it sits.
		"@a#x",
	}

	for _, position := range v2AliasPositions {
		for _, alias := range forged {
			t.Run(position.name+" "+alias, func(t *testing.T) {
				t.Parallel()

				got, err := position.build(alias).Translate(false)
				require.Errorf(t, err, "alias %q must be rejected on the %s position", alias, position.name)

				var vErr pkg.ValidationError
				require.ErrorAs(t, err, &vErr, "an invalid alias character is a request-shape error (400)")
				assert.Equal(t, constant.ErrAccountAliasInvalid.Error(), vErr.Code)
				assert.True(t, got.IsEmpty(), "the error path must not leak a populated transaction")
			})
		}
	}
}

// TestV2Alias_AcceptedAliasReachesTheLegUnchanged pins that the guard is the single forbidden
// character and NOT the registered `invalidaliascharacters` charset. That charset also excludes
// `/`, which would reject `@external/<ASSET>` — the alias every ledger's external account
// carries, and the only way to spell funding or withdrawal on a surface that publishes no
// inflow/outflow action. Sweeping the same aliases over all four positions is what keeps the
// regression closed on the scalar spelling as well as the arrays.
func TestV2Alias_AcceptedAliasReachesTheLegUnchanged(t *testing.T) {
	t.Parallel()

	accepted := []string{"@external/USD", "@alice"}

	for _, position := range v2AliasPositions {
		for _, alias := range accepted {
			t.Run(position.name+" "+alias, func(t *testing.T) {
				t.Parallel()

				got, err := position.build(alias).Translate(false)
				require.NoErrorf(t, err, "alias %q must be accepted on the %s position", alias, position.name)

				require.Len(t, got.Send.Source.From, 1)
				require.Len(t, got.Send.Distribute.To, 1)
				assert.Equal(t, alias, position.read(got),
					"an accepted alias must reach the canonical leg unchanged")
			})
		}
	}
}

// TestV2LegInput_EmptyAccountRejectedByTranslate proves Translate rejects a leg with no
// account on its own, rather than relying on the decoder's `required` tag. Translate is
// exported from a shared package, so a caller that assembles the input in Go and never runs
// DecodeAndValidate gets no tag evaluation — and an empty alias reaching the funnel names no
// account at all. The tag stays because only it can name the offending leg by index in the
// missing-field rejection; the two guards are complementary, not redundant.
func TestV2LegInput_EmptyAccountRejectedByTranslate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input mtransaction.CreateTransactionV2Input
	}{
		{
			name: "source leg",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Account: "@person2", Amount: "1000"}},
			),
		},
		{
			name: "destination leg",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "@person1", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Account: "", Amount: "1000"}},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.input.Translate(false)
			require.Error(t, err, "a leg with no account must not reach the canonical transaction")

			var vErr pkg.ValidationError
			require.ErrorAs(t, err, &vErr)
			assert.Equal(t, constant.ErrMissingFieldsInRequest.Error(), vErr.Code)
			assert.True(t, got.IsEmpty(), "the error path must not leak a populated transaction")
		})
	}
}

// TestV2LegInput_ValueExpressionErrorNamesTheLeg locks the two properties of the
// one-value-expression rejection that a caller at the 500-leg cap depends on:
//
//   - the message names the two expressions THIS surface accepts. The sentinel is shared with
//     the detailed body, which accepts a third (`remaining`) that v2 does not publish; naming
//     it here would send the caller into an unresolvable loop, because adding `remaining` to a
//     v2 leg is answered with a different 400.
//   - the message names the offending leg by INDEX, matching the shape the decoder's per-leg
//     tag rejections use, so both classes of leg error are equally locatable.
func TestV2LegInput_ValueExpressionErrorNamesTheLeg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    mtransaction.CreateTransactionV2Input
		wantRef  string
		wantCode error
	}{
		{
			name: "second source leg carries neither expression",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "@srcA", Amount: "60"}, {Account: "@srcB"}},
				[]mtransaction.V2LegInput{{Account: "@dstA", Amount: "100"}},
			),
			wantRef:  "sources[1]",
			wantCode: constant.ErrInvalidTransactionType,
		},
		{
			name: "second destination leg carries both expressions",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "@srcA", Amount: "100"}},
				[]mtransaction.V2LegInput{
					{Account: "@dstA", Amount: "60"},
					{Account: "@dstB", Amount: "40", Share: &mtransaction.V2ShareInput{Percentage: 40}},
				},
			),
			wantRef:  "destinations[1]",
			wantCode: constant.ErrInvalidTransactionType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.input.Translate(false)
			require.Error(t, err)

			var vErr pkg.ValidationError
			require.ErrorAs(t, err, &vErr)
			assert.Equal(t, tt.wantCode.Error(), vErr.Code)

			assert.Contains(t, vErr.Message, tt.wantRef,
				"the rejection must name the offending leg by index")
			assert.NotContains(t, vErr.Message, "remaining",
				"the v2 surface publishes no `remaining` expression, so the rejection must not offer it")
			assert.Contains(t, vErr.Message, "'amount' or 'share'",
				"the rejection must name the two expressions a v2 leg accepts")
		})
	}
}

// TestCreateTransactionV2Input_NullScalarSideIsRejected pins the answer a client gets for an
// explicit `"from": null`. Dropping json `omitempty` from the side fields is what keeps an
// explicit `""` a KNOWN field, and it also makes the re-marshal the decoder diffs against the
// submitted body always emit the key — so a submitted `null` never matches the emitted `""`
// and comes back as an unexpected field.
//
// The behaviour is pinned rather than smoothed over: the alternative is teaching the shared
// unknown-field decoder to treat null-against-present as a known field, which would change
// every endpoint's answer for every nullable field. A side is left unspelled by omitting it,
// which is what the published body description now says.
func TestCreateTransactionV2Input_NullScalarSideIsRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		body  string
		field string
	}{
		{
			name:  "null from",
			body:  `{"asset":"BRL","amount":"100","from":null,"to":"@dstA"}`,
			field: "from",
		},
		{
			name:  "null to",
			body:  `{"asset":"BRL","amount":"100","from":"@srcA","to":null}`,
			field: "to",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(tt.body), &in)
			require.Error(t, err, "an explicit null side field must be rejected")

			var unknown pkg.ValidationUnknownFieldsError
			require.ErrorAs(t, err, &unknown)
			assert.Equal(t, constant.ErrUnexpectedFieldsInTheRequest.Error(), unknown.Code)
			assert.Contains(t, unknown.Fields, tt.field,
				"the rejection must name the field the client nulled")
		})
	}
}
