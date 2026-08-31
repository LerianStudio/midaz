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

// legArrayBody spells a body whose debit array holds count legs, each with an explicit
// amount, so a test can push the array past the published per-side cap without any other
// field changing.
func legArrayBody(count int) string {
	legs := make([]string, 0, count)

	for i := range count {
		legs = append(legs, `{"alias":"@src`+strconv.Itoa(i)+`",`+scopeJSON+`,"amount":"1"}`)
	}

	return `{"asset":"BRL","amount":"` + strconv.Itoa(count) + `",` +
		`"debits":[` + strings.Join(legs, ",") + `],` +
		`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"` + strconv.Itoa(count) + `"}]}`
}

// shareLegsAt spells a debit array whose leg at index carries the given share expression and
// whose preceding legs carry a valid one, so a share rule can be aimed at any index.
func shareLegsAt(index int, share string) string {
	legs := make([]string, 0, index+1)

	for i := range index {
		legs = append(legs, `{"alias":"@filler`+strconv.Itoa(i)+`",`+scopeJSON+`,"share":{"percentage":1}}`)
	}

	return strings.Join(append(legs, `{"alias":"@srcA",`+scopeJSON+`,"share":`+share+`}`), ",")
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
		`"debits":[{"alias":"@srcA",` + scopeJSON + `,"amount":"60"},{"alias":"@srcB",` + scopeJSON + `,"remaining":true}],` +
		`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"100"}]}`

	var in mtransaction.CreateTransactionV2Input

	_, err := nethttp.DecodeAndValidate([]byte(body), &in)
	require.Error(t, err, "a leg spelling `remaining` must be rejected")

	var unknown pkg.ValidationUnknownFieldsError
	require.ErrorAs(t, err, &unknown, "an expression the leg does not publish is an unknown field")
	assert.Equal(t, constant.ErrUnexpectedFieldsInTheRequest.Error(), unknown.Code)
	assert.Equal(t, pkg.UnknownFields{"debits": []any{map[string]any{"remaining": true}}}, unknown.Fields,
		"the rejection must point the caller at the unsupported expression")
}

// TestV2LegInput_DescriptionIsAKnownField pins that a leg may spell its own `description`: the
// description the operation that leg produces is persisted with.
//
// The decoder answers any field the struct does not publish with the unknown-field rejection, so
// a leg description is only reachable while the field stays on the struct — dropping it turns
// every describing body into a 400. Both sides are swept because a debit and a credit produce
// separate operations, so each has to carry its own value rather than share one.
func TestV2LegInput_DescriptionIsAKnownField(t *testing.T) {
	t.Parallel()

	body := `{"asset":"BRL","amount":"100","description":"transaction note",` +
		`"debits":[{"alias":"@srcA",` + scopeJSON + `,"amount":"100","description":"debit leg note"}],` +
		`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"100","description":"credit leg note"}]}`

	var in mtransaction.CreateTransactionV2Input

	_, err := nethttp.DecodeAndValidate([]byte(body), &in)
	require.NoError(t, err, "a leg spelling `description` must decode as a known field")

	require.Len(t, in.Debits, 1)
	assert.Equal(t, "debit leg note", in.Debits[0].Description)

	require.Len(t, in.Credits, 1)
	assert.Equal(t, "credit leg note", in.Credits[0].Description)

	assert.Equal(t, "transaction note", in.Description,
		"a leg description must decode alongside the transaction-level one, not over it")
}

