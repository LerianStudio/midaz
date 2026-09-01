// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bsondecimal

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// stringType is the BSON string type tag as the raw byte the v2
// ValueMarshaler/ValueUnmarshaler interfaces exchange.
var stringType = byte(bson.TypeString)

func TestDecimal_MarshalBSONValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		decimal  Decimal
		wantType byte
		wantErr  bool
	}{
		{
			name:     "Valid decimal value",
			decimal:  Decimal{Decimal: decimal.NewFromInt(100)},
			wantType: stringType,
			wantErr:  false,
		},
		{
			name:     "Zero decimal value",
			decimal:  Decimal{Decimal: decimal.Zero},
			wantType: stringType,
			wantErr:  false,
		},
		{
			name:     "Negative decimal value",
			decimal:  Decimal{Decimal: decimal.NewFromInt(-100)},
			wantType: stringType,
			wantErr:  false,
		},
		{
			name:     "Large decimal value",
			decimal:  Decimal{Decimal: decimal.RequireFromString("123456789.123456789")},
			wantType: stringType,
			wantErr:  false,
		},
		{
			name:     "Small decimal value",
			decimal:  Decimal{Decimal: decimal.RequireFromString("0.0000001")},
			wantType: stringType,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotBytes, err := tt.decimal.MarshalBSONValue()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantType, gotType)
			assert.NotNil(t, gotBytes)
			assert.Greater(t, len(gotBytes), 0)
		})
	}
}

func TestDecimal_UnmarshalBSONValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		raw         string
		wantDecimal string
		wantErr     bool
	}{
		{name: "Valid string decimal", raw: "100.50", wantDecimal: "100.5", wantErr: false},
		{name: "Zero string", raw: "0", wantDecimal: "0", wantErr: false},
		{name: "Negative string", raw: "-100.50", wantDecimal: "-100.5", wantErr: false},
		{name: "Invalid decimal string", raw: "invalid", wantErr: true},
		{name: "Empty string", raw: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bsonType, data, err := bson.MarshalValue(tt.raw)
			require.NoError(t, err)
			require.Equal(t, bson.TypeString, bsonType)

			d := &Decimal{}
			err = d.UnmarshalBSONValue(byte(bsonType), data)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantDecimal, d.String())
		})
	}
}

// TestDecimal_RoundTrip_DecimalExact is the money-serialization gate: every
// value must survive marshal→unmarshal across the BSON boundary under
// decimal.Equal with ZERO tolerance. Inputs are constructed from STRINGS (not
// float64) so no precision is lost before the codec sees them — this proves the
// codec itself is lossless, not that float happened to round-trip.
func TestDecimal_RoundTrip_DecimalExact(t *testing.T) {
	t.Parallel()

	values := []string{
		"0",
		"100",
		"-100",
		"100.5",
		"-100.5",
		"0.01",
		"0.0000001",
		"123456789.123456789",
		"999999999999999.999999999999999",
		// 18-decimal precision (ETH scale) — the case the conservation work flagged
		"0.000000000000000001",
		"1.000000000000000001",
		"-0.123456789012345678",
		// many significant digits, no trailing-zero ambiguity
		"12345678901234567890.12345678901234567890",
	}

	for _, v := range values {
		t.Run(v, func(t *testing.T) {
			in := Decimal{Decimal: decimal.RequireFromString(v)}

			typ, data, err := in.MarshalBSONValue()
			require.NoError(t, err)
			require.Equal(t, stringType, typ, "decimal must serialize as BSON string")

			var out Decimal
			require.NoError(t, out.UnmarshalBSONValue(typ, data))

			// Exact numeric (decimal) equality — no float epsilon. This is the
			// money requirement: fees compute on the exact decimal VALUE and the
			// asset's ISO-4217 precision governs, not the stored scale.
			assert.Truef(t, in.Equal(out.Decimal),
				"round-trip not decimal-exact: in=%s out=%s", in.String(), out.String())
			// For canonical-form inputs (no trailing zeros) the string is stable
			// too. Scale-normalization of trailing zeros (1.50 -> 1.5) is covered
			// separately by TestDecimal_RoundTrip_ScaleNormalization.
			assert.Equalf(t, in.String(), out.String(),
				"round-trip string drift: in=%s out=%s", in.String(), out.String())
		})
	}
}

