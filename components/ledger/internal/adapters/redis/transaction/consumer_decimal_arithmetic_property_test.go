//go:build integration && property

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

// This test needs testcontainers (redistestutil), which in this package is
// gated behind the `integration` tag, not `property` — every other
// `property`-tagged file here runs pure in-process logic with no container.
// It carries both tags so it stays out of a bare `-tags property` run (and
// out of `-tags integration`, which would otherwise pull it into the default
// integration suite); run it explicitly with `-tags "integration property"`.

import (
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// PROPERTY-BASED TESTS — Decimal Arithmetic vs shopspring/decimal
//
// Random (a, b) decimal string pairs (integer part 1-30 digits, fractional
// part 0-24 digits, either sign) are run through sub_decimal/add_decimal/
// min_decimal/cmp_decimal on the real engine and checked against
// shopspring/decimal as the arbitrary-precision oracle.
//
// Run with:
//
//	go test -tags "integration property" -run TestProperty_DecimalArithmetic -v -count=1 \
//	    ./components/ledger/internal/adapters/redis/transaction/
// =============================================================================

const (
	decimalPropertySeed       = 1337
	decimalPropertyIterations = 300
	decimalPropertyMaxIntLen  = 30
	decimalPropertyMaxFracLen = 24
)

// randomDecimalString generates an arbitrary-magnitude, arbitrary-scale
// decimal string: 1-30 integer digits (leading zeros allowed, matching what
// split_decimal accepts), 0-24 fractional digits, either sign.
func randomDecimalString(r *rand.Rand) string {
	intLen := r.Intn(decimalPropertyMaxIntLen) + 1
	fracLen := r.Intn(decimalPropertyMaxFracLen + 1)
	negative := r.Intn(2) == 0

	var sb strings.Builder

	if negative {
		sb.WriteByte('-')
	}

	for i := 0; i < intLen; i++ {
		sb.WriteByte(byte('0' + r.Intn(10)))
	}

	if fracLen > 0 {
		sb.WriteByte('.')

		for i := 0; i < fracLen; i++ {
			sb.WriteByte(byte('0' + r.Intn(10)))
		}
	}

	return sb.String()
}

func TestProperty_DecimalArithmetic_MatchesShopspringOracle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping property test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	harness := newDecimalArithmeticHarness(t)

	r := rand.New(rand.NewSource(decimalPropertySeed))

	for i := 0; i < decimalPropertyIterations; i++ {
		a := randomDecimalString(r)
		b := randomDecimalString(r)

		aDec, err := decimal.NewFromString(a)
		require.NoErrorf(t, err, "iteration %d: generated operand %q must parse", i, a)
		bDec, err := decimal.NewFromString(b)
		require.NoErrorf(t, err, "iteration %d: generated operand %q must parse", i, b)

		gotSub, err := evalDecimalOp(t, harness, infra.redisContainer.Client, "sub", a, b)
		require.NoError(t, err)
		gotSubDec, err := decimal.NewFromString(gotSub)
		require.NoErrorf(t, err, "iteration %d: sub_decimal(%s, %s) = %q must parse", i, a, b, gotSub)
		require.Truef(t, gotSubDec.Equal(aDec.Sub(bDec)),
			"iteration %d: sub_decimal(%s, %s) = %s, want %s", i, a, b, gotSubDec, aDec.Sub(bDec))

		gotAdd, err := evalDecimalOp(t, harness, infra.redisContainer.Client, "add", a, b)
		require.NoError(t, err)
		gotAddDec, err := decimal.NewFromString(gotAdd)
		require.NoErrorf(t, err, "iteration %d: add_decimal(%s, %s) = %q must parse", i, a, b, gotAdd)
		require.Truef(t, gotAddDec.Equal(aDec.Add(bDec)),
			"iteration %d: add_decimal(%s, %s) = %s, want %s", i, a, b, gotAddDec, aDec.Add(bDec))

		gotMin, err := evalDecimalOp(t, harness, infra.redisContainer.Client, "min", a, b)
		require.NoError(t, err)
		gotMinDec, err := decimal.NewFromString(gotMin)
		require.NoErrorf(t, err, "iteration %d: min_decimal(%s, %s) = %q must parse", i, a, b, gotMin)
		require.Truef(t, gotMinDec.Equal(decimal.Min(aDec, bDec)),
			"iteration %d: min_decimal(%s, %s) = %s, want %s", i, a, b, gotMinDec, decimal.Min(aDec, bDec))

		gotCmp, err := evalDecimalOp(t, harness, infra.redisContainer.Client, "cmp", a, b)
		require.NoError(t, err)
		gotCmpInt, err := strconv.Atoi(gotCmp)
		require.NoErrorf(t, err, "iteration %d: cmp_decimal(%s, %s) = %q must be an integer", i, a, b, gotCmp)
		require.Equalf(t, aDec.Cmp(bDec), gotCmpInt, "iteration %d: cmp_decimal(%s, %s)", i, a, b)
	}
}
