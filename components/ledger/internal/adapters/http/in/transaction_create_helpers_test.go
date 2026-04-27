// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCollectAccountIDsFromBalances_Empty asserts both nil and empty slice cases
// short-circuit to a nil result. The downstream eligibility gate uses len(ids) == 0
// to skip the lookup, so returning a non-nil empty slice would be wasteful but not
// incorrect — we still pin the contract.
func TestCollectAccountIDsFromBalances_Empty(t *testing.T) {
	t.Parallel()

	t.Run("nil slice returns nil", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, collectAccountIDsFromBalances(nil))
	})

	t.Run("empty slice returns nil", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, collectAccountIDsFromBalances([]*mmodel.Balance{}))
	})
}

// TestCollectAccountIDsFromBalances_Deduplicates is the primary regression guard:
// the same AccountID across multiple balance rows must produce a single entry. Without
// this, transactions touching multiple balances of the same account would multiply the
// eligibility-gate query size for no reason.
func TestCollectAccountIDsFromBalances_Deduplicates(t *testing.T) {
	t.Parallel()

	id1 := uuid.New()
	id2 := uuid.New()

	balances := []*mmodel.Balance{
		{AccountID: id1.String()},
		{AccountID: id2.String()},
		{AccountID: id1.String()}, // duplicate of first
		{AccountID: id2.String()}, // duplicate of second
		{AccountID: id1.String()}, // another duplicate of first
	}

	got := collectAccountIDsFromBalances(balances)

	require.Len(t, got, 2, "duplicates must be collapsed to two unique IDs")
	assert.ElementsMatch(t, []uuid.UUID{id1, id2}, got)
}

// TestCollectAccountIDsFromBalances_SkipsInvalid covers the three skip branches:
// nil pointer, empty AccountID string, and malformed UUID. Any of these getting through
// would corrupt the eligibility query input and produce 500s downstream.
func TestCollectAccountIDsFromBalances_SkipsInvalid(t *testing.T) {
	t.Parallel()

	valid := uuid.New()

	balances := []*mmodel.Balance{
		nil,                                  // nil pointer
		{AccountID: ""},                      // empty AccountID
		{AccountID: "not-a-uuid"},            // malformed UUID
		{AccountID: "00000000-0000"},         // truncated UUID
		{AccountID: valid.String()},          // the one survivor
		{AccountID: "another-bad-uuid-here"}, // malformed again
	}

	got := collectAccountIDsFromBalances(balances)

	require.Len(t, got, 1, "only the one valid UUID must survive")
	assert.Equal(t, valid, got[0])
}

// TestCollectAccountIDsFromBalances_PreservesFirstSeenOrder asserts the order is
// stable: the first occurrence of each AccountID determines its position. This matters
// because callers downstream use the slice as a deterministic gate input — flaky order
// would surface as flaky logs even when behaviour is identical.
func TestCollectAccountIDsFromBalances_PreservesFirstSeenOrder(t *testing.T) {
	t.Parallel()

	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()

	balances := []*mmodel.Balance{
		{AccountID: id2.String()},
		{AccountID: id1.String()},
		{AccountID: id3.String()},
		{AccountID: id1.String()}, // duplicate; must not move id1's position
		{AccountID: id2.String()}, // duplicate; must not move id2's position
	}

	got := collectAccountIDsFromBalances(balances)

	require.Len(t, got, 3)
	assert.Equal(t, id2, got[0], "first ID must be id2 (first seen)")
	assert.Equal(t, id1, got[1], "second ID must be id1 (second seen)")
	assert.Equal(t, id3, got[2], "third ID must be id3 (third seen)")
}
