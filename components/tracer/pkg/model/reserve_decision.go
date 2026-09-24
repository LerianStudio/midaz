// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"bytes"
	"crypto/sha256"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// ReserveOperationKey is scoped to the resolved tenant database. Both IDs are
// unique within an authenticated integration; context is content, not identity.
type ReserveOperationKey struct {
	IntegrationID string
	TransactionID uuid.UUID
	RequestID     uuid.UUID
}

func (k ReserveOperationKey) Validate() error {
	if k.TransactionID == uuid.Nil || k.RequestID == uuid.Nil || k.IntegrationID == "" || len(k.IntegrationID) > 256 ||
		!utf8.ValidString(k.IntegrationID) || strings.TrimSpace(k.IntegrationID) != k.IntegrationID || strings.ContainsRune(k.IntegrationID, 0) {
		return constant.ErrInvalidRequestBody
	}

	return nil
}

// ReserveDecisionPolicy freezes the selected policy and binding version and
// every evaluated/matched rule revision. It never reads a current binding during
// replay, and stores no rule expressions in the response.
type ReserveDecisionPolicy struct {
	ID             uuid.UUID      `json:"id"`
	Revision       int64          `json:"revision"`
	BindingVersion int64          `json:"bindingVersion"`
	DefaultUsed    bool           `json:"defaultUsed"`
	EvaluatedRules []RuleRevision `json:"evaluatedRules"`
	MatchedRules   []RuleRevision `json:"matchedRules"`
}

// ReserveDecision is immutable after commit. Accounting completion and capacity
// settlement belong to separate records; neither may rewrite this outcome.
type ReserveDecision struct {
	Key            ReserveOperationKey
	ContextID      string
	Fingerprint    [sha256.Size]byte
	ValidationMode tracercontract.ValidationMode
	Policy         *ReserveDecisionPolicy
	Result         tracercontract.ReserveResult
	CreatedAt      time.Time
}

// Validate applies storage bounds, not the current policy's runtime limits.
// Deployments must keep these bounds sufficient for recoverable frozen data.
func (d ReserveDecision) Validate(maxRules, maxReservations int) error {
	if err := d.Key.Validate(); err != nil {
		return err
	}

	if err := (PolicyBindingKey{IntegrationID: d.Key.IntegrationID, ContextID: d.ContextID}).Validate(); err != nil {
		return err
	}

	if d.CreatedAt.IsZero() || d.Result.TransactionID != d.Key.TransactionID || maxRules <= 0 {
		return constant.ErrInvalidRequestBody
	}

	if err := d.Result.Validate(maxReservations); err != nil {
		return err
	}

	switch d.ValidationMode {
	case tracercontract.ValidationLimits:
		if d.Policy != nil || d.Result.Controls.Rules != tracercontract.RulesNotRequested {
			return constant.ErrInvalidRequestBody
		}
	case tracercontract.ValidationRulesAndLimits:
		if d.Policy == nil || d.Result.Controls.Rules != tracercontract.RulesEvaluated {
			return constant.ErrInvalidRequestBody
		}

		return d.Policy.validate(maxRules)
	default:
		return constant.ErrInvalidRequestBody
	}

	return nil
}

func (p ReserveDecisionPolicy) validate(maxRules int) error {
	if p.ID == uuid.Nil || p.Revision <= 0 || p.BindingVersion <= 0 || p.EvaluatedRules == nil || p.MatchedRules == nil ||
		len(p.EvaluatedRules) > maxRules || len(p.MatchedRules) > len(p.EvaluatedRules) || p.DefaultUsed != (len(p.MatchedRules) == 0) {
		return constant.ErrInvalidRequestBody
	}

	if err := validateDecisionRuleRevisions(p.EvaluatedRules); err != nil {
		return err
	}

	if err := validateDecisionRuleRevisions(p.MatchedRules); err != nil {
		return err
	}

	evaluated := make(map[uuid.UUID]int64, len(p.EvaluatedRules))
	for _, rule := range p.EvaluatedRules {
		evaluated[rule.ID] = rule.Revision
	}

	for _, rule := range p.MatchedRules {
		if evaluated[rule.ID] != rule.Revision {
			return constant.ErrInvalidRequestBody
		}
	}

	return nil
}

func validateDecisionRuleRevisions(rules []RuleRevision) error {
	for i, rule := range rules {
		if rule.ID == uuid.Nil || rule.Revision <= 0 || (i > 0 && bytes.Compare(rules[i-1].ID[:], rule.ID[:]) >= 0) {
			return constant.ErrInvalidRequestBody
		}
	}

	return nil
}

// Clone detaches all mutable collections before handing a stored snapshot to a
// caller. A later caller cannot change the next replay through shared slices.
func (d ReserveDecision) Clone() ReserveDecision {
	d.Result.ReservationIDs = slices.Clone(d.Result.ReservationIDs)

	d.Result.Reasons = slices.Clone(d.Result.Reasons)
	if d.Policy != nil {
		policy := *d.Policy
		policy.EvaluatedRules = slices.Clone(policy.EvaluatedRules)
		policy.MatchedRules = slices.Clone(policy.MatchedRules)
		d.Policy = &policy
	}

	return d
}
