// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import "github.com/google/uuid"

// DecisionMaker applies precedence logic to determine the final validation decision.
// Precedence order: DENY > REVIEW > ALLOW > default.
// This is a pure type (no I/O, no side effects) used for domain logic.
type DecisionMaker struct{}

// NewDecisionMaker creates a new DecisionMaker instance.
func NewDecisionMaker() *DecisionMaker {
	return &DecisionMaker{}
}

// MakeDecision determines the final decision based on rule matches and precedence.
// Precedence: DENY > REVIEW > ALLOW > default.
// Returns an EvaluationResult with the decision, ALL matched rule IDs, and reason.
// matchedRuleIds contains ALL rules that matched (DENY + REVIEW + ALLOW), not just the winning category.
// Returns error if defaultDecision is invalid (propagates from NewNoMatchResult).
func (d *DecisionMaker) MakeDecision(
	denyRuleIDs, allowRuleIDs, reviewRuleIDs, evaluatedRuleIDs []uuid.UUID,
	defaultDecision Decision,
) (*EvaluationResult, error) {
	// Collect ALL matched rule IDs (not just the winning category)
	// Per API design: matchedRuleIds contains all rules that matched, regardless of action
	allMatchedRuleIDs := make([]uuid.UUID, 0, len(denyRuleIDs)+len(reviewRuleIDs)+len(allowRuleIDs))
	allMatchedRuleIDs = append(allMatchedRuleIDs, denyRuleIDs...)
	allMatchedRuleIDs = append(allMatchedRuleIDs, reviewRuleIDs...)
	allMatchedRuleIDs = append(allMatchedRuleIDs, allowRuleIDs...)

	// Precedence: DENY > REVIEW > ALLOW > default
	// Note: len(nil) == 0 in Go, and NewEvaluationResult/NewNoMatchResult
	// handle nil slice normalization for consistent JSON serialization.
	if len(denyRuleIDs) > 0 {
		// DecisionDeny is always valid, error can be ignored
		result, _ := NewEvaluationResult(DecisionDeny, allMatchedRuleIDs, evaluatedRuleIDs, "Rule matched with DENY action")
		return result, nil
	}

	if len(reviewRuleIDs) > 0 {
		// DecisionReview is always valid, error can be ignored
		result, _ := NewEvaluationResult(DecisionReview, allMatchedRuleIDs, evaluatedRuleIDs, "Rule matched with REVIEW action")
		return result, nil
	}

	if len(allowRuleIDs) > 0 {
		// DecisionAllow is always valid, error can be ignored
		result, _ := NewEvaluationResult(DecisionAllow, allMatchedRuleIDs, evaluatedRuleIDs, "Rule matched with ALLOW action")
		return result, nil
	}

	// No matching rules - use default decision
	// defaultDecision may be invalid, so propagate error
	return NewNoMatchResult(defaultDecision, evaluatedRuleIDs)
}
