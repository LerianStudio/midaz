// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

import (
	"slices"

	"github.com/google/uuid"
)

// Decision is the completed outcome returned by Tracer, not a policy owned by
// the producer. A technical failure must never be represented as a decision.
type Decision string

const (
	DecisionAllow  Decision = "ALLOW"
	DecisionDeny   Decision = "DENY"
	DecisionReview Decision = "REVIEW"
)

type (
	RulesControl  string
	LimitsControl string
)

const (
	RulesEvaluated        RulesControl  = "EVALUATED"
	RulesNotRequested     RulesControl  = "NOT_REQUESTED"
	LimitsEvaluated       LimitsControl = "EVALUATED"
	LimitsSkippedRuleDeny LimitsControl = "SKIPPED_RULE_DENY"
)

// ReserveControls explicitly reports which requested controls completed.
type ReserveControls struct {
	Rules  RulesControl  `json:"rules"`
	Limits LimitsControl `json:"limits"`
}

// ReserveReason is a stable explanation code, never an expression or payload.
type ReserveReason string

const (
	ReasonRuleDeny           ReserveReason = "RULE_DENY"
	ReasonRuleReview         ReserveReason = "RULE_REVIEW"
	ReasonRuleAllow          ReserveReason = "RULE_ALLOW"
	ReasonPolicyDefaultDeny  ReserveReason = "POLICY_DEFAULT_DENY"
	ReasonPolicyDefaultAllow ReserveReason = "POLICY_DEFAULT_ALLOW"
	ReasonLimitExceeded      ReserveReason = "LIMIT_EXCEEDED"
	ReasonLimitsSatisfied    ReserveReason = "LIMITS_SATISFIED"
)

// ReserveResult is the original complete response preserved for replay. An
// empty but present reservation list is meaningful, including ALLOW without
// applicable limits. Reasons are unique and ordered lexicographically.
type ReserveResult struct {
	ContractRevision string          `json:"contractRevision"`
	TransactionID    uuid.UUID       `json:"transactionId"`
	EvaluationID     uuid.UUID       `json:"evaluationId"`
	Decision         Decision        `json:"decision"`
	Controls         ReserveControls `json:"controls"`
	ReservationIDs   []uuid.UUID     `json:"reservationIds"`
	Reasons          []ReserveReason `json:"reasons"`
}

// Validate checks transport invariants. It does not select policy actions or
// recompute a decision. maxReservations is an explicit storage/transport bound.
func (r ReserveResult) Validate(maxReservations int) error {
	if r.ContractRevision != ReserveContractRevision || r.TransactionID == uuid.Nil || r.EvaluationID == uuid.Nil {
		return invalid("reserve result identity")
	}

	if r.Decision != DecisionAllow && r.Decision != DecisionDeny && r.Decision != DecisionReview {
		return invalid("reserve decision")
	}

	if err := r.validateControls(); err != nil {
		return err
	}

	if err := r.validateReservations(maxReservations); err != nil {
		return err
	}

	return validateReserveReasons(r.Reasons)
}

func (r ReserveResult) validateControls() error {
	if r.Controls.Rules != RulesEvaluated && r.Controls.Rules != RulesNotRequested {
		return invalid("rules control")
	}

	if r.Controls.Limits != LimitsEvaluated && r.Controls.Limits != LimitsSkippedRuleDeny {
		return invalid("limits control")
	}

	if (r.Controls.Limits == LimitsSkippedRuleDeny && (r.Decision != DecisionDeny || r.Controls.Rules != RulesEvaluated)) ||
		(r.Decision == DecisionReview && r.Controls.Rules != RulesEvaluated) {
		return invalid("incomplete controls")
	}

	return nil
}

func (r ReserveResult) validateReservations(maxReservations int) error {
	if maxReservations <= 0 || r.ReservationIDs == nil || len(r.ReservationIDs) > maxReservations ||
		(r.Decision != DecisionAllow && len(r.ReservationIDs) != 0) {
		return invalid("reserve result capacity")
	}

	seen := make(map[uuid.UUID]struct{}, len(r.ReservationIDs))
	for _, id := range r.ReservationIDs {
		if _, duplicate := seen[id]; id == uuid.Nil || duplicate {
			return invalid("reservation identity")
		}

		seen[id] = struct{}{}
	}

	return nil
}

func validateReserveReasons(reasons []ReserveReason) error {
	if len(reasons) == 0 || len(reasons) > 7 || !slices.IsSorted(reasons) {
		return invalid("reserve reasons")
	}

	for i, reason := range reasons {
		if i > 0 && reasons[i-1] == reason {
			return invalid("duplicate reserve reason")
		}

		switch reason {
		case ReasonRuleDeny, ReasonRuleReview, ReasonRuleAllow, ReasonPolicyDefaultDeny, ReasonPolicyDefaultAllow, ReasonLimitExceeded, ReasonLimitsSatisfied:
		default:
			return invalid("reserve reason")
		}
	}

	return nil
}
