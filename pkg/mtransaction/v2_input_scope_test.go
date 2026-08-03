// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	nethttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// otherLedgerID and otherOrgID are the second scope the disagreement cases plant on one leg.
// They differ from testLedgerID / testOrgID in every character position that matters, so a
// comparison that only looks at a prefix still sees two values.
const (
	otherOrgID    = "55555555-5555-5555-5555-555555555555"
	otherLedgerID = "66666666-6666-6666-6666-666666666666"
)

// TestCreateTransactionV2Input_TranslateResolvesScope pins that Translate hands the caller the
// (organization, ledger) pair the body named, for every spelling of the two sides. The scope is
// what the caller needs to know which ledger the transaction posts to, so a translation that
// succeeds without producing it is unusable.
//
// The mixed spelling is covered here as well: each side chooses independently, so a scalar payer
// paired with an array of payees is a valid request that must resolve one scope like any other.
func TestCreateTransactionV2Input_TranslateResolvesScope(t *testing.T) {
	t.Parallel()

	legRoute := "22222222-2222-2222-2222-222222222222"

	tests := []struct {
		name         string
		input        mtransaction.CreateTransactionV2Input
		wantFromLegs int
		wantToLegs   int
		verify       func(t *testing.T, got mtransaction.Transaction)
	}{
		{
			name:         "scalar on both sides",
			input:        validV2Input(),
			wantFromLegs: 1,
			wantToLegs:   1,
		},
		{
			name: "array on both sides with several legs per side",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{
					{Alias: "@srcA", Amount: "600"},
					{Alias: "@srcB", Amount: "400"},
				},
				[]mtransaction.V2LegInput{
					{Alias: "@dstA", Amount: "250"},
					{Alias: "@dstB", Amount: "750"},
				},
			),
			wantFromLegs: 2,
			wantToLegs:   2,
		},
		{
			// The Phase 4 per-side independence: `from` paired with `destinations` stays a
			// valid body, and the scope resolves across the two spellings.
			name: "scalar source with array destinations",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.To = mtransaction.V2AccountInput{}
				in.Destinations = scopedLegs([]mtransaction.V2LegInput{
					{Alias: "@dstA", Amount: "600"},
					{Alias: "@dstB", Amount: "400"},
				})

				return in
			}(),
			wantFromLegs: 1,
			wantToLegs:   2,
		},
		{
			// The mirror: `sources` paired with `to`.
			name: "array sources with scalar destination",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.From = mtransaction.V2AccountInput{}
				in.Sources = scopedLegs([]mtransaction.V2LegInput{
					{Alias: "@srcA", Amount: "600"},
					{Alias: "@srcB", Amount: "400"},
				})

				return in
			}(),
			wantFromLegs: 2,
			wantToLegs:   1,
		},
		{
			// Share expressions and the per-leg operation route are Phase 4 behaviour that the
			// scope must not disturb: the leg still carries its share and its own route.
			name: "share legs with a per-leg operation route",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{
					{Alias: "@srcA", Share: &mtransaction.V2ShareInput{Percentage: 60, PercentageOfPercentage: 50}, OperationRouteID: &legRoute},
					{Alias: "@srcB", Share: &mtransaction.V2ShareInput{Percentage: 70}},
				},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			),
			wantFromLegs: 2,
			wantToLegs:   1,
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				require.NotNil(t, got.Send.Source.From[0].Share)
				assert.Equal(t, int64(60), got.Send.Source.From[0].Share.Percentage)
				assert.Equal(t, int64(50), got.Send.Source.From[0].Share.PercentageOfPercentage)

				require.NotNil(t, got.Send.Source.From[0].RouteID)
				assert.Equal(t, "22222222-2222-2222-2222-222222222222", *got.Send.Source.From[0].RouteID,
					"a leg route must still override the request-level operation route")

				require.NotNil(t, got.Send.Source.From[1].RouteID)
				assert.Equal(t, "11111111-1111-1111-1111-111111111111", *got.Send.Source.From[1].RouteID,
					"a leg without its own route must still inherit the request-level one")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, scope, err := tt.input.Translate(false)
			require.NoError(t, err, "a body whose legs agree on one scope must translate")

			assert.Equal(t, testScope(), scope,
				"the resolved scope must be the pair the body named")

			require.Len(t, got.Send.Source.From, tt.wantFromLegs)
			require.Len(t, got.Send.Distribute.To, tt.wantToLegs)

			if tt.verify != nil {
				tt.verify(t, got)
			}
		})
	}
}

