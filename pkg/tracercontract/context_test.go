// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func testLimits() tracercontract.Limits {
	return tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 100, MaxIntegerDigits: 128, MaxFractionDigits: 128}
}

func testContext() tracercontract.Context {
	blocked := false
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	asset := "BTC"
	return tracercontract.Context{
		Accounts: []tracercontract.Account{{ID: id, Type: "deposit", Status: "CUSTOM_ACTIVE", Blocked: &blocked, Asset: asset}},
		Entries: []tracercontract.Entry{
			{AccountID: id, Direction: tracercontract.Debit, Amount: "0.00000001", Asset: asset},
			{External: true, Direction: tracercontract.Credit, Amount: "0.00000001", Asset: asset},
		},
	}
}

func TestContextValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		change  func(*tracercontract.Context)
		invalid bool
	}{
		{name: "native type status and fractional asset"},
		{name: "blocked true is a fact not a rejection", change: func(c *tracercontract.Context) { *c.Accounts[0].Blocked = true }},
		{name: "absent blocked is not false", invalid: true, change: func(c *tracercontract.Context) { c.Accounts[0].Blocked = nil }},
		{name: "no invented account type", invalid: true, change: func(c *tracercontract.Context) { c.Accounts[0].Type = "" }},
		{name: "no invented status", invalid: true, change: func(c *tracercontract.Context) { c.Accounts[0].Status = "" }},
		{name: "duplicate account", invalid: true, change: func(c *tracercontract.Context) { c.Accounts = append(c.Accounts, c.Accounts[0]) }},
		{name: "dangling entry", invalid: true, change: func(c *tracercontract.Context) {
			c.Entries[0].AccountID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
		}},
		{name: "external cannot claim internal id", invalid: true, change: func(c *tracercontract.Context) { c.Entries[0].External = true }},
		{name: "missing participant", invalid: true, change: func(c *tracercontract.Context) { c.Entries[1].External = false }},
		{name: "negative magnitude", invalid: true, change: func(c *tracercontract.Context) { c.Entries[0].Amount = "-1" }},
		{name: "zero magnitude", invalid: true, change: func(c *tracercontract.Context) { c.Entries[0].Amount = "0.000" }},
		{name: "invalid direction", invalid: true, change: func(c *tracercontract.Context) { c.Entries[0].Direction = "out" }},
		{name: "entry asset differs from account asset", invalid: true, change: func(c *tracercontract.Context) { c.Entries[0].Asset = "USD" }},
		{name: "external entry may carry another asset", change: func(c *tracercontract.Context) { c.Entries[1].Asset = "USD" }},
		{name: "entry asset differs from account asset by case", invalid: true, change: func(c *tracercontract.Context) { c.Entries[0].Asset = "btc" }},
		{name: "stored lowercase entry asset is a fact", change: func(c *tracercontract.Context) { c.Entries[1].Asset = "btc" }},
		{name: "stored lowercase account asset is a fact", change: func(c *tracercontract.Context) {
			c.Accounts[0].Asset = "btc"
			c.Entries[0].Asset = "btc"
		}},
		{name: "empty entry asset", invalid: true, change: func(c *tracercontract.Context) { c.Entries[1].Asset = "" }},
		{name: "stored asset with digit is a fact", change: func(c *tracercontract.Context) { c.Entries[1].Asset = "BTC2" }},
		{name: "asset beyond text bound", invalid: true, change: func(c *tracercontract.Context) {
			c.Entries[1].Asset = strings.Repeat("A", 101)
		}},
		{name: "no entries", invalid: true, change: func(c *tracercontract.Context) { c.Entries = nil }},
		{name: "external only", change: func(c *tracercontract.Context) {
			c.Accounts = nil
			c.Entries[0].AccountID = uuid.Nil
			c.Entries[0].External = true
		}},
		{name: "entry ceiling", invalid: true, change: func(c *tracercontract.Context) { c.Entries = make([]tracercontract.Entry, 21) }},
		{name: "no unused account context", invalid: true, change: func(c *tracercontract.Context) { c.Entries = c.Entries[1:] }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := testContext()
			if tt.change != nil {
				tt.change(&c)
			}
			err := c.Validate(context.Background(), testLimits())
			if tt.invalid {
				require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestContextRequiresLimits(t *testing.T) {
	t.Parallel()
	c := testContext()
	require.Error(t, c.Validate(context.Background(), tracercontract.Limits{}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, c.Validate(ctx, testLimits()), context.Canceled)
}

func TestAssetFactsCarryStoredLedgerCode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		asset string
		valid bool
	}{
		{"uppercase", "BTC", true},
		{"long uppercase", "LERIANPOINTS", true},
		{"legacy lowercase", "usd", true},
		{"legacy mixed", "USDC2", true},
		{"hundred multi-byte letters", strings.Repeat("É", utils.MaxAssetCodeLength), true},
		{"empty", "", false},
		{"nul", "US\x00D", false},
		{"invalid utf8", "US\xff", false},
		{"surrounding space", " USD", false},
		{"over hundred characters", strings.Repeat("A", utils.MaxAssetCodeLength+1), false},
		{"over hundred multi-byte characters", strings.Repeat("É", utils.MaxAssetCodeLength+1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.valid, tracercontract.ValidAssetCodeFact(tc.asset))
			limits := testLimits()
			limits.MaxTextBytes = 256
			id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
			for _, c := range []tracercontract.Context{
				{Entries: []tracercontract.Entry{{External: true, Direction: tracercontract.Debit, Amount: "1", Asset: tc.asset}}},
				{
					Accounts: []tracercontract.Account{{ID: id, Type: "deposit", Status: "ACTIVE", Blocked: new(bool), Asset: tc.asset}},
					Entries:  []tracercontract.Entry{{AccountID: id, Direction: tracercontract.Debit, Amount: "1", Asset: tc.asset}},
				},
			} {
				err := c.Validate(context.Background(), limits)
				if tc.valid {
					require.NoError(t, err)
					continue
				}
				require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
			}
		})
	}
}

func TestAssetCodeBoundIsIndependentOfTextBytes(t *testing.T) {
	t.Parallel()
	limits := testLimits()
	limits.MaxTextBytes = 8
	c := tracercontract.Context{Entries: []tracercontract.Entry{{
		External: true, Direction: tracercontract.Debit, Amount: "1", Asset: strings.Repeat("É", utils.MaxAssetCodeLength),
	}}}
	require.NoError(t, c.Validate(context.Background(), limits))
}

func TestAmountExactAndBounded(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"0.00000001", "9007199254740993.00000001", strings.Repeat("9", 128) + "." + strings.Repeat("1", 128)} {
		t.Run(raw, func(t *testing.T) {
			value, err := tracercontract.Amount(raw).Decimal(context.Background(), testLimits())
			require.NoError(t, err)
			require.Equal(t, raw, value.String())
		})
	}
	for _, raw := range []string{"", "1e999999999", "NaN", "Infinity", "+1", ".1", "1.", " 1", "01", "1_000", strings.Repeat("1", 129), "0." + strings.Repeat("1", 129)} {
		t.Run("reject "+raw, func(t *testing.T) {
			_, err := tracercontract.Amount(raw).Decimal(context.Background(), testLimits())
			require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
		})
	}
}

