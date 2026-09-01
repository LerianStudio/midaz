//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// DECIMAL ARITHMETIC INTEGRATION TESTS
//
// These tests exercise sub_decimal/add_decimal/min_decimal/cmp_decimal from
// balance_atomic_operation.lua directly against the real engine (Valkey via
// testcontainers), independent of the balance-mutation path that main()
// drives. They lock the string-based comparison fix (cmp_decimal replacing
// the IEEE-754 double comparison) and the borrow-invariant tripwire.
// =============================================================================

// decimalArithmeticHarnessLua dispatches to the pure arithmetic helpers
// defined earlier in balance_atomic_operation.lua. It is appended to the
// embedded script after main()'s entrypoint is stripped, so the helpers run
// unmodified against the real Lua engine without invoking any balance
// mutation.
const decimalArithmeticHarnessLua = `
local op = ARGV[1]
local a = ARGV[2]
local b = ARGV[3]

if op == "sub" then
    return sub_decimal(a, b)
elseif op == "add" then
    return add_decimal(a, b)
elseif op == "min" then
    return min_decimal(a, b)
elseif op == "cmp" then
    return tostring(cmp_decimal(a, b))
end

return redis.error_reply("decimal harness: unknown op " .. tostring(op))
`

// decimalArithmeticScript strips the "return main()" entrypoint from the
// embedded script and appends decimalArithmeticHarnessLua, exposing the pure
// arithmetic helpers for direct EVAL.
func decimalArithmeticScript(t *testing.T) string {
	t.Helper()

	idx := strings.LastIndex(balanceAtomicOperationLua, "return main()")
	require.Greaterf(t, idx, -1, "expected balance_atomic_operation.lua to contain the main() entrypoint")

	return balanceAtomicOperationLua[:idx] + decimalArithmeticHarnessLua
}

// newDecimalArithmeticHarness builds the *redis.Script once for a test, so
// repeated evaluations benefit from server-side script caching (EVALSHA)
// instead of re-sending the source on every call.
func newDecimalArithmeticHarness(t *testing.T) *redis.Script {
	t.Helper()

	return redis.NewScript(decimalArithmeticScript(t))
}

// evalDecimalOp runs one of sub/add/min/cmp against the real engine and
// returns the raw string result the script produced.
func evalDecimalOp(t *testing.T, harness *redis.Script, client redis.Scripter, op, a, b string) (string, error) {
	t.Helper()

	result, err := harness.Run(context.Background(), client, []string{}, op, a, b).Result()
	if err != nil {
		return "", err
	}

	s, ok := result.(string)
	require.Truef(t, ok, "expected string result for op %q, got %T", op, result)

	return s, nil
}

// assertDecimalEquivalent cross-checks the script's canonical string result
// against shopspring/decimal as a numeric oracle, guarding against a golden
// case whose expected literal drifted from the value it names.
func assertDecimalEquivalent(t *testing.T, want, got string, expectedValue decimal.Decimal) {
	t.Helper()

	gotDec, err := decimal.NewFromString(got)
	require.NoErrorf(t, err, "script result %q for expected %q must parse as a decimal", got, want)
	assert.Truef(t, gotDec.Equal(expectedValue),
		"script result %q (%s) must equal the oracle value %s", got, gotDec, expectedValue)
}

func TestIntegration_DecimalArithmetic_SubDecimal_GoldenCases(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	harness := newDecimalArithmeticHarness(t)

	cases := []struct {
		name string
		a, b string
		want string
	}{
		// Audit repro cases (finding G1): the IEEE-754 double comparison in
		// the pre-fix sub_decimal broke at these magnitudes/scales.
		{"repro_18_decimal_places", "1.000000000000000001", "1.000000000000000002", "-0.000000000000000001"},
		{"repro_19_digit_integers", "1000000000000000000", "1000000000000000001", "-1"},
		{"repro_8_decimals_at_1e9", "1000000000.00000001", "1000000000.00000002", "-0.00000001"},
		{"repro_2_decimals_at_1e14", "100000000000000.01", "100000000000000.02", "-0.01"},

		// Small values that already worked pre-fix; must not regress.
		{"small_positive_result", "10.50", "3.25", "7.25"},
		{"small_negative_result", "3.25", "10.50", "-7.25"},

		// Both signs.
		{"both_negative_a_more_negative", "-10", "-3", "-7"},
		{"both_negative_b_more_negative", "-3", "-10", "7"},
		{"negative_minus_positive", "-5", "3", "-8"},
		{"positive_minus_negative", "5", "-3", "8"},

		// Zeros and equal values.
		{"zero_minus_zero", "0", "0", "0"},
		{"equal_decimals_result_zero", "42.5", "42.5", "0"},
		{"equal_integers_result_zero", "100", "100", "0"},

		// Leading zeros in the integer part.
		{"leading_zeros_positive_result", "007", "003", "4"},
		{"leading_zeros_negative_result", "003", "007", "-4"},

		// Fraction-only and integer-only operands.
		{"fraction_only_positive_result", "0.5", "0.25", "0.25"},
		{"fraction_only_negative_result", "0.25", "0.5", "-0.25"},
		{"integer_only", "1000000", "1", "999999"},

		// Mixed scales.
		{"mixed_scale_a_more_decimals", "1.23456", "1.2", "0.03456"},
		{"mixed_scale_b_more_decimals", "1.2", "1.23456", "-0.03456"},

		// High magnitude beyond double precision, no fraction.
		{"high_magnitude_integer", "999999999999999999999999999999", "1", "999999999999999999999999999998"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evalDecimalOp(t, harness, infra.redisContainer.Client, "sub", tc.a, tc.b)
			require.NoError(t, err)
			assert.Equalf(t, tc.want, got, "sub_decimal(%s, %s)", tc.a, tc.b)

			aDec, err := decimal.NewFromString(tc.a)
			require.NoError(t, err)
			bDec, err := decimal.NewFromString(tc.b)
			require.NoError(t, err)
			assertDecimalEquivalent(t, tc.want, got, aDec.Sub(bDec))
		})
	}
}

