// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	nethttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// testOrgID and testLedgerID are the scope every leg of a valid test body names. They are
// fixed so a case that is not about the scope never has to state one.
const (
	testOrgID    = "33333333-3333-3333-3333-333333333333"
	testLedgerID = "44444444-4444-4444-4444-444444444444"
)

// scopeJSON is the shared test scope spelled for a JSON body, ready to splice into a leg
// alongside its alias. It is derived from the constants above so a body and a Go literal can
// never name different scopes.
const scopeJSON = `"organizationId":"` + testOrgID + `","ledgerId":"` + testLedgerID + `"`

// testScope is the V2Scope a body built from the helpers below resolves to.
func testScope() mtransaction.V2Scope {
	return mtransaction.V2Scope{OrganizationID: testOrgID, LedgerID: testLedgerID}
}

// v2Leg spells a leg naming alias and amount inside the shared test scope.
func v2Leg(alias, amount string) mtransaction.V2LegInput {
	return mtransaction.V2LegInput{Alias: alias, Amount: amount, OrganizationID: testOrgID, LedgerID: testLedgerID}
}

// scopedLegs fills the shared test scope on every leg that leaves it blank, so a case's leg
// literal states only the fields the case is about. A leg that names its own scope is left
// alone, which is how the disagreement cases spell divergence.
func scopedLegs(legs []mtransaction.V2LegInput) []mtransaction.V2LegInput {
	out := make([]mtransaction.V2LegInput, 0, len(legs))

	for _, leg := range legs {
		if leg.OrganizationID == "" {
			leg.OrganizationID = testOrgID
		}

		if leg.LedgerID == "" {
			leg.LedgerID = testLedgerID
		}

		out = append(out, leg)
	}

	return out
}

// validV2Input returns a fully populated, valid CreateTransactionV2Input: one debit leg and
// one credit leg, each carrying the whole transaction total.
func validV2Input() mtransaction.CreateTransactionV2Input {
	routeID := "00000000-0000-0000-0000-000000000000"
	operationRouteID := "11111111-1111-1111-1111-111111111111"

	return mtransaction.CreateTransactionV2Input{
		Description:      "New Transaction",
		Code:             "TR12345",
		Asset:            "BRL",
		Amount:           "1000",
		Debits:           []mtransaction.V2LegInput{v2Leg("@person1", "1000")},
		Credits:          []mtransaction.V2LegInput{v2Leg("@person2", "1000")},
		RouteID:          &routeID,
		OperationRouteID: &operationRouteID,
		Metadata:         map[string]any{"reference": "TRANSACTION-001"},
	}
}

// arrayV2Input returns a valid CreateTransactionV2Input built from the caller's debit and
// credit leg groups. The transaction total stays on the request, since the legs' share
// expressions divide it.
func arrayV2Input(debits, credits []mtransaction.V2LegInput) mtransaction.CreateTransactionV2Input {
	in := validV2Input()
	in.Debits = scopedLegs(debits)
	in.Credits = scopedLegs(credits)

	return in
}

