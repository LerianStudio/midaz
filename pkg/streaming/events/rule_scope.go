// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// RuleScopePayload is the nested scope object shared by every rule event
// that carries scopes (rule.created, rule.updated). It mirrors the six
// structural fields of model.Scope, typed independently so domain evolution
// does not silently shift the wire contract.
//
// Every field is a *string so JSON null distinguishes "unset" from an empty
// value. The fence keeps this shape to structural identifiers and enums only:
// no free text beyond subType, which is a structural sub-classifier.
type RuleScopePayload struct {
	SegmentID       *string `json:"segmentId"`
	PortfolioID     *string `json:"portfolioId"`
	AccountID       *string `json:"accountId"`
	MerchantID      *string `json:"merchantId"`
	TransactionType *string `json:"transactionType"`
	SubType         *string `json:"subType"`
}

// newRuleScopePayloads maps a domain scope slice into the wire scope slice.
// It always returns a non-nil slice so the wire serializes "scopes": [] and
// never null when there are no scopes. Each domain pointer is dereferenced
// under its own nil-guard, so a scope with any nil field cannot panic.
func newRuleScopePayloads(scopes []model.Scope) []RuleScopePayload {
	payloads := make([]RuleScopePayload, 0, len(scopes))

	for i := range scopes {
		scope := scopes[i]

		var p RuleScopePayload

		if scope.SegmentID != nil {
			s := scope.SegmentID.String()
			p.SegmentID = &s
		}

		if scope.PortfolioID != nil {
			s := scope.PortfolioID.String()
			p.PortfolioID = &s
		}

		if scope.AccountID != nil {
			s := scope.AccountID.String()
			p.AccountID = &s
		}

		if scope.MerchantID != nil {
			s := scope.MerchantID.String()
			p.MerchantID = &s
		}

		if scope.TransactionType != nil {
			s := scope.TransactionType.String()
			p.TransactionType = &s
		}

		if scope.SubType != nil {
			s := *scope.SubType
			p.SubType = &s
		}

		payloads = append(payloads, p)
	}

	return payloads
}
