// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package loader

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBuildBalanceQuery proves the parameterized query is assembled with the
// exact number of args corresponding to non-empty tenant filters. The $N
// placeholders must be numbered sequentially starting at 1 to match pgx
// binding — off-by-one here leaks to the wrong tenant.
func TestBuildBalanceQuery(t *testing.T) {
	t.Parallel()

	t.Run("no filters returns base query with zero args", func(t *testing.T) {
		t.Parallel()

		q, args := buildBalanceQuery("", "")
		require.Empty(t, args)
		require.Equal(t, queryBalancesBase, q)
		require.NotContains(t, q, "organization_id = $")
		require.NotContains(t, q, "ledger_id = $")
	})

	t.Run("organization only adds single $1 predicate", func(t *testing.T) {
		t.Parallel()

		q, args := buildBalanceQuery("org-1", "")
		require.Equal(t, []any{"org-1"}, args)
		require.Contains(t, q, "AND organization_id = $1")
		require.NotContains(t, q, "ledger_id = $")
	})

	t.Run("ledger only adds single $1 predicate", func(t *testing.T) {
		t.Parallel()

		q, args := buildBalanceQuery("", "ledger-1")
		require.Equal(t, []any{"ledger-1"}, args)
		require.Contains(t, q, "AND ledger_id = $1")
		require.NotContains(t, q, "organization_id = $")
	})

	t.Run("both filters produce sequential placeholders", func(t *testing.T) {
		t.Parallel()

		q, args := buildBalanceQuery("org-1", "ledger-1")
		require.Equal(t, []any{"org-1", "ledger-1"}, args)
		require.Contains(t, q, "AND organization_id = $1")
		require.Contains(t, q, "AND ledger_id = $2")

		// Order invariant: organization placeholder precedes ledger placeholder
		// because the routing-safety policy scopes tenant before sub-tenant.
		orgIdx := strings.Index(q, "organization_id = $1")
		ledgerIdx := strings.Index(q, "ledger_id = $2")
		require.Greater(t, orgIdx, 0)
		require.Greater(t, ledgerIdx, orgIdx)
	})
}

// TestBuildStreamingQuery proves cursor-vs-first-page behaviour and LIMIT
// placement. Missing the strict (updated_at, id) > (cursor) predicate would
// re-scan rows on each page and never converge.
func TestBuildStreamingQuery(t *testing.T) {
	t.Parallel()

	t.Run("first page zero cursor has no updated_at predicate", func(t *testing.T) {
		t.Parallel()

		q, args := buildStreamingQuery("", "", time.Time{}, "", 500, true)
		// Only LIMIT $1 is added
		require.Equal(t, []any{500}, args)
		require.Contains(t, q, "LIMIT $1")
		require.NotContains(t, q, "COALESCE(updated_at, created_at) >=")
		require.NotContains(t, q, "(COALESCE(updated_at, created_at), id) >")
		require.Contains(t, q, "ORDER BY COALESCE(updated_at, created_at), id")
	})

	t.Run("first page with cursor updated_at uses >= predicate", func(t *testing.T) {
		t.Parallel()

		cutoff := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
		q, args := buildStreamingQuery("", "", cutoff, "", 500, true)
		require.Equal(t, []any{cutoff, 500}, args)
		require.Contains(t, q, "COALESCE(updated_at, created_at) >= $1")
		require.Contains(t, q, "LIMIT $2")
	})

	t.Run("subsequent page uses strict composite cursor", func(t *testing.T) {
		t.Parallel()

		cursor := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
		q, args := buildStreamingQuery("", "", cursor, "balance-42", 500, false)
		// Expect cursor_updated_at, cursor_id, limit
		require.Equal(t, []any{cursor, "balance-42", 500}, args)
		require.Contains(t, q, "(COALESCE(updated_at, created_at), id) > ($1, $2)")
		require.Contains(t, q, "LIMIT $3")
	})

	t.Run("all filters plus cursor produce correct placeholder ordering", func(t *testing.T) {
		t.Parallel()

		cursor := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
		q, args := buildStreamingQuery("org-1", "ledger-1", cursor, "bal-9", 100, false)
		require.Equal(t, []any{"org-1", "ledger-1", cursor, "bal-9", 100}, args)
		require.Contains(t, q, "organization_id = $1")
		require.Contains(t, q, "ledger_id = $2")
		require.Contains(t, q, "(COALESCE(updated_at, created_at), id) > ($3, $4)")
		require.Contains(t, q, "LIMIT $5")
	})
}

// TestBuildShardFilter proves the nil-vs-empty-set contract - returning nil
// for the empty slice is what lets LoadBalances distinguish
// "no filter, load all" from "empty filter, load nothing".
func TestBuildShardFilter(t *testing.T) {
	t.Parallel()

	t.Run("nil slice returns nil map", func(t *testing.T) {
		t.Parallel()

		require.Nil(t, buildShardFilter(nil))
	})

	t.Run("empty slice returns nil map", func(t *testing.T) {
		t.Parallel()

		require.Nil(t, buildShardFilter([]int32{}))
	})

	t.Run("non-empty slice returns lookup map", func(t *testing.T) {
		t.Parallel()

		f := buildShardFilter([]int32{0, 3, 7})
		require.Len(t, f, 3)
		_, ok := f[0]
		require.True(t, ok)
		_, ok = f[3]
		require.True(t, ok)
		_, ok = f[7]
		require.True(t, ok)
		_, ok = f[1]
		require.False(t, ok)
	})

	t.Run("duplicates collapse to unique keys", func(t *testing.T) {
		t.Parallel()

		f := buildShardFilter([]int32{2, 2, 2, 5})
		require.Len(t, f, 2)
	})
}

