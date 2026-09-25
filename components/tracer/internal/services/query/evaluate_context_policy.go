// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"bytes"
	"context"
	"errors"
	"slices"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// ContextExpressionEvaluator is the shared-context counterpart of the existing
// ExpressionEvaluator port. Prepared snapshots and checked programs are opaque
// CEL adapter handles; policy selection and decision precedence stay here.
type ContextExpressionEvaluator interface {
	Compile(context.Context, string) (*cel.ContextProgram, error)
	Prepare(context.Context, tracercontract.Context, string) (*cel.ContextActivation, error)
	Evaluate(context.Context, *cel.ContextProgram, *cel.ContextActivation, uint64) (bool, uint64, error)
}

// ContextPolicyConfig bounds the complete rules stage. TotalCost is a CEL work
// budget, not a deadline; the caller must also set an end-to-end context deadline.
type ContextPolicyConfig struct {
	MaxRules  int
	TotalCost uint64
}

// ContextPolicyEvaluator compiles a trusted policy revision, then evaluates every
// rule against one detached snapshot without partial results or truncation.
type ContextPolicyEvaluator struct {
	engine ContextExpressionEvaluator
	config ContextPolicyConfig
}

// CompiledContextPolicy is immutable and bound to its evaluator's resource limits.
// Cache entries must include authenticated tenant and policy revision;
// the policy ID alone is not a sufficient cache key across tenants.
type CompiledContextPolicy struct {
	owner           *ContextPolicyEvaluator
	id              uuid.UUID
	revision        int64
	defaultDecision model.Decision
	rules           []compiledContextRule
}

type compiledContextRule struct {
	reference model.RuleRevision
	action    model.Decision
	program   *cel.ContextProgram
}

// NewContextPolicyEvaluator requires explicit, positive aggregate limits.
func NewContextPolicyEvaluator(engine ContextExpressionEvaluator, config ContextPolicyConfig) (*ContextPolicyEvaluator, error) {
	if engine == nil || config.MaxRules <= 0 || config.TotalCost == 0 {
		return nil, constant.ErrExpressionProgram
	}

	return &ContextPolicyEvaluator{engine: engine, config: config}, nil
}

