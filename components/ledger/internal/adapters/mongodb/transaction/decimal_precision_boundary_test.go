// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// P4-T23 — serialization decimal-precision boundary, seam #3 (Mongo metadata
// mirror). Transaction/operation metadata is persisted as MetadataMongoDBModel
// with Data JSON (map[string]any) under the bson:"metadata" tag, encoded by the
// go.mongodb.org/mongo-driver/v2 bson codec. These tests round-trip
// high-precision decimal-derived values through the real bson codec and assert
// decimal.Equal, locking the "no lossy boundary" guarantee for this seam.
//
// IMPORTANT: bson has no native arbitrary-precision decimal type that
// decimal.Decimal marshals to automatically (its fields are unexported, so a
// raw decimal.Decimal in a map[string]any does NOT round-trip through bson).
// The metadata contract is flat free-form key/value; any decimal-derived value
// MUST be stored as its String() form to survive the seam. The first test
// proves String-form is lossless; the second documents that the raw-typed form
// is NOT supported by the bson codec (which is why the contract is string-only).

func boundaryDecimals(t *testing.T) []decimal.Decimal {
	t.Helper()

	return []decimal.Decimal{
		decimal.RequireFromString("0.333333333333333333333333333333"),
		decimal.RequireFromString("0.000000000000000001"),
		decimal.RequireFromString("123456789012345678.987654321098765432"),
	}
}

// TestDecimalBoundary_MongoMetadata_StringForm_RoundTrip proves that a
// decimal-derived value stored as its String() representation inside the
// metadata document survives a full bson Marshal/Unmarshal round-trip and
// reparses to a decimal.Equal value. This is the supported contract.
func TestDecimalBoundary_MongoMetadata_StringForm_RoundTrip(t *testing.T) {
	t.Parallel()

	for _, d := range boundaryDecimals(t) {
		d := d

		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			model := MetadataMongoDBModel{
				EntityID:   "tx-1",
				EntityName: "Transaction",
				Data:       JSON{"feeResidual": d.String()},
			}

			raw, err := bson.Marshal(model)
			require.NoError(t, err)

			var got MetadataMongoDBModel
			require.NoError(t, bson.Unmarshal(raw, &got))

			str, ok := got.Data["feeResidual"].(string)
			require.Truef(t, ok, "metadata value did not round-trip as string: %T", got.Data["feeResidual"])

			parsed, err := decimal.NewFromString(str)
			require.NoError(t, err)
			require.Truef(t, parsed.Equal(d),
				"Mongo metadata String-form lost precision: got=%s want=%s", parsed, d)
		})
	}
}

// TestDecimalBoundary_MongoMetadata_RawDecimal_NotPreserved documents that the
// bson codec does NOT preserve a raw decimal.Decimal placed in metadata
// (unexported fields marshal to an empty document). This is the negative
// boundary: it is why the metadata contract requires decimals to be stored as
// strings, not as raw decimal.Decimal values.
func TestDecimalBoundary_MongoMetadata_RawDecimal_NotPreserved(t *testing.T) {
	t.Parallel()

	d := decimal.RequireFromString("0.333333333333333333333333333333")

	model := MetadataMongoDBModel{
		EntityID:   "tx-1",
		EntityName: "Transaction",
		Data:       JSON{"feeResidual": d},
	}

	raw, err := bson.Marshal(model)
	require.NoError(t, err)

	var got MetadataMongoDBModel
	require.NoError(t, bson.Unmarshal(raw, &got))

	// The raw decimal did NOT come back as a decimal.Decimal — it round-trips
	// as an (empty) sub-document, confirming the seam cannot carry raw decimals.
	_, isDecimal := got.Data["feeResidual"].(decimal.Decimal)
	require.Falsef(t, isDecimal,
		"unexpected: bson preserved a raw decimal.Decimal; the string-only contract may no longer be required")
}
