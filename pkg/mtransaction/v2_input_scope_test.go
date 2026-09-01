// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction_test

import (
	"context"
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
// (organization, ledger) pair the body named. The scope is what the caller needs to know which
// ledger the transaction posts to, so a translation that succeeds without producing it is
// unusable.
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
			name:         "single leg on both sides",
			input:        validV2Input(),
			wantFromLegs: 1,
			wantToLegs:   1,
		},
		{
			name: "several legs per side",
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
			// One debit paired with many credits stays a valid body, and the scope resolves
			// across the asymmetric leg counts.
			name: "one debit leg with many credit legs",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "1000"}},
				[]mtransaction.V2LegInput{
					{Alias: "@dstA", Amount: "600"},
					{Alias: "@dstB", Amount: "400"},
				},
			),
			wantFromLegs: 1,
			wantToLegs:   2,
		},
		{
			// The mirror: many debit legs with one credit leg.
			name: "many debit legs with one credit leg",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{
					{Alias: "@srcA", Amount: "600"},
					{Alias: "@srcB", Amount: "400"},
				},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			),
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
// parts of its account reference. A leg with no organization or no ledger says nothing about
// where its account lives, so there is no scope to resolve and nothing downstream can pick one
// for it.
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
			name: "debit leg without an alias",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			),
			wantMessagePart: "debits[0].alias",
		},
		{
			name: "debit leg without an organization",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Debits[0].OrganizationID = ""
			}),
			wantMessagePart: "debits[0].organizationId",
		},
		{
			name: "debit leg without a ledger",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Debits[0].LedgerID = ""
			}),
			wantMessagePart: "debits[0].ledgerId",
		},
		{
			// The second leg, not the first: a guard that only reads the head of the array
			// passes every single-leg case while leaving the rest unscoped.
			name: "second credit leg without a ledger",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "1000"}},
				[]mtransaction.V2LegInput{
					{Alias: "@dstA", Amount: "600"},
					{Alias: "@dstB", Amount: "400"},
				},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Credits[1].LedgerID = ""
			}),
			wantMessagePart: "credits[1].ledgerId",
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
			name: "credit leg on another ledger",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Credits[0].LedgerID = otherLedgerID
			}),
		},
		{
			name: "credit leg in another organization",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Credits[0].OrganizationID = otherOrgID
			}),
		},
		{
			// Two legs of the SAME side: the rule is about the whole request, not about the
			// two sides matching each other.
			name: "two debit legs on different ledgers",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{
					{Alias: "@srcA", Amount: "600"},
					{Alias: "@srcB", Amount: "400"},
				},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Debits[1].LedgerID = otherLedgerID
			}),
		},
		{
			// An external account is the one alias a reader is tempted to treat as belonging
			// everywhere, which makes it the natural place to carve an exception. It is bound
			// to a ledger like any other account, so a divergent one is refused too.
			name: "external debit leg on another ledger",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@external/BRL", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@dstA", Amount: "1000"}},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Debits[0].LedgerID = otherLedgerID
			}),
		},
		{
			name: "external credit leg in another organization",
			input: mutateV2Legs(arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@srcA", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@external/BRL", Amount: "1000"}},
			), func(in *mtransaction.CreateTransactionV2Input) {
				in.Credits[0].OrganizationID = otherOrgID
			}),
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
	in.Credits[0].OrganizationID = strings.ToUpper(testOrgID)
	in.Credits[0].LedgerID = strings.ToUpper(testLedgerID)

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
			name: "debit leg with a malformed organization",
			body: `{"asset":"BRL","amount":"1000",` +
				`"debits":[{"alias":"@srcA",` + badScope + `,"amount":"1000"}],` +
				`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"1000"}]}`,
			wantField: "organizationId",
		},
		{
			name: "debit leg with a malformed ledger",
			body: `{"asset":"BRL","amount":"1000",` +
				`"debits":[{"alias":"@srcA",` + badLedger + `,"amount":"1000"}],` +
				`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"1000"}]}`,
			wantField: "ledgerId",
		},
		{
			name: "credit leg with a malformed organization",
			body: `{"asset":"BRL","amount":"1000",` +
				`"debits":[{"alias":"@srcA",` + scopeJSON + `,"amount":"1000"}],` +
				`"credits":[{"alias":"@dstA",` + badScope + `,"amount":"1000"}]}`,
			wantField: "organizationId",
		},
		{
			name: "credit leg with a malformed ledger",
			body: `{"asset":"BRL","amount":"1000",` +
				`"debits":[{"alias":"@srcA",` + scopeJSON + `,"amount":"1000"}],` +
				`"credits":[{"alias":"@dstA",` + badLedger + `,"amount":"1000"}]}`,
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
		`"debits":[{"alias":"@srcA",` + scopeJSON + `,"amount":"1000"}],` +
		`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"600"},` +
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

// TestCreateTransactionV2Input_SameAliasAcrossLedgersIsNotOneAccount pins that the two sides
// sharing an alias in DIFFERENT ledgers is answered by the scope rule, not the ambiguity rule.
// An alias identifies an account only inside a ledger, so the same text in two ledgers names two
// accounts: that request is a scope disagreement, not a transaction paying itself. Answering it
// as ambiguity would tell the caller its debit and credit are one account when they are not.
//
// This is Translate's own check (resolveScope), so it fires before the funnel runs — unlike the
// same-ledger case, which Translate cannot decide on its own; see
// TestCreateTransactionV2Input_SameAliasIsAmbiguousAtTheFunnel.
func TestCreateTransactionV2Input_SameAliasAcrossLedgersIsNotOneAccount(t *testing.T) {
	t.Parallel()

	in := arrayV2Input(
		[]mtransaction.V2LegInput{{Alias: "@shared", Amount: "1000"}},
		[]mtransaction.V2LegInput{{Alias: "@shared", Amount: "1000", LedgerID: otherLedgerID}},
	)

	got, scope, err := in.Translate(false)
	require.Error(t, err)

	var uoErr pkg.UnprocessableOperationError
	require.ErrorAs(t, err, &uoErr)
	assert.Equal(t, constant.ErrTransactionScopeMismatch.Error(), uoErr.Code)

	assert.True(t, got.IsEmpty(), "the error path must not leak a populated transaction")
	assert.Equal(t, mtransaction.V2Scope{}, scope, "the error path must not leak a scope")
}

// TestCreateTransactionV2Input_SameAliasIsAmbiguousAtTheFunnel pins WHERE the same-account
// guarantee lives, and it is later than a reader expects.
//
// Translate carries the legs forward without comparing the two sides: it has no single pair to
// compare, and the array case needs the resolved balance-key entries only the funnel builds. So
// the body clears Translate and the funnel owns the rule.
//
// The funnel validates TWICE, and only the second pass catches this. Its ambiguity loop probes a
// map keyed on each leg's alias AS IT STANDS with a key built by ConcatAlias, so the two only meet
// once MutateConcatAliases has rewritten the aliases in place — which happens between the two
// validate calls. The consequence is that the rejection lands AFTER the idempotency claim, the
// ledger-settings read and the fee engine, not before them. Both passes are asserted below so the
// ordering is a tested fact rather than an assumption, and so that moving either mutator or either
// validate call fails here.
func TestCreateTransactionV2Input_SameAliasIsAmbiguousAtTheFunnel(t *testing.T) {
	t.Parallel()

	in := arrayV2Input(
		[]mtransaction.V2LegInput{{Alias: "@shared", Amount: "1000"}},
		[]mtransaction.V2LegInput{{Alias: "@shared", Amount: "1000"}},
	)

	transaction, scope, err := in.Translate(false)
	require.NoError(t, err, "Translate does not compare the two sides, so a shared alias clears it")
	assert.Equal(t, testScope(), scope)

	// The state the FIRST validate sees: balance keys applied, aliases not yet concatenated.
	mtransaction.ApplyDefaultBalanceKeys(transaction.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(transaction.Send.Distribute.To)

	_, err = mtransaction.ValidateSendSourceAndDistribute(context.Background(), transaction, constant.CREATED)
	require.NoError(t, err,
		"the first validate cannot see the collision: the probe is concatenated and the map keys are not")

	// What the funnel does between its two validate calls.
	transaction.Send.Source.From = mtransaction.MutateConcatAliases(transaction.Send.Source.From)
	transaction.Send.Distribute.To = mtransaction.MutateConcatAliases(transaction.Send.Distribute.To)

	_, err = mtransaction.ValidateSendSourceAndDistribute(context.Background(), transaction, constant.CREATED)
	require.Error(t, err, "the same account on both sides must be rejected as ambiguous")

	var uoErr pkg.UnprocessableOperationError
	require.ErrorAs(t, err, &uoErr)
	assert.Equal(t, constant.ErrTransactionAmbiguous.Error(), uoErr.Code)
}

// mutateV2Legs applies mutate to a built input and hands it back, so a table case can plant a
// per-leg value inline instead of building the whole input in a closure.
func mutateV2Legs(in mtransaction.CreateTransactionV2Input, mutate func(*mtransaction.CreateTransactionV2Input)) mtransaction.CreateTransactionV2Input {
	mutate(&in)

	return in
}