// Compile validates every rule before producing a reusable policy snapshot.
// It must run before activation/cache publication, not once per transaction.
func (q *ContextPolicyEvaluator) Compile(ctx context.Context, policy model.ContextPolicy) (_ *CompiledContextPolicy, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.compile_context_policy")
	defer span.End()
	defer func() { recordContextPolicyError(span, retErr) }()

	logger = logging.WithTrace(ctx, logger)

	if err := policy.Validate(q.config.MaxRules); err != nil {
		return nil, err
	}

	// Stable order makes snapshots and budget exhaustion independent of database
	// row order. This is not rule priority: all rules must finish successfully.
	rules := slices.Clone(policy.Rules)
	slices.SortFunc(rules, func(a, b model.ContextPolicyRule) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	compiled := &CompiledContextPolicy{
		owner: q, id: policy.ID, revision: policy.Revision, defaultDecision: policy.DefaultDecision,
		rules: make([]compiledContextRule, 0, len(rules)),
	}

	for _, rule := range rules {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		program, err := q.engine.Compile(ctx, rule.Expression)
		if err != nil {
			return nil, err
		}

		compiled.rules = append(compiled.rules, compiledContextRule{
			reference: model.RuleRevision{ID: rule.ID, Revision: rule.Revision}, action: rule.Action, program: program,
		})
	}

	logger.With(libLog.Int("rules.count", len(rules))).Log(ctx, libLog.LevelDebug, "Context policy compiled")

	return compiled, nil
}

// Execute evaluates the complete resolved policy. Errors remain errors even
// after a matching ALLOW or DENY; a partially evaluated policy cannot decide.
// Policy resolution, limit checks and durable recording belong to the enclosing
// reservation use case and are deliberately not implied by this result.
func (q *ContextPolicyEvaluator) Execute(ctx context.Context, policy *CompiledContextPolicy, facts tracercontract.Context, namespace string) (_ *model.ContextPolicyResult, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.evaluate_context_policy")
	defer span.End()
	defer func() { recordContextPolicyError(span, retErr) }()

	logger = logging.WithTrace(ctx, logger)

	if policy == nil || policy.owner != q {
		return nil, constant.ErrExpressionProgram
	}

	activation, err := q.engine.Prepare(ctx, facts, namespace)
	if err != nil {
		return nil, err
	}

	result := &model.ContextPolicyResult{
		PolicyID: policy.id, PolicyRevision: policy.revision,
		EvaluatedRules: make([]model.RuleRevision, 0, len(policy.rules)),
		MatchedRules:   make([]model.RuleRevision, 0, len(policy.rules)),
	}
	collector := EvaluationCollector{}
	remaining := q.config.TotalCost

	for _, rule := range policy.rules {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if remaining == 0 {
			return nil, constant.ErrExpressionCostExceeded
		}

		matched, cost, err := q.engine.Evaluate(ctx, rule.program, activation, remaining)
		if err != nil {
			return nil, err
		}

		if cost > remaining {
			return nil, constant.ErrExpressionCostExceeded
		}

		remaining -= cost

		result.EvaluatedRules = append(result.EvaluatedRules, rule.reference)
		collector.EvaluatedRuleIDs = append(collector.EvaluatedRuleIDs, rule.reference.ID)

		if matched {
			result.MatchedRules = append(result.MatchedRules, rule.reference)
			collectContextMatch(&collector, rule)
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	decision, err := model.NewDecisionMaker().MakeDecision(collector.DenyRuleIDs, collector.AllowRuleIDs,
		collector.ReviewRuleIDs, collector.EvaluatedRuleIDs, policy.defaultDecision)
	if err != nil {
		return nil, err
	}

	result.Decision = decision.Decision
	result.Cost = q.config.TotalCost - remaining
	span.SetAttributes(attribute.Int("app.response.evaluated_count", len(result.EvaluatedRules)),
		attribute.Int("app.response.matched_count", len(result.MatchedRules)),
		attribute.String("app.response.decision", string(result.Decision)))
	logger.With(libLog.Int("rules.count", len(result.EvaluatedRules)), libLog.Any("evaluation.cost", result.Cost)).
		Log(ctx, libLog.LevelDebug, "Context policy evaluated")

	return result, nil
}

func collectContextMatch(collector *EvaluationCollector, rule compiledContextRule) {
	switch rule.action {
	case model.DecisionDeny:
		collector.DenyRuleIDs = append(collector.DenyRuleIDs, rule.reference.ID)
	case model.DecisionReview:
		collector.ReviewRuleIDs = append(collector.ReviewRuleIDs, rule.reference.ID)
	case model.DecisionAllow:
		collector.AllowRuleIDs = append(collector.AllowRuleIDs, rule.reference.ID)
	}
}

func recordContextPolicyError(span trace.Span, err error) {
	if err == nil {
		return
	}

	// 0094 is registered but belongs to the unmarshalling/validation path, not
	// ValidateBusinessError's map. Preserve the same typed classification used
	// by the HTTP handlers without changing its global response mapping.
	classified := err
	if errors.Is(err, constant.ErrInvalidRequestBody) {
		classified = pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Err: err}
	}

	// The canonical registry maps exact sentinels, while context validators wrap
	// them with bounded structural diagnostics. Classify the complete chain.
	for cause := classified; cause != nil; cause = errors.Unwrap(cause) {
		if pkg.IsBusinessError(pkg.ValidateBusinessError(cause, constant.EntityRule)) {
			libOtel.HandleSpanBusinessErrorEvent(span, "context policy rejected", err)
			return
		}
	}

	libOtel.HandleSpanError(span, "context policy failed", err)
}
