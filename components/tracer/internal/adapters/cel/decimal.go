// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	celgo "github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker"
	"github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/overloads"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

const decimalTypeName = "tracer.Decimal"

var decimalCELType = celgo.OpaqueType(decimalTypeName)

// decimalValue implements only exact equality and the explicitly registered
// comparisons. It has no arithmetic, ordering trait, string or numeric cast.
type decimalValue struct {
	amount decimal.Decimal
	size   uint64
}

func (d decimalValue) ConvertToNative(target reflect.Type) (any, error) {
	if target == reflect.TypeFor[decimalValue]() {
		return d, nil
	}

	return nil, fmt.Errorf("decimal conversion is not supported: %w", constant.ErrExpressionType)
}

func (d decimalValue) ConvertToType(target ref.Type) ref.Val {
	if target == types.TypeType {
		return decimalCELType
	}

	if target.TypeName() == decimalTypeName {
		return d
	}

	return types.NewErr("Decimal conversion is not supported")
}

func (d decimalValue) Equal(other ref.Val) ref.Val {
	value, ok := other.(decimalValue)
	if !ok {
		return types.MaybeNoSuchOverloadErr(other)
	}

	return types.Bool(d.amount.Equal(value.amount))
}

func (d decimalValue) Type() ref.Type { return decimalCELType }
func (d decimalValue) Value() any     { return d.amount }

type decimalLibrary struct {
	limits tracercontract.Limits
}

func (decimalLibrary) LibraryName() string { return "tracer.decimal.v1" }

func (l decimalLibrary) CompileOptions() []celgo.EnvOption {
	options := make([]celgo.EnvOption, 0, 7)
	options = append(
		options,
		celgo.Function("decimal", celgo.Overload("tracer_decimal_string", []*celgo.Type{celgo.StringType}, decimalCELType,
			celgo.UnaryBinding(func(value ref.Val) ref.Val {
				raw, ok := value.(types.String)
				if !ok {
					return types.MaybeNoSuchOverloadErr(value)
				}

				// Only compile-validated literals reach this binding. Recheck bounds
				// at runtime as defense in depth; parsing never accepts an exponent.
				amount, err := tracercontract.Amount(raw).Decimal(context.Background(), l.limits)
				if err != nil {
					return types.NewErr("invalid Decimal literal")
				}

				return decimalValue{amount: amount, size: uint64(len(raw))}
			}))),
		celgo.ASTValidators(decimalLiteralValidator(l)),
	)

	for _, name := range []string{"equal", "lessThan", "lessOrEqual", "greaterThan", "greaterOrEqual"} {
		options = append(options, celgo.Function(name,
			celgo.MemberOverload("tracer_decimal_"+name, []*celgo.Type{decimalCELType, decimalCELType}, celgo.BoolType,
				celgo.BinaryBinding(func(left, right ref.Val) ref.Val { return compareDecimals(name, left, right) }))))
	}

	return options
}

func (l decimalLibrary) ProgramOptions() []celgo.ProgramOption {
	return []celgo.ProgramOption{celgo.CostTracking(decimalCostEstimator(l))}
}

func compareDecimals(operation string, left, right ref.Val) ref.Val {
	a, leftOK := left.(decimalValue)

	b, rightOK := right.(decimalValue)
	if !leftOK || !rightOK {
		return types.NoSuchOverloadErr()
	}

	comparison := a.amount.Cmp(b.amount)

	switch operation {
	case "equal":
		return types.Bool(comparison == 0)
	case "lessThan":
		return types.Bool(comparison < 0)
	case "lessOrEqual":
		return types.Bool(comparison <= 0)
	case "greaterThan":
		return types.Bool(comparison > 0)
	case "greaterOrEqual":
		return types.Bool(comparison >= 0)
	default:
		return types.NoSuchOverloadErr()
	}
}

type decimalLiteralValidator struct {
	limits tracercontract.Limits
}

func (decimalLiteralValidator) Name() string { return "tracer.decimal.literals" }

func (v decimalLiteralValidator) Validate(_ *celgo.Env, _ celgo.ValidatorConfig, tree *ast.AST, issues *celgo.Issues) {
	for _, call := range ast.MatchDescendants(ast.NavigateAST(tree), ast.FunctionMatcher("decimal")) {
		args := call.AsCall().Args()
		if len(args) != 1 || args[0].Kind() != ast.LiteralKind {
			issues.ReportErrorAtID(call.ID(), "decimal requires a string literal")
			continue
		}

		raw, ok := args[0].AsLiteral().(types.String)
		if !ok {
			issues.ReportErrorAtID(call.ID(), "decimal requires a string literal")
			continue
		}

		// CEL validators have no caller context; the adapter bounds the expression
		// and checks cancellation before/after compilation, and this parser bounds
		// the literal before allocating decimal coefficients.
		if _, err := tracercontract.Amount(raw).Decimal(context.Background(), v.limits); err != nil {
			issues.ReportErrorAtID(call.ID(), "invalid or oversized Decimal literal")
		}
	}
}

// decimalCostEstimator accounts for coefficient length and scale alignment.
// Cost units are conservative work estimates, not milliseconds or memory limits.
type decimalCostEstimator struct {
	limits tracercontract.Limits
}

func (e decimalCostEstimator) EstimateSize(node checker.AstNode) *checker.SizeEstimate {
	maximum := max(e.limits.MaxTextBytes, 36) // UUID strings have a fixed width.
	if node.Type().Kind() == types.ListKind {
		maximum = max(e.limits.MaxEntries, e.limits.MaxAccounts)

		if path := node.Path(); len(path) == 1 {
			switch path[0] {
			case "accounts", "debits":
				maximum = e.limits.MaxAccounts
			case "entries":
				maximum = e.limits.MaxEntries
			}
		}
	}

	estimate := checker.SizeEstimate{Min: 0, Max: uint64(maximum)}

	return &estimate
}

func (e decimalCostEstimator) EstimateCallCost(_ string, overload string, _ *checker.AstNode, args []checker.AstNode) *checker.CallEstimate {
	if !strings.HasPrefix(overload, "tracer_decimal_") && !isDecimalEquality(overload, args) {
		return nil
	}

	integerDigits, fractionDigits := e.limits.MaxIntegerDigits, e.limits.MaxFractionDigits
	if integerDigits <= 0 || fractionDigits < 0 {
		return nil // Adapter construction rejects invalid numeric bounds.
	}

	bound := checker.CostEstimate{Min: 1, Max: uint64(integerDigits)}.
		Add(checker.CostEstimate{Max: uint64(fractionDigits)}).
		Add(checker.CostEstimate{Max: 3}) // sign, decimal point and call overhead
	if overload != "tracer_decimal_string" {
		bound = bound.Add(bound)
	}

	return &checker.CallEstimate{CostEstimate: bound}
}

func isDecimalEquality(overload string, args []checker.AstNode) bool {
	return (overload == overloads.Equals || overload == overloads.NotEquals) && len(args) == 2 &&
		args[0].Type().TypeName() == decimalTypeName
}

func (decimalCostEstimator) CallCost(_ string, overload string, args []ref.Val, _ ref.Val) *uint64 {
	if overload == "tracer_decimal_string" && len(args) == 1 {
		if raw, ok := args[0].(types.String); ok {
			cost := uint64(len(raw)) + 1
			return &cost
		}
	}

	if len(args) == 2 {
		left, leftOK := args[0].(decimalValue)

		right, rightOK := args[1].(decimalValue)
		if leftOK && rightOK {
			cost := left.size + right.size + 1
			return &cost
		}
	}

	return nil
}