// TestDecimal_RoundTrip_ScaleNormalization documents and locks the EXPECTED
// behavior that the codec is value-exact but NOT scale-preserving: it serializes
// via decimal.String(), which strips trailing zeros (1.50 -> "1.5", 100.00 ->
// "100"). This is pre-existing (the v1 codec did the same) and acceptable —
// numeric equality is the money invariant (conservation is decimal.Equal; the
// asset's ISO-4217 scale governs presentation, not the stored representation).
// The test asserts BOTH the normalized string form AND value-equality, so a
// future change to scale handling breaks loudly and forces a conscious decision.
func TestDecimal_RoundTrip_ScaleNormalization(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		wantStr string // canonical (trailing-zero-stripped) form after round-trip
	}{
		{in: "1.50", wantStr: "1.5"},
		{in: "100.00", wantStr: "100"},
		{in: "0.10", wantStr: "0.1"},
		{in: "-2.500", wantStr: "-2.5"},
		{in: "0.000", wantStr: "0"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			in := Decimal{Decimal: decimal.RequireFromString(tc.in)}

			typ, data, err := in.MarshalBSONValue()
			require.NoError(t, err)

			var out Decimal
			require.NoError(t, out.UnmarshalBSONValue(typ, data))

			// Scale is normalized to canonical form (expected, not a bug).
			assert.Equalf(t, tc.wantStr, out.String(),
				"expected canonical-scale %q for input %q, got %q", tc.wantStr, tc.in, out.String())
			// Numeric value is exact regardless of scale normalization.
			assert.Truef(t, in.Equal(out.Decimal),
				"value must be exact across scale normalization: in=%s out=%s", tc.in, out.String())
		})
	}
}

// TestDecimal_RoundTrip_ThroughBSONDocument proves the codec works when driven
// by the v2 driver's document marshal/unmarshal machinery, exercised the way
// fees actually persists: InsertOne(ctx, &model) and cursor.Decode(&model) pass
// an ADDRESSABLE pointer, so the *Decimal ValueMarshaler/ValueUnmarshaler is in
// the resolved method set and the amount serializes as a BSON string (type 0x02),
// not an embedded document. (A non-addressable value struct would bypass the
// pointer-receiver codec — a Go method-set rule, not a driver behavior — but
// fees never persists by value; create.go uses record := &PackageMongoDBModel{}.)
func TestDecimal_RoundTrip_ThroughBSONDocument(t *testing.T) {
	t.Parallel()

	type doc struct {
		Amount Decimal `bson:"amount"`
	}

	values := []string{"0", "100.5", "-7.25", "0.000000000000000001", "123456789.123456789"}

	for _, v := range values {
		t.Run(v, func(t *testing.T) {
			in := &doc{Amount: Decimal{Decimal: decimal.RequireFromString(v)}}

			b, err := bson.Marshal(in)
			require.NoError(t, err)

			// Must serialize as a BSON string (0x02), proving the codec fired
			// rather than falling back to embedded-document struct marshaling.
			require.Equalf(t, byte(bson.TypeString), b[4],
				"amount field must be BSON string (codec fired), raw=%x", b)

			out := &doc{}
			require.NoError(t, bson.Unmarshal(b, out))

			assert.Truef(t, in.Amount.Equal(out.Amount.Decimal),
				"document round-trip not decimal-exact: in=%s out=%s", in.Amount.String(), out.Amount.String())
			assert.Equal(t, in.Amount.String(), out.Amount.String())
		})
	}
}

// TestDecimal_ValueStruct_BypassesCodec locks the value-marshal footgun. The
// codec is a POINTER-receiver marshaler, so marshaling a Decimal field on a
// non-addressable VALUE struct bypasses MarshalBSONValue entirely and emits an
// embedded document (BSON type 0x03) instead of a string (0x02) — silently
// storing money in the wrong shape. Production is safe (it persists via
// pointers — InsertOne(&model)/Decode(&model)), but if a future refactor
// marshals by value, or someone changes the receiver, this test fails loudly
// and forces a conscious decision rather than corrupting stored amounts.
func TestDecimal_ValueStruct_BypassesCodec(t *testing.T) {
	t.Parallel()

	type doc struct {
		Amount Decimal `bson:"amount"`
	}

	// Marshal a VALUE (not &doc): the pointer-receiver codec is NOT in the
	// method set, so the driver falls back to embedded-document marshaling.
	b, err := bson.Marshal(doc{Amount: Decimal{Decimal: decimal.RequireFromString("100.5")}})
	require.NoError(t, err)

	assert.Equalf(t, byte(bson.TypeEmbeddedDocument), b[4],
		"value-struct marshal is expected to BYPASS the pointer codec and emit an "+
			"embedded document (0x03); if this changed, the value-path is now codec-driven "+
			"and the production-safety reasoning must be re-checked. raw=%x", b)
}

func TestDecimal_StringRepresentation(t *testing.T) {
	t.Parallel()

	d := Decimal{Decimal: decimal.RequireFromString("123.456789")}
	bsonType, data, err := d.MarshalBSONValue()
	require.NoError(t, err)
	assert.Equal(t, stringType, bsonType)

	var str string
	err = bson.UnmarshalValue(bson.Type(bsonType), data, &str)
	require.NoError(t, err)
	assert.Equal(t, "123.456789", str)
}

func TestDecimal_NilHandling(t *testing.T) {
	t.Parallel()

	var d *Decimal
	assert.Nil(t, d)

	bsonType, data, err := (&Decimal{}).MarshalBSONValue()
	require.NoError(t, err)
	assert.Equal(t, stringType, bsonType)
	assert.NotNil(t, data)
}

func TestDecimal_InvalidBSONType(t *testing.T) {
	t.Parallel()

	d := &Decimal{}
	err := d.UnmarshalBSONValue(byte(bson.TypeInt32), []byte{1, 2, 3, 4})
	assert.Error(t, err)
}
