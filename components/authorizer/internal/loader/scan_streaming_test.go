// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package loader

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

// TestScanStreamingBalance_AdvancesCursorOnShardFilter proves that rows
// filtered out by the shard filter still update the cursor - otherwise
// keyset pagination would stall on the first filtered batch and re-scan
// the same window on the next page.
func TestScanStreamingBalance_AdvancesCursorOnShardFilter(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	available := "100"
	onHold := "0"

	row := &fakeRow{values: []any{
		"bal-filtered",
		"org", "ledger", "account-1", "@alice", "default", "USD",
		&available, &onHold,
		int64(1), "deposit",
		sql.NullBool{Bool: true, Valid: true},
		sql.NullBool{Bool: true, Valid: true},
		updatedAt,
	}}

	router := shard.NewRouter(shard.DefaultShardCount)
	// Construct a shard filter that cannot match this balance's shard. The
	// resolved shard is deterministic for a given (alias, key); we pick a
	// value that cannot possibly collide by using the max int32.
	impossibleShard := int32(1 << 30)
	filter := map[int32]struct{}{impossibleShard: {}}

	cr, err := scanStreamingBalance(row, router, filter)
	require.NoError(t, err)

	// Cursor must still advance even though the balance is dropped.
	require.Equal(t, "bal-filtered", cr.cursorID)
	require.Equal(t, updatedAt, cr.cursorUpdatedAt)
	require.Nil(t, cr.balance, "shard-filtered rows drop the balance payload")
}

// TestScanStreamingBalance_EmptyFilterAcceptsAllRows proves that a nil/empty
// filter is interpreted as "no filter" (not "reject all") — the same contract
// buildShardFilter implements.
func TestScanStreamingBalance_EmptyFilterAcceptsAllRows(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	available := "42"
	onHold := "0"

	row := &fakeRow{values: []any{
		"bal-accepted",
		"org", "ledger", "account-1", "@alice", "default", "USD",
		&available, &onHold,
		int64(1), "deposit",
		sql.NullBool{Bool: true, Valid: true},
		sql.NullBool{Bool: true, Valid: true},
		updatedAt,
	}}

	cr, err := scanStreamingBalance(row, shard.NewRouter(shard.DefaultShardCount), nil)
	require.NoError(t, err)
	require.Equal(t, "bal-accepted", cr.cursorID)
	require.NotNil(t, cr.balance)
	// Available is integer-scaled; 42 at the default scale (2) = 4200.
	require.Positive(t, cr.balance.Available)
	require.Equal(t, "bal-accepted", cr.balance.ID)
}

// TestScanStreamingBalance_DefaultBalanceKeyFilledIn proves the fail-safe
// default for a NULL/empty key column — an empty key would otherwise hash
// through the router to a different shard than the live account.
func TestScanStreamingBalance_DefaultBalanceKeyFilledIn(t *testing.T) {
	t.Parallel()

	available := "1"
	onHold := "0"
	updatedAt := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)

	row := &fakeRow{values: []any{
		"bal-no-key",
		"org", "ledger", "account-1", "@alice",
		"", // empty balance key (NULL coerced to "" by earlier coalesce)
		"USD",
		&available, &onHold,
		int64(1), "deposit",
		sql.NullBool{Bool: true, Valid: true},
		sql.NullBool{Bool: true, Valid: true},
		updatedAt,
	}}

	cr, err := scanStreamingBalance(row, shard.NewRouter(shard.DefaultShardCount), nil)
	require.NoError(t, err)
	require.NotNil(t, cr.balance)
	require.NotEmpty(t, cr.balance.BalanceKey,
		"empty key must be populated to the default so routing is deterministic")
}

// TestScanStreamingBalance_ScanErrorPropagates proves that an underlying
// Scan error is wrapped (not swallowed) so the keyset loop fail-closes
// instead of silently skipping the row and losing balance state.
func TestScanStreamingBalance_ScanErrorPropagates(t *testing.T) {
	t.Parallel()

	// Deliberately wrong arity to trigger fakeRow's errScanArity.
	row := &fakeRow{values: []any{"only-one-field"}}

	_, err := scanStreamingBalance(row, shard.NewRouter(shard.DefaultShardCount), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "scan streaming balance row")
}

// TestScanStreamingBalance_InvalidDecimalPropagates proves that a malformed
// decimal string from the DB fails the row decode instead of silently
// corrupting the balance — again the fail-closed contract.
func TestScanStreamingBalance_InvalidDecimalPropagates(t *testing.T) {
	t.Parallel()

	bad := "definitely-not-a-number"
	onHold := "0"
	updatedAt := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)

	row := &fakeRow{values: []any{
		"bal-corrupt",
		"org", "ledger", "account-1", "@alice", "default", "USD",
		&bad, &onHold,
		int64(1), "deposit",
		sql.NullBool{Bool: true, Valid: true},
		sql.NullBool{Bool: true, Valid: true},
		updatedAt,
	}}

	_, err := scanStreamingBalance(row, shard.NewRouter(shard.DefaultShardCount), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bal-corrupt")
}
