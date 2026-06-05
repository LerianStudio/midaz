// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// P4-T23 — serialization decimal-precision boundary.
//
// These tests round-trip high-precision decimals through the two real
// serialization seams that the persisted transaction body and the async
// (RabbitMQ) recovery payload pass through, and assert decimal.Equal on the
// way back. They lock the "no lossy boundary" finding documented in
// docs/monorepo/plan/artifacts/P4-decimal-precision-boundary.md: both codecs
// route decimal.Decimal through its full string representation, so neither
// truncates. The Postgres amount column is unbounded DECIMAL and out of scope.

// highPrecisionDecimals are the adversarial values exercised on each seam: a
// 1/3 repeating residual carried to 30 places (well past decimal's
// DivisionPrecision of 16), an 18-decimal ETH-scale dust value, and a large
// integer with a long fractional tail. If any seam truncated, decimal.Equal
// on the round-trip would fail for at least one of these.
func highPrecisionDecimals(t *testing.T) []decimal.Decimal {
	t.Helper()

	return []decimal.Decimal{
		decimal.RequireFromString("0.333333333333333333333333333333"),
		decimal.RequireFromString("0.000000000000000001"),
		decimal.RequireFromString("123456789012345678.987654321098765432"),
		decimal.NewFromInt(1).Div(decimal.NewFromInt(3)), // bounded at DivisionPrecision=16
		decimal.RequireFromString("999999999999999999999999.999999999999999999"),
	}
}

// TestDecimalBoundary_JSONBBody_RoundTripFullPrecision marshals a Transaction
// (the value persisted into the JSONB `body` column via json.Marshal, mirrored
// by the json.Unmarshal at transaction.postgresql.go) and asserts every
// decimal field survives decimal.Equal. This is seam #1.
func TestDecimalBoundary_JSONBBody_RoundTripFullPrecision(t *testing.T) {
	t.Parallel()

	for _, d := range highPrecisionDecimals(t) {
		d := d

		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			body := mtransaction.Transaction{
				Send: mtransaction.Send{
					Asset: "USD",
					Value: d,
					Source: mtransaction.Source{
						From: []mtransaction.FromTo{
							{AccountAlias: "@a", Amount: &mtransaction.Amount{Asset: "USD", Value: d}},
						},
					},
					Distribute: mtransaction.Distribute{
						To: []mtransaction.FromTo{
							{AccountAlias: "@b", Amount: &mtransaction.Amount{Asset: "USD", Value: d}},
						},
					},
				},
			}

			raw, err := json.Marshal(body)
			require.NoError(t, err)

			var got mtransaction.Transaction
			require.NoError(t, json.Unmarshal(raw, &got))

			require.Truef(t, got.Send.Value.Equal(d),
				"JSONB body Send.Value lost precision: got=%s want=%s", got.Send.Value, d)
			require.Truef(t, got.Send.Source.From[0].Amount.Value.Equal(d),
				"JSONB body From leg lost precision: got=%s want=%s", got.Send.Source.From[0].Amount.Value, d)
			require.Truef(t, got.Send.Distribute.To[0].Amount.Value.Equal(d),
				"JSONB body To leg lost precision: got=%s want=%s", got.Send.Distribute.To[0].Amount.Value, d)
		})
	}
}

// TestDecimalBoundary_MsgpackPayload_RoundTripFullPrecision encodes the actual
// TransactionProcessingPayload struct (the async RabbitMQ / crash-recovery
// payload — Validate *mtransaction.Responses + Input *mtransaction.Transaction
// + Balances) with the real vmihailenco/msgpack/v5 codec and asserts every
// decimal survives decimal.Equal. This is seam #2 (the one P4-T25 worries
// about): if it truncated, a worker reconstructing from the backup seed would
// under/over-charge.
func TestDecimalBoundary_MsgpackPayload_RoundTripFullPrecision(t *testing.T) {
	t.Parallel()

	for _, d := range highPrecisionDecimals(t) {
		d := d

		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			payload := TransactionProcessingPayload{
				Validate: &mtransaction.Responses{
					Total: d,
					Asset: "USD",
					From:  map[string]mtransaction.Amount{"@a": {Asset: "USD", Value: d}},
					To:    map[string]mtransaction.Amount{"@b": {Asset: "USD", Value: d}},
				},
				Balances: []*mmodel.Balance{
					{Available: d, OnHold: d, AssetCode: "USD"},
				},
				Input: &mtransaction.Transaction{
					Send: mtransaction.Send{Asset: "USD", Value: d},
				},
				Version: "v2",
			}

			raw, err := msgpack.Marshal(payload)
			require.NoError(t, err)

			var got TransactionProcessingPayload
			require.NoError(t, msgpack.Unmarshal(raw, &got))

			require.Truef(t, got.Validate.Total.Equal(d),
				"msgpack Validate.Total lost precision: got=%s want=%s", got.Validate.Total, d)
			require.Truef(t, got.Validate.From["@a"].Value.Equal(d),
				"msgpack Validate.From leg lost precision: got=%s want=%s", got.Validate.From["@a"].Value, d)
			require.Truef(t, got.Validate.To["@b"].Value.Equal(d),
				"msgpack Validate.To leg lost precision: got=%s want=%s", got.Validate.To["@b"].Value, d)
			require.Truef(t, got.Balances[0].Available.Equal(d),
				"msgpack Balance.Available lost precision: got=%s want=%s", got.Balances[0].Available, d)
			require.Truef(t, got.Input.Send.Value.Equal(d),
				"msgpack Input.Send.Value lost precision: got=%s want=%s", got.Input.Send.Value, d)
		})
	}
}
