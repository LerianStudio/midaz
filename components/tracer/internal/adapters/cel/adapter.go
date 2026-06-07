// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package cel provides CEL expression compilation and evaluation.
package cel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// missingKeyErrPrefix is the prefix cel-go uses when a map key lookup misses.
// Source: github.com/google/cel-go/common/types/map.go and interpreter/attributes.go
// emit errors formatted as "no such key: <key>". The string format is stable
// across cel-go versions used by this project.
const missingKeyErrPrefix = "no such key:"

// IsMissingKeyError reports whether err originated from a cel-go map lookup
// for a key that is not present in the activation (e.g. metadata["channel"]
// when "channel" is absent). Detection walks the full error tree (including
// multi-error wrappers via Unwrap() []error) and matches on the stable cel-go
// message prefix because the underlying resolutionError type is unexported in
// cel-go.
//
// Returns false for other CEL runtime errors (type mismatch, division by zero,
// no such overload, etc.) so the caller can still propagate genuine failures.
func IsMissingKeyError(err error) bool {
	if err == nil {
		return false
	}

	type multiUnwrap interface{ Unwrap() []error }

	type singleUnwrap interface{ Unwrap() error }

	for e := err; e != nil; {
		if strings.HasPrefix(e.Error(), missingKeyErrPrefix) {
			return true
		}

		if mu, ok := e.(multiUnwrap); ok {
			for _, child := range mu.Unwrap() {
				if IsMissingKeyError(child) {
					return true
				}
			}

			return false
		}

		if su, ok := e.(singleUnwrap); ok {
			e = su.Unwrap()
			continue
		}

		return false
	}

	return false
}

// defaultCostEstimator provides default size estimates for CEL cost calculation.
// It implements checker.CostEstimator interface with conservative estimates.
type defaultCostEstimator struct{}

// EstimateSize returns a default size estimate for variable-sized elements.
// Uses conservative estimates to prevent underestimation of expression cost.
func (e *defaultCostEstimator) EstimateSize(element checker.AstNode) *checker.SizeEstimate {
	// Return a conservative default size for unknown elements
	// This ensures expressions with variable-sized inputs are properly costed
	estimate := checker.SizeEstimate{Min: 0, Max: 100}
	return &estimate
}

// EstimateCallCost returns nil to use default cost estimation for function calls.
func (e *defaultCostEstimator) EstimateCallCost(function, overloadID string, target *checker.AstNode, args []checker.AstNode) *checker.CallEstimate {
	return nil
}

// safePrefix returns the first n characters of s, or the whole string if shorter.
func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n]
}

// ExpressionEngine compiles and evaluates CEL expressions.
// Interface defined locally per Ring pattern (depend on abstractions, not concretions).
type ExpressionEngine interface {
	// Compile validates and compiles a CEL expression.
	// Returns a CompiledProgram that can be used for evaluation.
	Compile(ctx context.Context, expression string) (*CompiledProgram, error)

	// Evaluate runs a compiled program against a ValidationRequest.
	// Returns the boolean result of the expression.
	Evaluate(ctx context.Context, program *CompiledProgram, req *model.ValidationRequest) (bool, error)
}

// AdapterConfig holds configuration for the CEL adapter.
type AdapterConfig struct {
	// CostLimit is the maximum cost for CEL expression evaluation.
	// Read from CEL_COST_LIMIT env var (default: 10000).
	CostLimit uint64
}

// Adapter implements ExpressionEngine using google/cel-go.
type Adapter struct {
	env       *Environment
	logger    libLog.Logger
	costLimit uint64
}

// NewAdapter creates a CEL adapter with the given configuration and logger.
// Returns an error if logger is nil or environment creation fails.
func NewAdapter(cfg AdapterConfig, logger libLog.Logger) (*Adapter, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	env, err := NewEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	costLimit := cfg.CostLimit
	if costLimit == 0 {
		costLimit = DefaultCostLimit
	}

	return &Adapter{
		env:       env,
		logger:    logger,
		costLimit: costLimit,
	}, nil
}

