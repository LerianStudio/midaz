// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

// EvaluationResult contains the result of rule evaluation per API Design v1.3.0.
type EvaluationResult struct {
	Decision         Decision    `json:"decision"`
	MatchedRuleIDs   []uuid.UUID `json:"matchedRuleIds" swaggertype:"array,string" format:"uuid"`
	EvaluatedRuleIDs []uuid.UUID `json:"evaluatedRuleIds" swaggertype:"array,string" format:"uuid"`
	Reason           string      `json:"reason"`
	TotalRulesLoaded int         `json:"totalRulesLoaded"`
	Truncated        bool        `json:"truncated"`
}

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
