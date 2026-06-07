// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

//go:generate mockgen -source=rule_evaluator.go -destination=rule_evaluator_mock.go -package=query

import (
	"context"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// ErrNilExpressionEvaluator is returned when NewRuleEvaluator is called with a nil ExpressionEvaluator.
var ErrNilExpressionEvaluator = errors.New("expression evaluator is nil")

// ErrNilRule is returned when Evaluate is called with a nil rule.
var ErrNilRule = errors.New("rule cannot be nil")

// ErrNilRequest is defined in complete_evaluator.go and reused here.

// ExpressionEvaluator defines interface for CEL expression evaluation.
// Note: Uses ValidationRequest since that's what CEL adapter expects.
// Interface defined in the package that USES it (per PROJECT_RULES.md).
type ExpressionEvaluator interface {
	Compile(ctx context.Context, expression string) (*cel.CompiledProgram, error)
	Evaluate(ctx context.Context, program *cel.CompiledProgram, req *model.ValidationRequest) (bool, error)
}

// RuleEvaluator evaluates a single rule's expression using CEL adapter.
type RuleEvaluator struct {
	exprEval ExpressionEvaluator
}

// NewRuleEvaluator creates a new RuleEvaluator instance.
// Returns ErrNilExpressionEvaluator if exprEval is nil.
func NewRuleEvaluator(exprEval ExpressionEvaluator) (*RuleEvaluator, error) {
	if exprEval == nil {
		return nil, ErrNilExpressionEvaluator
	}

	return &RuleEvaluator{
		exprEval: exprEval,
	}, nil
}

// Evaluate evaluates a rule's expression against a validation request.
// Returns true if the rule matches (expression evaluates to true), false otherwise.
// Rules with scopes that don't match the transaction scopes return false without evaluating the expression.
func (e *RuleEvaluator) Evaluate(ctx context.Context, rule *model.Rule, req *model.ValidationRequest) (bool, error) {
	// Validate inputs
	if rule == nil {
		return false, ErrNilRule
	}

	if req == nil {
		return false, ErrNilRequest
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.rules.evaluate_expression")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	logger.With(
		libLog.String("rule.id", rule.ID.String()),
		libLog.String("rule.name", rule.Name),
	).Log(ctx, libLog.LevelInfo, "Evaluating rule expression")

	// Check if rule scopes match transaction scope before evaluating expression
	txScope := req.ToTransactionScope()
	if !model.RuleScopesMatch(rule.Scopes, txScope) {
		logger.With(
			libLog.String("rule.id", rule.ID.String()),
			libLog.String("rule.name", rule.Name),
		).Log(ctx, libLog.LevelInfo, "Rule scopes do not match transaction - skipping evaluation")

		return false, nil
	}

	// Set span attributes for rule being evaluated
	span.SetAttributes(
		attribute.String("app.request.rule_id", rule.ID.String()),
		attribute.String("app.request.rule_name", rule.Name),
	)

	// Use pre-compiled program from cache if available (hot-path optimization).
	// Falls back to Compile() if the program is nil or wrong type (defense-in-depth).
	program, ok := rule.CompiledProgram.(*cel.CompiledProgram)
	if !ok || program == nil {
		var err error

		program, err = e.exprEval.Compile(ctx, rule.Expression)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to compile expression", err)

			logger.With(
				libLog.String("rule.id", rule.ID.String()),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to compile expression")

			return false, fmt.Errorf("failed to compile expression: %w", err)
		}
	}

	// Evaluate the compiled expression
	matched, err := e.exprEval.Evaluate(ctx, program, req)
	if err != nil {
		// Missing map keys (e.g. metadata["channel"] when "channel" is absent)
		// are treated as non-match for this rule rather than fatal: the rule
		// simply does not apply to a request lacking the referenced key. Other
		// rules continue to be evaluated, and the decision uses the configured
		// default-when-no-match. Genuine runtime errors (type mismatch, division
		// by zero, etc.) still propagate.
		if cel.IsMissingKeyError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule expression referenced missing key", err)

			logger.With(
				libLog.String("operation", "service.rules.evaluate_expression"),
				libLog.String("rule.id", rule.ID.String()),
				libLog.String("rule.name", rule.Name),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelWarn, "Rule expression referenced missing key - treating as non-match")

			span.SetAttributes(
				attribute.Bool("app.response.matched", false),
				attribute.Bool("app.response.missing_key", true),
				attribute.String("app.request.rule_id", rule.ID.String()),
				attribute.String("app.request.rule_name", rule.Name),
			)

			return false, nil
		}

		libOpentelemetry.HandleSpanError(span, "Failed to evaluate expression", err)

		logger.With(
			libLog.String("rule.id", rule.ID.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to evaluate expression")

		return false, fmt.Errorf("failed to evaluate expression: %w", err)
	}

	logger.With(
		libLog.String("rule.id", rule.ID.String()),
		libLog.String("rule.name", rule.Name),
		libLog.Bool("matched", matched),
	).Log(ctx, libLog.LevelInfo, "Rule expression evaluated")

	return matched, nil
}
