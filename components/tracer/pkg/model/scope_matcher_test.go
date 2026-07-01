// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestRuleScopesMatch_VariousScopes_ReturnsExpectedMatch(t *testing.T) {
	accountID1 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	accountID2 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	segmentID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")
	segmentID2 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440007")
	portfolioID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440004")
	merchantID1 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440005")
	merchantID2 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440006")
	txTypeCard := TransactionTypeCard
	txTypePix := TransactionTypePix
	subTypeDebit := "debit"
	subTypeCredit := "credit"

	tests := []struct {
		name       string
		ruleScopes []Scope
		txScope    *Scope
		expected   bool
	}{
		{
			name: "rule scope matches transaction scope",
			ruleScopes: []Scope{
				{AccountID: &accountID1},
			},
			txScope:  &Scope{AccountID: &accountID1},
			expected: true,
		},
		{
			name: "any rule scope matches - second scope matches",
			ruleScopes: []Scope{
				{AccountID: &accountID1},
				{AccountID: &accountID2},
			},
			txScope:  &Scope{AccountID: &accountID2},
			expected: true,
		},
		{
			name: "returns false when no match",
			ruleScopes: []Scope{
				{AccountID: &accountID1},
			},
			txScope:  &Scope{AccountID: &accountID2},
			expected: false,
		},
		{
			name: "matches by transaction type",
			ruleScopes: []Scope{
				{TransactionType: &txTypeCard},
			},
			txScope:  &Scope{TransactionType: &txTypeCard},
			expected: true,
		},
		{
			name:       "empty rule scopes match all (global rule)",
			ruleScopes: []Scope{},
			txScope:    &Scope{AccountID: &accountID1},
			expected:   true,
		},
		{
			name:       "nil rule scopes match all (global rule)",
			ruleScopes: nil,
			txScope:    &Scope{AccountID: &accountID1},
			expected:   true,
		},
		{
			name: "returns false when rule has more fields than tx scope",
			ruleScopes: []Scope{
				{AccountID: &accountID1, SegmentID: &segmentID},
			},
			txScope:  &Scope{AccountID: &accountID1},
			expected: false,
		},
		{
			name: "tx scope has more fields than rule scope - matches",
			ruleScopes: []Scope{
				{AccountID: &accountID1},
			},
			txScope:  &Scope{AccountID: &accountID1, SegmentID: &segmentID},
			expected: true,
		},
		{
			name: "nil tx scope with non-empty rule scopes - no match",
			ruleScopes: []Scope{
				{AccountID: &accountID1},
			},
			txScope:  nil,
			expected: false,
		},
		{
			name:       "nil tx scope with empty rule scopes - global rule matches",
			ruleScopes: []Scope{},
			txScope:    nil,
			expected:   true,
		},
		// Multi-field matching tests
		{
			name: "multi-field match - AccountID and TransactionType both match",
			ruleScopes: []Scope{
				{AccountID: &accountID1, TransactionType: &txTypeCard},
			},
			txScope:  &Scope{AccountID: &accountID1, TransactionType: &txTypeCard},
			expected: true,
		},
		{
			name: "multi-field no match - AccountID matches but TransactionType differs",
			ruleScopes: []Scope{
				{AccountID: &accountID1, TransactionType: &txTypeCard},
			},
			txScope:  &Scope{AccountID: &accountID1, TransactionType: &txTypePix},
			expected: false,
		},
		{
			name: "MerchantID field matching",
			ruleScopes: []Scope{
				{MerchantID: &merchantID1},
			},
			txScope:  &Scope{MerchantID: &merchantID1},
			expected: true,
		},
		{
			name: "MerchantID field no match",
			ruleScopes: []Scope{
				{MerchantID: &merchantID1},
			},
			txScope:  &Scope{MerchantID: &merchantID2},
			expected: false,
		},
		{
			name: "PortfolioID field matching",
			ruleScopes: []Scope{
				{PortfolioID: &portfolioID},
			},
			txScope:  &Scope{PortfolioID: &portfolioID, AccountID: &accountID1},
			expected: true,
		},
		{
			name: "SegmentID field matching",
			ruleScopes: []Scope{
				{SegmentID: &segmentID},
			},
			txScope:  &Scope{SegmentID: &segmentID, AccountID: &accountID1},
			expected: true,
		},
		{
			name: "SegmentID field no match",
			ruleScopes: []Scope{
				{SegmentID: &segmentID},
			},
			txScope:  &Scope{SegmentID: &segmentID2},
			expected: false,
		},
		{
			name: "SubType field matching",
			ruleScopes: []Scope{
				{SubType: &subTypeDebit},
			},
			txScope:  &Scope{SubType: &subTypeDebit},
			expected: true,
		},
		{
			name: "SubType field no match",
			ruleScopes: []Scope{
				{SubType: &subTypeDebit},
			},
			txScope:  &Scope{SubType: &subTypeCredit},
			expected: false,
		},
		{
			name: "multiple rule scopes - second one matches",
			ruleScopes: []Scope{
				{AccountID: &accountID1, MerchantID: &merchantID1},
				{AccountID: &accountID2, TransactionType: &txTypeCard},
			},
			txScope:  &Scope{AccountID: &accountID2, TransactionType: &txTypeCard, SegmentID: &segmentID},
			expected: true,
		},
		{
			name: "multiple rule scopes - none match",
			ruleScopes: []Scope{
				{AccountID: &accountID1, MerchantID: &merchantID1},
				{AccountID: &accountID2, TransactionType: &txTypePix},
			},
			txScope:  &Scope{AccountID: &accountID2, TransactionType: &txTypeCard},
			expected: false,
		},
		{
			name: "complex multi-field - all fields match",
			ruleScopes: []Scope{
				{AccountID: &accountID1, SegmentID: &segmentID, TransactionType: &txTypeCard, SubType: &subTypeDebit},
			},
			txScope:  &Scope{AccountID: &accountID1, SegmentID: &segmentID, TransactionType: &txTypeCard, SubType: &subTypeDebit, MerchantID: &merchantID1},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RuleScopesMatch(tt.ruleScopes, tt.txScope)
			assert.Equal(t, tt.expected, result)
		})
	}
}