// TestCreateTransactionV2Input_TranslateRequiresLegScope pins that every leg must name all three
// parts of its account reference, in both spellings. A leg with no organization or no ledger says
// nothing about where its account lives, so there is no scope to resolve and nothing downstream
// can pick one for it.
//
// The obligation is checked at Translate and not only by the leg tags because Translate is
// exported from a shared package: a caller that builds the input in Go meets no tag at all.
func TestCreateTransactionV2Input_TranslateRequiresLegScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		input           mtransaction.CreateTransactionV2Input
		wantMessagePart string
	}{
		{
			name: "source leg without an alias",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			),
			wantMessagePart: "sources[0].alias",
		},
		{
			name: "source leg without an organization",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Sources[0].OrganizationID = ""
			}),
			wantMessagePart: "sources[0].organizationId",
		},
		{
			name: "source leg without a ledger",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Sources[0].LedgerID = ""
			}),
			wantMessagePart: "sources[0].ledgerId",
		},
		{
			// The second leg, not the first: a guard that only reads the head of the array
			// passes every single-leg case while leaving the rest unscoped.
			name: "second destination leg without a ledger",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "1000"}},
				[]mtransaction.V2LegInput{
					{Alias: "@dstA", Amount: "600"},
					{Alias: "@dstB", Amount: "400"},
				},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Destinations[1].LedgerID = ""
			}),
			wantMessagePart: "destinations[1].ledgerId",
		},
		{
			name: "scalar source without an organization",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.From.OrganizationID = ""

				return in
			}(),
			wantMessagePart: "from.organizationId",
		},
		{
			name: "scalar destination without a ledger",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.To.LedgerID = ""

				return in
			}(),
			wantMessagePart: "to.ledgerId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, scope, err := tt.input.Translate(false)
			require.Error(t, err, "a leg that does not name its account and scope must be rejected")

			var vErr pkg.ValidationError
			require.ErrorAs(t, err, &vErr, "a missing field is a request-shape error (400)")
			assert.Equal(t, constant.ErrMissingFieldsInRequest.Error(), vErr.Code)
			assert.Contains(t, vErr.Message, tt.wantMessagePart,
				"the rejection must name the leg and the field the caller has to fill")

			assert.True(t, got.IsEmpty(), "the error path must not leak a populated transaction")
			assert.Equal(t, mtransaction.V2Scope{}, scope, "the error path must not leak a scope")
		})
	}
}

// TestCreateTransactionV2Input_TranslateRejectsScopeDisagreement is the rule that keeps a v2
// request inside ONE ledger. A body whose legs name two different ledgers has no single scope to
// carry, and posting its two halves against different ledgers would move value in one direction
// only on each of them. It is refused rather than partially honoured.
//
// The organization case is asserted independently of the ledger case: a comparison that only
// looks at the ledger accepts every body that keeps the ledger and changes the organization.
func TestCreateTransactionV2Input_TranslateRejectsScopeDisagreement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input mtransaction.CreateTransactionV2Input
	}{
		{
			name: "destination leg on another ledger",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Destinations[0].LedgerID = otherLedgerID
			}),
		},
		{
			name: "destination leg in another organization",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Destinations[0].OrganizationID = otherOrgID
			}),
		},
		{
			// Two legs of the SAME side: the rule is about the whole request, not about the
			// two sides matching each other.
			name: "two source legs on different ledgers",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{
					{Alias: "@srcA", Amount: "600"},
					{Alias: "@srcB", Amount: "400"},
				},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Sources[1].LedgerID = otherLedgerID
			}),
		},
		{
			name: "scalar sides on different ledgers",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.To.LedgerID = otherLedgerID

				return in
			}(),
		},
		{
			// The mixed spelling has to be covered too: the scalar side and the array side
			// are compared against each other, not each against itself.
			name: "scalar source and array destination on different ledgers",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.To = mtransaction.V2AccountInput{}
				in.Destinations = []mtransaction.V2LegInput{
					{Alias: "@dstA", Amount: "1000", OrganizationID: testOrgID, LedgerID: otherLedgerID},
				}

				return in
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, scope, err := tt.input.Translate(false)
			require.Error(t, err, "legs that name different scopes must be refused")

			var uoErr pkg.UnprocessableOperationError
			require.ErrorAs(t, err, &uoErr, "a scope disagreement is a business-rule error (422)")
			assert.Equal(t, constant.ErrTransactionScopeMismatch.Error(), uoErr.Code)

			assert.True(t, got.IsEmpty(), "the error path must not leak a populated transaction")
			assert.Equal(t, mtransaction.V2Scope{}, scope, "the error path must not leak a scope")
		})
	}
}