func TestCreateTransactionV2Input_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(in *mtransaction.CreateTransactionV2Input)
		wantErr bool
	}{
		{
			name:    "fully populated valid input passes",
			mutate:  func(_ *mtransaction.CreateTransactionV2Input) {},
			wantErr: false,
		},
		{
			name:    "missing asset fails",
			mutate:  func(in *mtransaction.CreateTransactionV2Input) { in.Asset = "" },
			wantErr: true,
		},
		{
			name:    "missing amount fails",
			mutate:  func(in *mtransaction.CreateTransactionV2Input) { in.Amount = "" },
			wantErr: true,
		},
		{
			name:    "empty debits fails (min=1)",
			mutate:  func(in *mtransaction.CreateTransactionV2Input) { in.Debits = []mtransaction.V2LegInput{} },
			wantErr: true,
		},
		{
			name:    "nil debits fails (min=1)",
			mutate:  func(in *mtransaction.CreateTransactionV2Input) { in.Debits = nil },
			wantErr: true,
		},
		{
			name:    "empty credits fails (min=1)",
			mutate:  func(in *mtransaction.CreateTransactionV2Input) { in.Credits = []mtransaction.V2LegInput{} },
			wantErr: true,
		},
		{
			name: "leg array form with several legs per side passes struct validation",
			mutate: func(in *mtransaction.CreateTransactionV2Input) {
				in.Debits = scopedLegs([]mtransaction.V2LegInput{
					{Alias: "@person1", Amount: "600"},
					{Alias: "@person3", Amount: "400"},
				})
				in.Credits = scopedLegs([]mtransaction.V2LegInput{
					{Alias: "@person2", Amount: "250"},
					{Alias: "@person4", Amount: "750"},
				})
			},
			wantErr: false,
		},
		{
			// The leg names its scope, so the route is the only rule the body leaves broken —
			// which is what keeps the rejection attributable to the uuid tag under test.
			name: "non-UUID per-leg operationRouteId fails (dive + uuid tag)",
			mutate: func(in *mtransaction.CreateTransactionV2Input) {
				bad := "not-a-uuid"
				in.Debits = scopedLegs([]mtransaction.V2LegInput{{Alias: "@person1", OperationRouteID: &bad}})
			},
			wantErr: true,
		},
		{
			name: "metadata key over 100 chars fails (keymax)",
			mutate: func(in *mtransaction.CreateTransactionV2Input) {
				in.Metadata = map[string]any{strings.Repeat("k", 101): "value"}
			},
			wantErr: true,
		},
		{
			name: "metadata value over 2000 chars fails (valuemax)",
			mutate: func(in *mtransaction.CreateTransactionV2Input) {
				in.Metadata = map[string]any{"key": strings.Repeat("v", 2001)}
			},
			wantErr: true,
		},
		{
			name: "nested metadata value fails (nonested)",
			mutate: func(in *mtransaction.CreateTransactionV2Input) {
				in.Metadata = map[string]any{"key": map[string]any{"nested": "value"}}
			},
			wantErr: true,
		},
		{
			name: "non-UUID routeId fails (uuid tag)",
			mutate: func(in *mtransaction.CreateTransactionV2Input) {
				bad := "not-a-uuid"
				in.RouteID = &bad
			},
			wantErr: true,
		},
		{
			name: "non-UUID operationRouteId fails (uuid tag)",
			mutate: func(in *mtransaction.CreateTransactionV2Input) {
				bad := "not-a-uuid"
				in.OperationRouteID = &bad
			},
			wantErr: true,
		},
		{
			name: "nil route ids pass (omitempty)",
			mutate: func(in *mtransaction.CreateTransactionV2Input) {
				in.RouteID = nil
				in.OperationRouteID = nil
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := validV2Input()
			tt.mutate(&in)

			err := nethttp.ValidateStruct(in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

// TestCreateTransactionV2Input_SideFieldsMirrorTheWireShape asserts the two side fields are
// slices of the v2 leg type, typed independently of the canonical transaction. Embedding a
// canonical or mmodel type here would leak domain evolution straight onto the published wire
// contract, and nothing else in the suite would notice — the shapes coincide today, so every
// behavioural test would still pass.
//
// The tag assertions this test used to carry were dropped. Each tag it pinned has a behavioural
// sibling that fails for the same change, which makes a reflective restatement a change detector
// rather than a second guarantee: no json `omitempty` is proved by
// TestCreateTransactionV2Input_EmptySideArrayIsAKnownField, `min=1` by
// TestCreateTransactionV2Input_Validation, `dive` by TestV2LegInput_AliasRequired, and
// `max=500` by TestCreateTransactionV2Input_LegArrayCap. The field TYPE has no such sibling,
// which is why it stays.
func TestCreateTransactionV2Input_SideFieldsMirrorTheWireShape(t *testing.T) {
	t.Parallel()

	inputType := reflect.TypeFor[mtransaction.CreateTransactionV2Input]()

	tests := []struct {
		field    string
		wantType reflect.Type
	}{
		{field: "Debits", wantType: reflect.TypeFor[[]mtransaction.V2LegInput]()},
		{field: "Credits", wantType: reflect.TypeFor[[]mtransaction.V2LegInput]()},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			t.Parallel()

			field, ok := inputType.FieldByName(tt.field)
			require.Truef(t, ok, "CreateTransactionV2Input should carry a %s field", tt.field)
			assert.Equalf(t, tt.wantType, field.Type,
				"%s must mirror the v2 wire shape explicitly, not embed a canonical type", tt.field)
		})
	}
}

// TestCreateTransactionV2Input_DecodeLegGroups drives the leg groups through the real
// request pipeline (DecodeAndValidate: unmarshal -> unknown-field re-marshal ->
// struct validation), which is where the no-`omitempty` and `dive` tag choices become
// observable: explicit empty arrays stay known fields (and are then rejected by
// `min=1`, proved in TestCreateTransactionV2Input_EmptySideArrayIsAKnownField), a leg
// field the group does not expose is rejected, and a malformed per-leg route is caught
// at the decode boundary instead of deep in the funnel.
func TestCreateTransactionV2Input_DecodeLegGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr bool
		verify  func(t *testing.T, in mtransaction.CreateTransactionV2Input)
	}{
		{
			name: "populated leg arrays decode every value expression",
			body: `{"asset":"BRL","amount":"1000",` +
				`"debits":[{"alias":"@person1",` + scopeJSON + `,"share":{"percentage":60,"percentageOfPercentage":50}},` +
				`{"alias":"@person2",` + scopeJSON + `,"amount":"400"}],` +
				`"credits":[{"alias":"@person3",` + scopeJSON + `,"amount":"1000","operationRouteId":"11111111-1111-1111-1111-111111111111"}]}`,
			verify: func(t *testing.T, in mtransaction.CreateTransactionV2Input) {
				t.Helper()

				require.Len(t, in.Debits, 2)
				assert.Equal(t, "@person1", in.Debits[0].Alias)
				require.NotNil(t, in.Debits[0].Share)
				assert.Equal(t, int64(60), in.Debits[0].Share.Percentage)
				assert.Equal(t, int64(50), in.Debits[0].Share.PercentageOfPercentage)
				assert.Empty(t, in.Debits[0].Amount)

				assert.Equal(t, "@person2", in.Debits[1].Alias)
				assert.Equal(t, "400", in.Debits[1].Amount)
				assert.Nil(t, in.Debits[1].Share)

				require.Len(t, in.Credits, 1)
				assert.Equal(t, "@person3", in.Credits[0].Alias)
				assert.Equal(t, "1000", in.Credits[0].Amount)
				require.NotNil(t, in.Credits[0].OperationRouteID)
				assert.Equal(t, "11111111-1111-1111-1111-111111111111", *in.Credits[0].OperationRouteID)
			},
		},
		{
			name: "malformed per-leg operationRouteId is rejected at decode",
			body: `{"asset":"BRL","amount":"1000",` +
				`"debits":[{"alias":"@person1",` + scopeJSON + `,"operationRouteId":"not-a-uuid"}],` +
				`"credits":[{"alias":"@person2",` + scopeJSON + `,"amount":"1000"}]}`,
			wantErr: true,
		},
		{
			name: "leg field outside the exposed group is an unknown field",
			body: `{"asset":"BRL","amount":"1000",` +
				`"debits":[{"alias":"@person1",` + scopeJSON + `,"balanceKey":"default"}],` +
				`"credits":[{"alias":"@person2",` + scopeJSON + `,"amount":"1000"}]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var in mtransaction.CreateTransactionV2Input

			_, err := nethttp.DecodeAndValidate([]byte(tt.body), &in)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, tt.verify)
			tt.verify(t, in)
		})
	}
}

// TestCreateTransactionV2Input_Translate exercises the flat -> canonical mapping
// for both spellings: happy-path field propagation, two-level route mapping
// (transaction route vs per-leg operation route), the pending flag, leg-array
// expansion across both value expressions, every combination of the two per-side
// spellings, and the named error edge cases.
//
// wantValidationError picks the asserted error CLASS, because the two classes carry
// different HTTP statuses: request-shape violations are ValidationError (400) and
// value violations are UnprocessableOperationError (422). Its zero value asserts
// the 422 class, so every pre-existing case keeps its original expectation.
func TestCreateTransactionV2Input_Translate(t *testing.T) {
	t.Parallel()

	legRoute := "22222222-2222-2222-2222-222222222222"

	tests := []struct {
		name                string
		input               mtransaction.CreateTransactionV2Input
		pending             bool
		wantErr             bool
		wantCode            string
		wantValidationError bool
		wantMessagePart     string
		verify              func(t *testing.T, got mtransaction.Transaction)
	}{
		{
			name:    "single debit/credit happy path maps every field",
			input:   validV2Input(),
			pending: false,
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				assert.Equal(t, "New Transaction", got.Description)
				assert.Equal(t, "TR12345", got.Code)
				assert.False(t, got.Pending)
				assert.Equal(t, map[string]any{"reference": "TRANSACTION-001"}, got.Metadata)

				// Transaction route (level 1) maps from RouteID.
				require.NotNil(t, got.RouteID)
				assert.Equal(t, "00000000-0000-0000-0000-000000000000", *got.RouteID)

				// Top-level asset/amount propagate to Send.
				assert.Equal(t, "BRL", got.Send.Asset)
				assert.True(t, decimal.RequireFromString("1000").Equal(got.Send.Value),
					"send value = %s", got.Send.Value)

				// Source (debit) leg.
				require.Len(t, got.Send.Source.From, 1)
				from := got.Send.Source.From[0]
				assert.Equal(t, "@person1", from.AccountAlias)
				assert.True(t, from.IsFrom, "source leg must carry IsFrom=true")
				require.NotNil(t, from.Amount)
				assert.Equal(t, "BRL", from.Amount.Asset)
				assert.True(t, decimal.RequireFromString("1000").Equal(from.Amount.Value))
				require.NotNil(t, from.RouteID, "source leg operation route")
				assert.Equal(t, "11111111-1111-1111-1111-111111111111", *from.RouteID)

				// Distribute (credit) leg.
				require.Len(t, got.Send.Distribute.To, 1)
				to := got.Send.Distribute.To[0]
				assert.Equal(t, "@person2", to.AccountAlias)
				assert.False(t, to.IsFrom, "distribute leg must not carry IsFrom")
				require.NotNil(t, to.Amount)
				assert.Equal(t, "BRL", to.Amount.Asset)
				assert.True(t, decimal.RequireFromString("1000").Equal(to.Amount.Value))
				require.NotNil(t, to.RouteID, "distribute leg operation route")
				assert.Equal(t, "11111111-1111-1111-1111-111111111111", *to.RouteID)

				// Per-leg operation-route pointers must be independent copies,
				// not the same shared pointer aliased onto both legs.
				assert.NotSame(t, from.RouteID, to.RouteID)
			},
		},
		{
			// The top-level Amount is the transaction TOTAL that share expressions divide, not
			// a value copied onto every leg: an explicit-amount leg keeps its own figure
			// regardless of what the request-level amount is. Asset still propagates to every
			// leg's Amount, since a leg names no asset of its own.
			name: "asset propagates to every leg; top-level amount sets only the transaction total",
			input: func() mtransaction.CreateTransactionV2Input {
				in := arrayV2Input(
					[]mtransaction.V2LegInput{{Alias: "@person1", Amount: "1000"}},
					[]mtransaction.V2LegInput{{Alias: "@person2", Amount: "1000"}},
				)
				in.Asset = "USD"
				in.Amount = "42.55"

				return in
			}(),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				assert.Equal(t, "USD", got.Send.Asset)
				assert.True(t, decimal.RequireFromString("42.55").Equal(got.Send.Value),
					"the transaction total is the top-level amount")

				require.Len(t, got.Send.Source.From, 1)
				require.NotNil(t, got.Send.Source.From[0].Amount)
				assert.Equal(t, "USD", got.Send.Source.From[0].Amount.Asset, "every leg amount inherits the request asset")
				assert.True(t, decimal.RequireFromString("1000").Equal(got.Send.Source.From[0].Amount.Value),
					"an explicit-amount leg keeps its own value, independent of the request total")
			},
		},
		{
			name:    "pending flag propagates to Transaction.Pending",
			input:   validV2Input(),
			pending: true,
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				assert.True(t, got.Pending)
				assert.Equal(t, constant.PENDING, got.InitialStatus())
			},
		},
		{
			name: "nil RouteID leaves transaction route unset",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.RouteID = nil

				return in
			}(),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				assert.Nil(t, got.RouteID, "nil transaction route must stay nil")
			},
		},
		{
			name: "nil OperationRouteID leaves both legs without an operation route",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.OperationRouteID = nil

				return in
			}(),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				require.Len(t, got.Send.Source.From, 1)
				require.Len(t, got.Send.Distribute.To, 1)
				assert.Nil(t, got.Send.Source.From[0].RouteID)
				assert.Nil(t, got.Send.Distribute.To[0].RouteID)
			},
		},
		{
			name: "empty amount is a business error",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.Amount = ""

				return in
			}(),
			wantErr:  true,
			wantCode: constant.ErrInvalidTransactionNonPositiveValue.Error(),
		},
		{
			name: "non-numeric amount is a business error",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.Amount = "abc"

				return in
			}(),
			wantErr:  true,
			wantCode: constant.ErrInvalidTransactionNonPositiveValue.Error(),
		},
		{
			name: "zero amount is a non-positive business error",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.Amount = "0"

				return in
			}(),
			wantErr:  true,
			wantCode: constant.ErrInvalidTransactionNonPositiveValue.Error(),
		},
		// The same-account-on-both-sides case moved: Translate no longer resolves the
		// funnel's balance-key entries, so it cannot decide ambiguity on its own. See
		// TestCreateTransactionV2Input_SameAliasIsAmbiguousAtTheFunnel in
		// v2_input_scope_test.go, which runs Translate's output through
		// ValidateSendSourceAndDistribute — the same path production code takes.

		// --- expansion ---

		{
			name: "one debit to many credits expands per-leg amounts",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@person1", Amount: "1000"}},
				[]mtransaction.V2LegInput{
					{Alias: "@person2", Amount: "600"},
					{Alias: "@person3", Amount: "400"},
				},
			),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				assert.Equal(t, "BRL", got.Send.Asset)
				assert.True(t, decimal.RequireFromString("1000").Equal(got.Send.Value))

				require.Len(t, got.Send.Source.From, 1)
				assert.Equal(t, "@person1", got.Send.Source.From[0].AccountAlias)
				assert.True(t, got.Send.Source.From[0].IsFrom, "source legs must carry IsFrom=true")
				require.NotNil(t, got.Send.Source.From[0].Amount)
				assert.Equal(t, "BRL", got.Send.Source.From[0].Amount.Asset, "leg amounts inherit the request asset")
				assert.True(t, decimal.RequireFromString("1000").Equal(got.Send.Source.From[0].Amount.Value))

				require.Len(t, got.Send.Distribute.To, 2)
				for i, want := range []string{"600", "400"} {
					leg := got.Send.Distribute.To[i]
					assert.False(t, leg.IsFrom, "distribute leg %d must not carry IsFrom", i)
					require.NotNil(t, leg.Amount)
					assert.Equal(t, "BRL", leg.Amount.Asset)
					assert.Truef(t, decimal.RequireFromString(want).Equal(leg.Amount.Value), "leg %d amount = %s", i, leg.Amount.Value)
					require.NotNil(t, leg.RouteID, "leg %d inherits the request operation route", i)
					assert.Equal(t, "11111111-1111-1111-1111-111111111111", *leg.RouteID)
				}

				assert.Equal(t, "@person2", got.Send.Distribute.To[0].AccountAlias)
				assert.Equal(t, "@person3", got.Send.Distribute.To[1].AccountAlias)
			},
		},
		{
			name: "many debits to one credit expands per-leg amounts",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{
					{Alias: "@person1", Amount: "600"},
					{Alias: "@person2", Amount: "400"},
				},
				[]mtransaction.V2LegInput{{Alias: "@person3", Amount: "1000"}},
			),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				require.Len(t, got.Send.Source.From, 2)
				for i, want := range []string{"600", "400"} {
					leg := got.Send.Source.From[i]
					assert.True(t, leg.IsFrom, "source leg %d must carry IsFrom=true", i)
					require.NotNil(t, leg.Amount)
					assert.Truef(t, decimal.RequireFromString(want).Equal(leg.Amount.Value), "leg %d amount = %s", i, leg.Amount.Value)
				}

				assert.Equal(t, "@person1", got.Send.Source.From[0].AccountAlias)
				assert.Equal(t, "@person2", got.Send.Source.From[1].AccountAlias)

				require.Len(t, got.Send.Distribute.To, 1)
				assert.Equal(t, "@person3", got.Send.Distribute.To[0].AccountAlias)
				assert.False(t, got.Send.Distribute.To[0].IsFrom)
			},
		},
		{
			name: "many debits to many credits expands both sides",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{
					{Alias: "@person1", Amount: "700"},
					{Alias: "@person2", Amount: "300"},
				},
				[]mtransaction.V2LegInput{
					{Alias: "@person3", Amount: "250"},
					{Alias: "@person4", Amount: "750"},
				},
			),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				require.Len(t, got.Send.Source.From, 2)
				require.Len(t, got.Send.Distribute.To, 2)

				assert.True(t, got.Send.Source.From[0].IsFrom)
				assert.True(t, got.Send.Source.From[1].IsFrom)
				assert.False(t, got.Send.Distribute.To[0].IsFrom)
				assert.False(t, got.Send.Distribute.To[1].IsFrom)

				require.NotNil(t, got.Send.Source.From[1].Amount)
				assert.True(t, decimal.RequireFromString("300").Equal(got.Send.Source.From[1].Amount.Value))
				require.NotNil(t, got.Send.Distribute.To[0].Amount)
				assert.True(t, decimal.RequireFromString("250").Equal(got.Send.Distribute.To[0].Amount.Value))
			},
		},
		{
			name: "amount and share legs map to their own value expressions",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{
					{Alias: "@person1", Share: &mtransaction.V2ShareInput{Percentage: 60, PercentageOfPercentage: 50}},
					{Alias: "@person2", Amount: "400"},
				},
				[]mtransaction.V2LegInput{{Alias: "@person3", Amount: "1000"}},
			),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				require.Len(t, got.Send.Source.From, 2)

				share := got.Send.Source.From[0]
				require.NotNil(t, share.Share)
				assert.Equal(t, int64(60), share.Share.Percentage)
				assert.Equal(t, int64(50), share.Share.PercentageOfPercentage)
				assert.Nil(t, share.Amount, "a share leg carries no amount")
				assert.Empty(t, share.Remaining, "the v2 surface publishes no remaining expression")

				explicit := got.Send.Source.From[1]
				require.NotNil(t, explicit.Amount, "an explicit-amount leg carries an amount")
				assert.True(t, decimal.RequireFromString("400").Equal(explicit.Amount.Value))
				assert.Nil(t, explicit.Share, "an explicit-amount leg carries no share")
				assert.Empty(t, explicit.Remaining, "the v2 surface publishes no remaining expression")
			},
		},
		{
			name: "share leg without percentage-of-percentage leaves it zero",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@person1", Share: &mtransaction.V2ShareInput{Percentage: 100}}},
				[]mtransaction.V2LegInput{{Alias: "@person2", Amount: "1000"}},
			),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				require.Len(t, got.Send.Source.From, 1)
				require.NotNil(t, got.Send.Source.From[0].Share)
				assert.Equal(t, int64(100), got.Send.Source.From[0].Share.Percentage)
				assert.Zero(t, got.Send.Source.From[0].Share.PercentageOfPercentage)
			},
		},
		{
			// A leg's description is the description of the operation that leg produces, and
			// each side carries its own: a debit and a credit of one transaction describe
			// opposite halves of it, so the two must not be collapsed into one value.
			name: "each leg's description reaches its own built leg",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@person1", Amount: "1000", Description: "debit leg note"}},
				[]mtransaction.V2LegInput{{Alias: "@person2", Amount: "1000", Description: "credit leg note"}},
			),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				require.Len(t, got.Send.Source.From, 1)
				assert.Equal(t, "debit leg note", got.Send.Source.From[0].Description)

				require.Len(t, got.Send.Distribute.To, 1)
				assert.Equal(t, "credit leg note", got.Send.Distribute.To[0].Description)

				assert.Equal(t, "New Transaction", got.Description,
					"a leg description must not overwrite the transaction-level one")
			},
		},
		{
			// Translate itself does NOT inherit: a leg that names no description reaches the
			// operation builders empty, and THEY substitute the transaction-level description.
			// Filling it here instead would make the two indistinguishable downstream.
			name: "leg without a description leaves it empty at translate",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@person1", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@person2", Amount: "1000"}},
			),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				require.Len(t, got.Send.Source.From, 1)
				require.Len(t, got.Send.Distribute.To, 1)
				assert.Empty(t, got.Send.Source.From[0].Description)
				assert.Empty(t, got.Send.Distribute.To[0].Description)
				assert.Equal(t, "New Transaction", got.Description,
					"the transaction description the legs inherit downstream is still set")
			},
		},
		{
			name: "leg operation route wins over the request-level one",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@person1", Amount: "1000", OperationRouteID: &legRoute}},
				[]mtransaction.V2LegInput{{Alias: "@person2", Amount: "1000"}},
			),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				require.Len(t, got.Send.Source.From, 1)
				require.NotNil(t, got.Send.Source.From[0].RouteID)
				assert.Equal(t, "22222222-2222-2222-2222-222222222222", *got.Send.Source.From[0].RouteID,
					"an explicit leg route overrides the request-level operation route")
				assert.NotSame(t, &legRoute, got.Send.Source.From[0].RouteID, "the leg route must be an independent copy")

				require.Len(t, got.Send.Distribute.To, 1)
				require.NotNil(t, got.Send.Distribute.To[0].RouteID)
				assert.Equal(t, "11111111-1111-1111-1111-111111111111", *got.Send.Distribute.To[0].RouteID,
					"a leg without its own route falls back to the request-level one")
			},
		},
		{
			name: "no request and no leg operation route leaves every leg unrouted",
			input: func() mtransaction.CreateTransactionV2Input {
				in := arrayV2Input(
					[]mtransaction.V2LegInput{{Alias: "@person1", Amount: "1000"}},
					[]mtransaction.V2LegInput{{Alias: "@person2", Amount: "1000"}},
				)
				in.OperationRouteID = nil

				return in
			}(),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				require.Len(t, got.Send.Source.From, 1)
				require.Len(t, got.Send.Distribute.To, 1)
				assert.Nil(t, got.Send.Source.From[0].RouteID)
				assert.Nil(t, got.Send.Distribute.To[0].RouteID)
			},
		},

		// --- side and leg errors ---

		{
			name: "nil debits is a missing-field validation error",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.Debits = nil

				return in
			}(),
			wantErr:             true,
			wantCode:            constant.ErrMissingFieldsInRequest.Error(),
			wantValidationError: true,
			wantMessagePart:     "debits",
		},
		{
			name: "nil credits is a missing-field validation error",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.Credits = nil

				return in
			}(),
			wantErr:             true,
			wantCode:            constant.ErrMissingFieldsInRequest.Error(),
			wantValidationError: true,
			wantMessagePart:     "credits",
		},
		{
			name:                "explicit empty leg arrays leave both sides empty",
			input:               arrayV2Input([]mtransaction.V2LegInput{}, []mtransaction.V2LegInput{}),
			wantErr:             true,
			wantCode:            constant.ErrMissingFieldsInRequest.Error(),
			wantValidationError: true,
			wantMessagePart:     "debits",
		},
		{
			// The leg alias obligation is enforced BOTH by the struct tag at the decode
			// boundary and here, because Translate is exported from a shared package: a
			// caller that assembles the input in Go and skips the decoder gets no tag
			// evaluation, and an empty alias reaching the funnel names no account at all.
			name: "leg without an alias is rejected",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@person2", Amount: "1000"}},
			),
			wantErr:             true,
			wantCode:            constant.ErrMissingFieldsInRequest.Error(),
			wantValidationError: true,
			wantMessagePart:     "debits[0].alias",
		},
		{
			name: "leg with both amount and share is an invalid transaction type",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@person1", Amount: "1000", Share: &mtransaction.V2ShareInput{Percentage: 100}}},
				[]mtransaction.V2LegInput{{Alias: "@person2", Amount: "1000"}},
			),
			wantErr:             true,
			wantCode:            constant.ErrInvalidTransactionType.Error(),
			wantValidationError: true,
			wantMessagePart:     "'debits[0]'",
		},
		{
			name: "leg without any value expression is an invalid transaction type",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@person1"}},
				[]mtransaction.V2LegInput{{Alias: "@person2", Amount: "1000"}},
			),
			wantErr:             true,
			wantCode:            constant.ErrInvalidTransactionType.Error(),
			wantValidationError: true,
			wantMessagePart:     "'debits[0]'",
		},
		{
			name: "credit leg without any value expression names the credits field",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@person1", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Alias: "@person2"}},
			),
			wantErr:             true,
			wantCode:            constant.ErrInvalidTransactionType.Error(),
			wantValidationError: true,
			wantMessagePart:     "'credits[0]'",
		},
		{
			name: "credit leg without an alias is rejected",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@person1", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Amount: "1000"}},
			),
			wantErr:             true,
			wantCode:            constant.ErrMissingFieldsInRequest.Error(),
			wantValidationError: true,
			wantMessagePart:     "credits[0].alias",
		},
		{
			name: "non-numeric leg amount is a non-positive business error",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@person1", Amount: "abc"}},
				[]mtransaction.V2LegInput{{Alias: "@person2", Amount: "1000"}},
			),
			wantErr:  true,
			wantCode: constant.ErrInvalidTransactionNonPositiveValue.Error(),
		},
		{
			name: "zero leg amount is a non-positive business error",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Alias: "@person1", Amount: "0"}},
				[]mtransaction.V2LegInput{{Alias: "@person2", Amount: "1000"}},
			),
			wantErr:  true,
			wantCode: constant.ErrInvalidTransactionNonPositiveValue.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, _, err := tt.input.Translate(tt.pending)

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, got.IsEmpty(), "error path must not leak a populated transaction")

				if tt.wantValidationError {
					var vErr pkg.ValidationError
					require.ErrorAs(t, err, &vErr, "request-shape errors must be ValidationError (400)")
					assert.Equal(t, tt.wantCode, vErr.Code)

					if tt.wantMessagePart != "" {
						assert.Contains(t, vErr.Message, tt.wantMessagePart,
							"the message must name the offending side AND leg index so the caller knows which entry to fix")
					}

					return
				}

				var uoErr pkg.UnprocessableOperationError
				require.ErrorAs(t, err, &uoErr, "business errors must be UnprocessableOperationError (422)")
				assert.Equal(t, tt.wantCode, uoErr.Code)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, tt.verify)
			tt.verify(t, got)
		})
	}
}
