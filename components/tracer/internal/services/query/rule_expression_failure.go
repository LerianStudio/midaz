// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ruleExpressionFailures are the error classes of a rule expression that cannot
// be evaluated against one request. They describe the rule and the request, not
// the tracer's health.
var ruleExpressionFailures = []error{
	constant.ErrExpressionEvaluation,
	constant.ErrExpressionType,
	constant.ErrExpressionCostExceeded,
	constant.ErrExpressionCostEstimation,
	constant.ErrExpressionProgram,
	constant.ErrExpressionSyntax,
}

// IsRuleExpressionFailure reports whether err is a rule-expression failure
// (see ruleExpressionFailures), including an amount beyond CEL's float64
// precision, rather than a failure to load or run the rule step.
func IsRuleExpressionFailure(err error) bool {
	for _, target := range ruleExpressionFailures {
		if errors.Is(err, target) {
			return true
		}
	}

	return false
}

// IsUnevaluableRule reports whether err is a rule-expression failure caused by
// the rule, which the rule step records against the rule while it evaluates the
// remaining rules. An amount beyond CEL's float64 precision is excluded: it is a
// fault of the request that no rule can be evaluated against. The reserve path
// still routes that amount to review, testing IsRuleExpressionFailure instead,
// because an error from the reserve seam reaches the ledger as 0178.
func IsUnevaluableRule(err error) bool {
	return IsRuleExpressionFailure(err) && !errors.Is(err, constant.ErrAmountExceedsPrecision)
}

// RuleExpressionFailureClass returns the error code of the rule-expression
// failure class err belongs to, or "" when err is not one. The code carries no
// expression text, so it is safe to log when the cause may quote request values.
func RuleExpressionFailureClass(err error) string {
	if !IsRuleExpressionFailure(err) {
		return ""
	}

	if errors.Is(err, constant.ErrAmountExceedsPrecision) {
		return constant.ErrAmountExceedsPrecision.Error()
	}

	for _, target := range ruleExpressionFailures {
		if errors.Is(err, target) {
			return target.Error()
		}
	}

	return ""
}

// RuleEvaluationFailures reports that more than one rule could not be
// evaluated against a request, in evaluation order. It unwraps to each
// RuleEvaluationError, so errors.Is matches every failure's class.
type RuleEvaluationFailures struct {
	Failures []*RuleEvaluationError
}

// Error renders every failure.
func (e *RuleEvaluationFailures) Error() string {
	messages := make([]string, 0, len(e.Failures))

	for _, failure := range e.Failures {
		messages = append(messages, failure.Error())
	}

	return strings.Join(messages, "; ")
}

// Unwrap exposes each failure to errors.Is and errors.As.
func (e *RuleEvaluationFailures) Unwrap() []error {
	out := make([]error, 0, len(e.Failures))

	for _, failure := range e.Failures {
		out = append(out, failure)
	}

	return out
}

// FailingRuleIDs returns, in evaluation order, the ids of the rules err's
// RuleEvaluationFailures or RuleEvaluationError names. Failures without a rule
// id are skipped; nil means no failure in err is attributed to a rule.
func FailingRuleIDs(err error) []uuid.UUID {
	var failures []*RuleEvaluationError

	var multi *RuleEvaluationFailures

	var single *RuleEvaluationError

	switch {
	case errors.As(err, &multi):
		failures = multi.Failures
	case errors.As(err, &single):
		failures = []*RuleEvaluationError{single}
	}

	var ids []uuid.UUID

	for _, failure := range failures {
		if failure != nil && failure.RuleID != uuid.Nil {
			ids = append(ids, failure.RuleID)
		}
	}

	return ids
}

// RedactedRuleExpressionFailure returns an error that carries only err's
// failure class (RuleExpressionFailureClass). The CEL error text can quote
// request values, including the amount, so telemetry records this instead.
func RedactedRuleExpressionFailure(err error) error {
	return errors.New(RuleExpressionFailureClass(err))
}

// IncompleteEvaluationError reports a rule step that did not evaluate every
// rule for a request, together with what the evaluation still established. It
// unwraps to the RuleEvaluationError or RuleEvaluationFailures that stopped it.
//
// EvaluatedRuleIDs lists, in evaluation order, the rules that did evaluate; a
// failing rule is never among them. ReviewRuleIDs lists the matched REVIEW
// rules. TotalRulesLoaded and Truncated describe the loaded rule set as on a
// complete evaluation.
type IncompleteEvaluationError struct {
	Err              error
	ReviewRuleIDs    []uuid.UUID
	EvaluatedRuleIDs []uuid.UUID
	TotalRulesLoaded int
	Truncated        bool
}

// Error renders the failure that stopped the evaluation.
func (e *IncompleteEvaluationError) Error() string {
	return e.Err.Error()
}

// Unwrap exposes the failure that stopped the evaluation.
func (e *IncompleteEvaluationError) Unwrap() error {
	return e.Err
}

// MatchedRuleIDs returns the rules a REVIEW for the failure is attributed to:
// the matched REVIEW rules in evaluation order, then the failing rules.
func (e *IncompleteEvaluationError) MatchedRuleIDs() []uuid.UUID {
	failing := FailingRuleIDs(e.Err)

	out := make([]uuid.UUID, 0, len(e.ReviewRuleIDs)+len(failing))
	out = append(out, e.ReviewRuleIDs...)

	return append(out, failing...)
}

// IncompleteEvaluationOf returns the IncompleteEvaluationError in err's chain.
func IncompleteEvaluationOf(err error) (*IncompleteEvaluationError, bool) {
	var incomplete *IncompleteEvaluationError
	if errors.As(err, &incomplete) {
		return incomplete, true
	}

	return nil, false
}
