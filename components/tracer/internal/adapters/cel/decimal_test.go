// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"strings"
	"testing"

	celgo "github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func decimalTestLimits() tracercontract.Limits {
	return tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
}

func decimalTestEnv(t *testing.T) *celgo.Env {
	t.Helper()
	env, err := celgo.NewEnv(celgo.Lib(decimalLibrary{limits: decimalTestLimits()}), celgo.Variable("raw", celgo.StringType))
	require.NoError(t, err)

	return env
}

func TestDecimalExactComparisons(t *testing.T) {
	t.Parallel()
	for _, expression := range []string{
		`decimal("0.1").equal(decimal("0.10"))`,
		`decimal("9007199254740993.00000001").greaterThan(decimal("9007199254740993"))`,
		`decimal("0.00000001").lessThan(decimal("0.00000002"))`,
		`decimal("-1").lessOrEqual(decimal("-1.00"))`,
		`decimal("2").greaterOrEqual(decimal("1.9999999999999999999999"))`,
		`decimal("2").lessThan(decimal("10"))`,
		`decimal("0").equal(decimal("-0.000"))`,
		`decimal("0.1") == decimal("0.10")`,
	} {
		t.Run(expression, func(t *testing.T) {
			t.Parallel()
			env := decimalTestEnv(t)
			checked, issues := env.Compile(expression)
			require.NoError(t, issues.Err())
			program, err := env.Program(checked, celgo.CostLimit(10000))
			require.NoError(t, err)
			value, detail, err := program.ContextEval(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.Equal(t, true, value.Value())
			require.NotNil(t, detail.ActualCost())
			require.Positive(t, *detail.ActualCost())
		})
	}
}

func TestDecimalRejectsUnsupportedSyntaxAtCompileTime(t *testing.T) {
	t.Parallel()
	for _, expression := range []string{
		`decimal(raw).equal(decimal("1"))`,
		`decimal("1" + "0").equal(decimal("10"))`,
		`decimal("1e1000000000").equal(decimal("1"))`,
		`decimal("NaN").equal(decimal("1"))`,
		`decimal("01").equal(decimal("1"))`,
		`decimal("1").greaterThan(0.5)`,
		`double(decimal("1")) > 0.0`,
		`int(decimal("1")) > 0`,
		`string(decimal("1")) == "1"`,
		`decimal("1") / decimal("3") == decimal("0.3")`,
		`decimal("1") < decimal("2")`,
		`decimal("` + strings.Repeat("9", 129) + `").equal(decimal("1"))`,
	} {
		t.Run(expression, func(t *testing.T) {
			t.Parallel()
			_, issues := decimalTestEnv(t).Compile(expression)
			require.Error(t, issues.Err())
		})
	}
}

func TestDecimalDynamicCastsStillFail(t *testing.T) {
	t.Parallel()
	for _, cast := range []string{"double", "int", "string"} {
		t.Run(cast, func(t *testing.T) {
			env := decimalTestEnv(t)
			checked, issues := env.Compile(cast + `(dyn(decimal("1")))`)
			require.NoError(t, issues.Err())
			program, err := env.Program(checked)
			require.NoError(t, err)
			_, _, err = program.ContextEval(context.Background(), map[string]any{})
			require.Error(t, err)
		})
	}
}

func TestDecimalCostScalesWithOperands(t *testing.T) {
	t.Parallel()
	env := decimalTestEnv(t)
	measure := func(raw string) uint64 {
		t.Helper()
		checked, issues := env.Compile(`decimal("` + raw + `").greaterThan(decimal("0"))`)
		require.NoError(t, issues.Err())
		program, err := env.Program(checked, celgo.CostLimit(10000))
		require.NoError(t, err)
		_, detail, err := program.ContextEval(context.Background(), map[string]any{})
		require.NoError(t, err)

		return *detail.ActualCost()
	}
	require.Greater(t, measure(strings.Repeat("9", 128)), measure("1"))
	checked, issues := env.Compile(`decimal("` + strings.Repeat("9", 128) + `").greaterThan(decimal("0"))`)
	require.NoError(t, issues.Err())
	program, err := env.Program(checked, celgo.CostLimit(10))
	require.NoError(t, err)
	_, _, err = program.ContextEval(context.Background(), map[string]any{})
	require.ErrorContains(t, err, "cost limit exceeded")
}

func TestDecimalEstimatedCostBoundsRuntimeAtMaximumScale(t *testing.T) {
	t.Parallel()
	env := decimalTestEnv(t)
	raw := "-" + strings.Repeat("9", 128) + "." + strings.Repeat("9", 128)
	for _, expression := range []string{
		`decimal("` + raw + `").equal(decimal("` + raw + `"))`,
		`decimal("` + raw + `") == decimal("` + raw + `")`,
	} {
		checked, issues := env.Compile(expression)
		require.NoError(t, issues.Err())
		estimated, err := checker.Cost(checked.NativeRep(), decimalCostEstimator{limits: decimalTestLimits()})
		require.NoError(t, err)
		program, err := env.Program(checked)
		require.NoError(t, err)
		_, details, err := program.ContextEval(context.Background(), map[string]any{})
		require.NoError(t, err)
		require.GreaterOrEqual(t, estimated.Max, *details.ActualCost())
	}
}
