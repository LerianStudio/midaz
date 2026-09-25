// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	celgo "github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker"
	"github.com/google/cel-go/interpreter"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// ContextAdapterConfig requires explicit resource limits. It supplies no
// implicit production precision or SLO. Cost is measured in CEL work units.
type ContextAdapterConfig struct {
	Limits             tracercontract.Limits
	CostLimit          uint64
	MaxExpressionBytes int
}

// ContextAdapter evaluates the shared accounts/entries contract using exact
// Decimal quantities. It has no legacy amount variable or float conversion.
type ContextAdapter struct {
	config ContextAdapterConfig
	env    *celgo.Env
}

// ContextProgram is immutable and bound to the environment that checked it.
// Keep its cache separate from programs compiled against ValidationRequest.
type ContextProgram struct {
	owner   *ContextAdapter
	checked *celgo.Ast
	program celgo.Program
	maxCost uint64
}

// EstimatedMaxCost is the conservative cost for the configured context bounds.
// Publication uses it to reject policies exceeding the aggregate work budget.
func (p *ContextProgram) EstimatedMaxCost() uint64 {
	return p.maxCost
}

// NewContextAdapter creates the strictly typed shared-contract environment.
func NewContextAdapter(config ContextAdapterConfig) (*ContextAdapter, error) {
	if err := config.Limits.Validate(); err != nil {
		return nil, err
	}

	if config.CostLimit == 0 || config.MaxExpressionBytes <= 0 {
		return nil, fmt.Errorf("explicit CEL cost and expression limits required: %w", constant.ErrExpressionProgram)
	}

	env, err := contextEnvironment(config)
	if err != nil {
		return nil, fmt.Errorf("create typed context environment: %w", err)
	}

	return &ContextAdapter{config: config, env: env}, nil
}

// Compile rejects unsupported money operations, nonliteral Decimal construction
// and excessive static cost before a policy can be activated.
func (a *ContextAdapter) Compile(ctx context.Context, expression string) (*ContextProgram, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "adapter.cel.compile_context")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if len(expression) == 0 || len(expression) > a.config.MaxExpressionBytes {
		return nil, constant.ErrExpressionSyntax
	}

	checked, issues := a.env.Compile(expression)
	if issues.Err() != nil {
		err := constant.ErrExpressionSyntax
		if classifyIssues(issues) {
			err = constant.ErrExpressionType
		}

		libOtel.HandleSpanBusinessErrorEvent(span, "invalid context expression", err)

		// CEL issues contain source literals; keep those out of runtime errors.
		return nil, err
	}

	if checked.OutputType() != celgo.BoolType {
		return nil, constant.ErrExpressionType
	}

	estimate, err := checker.Cost(checked.NativeRep(), decimalCostEstimator{limits: a.config.Limits})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constant.ErrExpressionCostEstimation, err)
	}

	if estimate.Max > a.config.CostLimit {
		libOtel.HandleSpanBusinessErrorEvent(span, "context expression cost exceeded", constant.ErrExpressionCostExceeded)
		return nil, constant.ErrExpressionCostExceeded
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	program, err := a.program(checked, a.config.CostLimit)
	if err != nil {
		libOtel.HandleSpanError(span, "context program creation failed", err)
		return nil, fmt.Errorf("%w: %w", constant.ErrExpressionProgram, err)
	}

	logger.With(libLog.Int("expression.bytes", len(expression))).Log(ctx, libLog.LevelDebug, "Context expression compiled")

	return &ContextProgram{owner: a, checked: checked, program: program, maxCost: estimate.Max}, nil
}

func (a *ContextAdapter) program(checked *celgo.Ast, cost uint64) (celgo.Program, error) {
	return a.env.Program(checked, celgo.CostLimit(cost), celgo.InterruptCheckFrequency(evaluationInterruptFrequency))
}

// Evaluate enforces both the expression's ceiling and the remaining budget of
// its enclosing evaluation. It returns actual cost even on runtime failure.
// Only the final rules near exhaustion need a lower-budget execution plan;
// the normal path reuses the cached immutable program.
func (a *ContextAdapter) Evaluate(ctx context.Context, program *ContextProgram, activation *ContextActivation, remaining uint64) (bool, uint64, error) {
	if err := ctx.Err(); err != nil {
		return false, 0, err
	}

	if program == nil || program.owner != a || activation == nil || activation.owner != a {
		return false, 0, constant.ErrExpressionProgram
	}

	if remaining == 0 {
		return false, 0, constant.ErrExpressionCostExceeded
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "adapter.cel.evaluate_context")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	executable := program.program

	if remaining < a.config.CostLimit {
		var err error

		executable, err = a.program(program.checked, remaining)
		if err != nil {
			libOtel.HandleSpanError(span, "context program creation failed", err)
			return false, 0, fmt.Errorf("%w: %w", constant.ErrExpressionProgram, err)
		}
	}

	result, details, err := executable.ContextEval(ctx, activation.values)

	var cost uint64

	if actual := details.ActualCost(); actual != nil {
		cost = *actual
	}

	span.SetAttributes(attribute.Int64("app.response.cost_actual", safeCostI64(cost)))

	if err != nil {
		var canceled interpreter.EvalCancelledError
		if errors.As(err, &canceled) && canceled.Cause == interpreter.CostLimitExceeded {
			libOtel.HandleSpanBusinessErrorEvent(span, "context expression cost exceeded", constant.ErrExpressionCostExceeded)
			return false, cost, constant.ErrExpressionCostExceeded
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			libOtel.HandleSpanError(span, "context evaluation canceled", ctxErr)
			return false, cost, ctxErr
		}

		// Native errors can contain keys and literals from the rule. Expose only
		// the stable execution category, including in telemetry.
		libOtel.HandleSpanError(span, "context evaluation failed", constant.ErrExpressionEvaluation)

		return false, cost, constant.ErrExpressionEvaluation
	}

	if err := ctx.Err(); err != nil {
		return false, cost, err
	}

	matched, ok := result.Value().(bool)
	if !ok {
		return false, cost, constant.ErrExpressionType
	}

	logger.With(libLog.Any("evaluation.cost", cost)).Log(ctx, libLog.LevelDebug, "Context expression evaluated")

	return matched, cost, nil
}
