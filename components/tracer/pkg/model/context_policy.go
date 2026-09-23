// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ContextPolicy is a revision selected by trusted Tracer policy configuration,
// never by the transaction payload. A resolver must enforce tenant/binding
// isolation and select exactly one active revision before supplying this value.
// It is not an administration or reservation transport DTO.
type ContextPolicy struct {
	ID              uuid.UUID
	Revision        int64
	DefaultDecision Decision
	Rules           []ContextPolicyRule
}

// ContextPolicyRule captures the expression and action at one immutable revision.
type ContextPolicyRule struct {
	ID         uuid.UUID
	Revision   int64
	Expression string
	Action     Decision
}

// Validate checks the complete snapshot without dropping invalid or excess rules.
// Empty policies still require an explicit ALLOW or DENY default.
func (p ContextPolicy) Validate(maxRules int) error {
	if p.ID == uuid.Nil || p.Revision <= 0 || maxRules <= 0 || len(p.Rules) > maxRules {
		return fmt.Errorf("invalid policy identity, revision or rule count: %w", constant.ErrInvalidRequestBody)
	}

	if p.DefaultDecision != DecisionDeny && p.DefaultDecision != DecisionAllow {
		return constant.ErrInvalidDefaultDecision
	}

	seen := make(map[uuid.UUID]struct{}, len(p.Rules))
	for _, rule := range p.Rules {
		if _, duplicate := seen[rule.ID]; duplicate || rule.ID == uuid.Nil || rule.Revision <= 0 {
			return fmt.Errorf("invalid or duplicate policy rule revision: %w", constant.ErrInvalidRequestBody)
		}

		if !rule.Action.IsValid() {
			return constant.ErrRuleInvalidAction
		}

		if rule.Expression == "" {
			return constant.ErrRuleExpressionRequired
		}

		seen[rule.ID] = struct{}{}
	}

	return nil
}

// RuleRevision identifies the exact rule revision evaluated or matched.
type RuleRevision struct {
	ID       uuid.UUID
	Revision int64
}

// ContextPolicyResult is a complete rules-only result. Limit checks and durable
// decision persistence must finish before it can authorize a reservation.
type ContextPolicyResult struct {
	PolicyID       uuid.UUID
	PolicyRevision int64
	Decision       Decision
	EvaluatedRules []RuleRevision
	MatchedRules   []RuleRevision
	Cost           uint64
}