// TestCreateTransactionV2Input_ScopeAgreementIgnoresLetterCase pins that agreement is decided on
// what the identifiers MEAN, not on how they are typed. A UUID's text spelling is
// case-insensitive, so two legs that spell one ledger with different letter case name the same
// ledger; refusing that body would reject a request that is inside a single ledger.
func TestCreateTransactionV2Input_ScopeAgreementIgnoresLetterCase(t *testing.T) {
	t.Parallel()

	in := arrayV2Input(
		[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "1000"}},
		[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
	)
	in.Destinations[0].OrganizationID = strings.ToUpper(testOrgID)
	in.Destinations[0].LedgerID = strings.ToUpper(testLedgerID)

	_, scope, err := in.Translate(false)
	require.NoError(t, err, "one scope spelled in two letter cases is still one scope")

	assert.Equal(t, testScope(), scope,
		"the resolved scope must keep the spelling of the first leg that named it")
}

// TestCreateTransactionV2Input_MalformedScopeRejectedAtDecode pins that a scope identifier which
// is not a UUID is answered at the decode boundary, naming the field, instead of travelling into
// the funnel where the failure would surface as something else entirely.
func TestCreateTransactionV2Input_MalformedScopeRejectedAtDecode(t *testing.T) {
	t.Parallel()

	const badScope = `"organizationId":"not-a-uuid","ledgerId":"` + testLedgerID + `"`

	const badLedger = `"organizationId":"` + testOrgID + `","ledgerId":"nope"`

	tests := []struct {
		name      string
		body      string
		wantField string
	}{
		{
			name: "leg with a malformed organization",
			body: `{"asset":"BRL","amount":"1000",` +
				`"sources":[{"alias":"@srcA",` + badScope + `,"amount":"1000"}],` +
				`"destinations":[{"alias":"@dstA",` + scopeJSON + `,"amount":"1000"}]}`,
			wantField: "organizationId",
		},
		{
			name: "leg with a malformed ledger",
			body: `{"asset":"BRL","amount":"1000",` +
				`"sources":[{"alias":"@srcA",` + badLedger + `,"amount":"1000"}],` +
				`"destinations":[{"alias":"@dstA",` + scopeJSON + `,"amount":"1000"}]}`,
			wantField: "ledgerId",
		},
		{
			name: "scalar side with a malformed organization",
			body: `{"asset":"BRL","amount":"1000",` +
				`"from":{"alias":"@srcA",` + badScope + `},` +
				`"to":{"alias":"@dstA",` + scopeJSON + `}}`,
			wantField: "organizationId",
		},
		{
			name: "scalar side with a malformed ledger",
			body: `{"asset":"BRL","amount":"1000",` +
				`"from":{"alias":"@srcA",` + scopeJSON + `},` +
				`"to":{"alias":"@dstA",` + badLedger + `}}`,
			wantField: "ledgerId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(tt.body), &in)
			require.Error(t, err, "a scope identifier that is not a UUID must be rejected at decode")

			fields := requireKnownFieldsError(t, err, constant.ErrBadRequest)
			assert.Contains(t, fields, tt.wantField,
				"the rejection must key on the malformed scope field")
		})
	}
}

// TestCreateTransactionV2Input_DecodeAndTranslateResolvesScope walks a body through the real
// request pipeline and out the far side of Translate, which is the only assertion that covers the
// decoder and the scope rule agreeing on one contract: a body the decoder accepts must produce a
// scope, and the fields it accepts must be the ones the rule reads.
func TestCreateTransactionV2Input_DecodeAndTranslateResolvesScope(t *testing.T) {
	t.Parallel()

	body := `{"asset":"BRL","amount":"1000",` +
		`"from":{"alias":"@srcA",` + scopeJSON + `},` +
		`"destinations":[{"alias":"@dstA",` + scopeJSON + `,"amount":"600"},` +
		`{"alias":"@dstB",` + scopeJSON + `,"amount":"400"}]}`

	var in mtransaction.CreateTransactionV2Input

	_, err := nethttp.DecodeAndValidate([]byte(body), &in)
	require.NoError(t, err, "a fully scoped body must clear decode and struct validation")

	got, scope, err := in.Translate(false)
	require.NoError(t, err)

	assert.Equal(t, testScope(), scope, "the resolved scope must be the pair the body named")
	require.Len(t, got.Send.Source.From, 1)
	require.Len(t, got.Send.Distribute.To, 2)
	assert.Equal(t, "@srcA", got.Send.Source.From[0].AccountAlias,
		"renaming the leg field must not change the alias that reaches the canonical leg")
}

// mutateV2Legs applies mutate to a built input and hands it back, so a table case can plant a
// per-leg value inline instead of building the whole input in a closure.
func mutateV2Legs(in mtransaction.CreateTransactionV2Input, mutate func(*mtransaction.CreateTransactionV2Input)) mtransaction.CreateTransactionV2Input {
	mutate(&in)

	return in
}