func TestContextJSONKeepsPresenceAndExactAmounts(t *testing.T) {
	t.Parallel()
	c := testContext()
	encoded, err := json.Marshal(c)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"blocked":false`)
	require.Contains(t, string(encoded), `"amount":"0.00000001"`)
	require.NotContains(t, string(encoded), `"accountId":"00000000-0000-0000-0000-000000000000"`)
	var decoded tracercontract.Context
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, c, decoded)
	require.Error(t, json.Unmarshal([]byte(`{"amount":0.1}`), &tracercontract.Entry{}))
}

func TestAmountFromDecimalBoundsBeforeFormatting(t *testing.T) {
	t.Parallel()
	amount, err := tracercontract.AmountFromDecimal(context.Background(), decimal.RequireFromString("9007199254740993.00000001"), testLimits())
	require.NoError(t, err)
	require.Equal(t, tracercontract.Amount("9007199254740993.00000001"), amount)
	for _, exponent := range []int32{1000000000, -1000000000, -2147483648} {
		_, err := tracercontract.AmountFromDecimal(context.Background(), decimal.New(1, exponent), testLimits())
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	}
}

func TestAmountFromDecimalUsesSignificantFractionDigits(t *testing.T) {
	t.Parallel()

	hundred := decimal.RequireFromString("100.00")
	cases := []struct {
		name     string
		value    decimal.Decimal
		fraction int
		want     string
	}{
		{"share", hundred.Mul(decimal.NewFromInt(50).Div(decimal.NewFromInt(100))).Mul(decimal.NewFromInt(100).Div(decimal.NewFromInt(100))), 0, "50"},
		{"percentage fee", hundred.Mul(decimal.RequireFromString("1.5").Div(decimal.NewFromInt(100))), 1, "1.5"},
		{"integer padding", decimal.RequireFromString("100.000000000"), 0, "100"},
		{"fraction padding", decimal.RequireFromString("-1.230000000"), 2, "-1.23"},
		{"leading fractional zeros", decimal.RequireFromString("0.00000001000"), 8, "0.00000001"},
		{"zero extreme negative exponent", decimal.New(0, -2147483648), 0, "0"},
		{"zero extreme positive exponent", decimal.New(0, 2147483647), 0, "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limits := testLimits()
			limits.MaxFractionDigits = tc.fraction
			amount, err := tracercontract.AmountFromDecimal(context.Background(), tc.value, limits)
			require.NoError(t, err)
			require.Equal(t, tracercontract.Amount(tc.want), amount)
			parsed, err := amount.Decimal(context.Background(), limits)
			require.NoError(t, err)
			if tc.value.IsZero() {
				require.True(t, parsed.IsZero())
			} else {
				require.True(t, tc.value.Equal(parsed))
			}
			if tc.fraction > 0 {
				limits.MaxFractionDigits--
				_, err := tracercontract.AmountFromDecimal(context.Background(), tc.value, limits)
				require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
			}
		})
	}
}
