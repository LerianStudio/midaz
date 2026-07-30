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
				in.Sources = []mtransaction.V2LegInput{{Account: "@person1", Remaining: true}}
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

// TestCreateTransactionV2Input_GroupFieldTags locks the struct-tag shape of the two
// discriminated side groups. Every tag asserted here is load-bearing:
//
//   - `from`/`to` carry no `validate:"required"`: a struct tag cannot express
//     "exactly one of a pair", so the side obligation is decided in Translate.
//   - `sources`/`destinations` carry no `omitempty`: an explicit `"sources": []`
//     would otherwise vanish from the unknown-field re-marshal and be rejected as an
//     unknown field instead of reaching the side discriminator.
//   - `sources`/`destinations` carry `required:"false"`: dropping `omitempty` is
//     what normally keeps a field out of the published `required` list, so without
//     the explicit tag the contract would declare the array form mandatory.
//   - `sources`/`destinations` carry `validate:"dive"`: without it the per-leg
//     `validate:"omitempty,uuid"` on a leg route is never evaluated.
func TestCreateTransactionV2Input_GroupFieldTags(t *testing.T) {
	t.Parallel()

	inputType := reflect.TypeFor[mtransaction.CreateTransactionV2Input]()
	legSliceType := reflect.TypeFor[[]mtransaction.V2LegInput]()

	tests := []struct {
		field           string
		wantType        reflect.Type
		wantJSON        string
		wantValidate    string
		wantRequiredTag string
		hasRequiredTag  bool
	}{
		{
			field:    "From",
			wantType: reflect.TypeFor[string](),
			wantJSON: "from,omitempty",
		},
		{
			field:    "To",
			wantType: reflect.TypeFor[string](),
			wantJSON: "to,omitempty",
		},
		{
			field:           "Sources",
			wantType:        legSliceType,
			wantJSON:        "sources",
			wantValidate:    "dive",
			wantRequiredTag: "false",
			hasRequiredTag:  true,
		},
		{
			field:           "Destinations",
			wantType:        legSliceType,
			wantJSON:        "destinations",
			wantValidate:    "dive",
			wantRequiredTag: "false",
			hasRequiredTag:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			t.Parallel()

			field, ok := inputType.FieldByName(tt.field)
			require.Truef(t, ok, "CreateTransactionV2Input should carry a %s field", tt.field)
			assert.Equalf(t, tt.wantType, field.Type, "%s should mirror the v2 leg shape, not a canonical type", tt.field)
			assert.Equalf(t, tt.wantJSON, field.Tag.Get("json"), "%s json tag", tt.field)
			assert.Equalf(t, tt.wantValidate, field.Tag.Get("validate"), "%s validate tag", tt.field)

			requiredTag, present := field.Tag.Lookup("required")
			assert.Equalf(t, tt.hasRequiredTag, present, "%s required tag presence", tt.field)
			assert.Equalf(t, tt.wantRequiredTag, requiredTag, "%s required tag value", tt.field)
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
				`{"account":"@person2","remaining":true}],` +
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
				assert.False(t, in.Sources[0].Remaining)
				assert.Empty(t, in.Sources[0].Amount)

				assert.Equal(t, "@person2", in.Sources[1].Account)
				assert.True(t, in.Sources[1].Remaining)
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
				`"destinations":[{"account":"@person2","remaining":true}]}`,
			wantErr: true,
		},
		{
			name: "leg field outside the exposed group is an unknown field",
			body: `{"asset":"BRL","amount":"1000",` +
				`"sources":[{"account":"@person1","balanceKey":"default"}],` +
				`"destinations":[{"account":"@person2","remaining":true}]}`,
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

// TestCreateTransactionV2Input_Translate exercises the flat -> canonical
// single-leg mapping: happy-path field propagation, two-level route mapping
// (transaction route vs per-leg operation route), the pending flag, and the
// named business-error edge cases.
func TestCreateTransactionV2Input_Translate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    mtransaction.CreateTransactionV2Input
		pending  bool
		wantErr  bool
		wantCode string
		verify   func(t *testing.T, got mtransaction.Transaction)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.input.Translate(tt.pending)

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, got.IsEmpty(), "error path must not leak a populated transaction")

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