func TestIntegration_DecimalArithmetic_AddDecimal_GoldenCases(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	harness := newDecimalArithmeticHarness(t)

	cases := []struct {
		name string
		a, b string
		want string
	}{
		{"high_magnitude_carries_a_new_digit", "999999999999999999999999999999", "1", "1000000000000000000000000000000"},
		{"repro_18_decimal_places", "1.000000000000000001", "1.000000000000000002", "2.000000000000000003"},
		{"repro_19_digit_integers", "1000000000000000000", "1000000000000000001", "2000000000000000001"},
		{"both_negative", "-5", "-3", "-8"},
		{"negative_plus_positive", "-5", "3", "-2"},
		{"positive_plus_negative", "5", "-3", "2"},
		{"small_values", "10.50", "3.25", "13.75"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evalDecimalOp(t, harness, infra.redisContainer.Client, "add", tc.a, tc.b)
			require.NoError(t, err)
			assert.Equalf(t, tc.want, got, "add_decimal(%s, %s)", tc.a, tc.b)

			aDec, err := decimal.NewFromString(tc.a)
			require.NoError(t, err)
			bDec, err := decimal.NewFromString(tc.b)
			require.NoError(t, err)
			assertDecimalEquivalent(t, tc.want, got, aDec.Add(bDec))
		})
	}
}

func TestIntegration_DecimalArithmetic_MinDecimal_GoldenCases(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	harness := newDecimalArithmeticHarness(t)

	cases := []struct {
		name string
		a, b string
		want string
	}{
		// Audit repro: pre-fix min_decimal (built on the buggy sub_decimal)
		// returned the LARGER operand at this magnitude.
		{"repro_min_18_digit_integers_a_smaller", "100000000000000001", "100000000000000002", "100000000000000001"},
		{"repro_min_18_digit_integers_b_smaller", "100000000000000002", "100000000000000001", "100000000000000001"},

		{"small_a_smaller", "3.25", "10.50", "3.25"},
		{"small_b_smaller", "10.50", "3.25", "3.25"},
		{"equal_returns_b", "5", "5", "5"},
		{"negative_a_smaller", "-5", "3", "-5"},
		{"negative_b_smaller", "5", "-3", "-3"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evalDecimalOp(t, harness, infra.redisContainer.Client, "min", tc.a, tc.b)
			require.NoError(t, err)
			assert.Equalf(t, tc.want, got, "min_decimal(%s, %s)", tc.a, tc.b)

			aDec, err := decimal.NewFromString(tc.a)
			require.NoError(t, err)
			bDec, err := decimal.NewFromString(tc.b)
			require.NoError(t, err)
			assertDecimalEquivalent(t, tc.want, got, decimal.Min(aDec, bDec))
		})
	}
}

func TestIntegration_DecimalArithmetic_CmpDecimal_GoldenCases(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	harness := newDecimalArithmeticHarness(t)

	cases := []struct {
		name string
		a, b string
		want int
	}{
		{"repro_19_digit_integers_a_smaller", "1000000000000000000", "1000000000000000001", -1},
		{"repro_18_decimal_places_a_smaller", "1.000000000000000001", "1.000000000000000002", -1},
		{"equal_integers", "100", "100", 0},
		{"equal_decimals", "42.50", "42.5", 0},
		{"a_greater", "10.50", "3.25", 1},
		{"a_smaller", "3.25", "10.50", -1},
		{"both_negative_a_greater", "-3", "-10", 1},
		{"both_negative_a_smaller", "-10", "-3", -1},

		// Zero normalization: -0, 0.00 and 0 all compare equal to zero,
		// regardless of the literal sign carried in the string.
		{"negative_zero_equals_zero", "-0", "0", 0},
		{"zero_equals_padded_zero", "0.00", "0", 0},
		{"negative_padded_zero_equals_zero", "-0.00", "0", 0},
		{"zero_equals_negative_zero_reversed", "0", "-0", 0},

		{"leading_zeros_equal", "007", "7", 0},
		{"leading_zeros_a_smaller", "007", "008", -1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evalDecimalOp(t, harness, infra.redisContainer.Client, "cmp", tc.a, tc.b)
			require.NoError(t, err)

			gotInt, err := strconv.Atoi(got)
			require.NoErrorf(t, err, "cmp_decimal(%s, %s) must return an integer, got %q", tc.a, tc.b, got)
			assert.Equalf(t, tc.want, gotInt, "cmp_decimal(%s, %s)", tc.a, tc.b)

			aDec, err := decimal.NewFromString(tc.a)
			require.NoError(t, err)
			bDec, err := decimal.NewFromString(tc.b)
			require.NoError(t, err)
			assert.Equal(t, aDec.Cmp(bDec), gotInt, "cmp_decimal must agree with the shopspring/decimal oracle")
		})
	}
}
