// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// EvaluationResult contains the result of rule evaluation per API Design v1.3.0.
//
// swagger:model EvaluationResult
//
//	@Description	Result produced by evaluating transaction context against the active rule set. Contains the final decision, the identifiers of matched and evaluated rules, a human-readable reason, and truncation metadata when the rule set exceeded the per-request maximum.
type EvaluationResult struct {
	// Final decision produced by rule evaluation
	// enums: ALLOW,DENY,REVIEW
	Decision Decision `json:"decision" swaggertype:"string" enums:"ALLOW,DENY,REVIEW" example:"ALLOW"`

	// IDs of rules that matched the transaction scope and CEL expression
	MatchedRuleIDs []uuid.UUID `json:"matchedRuleIds" swaggertype:"array,string" format:"uuid"`

	// IDs of all rules evaluated during this request
	EvaluatedRuleIDs []uuid.UUID `json:"evaluatedRuleIds" swaggertype:"array,string" format:"uuid"`

	// Human-readable explanation of the decision
	// example: Transaction denied by rule 'Block high-value checking transactions'
	Reason string `json:"reason" example:"Transaction denied by rule 'Block high-value checking transactions'"`

	// Total number of active rules loaded for evaluation before any truncation
	// example: 42
	TotalRulesLoaded int `json:"totalRulesLoaded" example:"42"`

	// True when active rules exceeded MAX_RULES_PER_REQUEST and were truncated
	// example: false
	Truncated bool `json:"truncated" example:"false"`
} //	@name	EvaluationResult

// normalizeUUIDs converts nil slice to empty slice for consistent JSON serialization.
func normalizeUUIDs(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}

	return ids
}

// NewEvaluationResult creates a result with matched rules.
// Returns error if decision is invalid or reason is empty (Always-Valid Domain Model).
// Normalizes nil slices to empty slices for consistent JSON serialization.
func NewEvaluationResult(decision Decision, matchedRuleIDs, evaluatedRuleIDs []uuid.UUID, reason string) (*EvaluationResult, error) {
	if !decision.IsValid() {
		return nil, constant.ErrInvalidDecision
	}

	if reason == "" {
		return nil, constant.ErrReasonRequired
	}

	return &EvaluationResult{
		Decision:         decision,
		MatchedRuleIDs:   normalizeUUIDs(matchedRuleIDs),
		EvaluatedRuleIDs: normalizeUUIDs(evaluatedRuleIDs),
		Reason:           reason,
	}, nil
}

// WithTruncationInfo sets truncation metadata on the result.
// This allows callers to know if rules were truncated due to MaxRulesPerRequest limit.
func (r *EvaluationResult) WithTruncationInfo(totalLoaded int, truncated bool) *EvaluationResult {
	r.TotalRulesLoaded = totalLoaded
	r.Truncated = truncated

	return r
}

// NewNoMatchResult creates a result when no rule matched.
// Returns error if defaultDecision is invalid (Always-Valid Domain Model).
// Normalizes nil slices to empty slices for consistent JSON serialization.
func NewNoMatchResult(defaultDecision Decision, evaluatedRuleIDs []uuid.UUID) (*EvaluationResult, error) {
	if !defaultDecision.IsValid() {
		return nil, constant.ErrInvalidDefaultDecision
	}

	return &EvaluationResult{
		Decision:         defaultDecision,
		MatchedRuleIDs:   []uuid.UUID{},
		EvaluatedRuleIDs: normalizeUUIDs(evaluatedRuleIDs),
		Reason:           "No matching rules found",
	}, nil
}
