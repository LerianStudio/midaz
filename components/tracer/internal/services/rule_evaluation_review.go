// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// ruleEvaluationReviewResult is the REVIEW a rule-expression failure yields,
// with reason rule_evaluation_error. It is attributed to the matched REVIEW
// rules, then the failing rules, and it reports as evaluated only the rules
// that did evaluate, with the loaded rule set as on a complete evaluation. When
// err carries no evaluation state, only the failing rules are known.
func ruleEvaluationReviewResult(err error) *model.EvaluationResult {
	result := &model.EvaluationResult{
		Decision:         model.DecisionReview,
		MatchedRuleIDs:   query.FailingRuleIDs(err),
		EvaluatedRuleIDs: []uuid.UUID{},
		Reason:           reasonRuleEvaluationError,
	}

	if incomplete, ok := query.IncompleteEvaluationOf(err); ok {
		result.MatchedRuleIDs = incomplete.MatchedRuleIDs()
		result.TotalRulesLoaded = incomplete.TotalRulesLoaded
		result.Truncated = incomplete.Truncated

		if incomplete.EvaluatedRuleIDs != nil {
			result.EvaluatedRuleIDs = incomplete.EvaluatedRuleIDs
		}
	}

	if result.MatchedRuleIDs == nil {
		result.MatchedRuleIDs = []uuid.UUID{}
	}

	return result
}

// ruleIDStrings renders rule ids for a log field.
func ruleIDStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))

	for _, id := range ids {
		out = append(out, id.String())
	}

	return out
}
