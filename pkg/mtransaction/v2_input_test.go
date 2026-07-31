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

// validV2Input returns a fully populated, valid CreateTransactionV2Input.
func validV2Input() mtransaction.CreateTransactionV2Input {
	routeID := "00000000-0000-0000-0000-000000000000"
	operationRouteID := "11111111-1111-1111-1111-111111111111"

	return mtransaction.CreateTransactionV2Input{
		Description:      "New Transaction",
		Code:             "TR12345",
		Asset:            "BRL",
		Amount:           "1000",
		From:             "@person1",
		To:               "@person2",
		RouteID:          &routeID,
		OperationRouteID: &operationRouteID,
		Metadata:         map[string]any{"reference": "TRANSACTION-001"},
	}
}

// arrayV2Input returns a valid array-form CreateTransactionV2Input: the scalar
// sides cleared and both leg groups supplied by the caller. The transaction total
// stays on the request, since the legs' share expressions divide it.
func arrayV2Input(sources, destinations []mtransaction.V2LegInput) mtransaction.CreateTransactionV2Input {
	in := validV2Input()
	in.From = ""
	in.To = ""
	in.Sources = sources
	in.Destinations = destinations

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
			// Struct tags cannot express "exactly one of (from, sources)", so the
			// side obligation is a Translate rule and struct validation lets a
			// missing scalar side through.
			name:    "missing from passes struct validation",
			mutate:  func(in *mtransaction.CreateTransactionV2Input) { in.From = "" },
			wantErr: false,
		},
		{
			name:    "missing to passes struct validation",
			mutate:  func(in *mtransaction.CreateTransactionV2Input) { in.To = "" },
			wantErr: false,
		},
		{
			name: "leg array form without either scalar side passes struct validation",
			mutate: func(in *mtransaction.CreateTransactionV2Input) {
				in.From = ""
				in.To = ""
				in.Sources = []mtransaction.V2LegInput{{Account: "@person1", Amount: "1000"}}
				in.Destinations = []mtransaction.V2LegInput{{Account: "@person2", Amount: "1000"}}
			},
			wantErr: false,
		},
		{
			name: "non-UUID per-leg operationRouteId fails (dive + uuid tag)",
			mutate: func(in *mtransaction.CreateTransactionV2Input) {
				bad := "not-a-uuid"
				in.Sources = []mtransaction.V2LegInput{{Account: "@person1", OperationRouteID: &bad}}
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

// TestCreateTransactionV2Input_SideFieldsMirrorTheWireShape asserts the four side fields are
// typed independently of the canonical transaction: two plain strings and two slices of the v2
// leg type. Embedding a canonical or mmodel type here would leak domain evolution straight onto
// the published wire contract, and nothing else in the suite would notice — the shapes coincide
// today, so every behavioural test would still pass.
//
// The tag assertions this test used to carry were dropped. Each tag it pinned has a behavioural
// sibling that fails for the same change, which makes a reflective restatement a change detector
// rather than a second guarantee: no json `omitempty` is proved by
// TestCreateTransactionV2Input_EmptyScalarSideIsAKnownField, `required:"false"` by the
// contract's required-list assertions, `dive` by TestV2LegInput_AccountRequiredByTag, and
// `max=500` by TestCreateTransactionV2Input_LegArrayCap. The field TYPE has no such sibling,
// which is why it stays.
func TestCreateTransactionV2Input_SideFieldsMirrorTheWireShape(t *testing.T) {
	t.Parallel()

	inputType := reflect.TypeFor[mtransaction.CreateTransactionV2Input]()

	tests := []struct {
		field    string
		wantType reflect.Type
	}{
		{field: "From", wantType: reflect.TypeFor[string]()},
		{field: "To", wantType: reflect.TypeFor[string]()},
		{field: "Sources", wantType: reflect.TypeFor[[]mtransaction.V2LegInput]()},
		{field: "Destinations", wantType: reflect.TypeFor[[]mtransaction.V2LegInput]()},
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

// TestCreateTransactionV2Input_DecodeLegGroups drives the array group through the real
// request pipeline (DecodeAndValidate: unmarshal -> unknown-field re-marshal ->
// struct validation), which is where the no-`omitempty` and `dive` tag choices become
// observable: explicit empty arrays stay known fields, a leg field the group does not
// expose is rejected, and a malformed per-leg route is caught at the decode boundary
// instead of deep in the funnel.
func TestCreateTransactionV2Input_DecodeLegGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr bool
		verify  func(t *testing.T, in mtransaction.CreateTransactionV2Input)
	}{
		{
			name: "explicit empty leg arrays stay known fields",
			body: `{"asset":"BRL","amount":"1000","sources":[],"destinations":[]}`,
			verify: func(t *testing.T, in mtransaction.CreateTransactionV2Input) {
				t.Helper()

				require.NotNil(t, in.Sources, "an explicit [] must decode to an empty, non-nil slice")
				require.NotNil(t, in.Destinations, "an explicit [] must decode to an empty, non-nil slice")
				assert.Empty(t, in.Sources)
				assert.Empty(t, in.Destinations)
			},
		},
		{
			name: "omitted leg arrays leave the scalar form intact",
			body: `{"asset":"BRL","amount":"1000","from":"@person1","to":"@person2"}`,
			verify: func(t *testing.T, in mtransaction.CreateTransactionV2Input) {
				t.Helper()

				assert.Equal(t, "@person1", in.From)
				assert.Equal(t, "@person2", in.To)
				assert.Nil(t, in.Sources)
				assert.Nil(t, in.Destinations)
			},
		},
		{
			name: "populated leg arrays decode every value expression",
			body: `{"asset":"BRL","amount":"1000",` +
				`"sources":[{"account":"@person1","share":{"percentage":60,"percentageOfPercentage":50}},` +
				`{"account":"@person2","amount":"400"}],` +
				`"destinations":[{"account":"@person3","amount":"1000","operationRouteId":"11111111-1111-1111-1111-111111111111"}]}`,
			verify: func(t *testing.T, in mtransaction.CreateTransactionV2Input) {
				t.Helper()

				assert.Empty(t, in.From, "array form must leave the scalar side empty")
				assert.Empty(t, in.To, "array form must leave the scalar side empty")

				require.Len(t, in.Sources, 2)
				assert.Equal(t, "@person1", in.Sources[0].Account)
				require.NotNil(t, in.Sources[0].Share)
				assert.Equal(t, int64(60), in.Sources[0].Share.Percentage)
				assert.Equal(t, int64(50), in.Sources[0].Share.PercentageOfPercentage)
				assert.Empty(t, in.Sources[0].Amount)

				assert.Equal(t, "@person2", in.Sources[1].Account)
				assert.Equal(t, "400", in.Sources[1].Amount)
				assert.Nil(t, in.Sources[1].Share)

				require.Len(t, in.Destinations, 1)
				assert.Equal(t, "@person3", in.Destinations[0].Account)
				assert.Equal(t, "1000", in.Destinations[0].Amount)
				require.NotNil(t, in.Destinations[0].OperationRouteID)
				assert.Equal(t, "11111111-1111-1111-1111-111111111111", *in.Destinations[0].OperationRouteID)
			},
		},
		{
			name: "malformed per-leg operationRouteId is rejected at decode",
			body: `{"asset":"BRL","amount":"1000",` +
				`"sources":[{"account":"@person1","operationRouteId":"not-a-uuid"}],` +
				`"destinations":[{"account":"@person2","amount":"1000"}]}`,
			wantErr: true,
		},
		{
			name: "leg field outside the exposed group is an unknown field",
			body: `{"asset":"BRL","amount":"1000",` +
				`"sources":[{"account":"@person1","balanceKey":"default"}],` +
				`"destinations":[{"account":"@person2","amount":"1000"}]}`,
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
			name:    "single from/to happy path maps every field",
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
			name: "distinct asset and amount propagate identically to both legs",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.Asset = "USD"
				in.Amount = "42.55"

				return in
			}(),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				want := decimal.RequireFromString("42.55")
				assert.Equal(t, "USD", got.Send.Asset)
				assert.True(t, want.Equal(got.Send.Value))

				require.Len(t, got.Send.Source.From, 1)
				require.NotNil(t, got.Send.Source.From[0].Amount)
				assert.Equal(t, "USD", got.Send.Source.From[0].Amount.Asset)
				assert.True(t, want.Equal(got.Send.Source.From[0].Amount.Value))

				require.Len(t, got.Send.Distribute.To, 1)
				require.NotNil(t, got.Send.Distribute.To[0].Amount)
				assert.Equal(t, "USD", got.Send.Distribute.To[0].Amount.Asset)
				assert.True(t, want.Equal(got.Send.Distribute.To[0].Amount.Value))
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
		{
			name: "from equal to to is an ambiguous business error",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.From = "@same"
				in.To = "@same"

				return in
			}(),
			wantErr:  true,
			wantCode: constant.ErrTransactionAmbiguous.Error(),
		},

		// --- array form: expansion ---

		{
			name: "one source to many destinations expands per-leg amounts",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "@person1", Amount: "1000"}},
				[]mtransaction.V2LegInput{
					{Account: "@person2", Amount: "600"},
					{Account: "@person3", Amount: "400"},
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
			name: "many sources to one destination expands per-leg amounts",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{
					{Account: "@person1", Amount: "600"},
					{Account: "@person2", Amount: "400"},
				},
				[]mtransaction.V2LegInput{{Account: "@person3", Amount: "1000"}},
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
			name: "many sources to many destinations expands both sides",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{
					{Account: "@person1", Amount: "700"},
					{Account: "@person2", Amount: "300"},
				},
				[]mtransaction.V2LegInput{
					{Account: "@person3", Amount: "250"},
					{Account: "@person4", Amount: "750"},
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
					{Account: "@person1", Share: &mtransaction.V2ShareInput{Percentage: 60, PercentageOfPercentage: 50}},
					{Account: "@person2", Amount: "400"},
				},
				[]mtransaction.V2LegInput{{Account: "@person3", Amount: "1000"}},
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
				[]mtransaction.V2LegInput{{Account: "@person1", Share: &mtransaction.V2ShareInput{Percentage: 100}}},
				[]mtransaction.V2LegInput{{Account: "@person2", Amount: "1000"}},
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
			name: "leg operation route wins over the request-level one",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "@person1", Amount: "1000", OperationRouteID: &legRoute}},
				[]mtransaction.V2LegInput{{Account: "@person2", Amount: "1000"}},
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
					[]mtransaction.V2LegInput{{Account: "@person1", Amount: "1000"}},
					[]mtransaction.V2LegInput{{Account: "@person2", Amount: "1000"}},
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

		// --- discriminator and leg errors ---

		{
			name: "neither from nor sources is a missing-field validation error",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.From = ""

				return in
			}(),
			wantErr:             true,
			wantCode:            constant.ErrMissingFieldsInRequest.Error(),
			wantValidationError: true,
		},
		{
			name: "neither to nor destinations is a missing-field validation error",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.To = ""

				return in
			}(),
			wantErr:             true,
			wantCode:            constant.ErrMissingFieldsInRequest.Error(),
			wantValidationError: true,
		},
		{
			name:                "explicit empty leg arrays leave both sides unspelled",
			input:               arrayV2Input([]mtransaction.V2LegInput{}, []mtransaction.V2LegInput{}),
			wantErr:             true,
			wantCode:            constant.ErrMissingFieldsInRequest.Error(),
			wantValidationError: true,
		},
		{
			// The leg account obligation is enforced BOTH by the struct tag at the decode
			// boundary and here, because Translate is exported from a shared package: a
			// caller that assembles the input in Go and skips the decoder gets no tag
			// evaluation, and an empty alias reaching the funnel names no account at all.
			name: "leg without an account is rejected",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Amount: "1000"}},
				[]mtransaction.V2LegInput{{Account: "@person2", Amount: "1000"}},
			),
			wantErr:             true,
			wantCode:            constant.ErrMissingFieldsInRequest.Error(),
			wantValidationError: true,
			wantMessagePart:     "sources[0].account",
		},
		{
			// Both spellings on the SOURCE side: the side has no single reading, so it is
			// rejected. The rule is per side, so the destination side being well-formed
			// does not rescue it.
			name: "source side spelled both ways is mutually exclusive",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.Sources = []mtransaction.V2LegInput{{Account: "@person3", Amount: "1000"}}

				return in
			}(),
			wantErr:             true,
			wantCode:            constant.ErrMutuallyExclusiveTransactionFields.Error(),
			wantValidationError: true,
		},
		{
			// The destination half of the same guard. Asserted independently because a
			// guard that only covers the source side passes every source-side case.
			name: "destination side spelled both ways is mutually exclusive",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.Destinations = []mtransaction.V2LegInput{{Account: "@person3", Amount: "1000"}}

				return in
			}(),
			wantErr:             true,
			wantCode:            constant.ErrMutuallyExclusiveTransactionFields.Error(),
			wantValidationError: true,
		},
		{
			// Each side picks its spelling independently, so an array source paired with a
			// scalar destination is a valid request rather than a mixed-spelling error.
			name: "array source with scalar destination translates",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.From = ""
				in.Sources = []mtransaction.V2LegInput{
					{Account: "@person1", Amount: "600"},
					{Account: "@person3", Amount: "400"},
				}

				return in
			}(),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				require.Len(t, got.Send.Source.From, 2, "the array side expands one leg per entry")
				assert.Equal(t, "@person1", got.Send.Source.From[0].AccountAlias)
				assert.Equal(t, "@person3", got.Send.Source.From[1].AccountAlias)

				require.Len(t, got.Send.Distribute.To, 1, "the scalar side stays a single leg")
				assert.Equal(t, "@person2", got.Send.Distribute.To[0].AccountAlias)
				require.NotNil(t, got.Send.Distribute.To[0].Amount)
				assert.True(t, decimal.RequireFromString("1000").Equal(got.Send.Distribute.To[0].Amount.Value),
					"the scalar leg carries the whole transaction total")
			},
		},
		{
			// The mirror shape, and the common one: one payer to many payees.
			name: "scalar source with array destinations translates",
			input: func() mtransaction.CreateTransactionV2Input {
				in := validV2Input()
				in.To = ""
				in.Destinations = []mtransaction.V2LegInput{
					{Account: "@person2", Amount: "600"},
					{Account: "@person3", Amount: "400"},
				}

				return in
			}(),
			verify: func(t *testing.T, got mtransaction.Transaction) {
				t.Helper()

				require.Len(t, got.Send.Source.From, 1, "the scalar side stays a single leg")
				assert.Equal(t, "@person1", got.Send.Source.From[0].AccountAlias)
				require.NotNil(t, got.Send.Source.From[0].Amount)
				assert.True(t, decimal.RequireFromString("1000").Equal(got.Send.Source.From[0].Amount.Value),
					"the scalar leg carries the whole transaction total")

				require.Len(t, got.Send.Distribute.To, 2, "the array side expands one leg per entry")
				assert.Equal(t, "@person2", got.Send.Distribute.To[0].AccountAlias)
				assert.Equal(t, "@person3", got.Send.Distribute.To[1].AccountAlias)
			},
		},
		{
			name: "leg with both amount and share is an invalid transaction type",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "@person1", Amount: "1000", Share: &mtransaction.V2ShareInput{Percentage: 100}}},
				[]mtransaction.V2LegInput{{Account: "@person2", Amount: "1000"}},
			),
			wantErr:             true,
			wantCode:            constant.ErrInvalidTransactionType.Error(),
			wantValidationError: true,
			wantMessagePart:     "'sources[0]'",
		},
		{
			name: "leg without any value expression is an invalid transaction type",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "@person1"}},
				[]mtransaction.V2LegInput{{Account: "@person2", Amount: "1000"}},
			),
			wantErr:             true,
			wantCode:            constant.ErrInvalidTransactionType.Error(),
			wantValidationError: true,
			wantMessagePart:     "'sources[0]'",
		},
		{
			name: "destination leg without any value expression names the destinations field",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "@person1", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Account: "@person2"}},
			),
			wantErr:             true,
			wantCode:            constant.ErrInvalidTransactionType.Error(),
			wantValidationError: true,
			wantMessagePart:     "'destinations[0]'",
		},
		{
			name: "destination leg without an account is rejected",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "@person1", Amount: "1000"}},
				[]mtransaction.V2LegInput{{Amount: "1000"}},
			),
			wantErr:             true,
			wantCode:            constant.ErrMissingFieldsInRequest.Error(),
			wantValidationError: true,
			wantMessagePart:     "destinations[0].account",
		},
		{
			name: "non-numeric leg amount is a non-positive business error",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "@person1", Amount: "abc"}},
				[]mtransaction.V2LegInput{{Account: "@person2", Amount: "1000"}},
			),
			wantErr:  true,
			wantCode: constant.ErrInvalidTransactionNonPositiveValue.Error(),
		},
		{
			name: "zero leg amount is a non-positive business error",
			input: arrayV2Input(
				[]mtransaction.V2LegInput{{Account: "@person1", Amount: "0"}},
				[]mtransaction.V2LegInput{{Account: "@person2", Amount: "1000"}},
			),
			wantErr:  true,
			wantCode: constant.ErrInvalidTransactionNonPositiveValue.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.input.Translate(tt.pending)

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
