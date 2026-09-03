//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// =============================================================================
// BLOCKED-ACCOUNTS SET — REDIS INTEGRATION TESTS
// =============================================================================
// The unit tests pin which commands the repository issues. These pin what those
// commands actually do to a real Redis: SADD/SREM/SMISMEMBER round-trip, a
// partial hydration reading as unhydrated, a concurrent block surviving a
// hydration, and the absence of a TTL — the invariant whose violation is a
// silent, unrequested unblock.

// blockedAccountsLedger returns a fresh org/ledger pair so tests sharing the
// reusable container never touch each other's SET.
func blockedAccountsLedger() (uuid.UUID, uuid.UUID) {
	return uuid.New(), uuid.New()
}

func TestIntegration_BlockedAccountsSet_AddProbeRemoveRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID, ledgerID := blockedAccountsLedger()
	blocked := uuid.New()
	free := uuid.New()

	// A ledger whose index was hydrated and then had one account blocked.
	require.NoError(t, infra.repo.HydrateBlockedAccounts(ctx, orgID, ledgerID, nil))
	require.NoError(t, infra.repo.AddBlockedAccount(ctx, orgID, ledgerID, blocked))

	hydrated, got, err := infra.repo.IsHydratedAndBlocked(ctx, orgID, ledgerID, []uuid.UUID{blocked, free})
	require.NoError(t, err)
	assert.True(t, hydrated)
	assert.Equal(t, []uuid.UUID{blocked}, got)

	// Unblock removes exactly that member and nothing else.
	require.NoError(t, infra.repo.RemoveBlockedAccount(ctx, orgID, ledgerID, blocked))

	hydrated, got, err = infra.repo.IsHydratedAndBlocked(ctx, orgID, ledgerID, []uuid.UUID{blocked, free})
	require.NoError(t, err)
	assert.True(t, hydrated, "unblocking must not de-hydrate the index")
	assert.Empty(t, got)
}

func TestIntegration_BlockedAccountsSet_AddAndRemoveAreIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID, ledgerID := blockedAccountsLedger()
	accountID := uuid.New()

	require.NoError(t, infra.repo.HydrateBlockedAccounts(ctx, orgID, ledgerID, nil))

	require.NoError(t, infra.repo.AddBlockedAccount(ctx, orgID, ledgerID, accountID))
	require.NoError(t, infra.repo.AddBlockedAccount(ctx, orgID, ledgerID, accountID),
		"re-blocking must be a silent no-op so a retried command converges")

	hydrated, got, err := infra.repo.IsHydratedAndBlocked(ctx, orgID, ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)
	require.True(t, hydrated)
	assert.Equal(t, []uuid.UUID{accountID}, got)

	require.NoError(t, infra.repo.RemoveBlockedAccount(ctx, orgID, ledgerID, accountID))
	require.NoError(t, infra.repo.RemoveBlockedAccount(ctx, orgID, ledgerID, accountID),
		"re-unblocking an absent member must be a silent no-op")
}

// TestIntegration_BlockedAccountsSet_PartialHydrationReadsAsUnhydrated is the
// fail-closed proof: members present without the sentinel — the shape an
// interrupted hydration leaves behind — must NOT be served as an answer.
func TestIntegration_BlockedAccountsSet_PartialHydrationReadsAsUnhydrated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID, ledgerID := blockedAccountsLedger()
	accountID := uuid.New()
	key := utils.BlockedAccountsInternalKey(orgID, ledgerID)

	// Simulate a hydration that wrote its members and died before the sentinel.
	require.NoError(t, infra.redisContainer.Client.SAdd(ctx, key, accountID.String()).Err())

	hydrated, got, err := infra.repo.IsHydratedAndBlocked(ctx, orgID, ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)
	assert.False(t, hydrated, "a SET without the sentinel is not a usable index")
	assert.Empty(t, got, "an unhydrated SET must yield no membership answer, not a partial one")

	// Re-hydrating completes it, and the survivor is still there.
	require.NoError(t, infra.repo.HydrateBlockedAccounts(ctx, orgID, ledgerID, []uuid.UUID{accountID}))

	hydrated, got, err = infra.repo.IsHydratedAndBlocked(ctx, orgID, ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)
	assert.True(t, hydrated)
	assert.Equal(t, []uuid.UUID{accountID}, got)
}

// TestIntegration_BlockedAccountsSet_HydrationNeverRemovesPreexistingMember is
// why hydration is additive and never DELs: an account blocked while the
// hydration was reading PostgreSQL is absent from that read, and a rebuild that
// replaced the SET would silently unblock it.
func TestIntegration_BlockedAccountsSet_HydrationNeverRemovesPreexistingMember(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID, ledgerID := blockedAccountsLedger()
	fromPostgres := uuid.New()
	blockedConcurrently := uuid.New()

	// The concurrent block lands first; the hydration's snapshot predates it.
	require.NoError(t, infra.repo.AddBlockedAccount(ctx, orgID, ledgerID, blockedConcurrently))
	require.NoError(t, infra.repo.HydrateBlockedAccounts(ctx, orgID, ledgerID, []uuid.UUID{fromPostgres}))

	hydrated, got, err := infra.repo.IsHydratedAndBlocked(ctx, orgID, ledgerID,
		[]uuid.UUID{fromPostgres, blockedConcurrently})
	require.NoError(t, err)

	require.True(t, hydrated)
	assert.ElementsMatch(t, []uuid.UUID{fromPostgres, blockedConcurrently}, got,
		"hydration is additive: a block that landed during the rebuild must survive it")
}

// TestIntegration_BlockedAccountsSet_CarriesNoTTL guards the one invariant that
// fails open: an expiring key would drop every blocked account from the index
// with no operator action and no trace.
func TestIntegration_BlockedAccountsSet_CarriesNoTTL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID, ledgerID := blockedAccountsLedger()
	key := utils.BlockedAccountsInternalKey(orgID, ledgerID)

	require.NoError(t, infra.repo.HydrateBlockedAccounts(ctx, orgID, ledgerID, []uuid.UUID{uuid.New()}))
	require.NoError(t, infra.repo.AddBlockedAccount(ctx, orgID, ledgerID, uuid.New()))

	ttl, err := infra.redisContainer.Client.TTL(ctx, key).Result()
	require.NoError(t, err)

	// go-redis reports -1ns for a key that exists with no expiration.
	assert.Negative(t, ttl, "the blocked-accounts SET must never carry a TTL: expiry is a silent unblock")
}

// TestIntegration_BlockedAccountsSet_IsScopedPerLedger proves the key carries
// the ledger scope: blocking in one ledger must not block in another.
func TestIntegration_BlockedAccountsSet_IsScopedPerLedger(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID := uuid.New()
	ledgerA, ledgerB := uuid.New(), uuid.New()
	accountID := uuid.New()

	require.NoError(t, infra.repo.HydrateBlockedAccounts(ctx, orgID, ledgerA, nil))
	require.NoError(t, infra.repo.HydrateBlockedAccounts(ctx, orgID, ledgerB, nil))
	require.NoError(t, infra.repo.AddBlockedAccount(ctx, orgID, ledgerA, accountID))

	_, gotA, err := infra.repo.IsHydratedAndBlocked(ctx, orgID, ledgerA, []uuid.UUID{accountID})
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{accountID}, gotA)

	_, gotB, err := infra.repo.IsHydratedAndBlocked(ctx, orgID, ledgerB, []uuid.UUID{accountID})
	require.NoError(t, err)
	assert.Empty(t, gotB, "the index is per-ledger; a block must not leak across ledgers")
}