// Compile validates and compiles a CEL expression.
// Uses OpenTelemetry tracing with span name: adapter.cel.compile
func (a *Adapter) Compile(ctx context.Context, expression string) (*CompiledProgram, error) {
	start := time.Now()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "adapter.cel.compile")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Validate expression is not empty (fail fast before any processing)
	if expression == "" {
		err := fmt.Errorf("%w: expression cannot be empty", constant.ErrExpressionSyntax)
		libOtel.HandleSpanBusinessErrorEvent(span, "empty expression", err)

		return nil, err
	}

	// Compute expression hash
	hash := HashExpression(expression)

	span.SetAttributes(
		attribute.String("app.request.expression_hash", hash),
		attribute.Int("app.request.expression_length", len(expression)),
	)

	// Compile to AST
	ast, err := a.env.Compile(expression)
	if err != nil {
		// Use structured error classification from CompileError
		var (
			wrappedErr error
			compileErr *CompileError
		)

		if errors.As(err, &compileErr) {
			// Use the structured IsTypeError flag for deterministic classification
			if compileErr.IsTypeError {
				wrappedErr = fmt.Errorf("%w: %w", constant.ErrExpressionType, err)
				libOtel.HandleSpanBusinessErrorEvent(span, "type error", wrappedErr)
			} else {
				wrappedErr = fmt.Errorf("%w: %w", constant.ErrExpressionSyntax, err)
				libOtel.HandleSpanBusinessErrorEvent(span, "compilation failed", wrappedErr)
			}
		} else {
			// Fallback for unexpected error types (shouldn't happen with our Environment)
			wrappedErr = fmt.Errorf("%w: %w", constant.ErrExpressionSyntax, err)
			libOtel.HandleSpanBusinessErrorEvent(span, "compilation failed", wrappedErr)
		}

		return nil, wrappedErr
	}

	// Validate boolean return type
	if ast.OutputType() != cel.BoolType {
		err := fmt.Errorf("%w: expression returns %v, expected bool", constant.ErrExpressionType, ast.OutputType())
		libOtel.HandleSpanBusinessErrorEvent(span, "type validation failed", err)

		return nil, err
	}

	// Estimate expression cost and reject if it exceeds the limit
	// This prevents creation of expensive rules that would cause DoS at evaluation time
	costEstimate, err := checker.Cost(ast.NativeRep(), &defaultCostEstimator{})
	if err != nil {
		// Use distinct error for estimation failures vs actual cost exceeded
		costErr := fmt.Errorf("%w: %w", constant.ErrExpressionCostEstimation, err)
		libOtel.HandleSpanError(span, "cost estimation failed", costErr)

		return nil, costErr
	}

	// Use the maximum estimated cost (worst case) for validation
	// costEstimate.Max is uint64, so we compare with costLimit
	if costEstimate.Max > a.costLimit {
		costErr := fmt.Errorf("%w: estimated cost %d exceeds limit %d", constant.ErrExpressionCostExceeded, costEstimate.Max, a.costLimit)
		libOtel.HandleSpanBusinessErrorEvent(span, "expression cost exceeds limit", costErr)

		span.SetAttributes(
			attribute.Int64("app.cost_estimate_min", int64(costEstimate.Min)),
			attribute.Int64("app.cost_estimate_max", int64(costEstimate.Max)),
			attribute.Int64("app.cost_limit", int64(a.costLimit)),
			attribute.Bool("app.cost_exceeded", true),
		)

		return nil, costErr
	}

	span.SetAttributes(
		attribute.Int64("app.cost_estimate_min", int64(costEstimate.Min)),
		attribute.Int64("app.cost_estimate_max", int64(costEstimate.Max)),
		attribute.Int64("app.cost_limit", int64(a.costLimit)),
		attribute.Bool("app.cost_exceeded", false),
	)

	// Create program (compile-time cost validation already done above via checker.Cost)
	program, err := a.env.Program(ast)
	if err != nil {
		progErr := fmt.Errorf("%w: %w", constant.ErrExpressionProgram, err)
		libOtel.HandleSpanError(span, "program creation failed", progErr)

		return nil, progErr
	}

	// Build compiled program
	compiledAt := time.Now()
	compileTimeMs := compiledAt.Sub(start).Milliseconds()

	compiled := &CompiledProgram{
		ExpressionHash:   hash,
		SourceExpression: expression,
		Program:          program,
		CompiledAt:       compiledAt,
		CompileTimeMs:    compileTimeMs,
	}

	span.SetAttributes(attribute.Int64("app.compile_time_ms", compileTimeMs))

	logger.With(
		libLog.String("operation", "adapter.cel.compile"),
		libLog.String("expression.hash", safePrefix(hash, 8)),
		libLog.Any("compile.time_ms", compileTimeMs),
	).Log(ctx, libLog.LevelInfo, "CEL expression compiled")

	return compiled, nil
}

// Evaluate runs a compiled program against a ValidationRequest.
// Uses OpenTelemetry tracing with span name: adapter.cel.evaluate
func (a *Adapter) Evaluate(ctx context.Context, program *CompiledProgram, req *model.ValidationRequest) (bool, error) {
	start := time.Now()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled // only tracer is needed from tracking context

	_, span := tracer.Start(ctx, "adapter.cel.evaluate")
	defer span.End()

	// Validate inputs
	if program == nil {
		err := fmt.Errorf("program is required")
		libOtel.HandleSpanError(span, "nil program", err)

		return false, err
	}

	if program.Program == nil {
		err := fmt.Errorf("compiled program is nil")
		libOtel.HandleSpanError(span, "nil compiled program", err)

		return false, err
	}

	span.SetAttributes(attribute.String("app.request.expression_hash", program.ExpressionHash))

	// Validate request is not nil
	if req == nil {
		err := fmt.Errorf("validation request is required")
		libOtel.HandleSpanError(span, "nil request", err)

		return false, err
	}

	// Build activation from request
	activation, err := BuildActivation(req)
	if err != nil {
		wrappedErr := fmt.Errorf("%w: failed to build activation: %w", constant.ErrExpressionEvaluation, err)
		libOtel.HandleSpanBusinessErrorEvent(span, "failed to build activation", wrappedErr)

		return false, wrappedErr
	}

	// Evaluate
	out, _, err := program.Program.Eval(activation)
	if err != nil {
		evalErr := fmt.Errorf("%w: %w", constant.ErrExpressionEvaluation, err)
		libOtel.HandleSpanBusinessErrorEvent(span, "evaluation failed", evalErr)

		return false, evalErr
	}

	// Extract boolean result
	result, ok := out.Value().(bool)
	if !ok {
		err := fmt.Errorf("%w: expected bool, got %T", constant.ErrExpressionType, out.Value())
		libOtel.HandleSpanBusinessErrorEvent(span, "type assertion failed", err)

		return false, err
	}

	// Record span attributes
	durationMs := time.Since(start).Milliseconds()

	span.SetAttributes(
		attribute.Int64("app.evaluate_duration_ms", durationMs),
		attribute.Bool("app.evaluate_result", result),
	)

	return result, nil
}

// Ensure Adapter implements ExpressionEngine at compile time.
var _ ExpressionEngine = (*Adapter)(nil)