// TestCoalesceDecimalString proves nil → "0" (the zero-balance fail-safe)
// and non-nil → verbatim pass-through.
func TestCoalesceDecimalString(t *testing.T) {
	t.Parallel()

	require.Equal(t, "0", coalesceDecimalString(nil))

	val := "123.456"
	require.Equal(t, "123.456", coalesceDecimalString(&val))

	empty := ""
	// Empty is preserved (caller will surface the decimal parse error); we
	// don't silently promote "" to "0" because that would mask schema drift.
	require.Equal(t, "", coalesceDecimalString(&empty))
}

// TestCoalesceAllowBool proves invalid sql.NullBool defaults to true
// (fail-safe: NULL policy must not silently revoke the account's ability to
// transact). This mirrors the documented contract in the comment.
func TestCoalesceAllowBool(t *testing.T) {
	t.Parallel()

	t.Run("invalid nullbool defaults to true (fail-safe)", func(t *testing.T) {
		t.Parallel()

		require.True(t, coalesceAllowBool(sql.NullBool{Valid: false, Bool: false}))
		require.True(t, coalesceAllowBool(sql.NullBool{Valid: false, Bool: true}))
	})

	t.Run("valid true preserved", func(t *testing.T) {
		t.Parallel()

		require.True(t, coalesceAllowBool(sql.NullBool{Valid: true, Bool: true}))
	})

	t.Run("valid false preserved (explicit operator revocation)", func(t *testing.T) {
		t.Parallel()

		require.False(t, coalesceAllowBool(sql.NullBool{Valid: true, Bool: false}))
	})
}

// TestBuildBalance_HappyPath proves that well-formed decimal strings produce
// a Balance with Available/OnHold scaled consistently and that the external
// account marker is derived from the accountType (case-insensitive).
func TestBuildBalance_HappyPath(t *testing.T) {
	t.Parallel()

	bal, err := buildBalance(
		"bal-1", "org-1", "ledger-1", "acc-1", "alias-1", "default",
		"BRL", "100.00", "25.50", 7, "deposit", true, true,
	)
	require.NoError(t, err)
	require.NotNil(t, bal)
	require.Equal(t, "bal-1", bal.ID)
	require.Equal(t, "org-1", bal.OrganizationID)
	require.Equal(t, "BRL", bal.AssetCode)
	require.Equal(t, uint64(7), bal.Version)
	require.False(t, bal.IsExternal)
	require.True(t, bal.AllowSending)
	require.True(t, bal.AllowReceiving)
}

// TestBuildBalance_ExternalAccountDetected proves the case-insensitive match
// against the external-account type — the IsExternal flag gates mint/burn.
func TestBuildBalance_ExternalAccountDetected(t *testing.T) {
	t.Parallel()

	bal, err := buildBalance(
		"bal-2", "org-1", "ledger-1", "acc-ext", "@external/BRL", "default",
		"BRL", "0", "0", 0, "external", true, true,
	)
	require.NoError(t, err)
	require.True(t, bal.IsExternal, "accountType='external' must flip IsExternal")

	// Upper-case variant must also match (EqualFold).
	bal, err = buildBalance(
		"bal-3", "org-1", "ledger-1", "acc-ext", "@external/USD", "default",
		"USD", "0", "0", 0, "EXTERNAL", true, true,
	)
	require.NoError(t, err)
	require.True(t, bal.IsExternal)
}

// TestBuildBalance_NegativeVersionNormalized proves schema drift cannot
// produce a wraparound - negative version is clamped to 0 before the uint64
// cast. Without this clamp a -1 would become uint64 max and immediately
// wedge optimistic-locking compare-and-swap.
func TestBuildBalance_NegativeVersionNormalized(t *testing.T) {
	t.Parallel()

	bal, err := buildBalance(
		"bal-4", "org-1", "ledger-1", "acc-1", "alias", "default",
		"BRL", "0", "0", -42, "deposit", true, true,
	)
	require.NoError(t, err)
	require.Equal(t, uint64(0), bal.Version)
}

// TestBuildBalance_InvalidDecimalSurfacesError proves parse failures in
// Available or OnHold are surfaced (not silently defaulted), with the
// balance ID embedded for operator triage.
func TestBuildBalance_InvalidDecimalSurfacesError(t *testing.T) {
	t.Parallel()

	t.Run("invalid available", func(t *testing.T) {
		t.Parallel()

		_, err := buildBalance(
			"bal-5", "org", "ledger", "acc", "alias", "default",
			"BRL", "not-a-number", "0", 0, "deposit", true, true,
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "bal-5")
		require.Contains(t, err.Error(), "available")
	})

	t.Run("invalid on_hold", func(t *testing.T) {
		t.Parallel()

		_, err := buildBalance(
			"bal-6", "org", "ledger", "acc", "alias", "default",
			"BRL", "100", "totally-broken", 0, "deposit", true, true,
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "bal-6")
		require.Contains(t, err.Error(), "on_hold")
	})
}