// TestV2LegInput_DescriptionLengthBound locks the published 256-character ceiling on a leg
// description. The bound is what keeps an oversized value a clean 400 naming the field instead of
// a persistence failure raised after the transaction has already been assembled.
func TestV2LegInput_DescriptionLengthBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		length  int
		wantErr bool
	}{
		{name: "at the cap", length: 256},
		{name: "one past the cap", length: 257, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := `{"asset":"BRL","amount":"100",` +
				`"debits":[{"alias":"@srcA",` + scopeJSON + `,"amount":"100",` +
				`"description":"` + strings.Repeat("d", tt.length) + `"}],` +
				`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"100"}]}`

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(body), &in)
			if tt.wantErr {
				require.Error(t, err, "a %d-character leg description must be rejected", tt.length)

				fields := requireKnownFieldsError(t, err, constant.ErrBadRequest)
				assert.Contains(t, fields, "description",
					"the rejection must name the field the caller has to shorten")

				return
			}

			require.NoError(t, err, "a %d-character leg description must be accepted", tt.length)
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
				assert.Equal(t, "debits must contain at maximum 500 items", fields["debits"],
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
// Both factors carry the same bounds at the top, so a leg cannot escape the cap by moving the
// large number from one factor to the other. Their LOWER bounds differ: 0 means "no narrowing"
// on PercentageOfPercentage, while a zero Percentage is a leg that moves nothing.
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
		{name: "percentage at the upper bound", share: `{"percentage":100,"percentageOfPercentage":50}`},
		{
			name: "percentage over 100", share: `{"percentage":101}`,
			wantErr: true, wantCode: constant.ErrBadRequest,
			wantField: "percentage", wantRule: "must be 100 or less",
		},
		{
			name: "percentage over 100 on the second leg", share: `{"percentage":101}`, atIndex: 1,
			wantErr: true, wantCode: constant.ErrBadRequest,
			wantField: "percentage", wantRule: "must be 100 or less",
		},
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
			// A negative share inverts the leg: a debit leg becomes a credit.
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
				`"debits":[` + shareLegsAt(tt.atIndex, tt.share) + `],` +
				`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"100"}]}`

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

// TestV2ShareInput_ResolvesPerLegAmounts pins what each share expression RESOLVES TO, per leg.
// The resolver computes total x (percentage/100) x (percentageOfPercentage/100), and the per-leg
// figure is the only observable that distinguishes one factor pair from another: the side's sum
// and the transaction total agree for many wrong splits, and Responses.Total is assigned from the
// amount the body declared rather than from anything the resolver derived.
//
// Every balancing row is asymmetric in BOTH factors, so dropping either one of them out of the
// product changes some leg's resolved amount instead of being absorbed by a sibling. The
// per-factor bounds themselves are locked in TestV2ShareInput_PercentageBounds.
func TestV2ShareInput_ResolvesPerLegAmounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		debitLegs string
		// wantPerLeg maps each debit alias to the amount the resolver must land on.
		// Empty means the side cannot balance and the funnel must reject it.
		wantPerLeg map[string]string
	}{
		{
			// 100 x 1.00 x 0.75 = 75 and 100 x 0.50 x 0.50 = 25. Percentage alone would sum
			// 150, PercentageOfPercentage alone 125 — neither reaches the total.
			name: "narrowed shares within both bounds",
			debitLegs: `{"alias":"@srcA",` + scopeJSON + `,"share":{"percentage":100,"percentageOfPercentage":75}},` +
				`{"alias":"@srcB",` + scopeJSON + `,"share":{"percentage":50,"percentageOfPercentage":50}}`,
			wantPerLeg: map[string]string{"@srcA": "75", "@srcB": "25"},
		},
		{
			// A zero narrowing factor means NO narrowing, so this leg takes the whole
			// Percentage: 100 x 0.50 x 1.00 = 50. Were zero read as a zero share the leg would
			// resolve to 0 and the side would sum to 50 against a 100 total.
			name: "a zero narrowing factor takes the whole percentage",
			debitLegs: `{"alias":"@srcA",` + scopeJSON + `,"share":{"percentage":50,"percentageOfPercentage":0}},` +
				`{"alias":"@srcB",` + scopeJSON + `,"share":{"percentage":50}}`,
			wantPerLeg: map[string]string{"@srcA": "50", "@srcB": "50"},
		},
		{
			// 100 x 0.80 x 0.25 = 20 and 100 x 1.00 x 0.80 = 80. A second witness with the
			// factors asymmetric the other way round, so neither leg's pair can be swapped for
			// the other's without changing what it resolves to.
			name: "asymmetric factors on both legs",
			debitLegs: `{"alias":"@srcA",` + scopeJSON + `,"share":{"percentage":80,"percentageOfPercentage":25}},` +
				`{"alias":"@srcB",` + scopeJSON + `,"share":{"percentage":100,"percentageOfPercentage":80}}`,
			wantPerLeg: map[string]string{"@srcA": "20", "@srcB": "80"},
		},
		{
			// Two legs at the full total each: 100 + 100 against a 100 total. Both factors stay
			// within their bounds, so nothing at decode can see the overshoot and the funnel's
			// whole-body total check is what rejects it.
			name: "in-range shares that overshoot the total",
			debitLegs: `{"alias":"@srcA",` + scopeJSON + `,"share":{"percentage":100}},` +
				`{"alias":"@srcB",` + scopeJSON + `,"share":{"percentage":100}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := `{"asset":"BRL","amount":"100",` +
				`"debits":[` + tt.debitLegs + `],` +
				`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"100"}]}`

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(body), &in)
			require.NoError(t, err, "a share expression is a whole-body property, not a request-shape violation")

			transaction, _, err := in.Translate(false)
			require.NoError(t, err, "Translate carries share expressions forward without resolving them")

			responses, err := mtransaction.ValidateSendSourceAndDistribute(
				context.Background(), transaction, constant.CREATED,
			)

			if tt.wantPerLeg == nil {
				require.Error(t, err, "a side that cannot sum to the total must be rejected by the funnel")

				// The sentinel, not merely the business class: the ambiguity guard in the same
				// function is also a business rejection, so asserting the class alone would
				// pass for a rejection this row is not about.
				var vErr pkg.UnprocessableOperationError
				require.ErrorAs(t, err, &vErr)
				assert.Equal(t, constant.ErrTransactionValueMismatch.Error(), vErr.Code,
					"the rejection must be the whole-body total check, not another business guard")

				return
			}

			require.NoError(t, err, "a side whose narrowed shares sum to the total must be accepted")
			require.NotNil(t, responses)

			for alias, want := range tt.wantPerLeg {
				resolved, ok := responses.From[alias]
				require.Truef(t, ok, "the resolved side must carry an entry for %s", alias)
				assert.Equalf(t, want, resolved.Value.String(),
					"leg %s must resolve to its share of the transaction total", alias)
			}
		})
	}
}

// TestV2LegInput_AliasRequired proves the leg alias obligation is enforced twice, and that
// the two guards are complementary rather than redundant. This case exercises the struct tag,
// which is the guard every HTTP caller meets: it fires at the decode boundary, before Translate
// runs. The imperative sibling in buildLeg is what covers a caller that builds the input in Go
// and never runs it through the decoder (TestCreateTransactionV2Input_Translate's
// "leg without an alias is rejected").
//
// Both name the offending entry by index. The tag does so inside the rendered message, which
// the shared decoder builds from the validator's full field namespace; the map KEY stays the
// bare leaf name.
func TestV2LegInput_AliasRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantPath string
	}{
		{
			name: "second debit leg without an alias",
			body: `{"asset":"BRL","amount":"100",` +
				`"debits":[{"alias":"@srcA",` + scopeJSON + `,"amount":"60"},{"amount":"40"}],` +
				`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"100"}]}`,
			wantPath: "debits[1].alias is a required field",
		},
		{
			name: "second credit leg without an alias",
			body: `{"asset":"BRL","amount":"100",` +
				`"debits":[{"alias":"@srcA",` + scopeJSON + `,"amount":"100"}],` +
				`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"60"},{"amount":"40"}]}`,
			wantPath: "credits[1].alias is a required field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(tt.body), &in)
			require.Error(t, err, "a leg without an alias must be rejected at decode")

			fields := requireKnownFieldsError(t, err, constant.ErrMissingFieldsInRequest)
			assert.Equal(t, tt.wantPath, fields["alias"],
				"the rejection must name the offending leg by index")
		})
	}
}

// TestV2LegInput_NonPositivePercentageRejectedByTranslate proves Translate rejects a share leg
// whose percentage is not positive on its own, rather than relying on the decoder's `gt=0` tag.
// Translate is exported from a shared package, so a caller that assembles the input in Go and
// never runs DecodeAndValidate gets no tag evaluation — and the funnel resolves a zero-percentage
// leg to no operation row at all while the transaction still commits, or inverts the leg's
// accounting direction when the percentage is negative.
//
// This mirrors the alias obligation, which is enforced by tag and imperatively for the same
// reason. The tag stays because only it names the offending leg's field in the decode rejection.
func TestV2LegInput_NonPositivePercentageRejectedByTranslate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		share *mtransaction.V2ShareInput
	}{
		{name: "zero percentage", share: &mtransaction.V2ShareInput{Percentage: 0}},
		{name: "negative percentage", share: &mtransaction.V2ShareInput{Percentage: -50}},
		{
			name:  "zero percentage with a narrowing factor",
			share: &mtransaction.V2ShareInput{Percentage: 0, PercentageOfPercentage: 50},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Share: tt.share}},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			)

			got, _, err := input.Translate(false)
			require.Error(t, err, "a share leg with a non-positive percentage must not reach the funnel")

			// Same sentinel the explicit-amount arm raises for a non-positive value, so both
			// value expressions answer a non-positive leg alike.
			var vErr pkg.UnprocessableOperationError
			require.ErrorAs(t, err, &vErr)
			assert.Equal(t, constant.ErrInvalidTransactionNonPositiveValue.Error(), vErr.Code)
			assert.True(t, got.IsEmpty(), "the error path must not leak a populated transaction")
		})
	}
}

// TestCreateTransactionV2Input_EmptySideArrayIsAKnownField pins that an explicit `"debits": []`
// (or `"credits": []`) stays a KNOWN field: the array carries no json `omitempty`, so it
// survives the re-marshal the decoder diffs against the submitted body and is answered by the
// `min=1` struct-validation rejection instead of the unknown-field one. Silently dropping the
// key would leave the client unable to tell "this side is empty" apart from "this side does not
// exist".
func TestCreateTransactionV2Input_EmptySideArrayIsAKnownField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "explicit empty debits",
			body: `{"asset":"BRL","amount":"100","debits":[],"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"100"}]}`,
		},
		{
			name: "explicit empty credits",
			body: `{"asset":"BRL","amount":"100","debits":[{"alias":"@srcA",` + scopeJSON + `,"amount":"100"}],"credits":[]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(tt.body), &in)
			require.Error(t, err, "an explicitly empty side array must be rejected by min=1")

			var unknown pkg.ValidationUnknownFieldsError
			require.NotErrorAs(t, err, &unknown,
				"an explicit [] must stay a KNOWN field, answered by min=1, not by the unknown-field class")
		})
	}
}

// TestCreateTransactionV2Input_RemovedFieldsAreUnknown pins that the retired scalar fields
// (`from`, `to`) and their retired array names (`sources`, `destinations`) are answered as
// unknown fields now that the wire contract exposes only `debits` and `credits`. A caller still
// spelling the old contract must be told the field does not exist, not have it silently dropped.
func TestCreateTransactionV2Input_RemovedFieldsAreUnknown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		body  string
		field string
	}{
		{
			name: "from",
			body: `{"asset":"BRL","amount":"100","from":{"alias":"@srcA",` + scopeJSON + `},` +
				`"debits":[{"alias":"@srcB",` + scopeJSON + `,"amount":"100"}],` +
				`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"100"}]}`,
			field: "from",
		},
		{
			name: "to",
			body: `{"asset":"BRL","amount":"100","to":{"alias":"@dstA",` + scopeJSON + `},` +
				`"debits":[{"alias":"@srcA",` + scopeJSON + `,"amount":"100"}],` +
				`"credits":[{"alias":"@dstB",` + scopeJSON + `,"amount":"100"}]}`,
			field: "to",
		},
		{
			name: "sources",
			body: `{"asset":"BRL","amount":"100","sources":[{"alias":"@srcA",` + scopeJSON + `,"amount":"100"}],` +
				`"debits":[{"alias":"@srcB",` + scopeJSON + `,"amount":"100"}],` +
				`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"100"}]}`,
			field: "sources",
		},
		{
			name: "destinations",
			body: `{"asset":"BRL","amount":"100","destinations":[{"alias":"@dstA",` + scopeJSON + `,"amount":"100"}],` +
				`"debits":[{"alias":"@srcA",` + scopeJSON + `,"amount":"100"}],` +
				`"credits":[{"alias":"@dstB",` + scopeJSON + `,"amount":"100"}]}`,
			field: "destinations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(tt.body), &in)
			require.Errorf(t, err, "a body carrying the retired %q field must be rejected", tt.field)

			var unknown pkg.ValidationUnknownFieldsError
			require.ErrorAs(t, err, &unknown, "a field the current contract does not expose is an unknown field")
			assert.Equal(t, constant.ErrUnexpectedFieldsInTheRequest.Error(), unknown.Code)
			assert.Contains(t, unknown.Fields, tt.field,
				"the rejection must name the retired field the client sent")
		})
	}
}

// v2AliasPositions enumerates the two aliases the v2 surface accepts: one per side. Each entry
// plants the caller's alias in ONE leg and fills the other side with a valid value, and reads the
// alias back off the leg the planted side produced. Sweeping the alias rules over both is what
// pins that a rule covers the whole surface.
var v2AliasPositions = []struct {
	name  string
	build func(alias string) mtransaction.CreateTransactionV2Input
	read  func(mtransaction.Transaction) string
}{
	{
		name: "debit leg",
		build: func(alias string) mtransaction.CreateTransactionV2Input {
			return arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: alias, Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@person2", Amount: "1000"}},
			)
		},
		read: func(tr mtransaction.Transaction) string { return tr.Send.Source.From[0].AccountAlias },
	},
	{
		name: "credit leg",
		build: func(alias string) mtransaction.CreateTransactionV2Input {
			return arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@person1", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: alias, Amount: "1000"}},
			)
		},
		read: func(tr mtransaction.Transaction) string { return tr.Send.Distribute.To[0].AccountAlias },
	},
}

// TestV2Alias_RejectsConcatMarkerOnEveryPosition locks the narrow alias guard across both
// aliases the surface accepts.
//
// An alias is rewritten in place into the composite "index#alias#balanceKey" form before
// downstream code keys its per-entry maps on it, and the "already composite?" test is "starts
// with digits followed by the separator" — see TestIsConcatedAlias. An alias submitted in that
// shape therefore keeps the client's own spelling and reaches those maps unmutated, where it can
// collide with another entry's key or match none of them. Either way an entry is lost, and a
// transaction that loses one side's entry moves value in one direction only. The separator is the
// whole vector, so rejecting that single character closes it.
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

				got, _, err := position.build(alias).Translate(false)
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
// inflow/outflow action.
func TestV2Alias_AcceptedAliasReachesTheLegUnchanged(t *testing.T) {
	t.Parallel()

	accepted := []string{"@external/USD", "@alice"}

	for _, position := range v2AliasPositions {
		for _, alias := range accepted {
			t.Run(position.name+" "+alias, func(t *testing.T) {
				t.Parallel()

				got, _, err := position.build(alias).Translate(false)
				require.NoErrorf(t, err, "alias %q must be accepted on the %s position", alias, position.name)

				require.Len(t, got.Send.Source.From, 1)
				require.Len(t, got.Send.Distribute.To, 1)
				assert.Equal(t, alias, position.read(got),
					"an accepted alias must reach the canonical leg unchanged")
			})
		}
	}
}

// TestV2LegInput_EmptyAliasRejectedByTranslate proves Translate rejects a leg with no
// account on its own, rather than relying on the decoder's `required` tag. Translate is
// exported from a shared package, so a caller that assembles the input in Go and never runs
// DecodeAndValidate gets no tag evaluation — and an empty alias reaching the funnel names no
// account at all. The tag stays because only it can name the offending leg by index in the
// missing-field rejection; the two guards are complementary, not redundant.
func TestV2LegInput_EmptyAliasRejectedByTranslate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input mtransaction.CreateTransactionV2Input
	}{
		{
			name: "debit leg",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@person2", Amount: "1000"}},
			),
		},
		{
			name: "credit leg",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@person1", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "", Amount: "1000"}},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, _, err := tt.input.Translate(false)
			require.Error(t, err, "a leg with no alias must not reach the canonical transaction")

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
			name: "second debit leg carries neither expression",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "60"}, {Alias: "@srcB"}},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "100"}},
			),
			wantRef:  "debits[1]",
			wantCode: constant.ErrInvalidTransactionType,
		},
		{
			name: "second credit leg carries both expressions",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "100"}},
				[]mtransaction.V2LegInput{
					{Alias: "@dstA", Amount: "60"},
					{Alias: "@dstB", Amount: "40", Share: &mtransaction.V2ShareInput{Percentage: 40}},
				},
			),
			wantRef:  "credits[1]",
			wantCode: constant.ErrInvalidTransactionType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := tt.input.Translate(false)
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
