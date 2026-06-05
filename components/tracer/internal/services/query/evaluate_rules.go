// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

//go:generate mockgen -source=evaluate_rules.go -destination=evaluate_rules_mock.go -package=query

import (
	"context"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// Sentinel errors for EvaluateRulesQuery.
var (
	ErrNilGetActiveRulesQuery = errors.New("get active rules query is nil")
	ErrNilCompleteEvaluator   = errors.New("complete evaluator is nil")
	ErrNilEvaluationConfig    = errors.New("evaluation config is nil")
	ErrNilValidationRequest   = errors.New("validation request is nil")
)

// EvaluationConfig holds feature flag configuration for rule evaluation.
type EvaluationConfig struct {
	DefaultDecisionWhenNoMatch model.Decision
	MaxRulesPerRequest         int
}

// GetActiveRulesExecutor defines the interface for loading active rules.
// Interface defined in the package that USES it (per PROJECT_RULES.md).
// Supports optional scope filtering for performance optimization.
type GetActiveRulesExecutor interface {
	Execute(ctx context.Context, txScope *model.Scope) ([]*model.Rule, error)
}

// CompleteRuleEvaluator defines the interface for evaluating all rules.
// Interface defined in the package that USES it (per PROJECT_RULES.md).
type CompleteRuleEvaluator interface {
	EvaluateAll(ctx context.Context, rules []*model.Rule, req *model.ValidationRequest) (*EvaluationCollector, error)
}

// EvaluateRulesQuery orchestrates complete rule evaluation.
type EvaluateRulesQuery struct {
	getActiveRules    GetActiveRulesExecutor
	completeEvaluator CompleteRuleEvaluator
	decisionMaker     *model.DecisionMaker
	config            *EvaluationConfig
}

// NewEvaluateRulesQuery creates a new orchestration query.
// Returns sentinel errors if any dependency is nil.
func NewEvaluateRulesQuery(
	getActiveRules GetActiveRulesExecutor,
	completeEvaluator CompleteRuleEvaluator,
	config *EvaluationConfig,
) (*EvaluateRulesQuery, error) {
	if getActiveRules == nil {
		return nil, ErrNilGetActiveRulesQuery
	}

	if completeEvaluator == nil {
		return nil, ErrNilCompleteEvaluator
	}

	if config == nil {
		return nil, ErrNilEvaluationConfig
	}

	return &EvaluateRulesQuery{
		getActiveRules:    getActiveRules,
		completeEvaluator: completeEvaluator,
		decisionMaker:     model.NewDecisionMaker(),
		config:            config,
	}, nil
}

// Execute performs complete rule evaluation.
func (q *EvaluateRulesQuery) Execute(ctx context.Context, req *model.ValidationRequest) (*model.EvaluationResult, error) {
	// Check context cancellation first (per PROJECT_RULES.md)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if req == nil {
		return nil, ErrNilValidationRequest
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.rules.evaluate")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	logger.With(
		libLog.String("operation", "service.rules.evaluate"),
		libLog.String("request.id", req.RequestID.String()),
	).Log(ctx, libLog.LevelInfo, "Starting rule evaluation")

	// Extract transaction scope for database-level filtering
	txScope := req.ToTransactionScope()

	// Load active rules with scope filter for performance optimization
	rules, err := q.getActiveRules.Execute(ctx, txScope)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to load rules", err)

		logger.With(
			libLog.String("operation", "service.rules.evaluate"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to load rules")

		return nil, fmt.Errorf("failed to load rules: %w", err)
	}

	// Track truncation info for response
	originalCount := len(rules)
	truncated := false

	// Apply max rules limit
	if q.config.MaxRulesPerRequest > 0 && len(rules) > q.config.MaxRulesPerRequest {
		logger.With(
			libLog.String("operation", "service.rules.evaluate"),
			libLog.Int("rules.original_count", len(rules)),
			libLog.Int("rules.truncated_to", q.config.MaxRulesPerRequest),
		).Log(ctx, libLog.LevelWarn, "Truncating rules due to max limit")

		rules = rules[:q.config.MaxRulesPerRequest]
		truncated = true
	}

	// Evaluate all rules (no short-circuit)
	collector, err := q.completeEvaluator.EvaluateAll(ctx, rules, req)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to evaluate rules", err)

		logger.With(
			libLog.String("operation", "service.rules.evaluate"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to evaluate rules")

		return nil, fmt.Errorf("failed to evaluate rules: %w", err)
	}

	// Make final decision
	result, err := q.decisionMaker.MakeDecision(
		collector.DenyRuleIDs,
		collector.AllowRuleIDs,
		collector.ReviewRuleIDs,
		collector.EvaluatedRuleIDs,
		q.config.DefaultDecisionWhenNoMatch,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to make decision", err)

		logger.With(
			libLog.String("operation", "service.rules.evaluate"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to make decision")

		return nil, fmt.Errorf("failed to make decision: %w", err)
	}

	if err := libOpentelemetry.SetSpanAttributesFromValue(span, "result", map[string]any{
		"decision":        result.Decision.String(),
		"matched_count":   len(result.MatchedRuleIDs),
		"evaluated_count": len(result.EvaluatedRuleIDs),
	}, nil); err != nil {
		logger.With(
			libLog.String("operation", "service.rules.evaluate"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelDebug, "Failed to set span attributes")
	}

	logger.With(
		libLog.String("operation", "service.rules.evaluate"),
		libLog.String("decision", result.Decision.String()),
		libLog.Int("rules.matched_count", len(result.MatchedRuleIDs)),
		libLog.Int("rules.evaluated_count", len(result.EvaluatedRuleIDs)),
		libLog.Int("rules.total_loaded", originalCount),
		libLog.Bool("rules.truncated", truncated),
	).Log(ctx, libLog.LevelInfo, "Evaluation complete")

	return result.WithTruncationInfo(originalCount, truncated), nil
}
