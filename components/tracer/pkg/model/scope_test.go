// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

func Test_ptrMatches(t *testing.T) {
	tests := []struct {
		name     string
		pattern  *string
		value    *string
		expected bool
	}{
		{
			name:     "nil pattern matches any value",
			pattern:  nil,
			value:    testutil.StringPtr("any"),
			expected: true,
		},
		{
			name:     "nil pattern matches nil value",
			pattern:  nil,
			value:    nil,
			expected: true,
		},
		{
			name:     "non-nil pattern with nil value does not match",
			pattern:  testutil.StringPtr("pattern"),
			value:    nil,
			expected: false,
		},
		{
			name:     "same values match",
			pattern:  testutil.StringPtr("same"),
			value:    testutil.StringPtr("same"),
			expected: true,
		},
		{
			name:     "different values do not match",
			pattern:  testutil.StringPtr("pattern"),
			value:    testutil.StringPtr("different"),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ptrMatches(tc.pattern, tc.value)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_ptrMatches_WithUUID(t *testing.T) {
	uuid1 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	uuid2 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")

	tests := []struct {
		name     string
		pattern  *uuid.UUID
		value    *uuid.UUID
		expected bool
	}{
		{
			name:     "nil pattern matches any UUID",
			pattern:  nil,
			value:    &uuid1,
			expected: true,
		},
		{
			name:     "same UUIDs match",
			pattern:  &uuid1,
			value:    &uuid1,
			expected: true,
		},
		{
			name:     "different UUIDs do not match",
			pattern:  &uuid1,
			value:    &uuid2,
			expected: false,
		},
		{
			name:     "non-nil pattern with nil value does not match",
			pattern:  &uuid1,
			value:    nil,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ptrMatches(tc.pattern, tc.value)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_ptrMatches_WithInt(t *testing.T) {
	tests := []struct {
		name     string
		pattern  *int
		value    *int
		expected bool
	}{
		{
			name:     "nil pattern matches any int",
			pattern:  nil,
			value:    testutil.Ptr(42),
			expected: true,
		},
		{
			name:     "same ints match",
			pattern:  testutil.Ptr(100),
			value:    testutil.Ptr(100),
			expected: true,
		},
		{
			name:     "different ints do not match",
			pattern:  testutil.Ptr(100),
			value:    testutil.Ptr(200),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ptrMatches(tc.pattern, tc.value)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestScope_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		scope    Scope
		expected bool
	}{
		{
			name:     "Success - empty scope returns true",
			scope:    Scope{},
			expected: true,
		},
		{
			name: "Success - scope with SegmentID is not empty",
			scope: Scope{
				SegmentID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)),
			},
			expected: false,
		},
		{
			name: "Success - scope with PortfolioID is not empty",
			scope: Scope{
				PortfolioID: testutil.UUIDPtr(testutil.MustDeterministicUUID(2)),
			},
			expected: false,
		},
		{
			name: "Success - scope with AccountID is not empty",
			scope: Scope{
				AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(3)),
			},
			expected: false,
		},
		{
			name: "Success - scope with MerchantID is not empty",
			scope: Scope{
				MerchantID: testutil.UUIDPtr(testutil.MustDeterministicUUID(4)),
			},
			expected: false,
		},
		{
			name: "Success - scope with TransactionType is not empty",
			scope: Scope{
				TransactionType: testutil.Ptr(TransactionTypeCard),
			},
			expected: false,
		},
		{
			name: "Success - scope with SubType is not empty",
			scope: Scope{
				SubType: testutil.StringPtr("CREDIT"),
			},
			expected: false,
		},
		{
			name: "Success - scope with multiple fields is not empty",
			scope: Scope{
				AccountID:       testutil.UUIDPtr(testutil.MustDeterministicUUID(5)),
				TransactionType: testutil.Ptr(TransactionTypePix),
				SubType:         testutil.StringPtr("INSTANT"),
			},
			expected: false,
		},
		{
			name: "Success - scope with all fields is not empty",
			scope: Scope{
				SegmentID:       testutil.UUIDPtr(testutil.MustDeterministicUUID(6)),
				PortfolioID:     testutil.UUIDPtr(testutil.MustDeterministicUUID(7)),
				AccountID:       testutil.UUIDPtr(testutil.MustDeterministicUUID(8)),
				MerchantID:      testutil.UUIDPtr(testutil.MustDeterministicUUID(9)),
				TransactionType: testutil.Ptr(TransactionTypeWire),
				SubType:         testutil.StringPtr("INTERNATIONAL"),
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.scope.IsEmpty()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestPtrMatchesFold(t *testing.T) {
	tests := []struct {
		name     string
		pattern  *string
		value    *string
		expected bool
	}{
		{
			name:     "nil pattern matches nil value",
			pattern:  nil,
			value:    nil,
			expected: true,
		},
		{
			name:     "nil pattern matches non-nil value",
			pattern:  nil,
			value:    testutil.StringPtr("sell"),
			expected: true,
		},
		{
			name:     "non-nil pattern with nil value does not match",
			pattern:  testutil.StringPtr("sell"),
			value:    nil,
			expected: false,
		},
		{
			name:     "equal same case matches",
			pattern:  testutil.StringPtr("sell"),
			value:    testutil.StringPtr("sell"),
			expected: true,
		},
		{
			name:     "equal different case matches",
			pattern:  testutil.StringPtr("sell"),
			value:    testutil.StringPtr("SELL"),
			expected: true,
		},
		{
			name:     "equal mixed case matches",
			pattern:  testutil.StringPtr("Sell"),
			value:    testutil.StringPtr("sElL"),
			expected: true,
		},
		{
			name:     "different values do not match",
			pattern:  testutil.StringPtr("sell"),
			value:    testutil.StringPtr("buy"),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ptrMatchesFold(tc.pattern, tc.value)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestScope_Matches_SubType_CaseInsensitive(t *testing.T) {
	scope := Scope{SubType: testutil.StringPtr("sell")}

	tests := []struct {
		name     string
		other    Scope
		expected bool
	}{
		{
			name:     "same case matches",
			other:    Scope{SubType: testutil.StringPtr("sell")},
			expected: true,
		},
		{
			name:     "uppercase matches",
			other:    Scope{SubType: testutil.StringPtr("SELL")},
			expected: true,
		},
		{
			name:     "title case matches",
			other:    Scope{SubType: testutil.StringPtr("Sell")},
			expected: true,
		},
		{
			name:     "mixed case matches",
			other:    Scope{SubType: testutil.StringPtr("sElL")},
			expected: true,
		},
		{
			name:     "different value does not match",
			other:    Scope{SubType: testutil.StringPtr("buy")},
			expected: false,
		},
		{
			name:     "empty string does not match non-empty",
			other:    Scope{SubType: testutil.StringPtr("")},
			expected: false,
		},
		{
			name:     "nil in other does not match non-nil scope",
			other:    Scope{SubType: nil},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := scope.Matches(&tc.other)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestScope_Matches(t *testing.T) {
	accountID1 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	accountID2 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	segmentID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")
	txTypeCard := TransactionTypeCard
	txTypePix := TransactionTypePix

	tests := []struct {
		name     string
		scope    Scope
		other    Scope
		expected bool
	}{
		{
			name:     "empty scope matches any",
			scope:    Scope{},
			other:    Scope{AccountID: &accountID1},
			expected: true,
		},
		{
			name:     "same accountID matches",
			scope:    Scope{AccountID: &accountID1},
			other:    Scope{AccountID: &accountID1},
			expected: true,
		},
		{
			name:     "different accountID does not match",
			scope:    Scope{AccountID: &accountID1},
			other:    Scope{AccountID: &accountID2},
			expected: false,
		},
		{
			name:     "scope field nil in other does not match",
			scope:    Scope{AccountID: &accountID1},
			other:    Scope{},
			expected: false,
		},
		{
			name:     "other has more fields - matches",
			scope:    Scope{AccountID: &accountID1},
			other:    Scope{AccountID: &accountID1, SegmentID: &segmentID},
			expected: true,
		},
		{
			name:     "multiple fields must all match",
			scope:    Scope{AccountID: &accountID1, SegmentID: &segmentID},
			other:    Scope{AccountID: &accountID1, SegmentID: &segmentID},
			expected: true,
		},
		{
			name:     "multiple fields - one missing in other",
			scope:    Scope{AccountID: &accountID1, SegmentID: &segmentID},
			other:    Scope{AccountID: &accountID1},
			expected: false,
		},
		{
			name:     "transaction type matches",
			scope:    Scope{TransactionType: &txTypeCard},
			other:    Scope{TransactionType: &txTypeCard},
			expected: true,
		},
		{
			name:     "transaction type differs",
			scope:    Scope{TransactionType: &txTypeCard},
			other:    Scope{TransactionType: &txTypePix},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.scope.Matches(&tc.other)
			assert.Equal(t, tc.expected, result)
		})
	}
}
