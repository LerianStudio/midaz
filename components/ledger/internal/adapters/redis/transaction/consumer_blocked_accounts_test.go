// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// =============================================================================
// BLOCKED-ACCOUNTS SET — REPOSITORY UNIT TESTS
// =============================================================================
// The SET behind utils.BlockedAccountsInternalKey is the enforcement index for
// account blocking. These tests pin the four properties the rest of the feature
// depends on and that no assertion on a return value can observe:
//
//   - block writes SADD, unblock writes SREM, both on the tenant-namespaced key;
//   - the read asks about the hydration sentinel in the SAME round-trip as the
//     accounts, so "not hydrated" can never be mistaken for "not blocked";
//   - hydration writes the sentinel LAST, so a partial hydration is detectable;
//   - hydration NEVER deletes, so a concurrent block is not erased by a rebuild.

// setMembersCall records one member-carrying SET command.
type setMembersCall struct {
	Key     string
	Members []any
}

// blockedAccountsRecorder records the SET commands the repository issues and
// fails the test on the ones it must never issue (DEL above all).
type blockedAccountsRecorder struct {
	t *testing.T

	saddCalls  []setMembersCall
	sremCalls  []setMembersCall
	smisCalls  []setMembersCall
	smisResult []bool

	// smisResults, when non-empty, answers successive SMISMEMBER calls with
	// successive entries — the only way to express an index that changes between
	// two probes, which is exactly what a repair-and-re-probe read does.
	smisResults [][]bool

	saddErr error
	sremErr error
	smisErr error

	redis.UniversalClient
}

func (r *blockedAccountsRecorder) SAdd(ctx context.Context, key string, members ...any) *redis.IntCmd {
	r.saddCalls = append(r.saddCalls, setMembersCall{Key: key, Members: members})

	cmd := redis.NewIntCmd(ctx)
	if r.saddErr != nil {
		cmd.SetErr(r.saddErr)

		return cmd
	}

	cmd.SetVal(int64(len(members)))

	return cmd
}

func (r *blockedAccountsRecorder) SRem(ctx context.Context, key string, members ...any) *redis.IntCmd {
	r.sremCalls = append(r.sremCalls, setMembersCall{Key: key, Members: members})

	cmd := redis.NewIntCmd(ctx)
	if r.sremErr != nil {
		cmd.SetErr(r.sremErr)

		return cmd
	}

	cmd.SetVal(int64(len(members)))

	return cmd
}

func (r *blockedAccountsRecorder) SMIsMember(ctx context.Context, key string, members ...any) *redis.BoolSliceCmd {
	r.smisCalls = append(r.smisCalls, setMembersCall{Key: key, Members: members})

	cmd := redis.NewBoolSliceCmd(ctx)
	if r.smisErr != nil {
		cmd.SetErr(r.smisErr)

		return cmd
	}

	if len(r.smisResults) > 0 {
		next := r.smisResults[0]
		r.smisResults = r.smisResults[1:]

		cmd.SetVal(next)

		return cmd
	}

	cmd.SetVal(r.smisResult)

	return cmd
}

// Del fails the test on sight: hydration is additive by contract. A DEL would
// erase a block that landed concurrently with the rebuild.
func (r *blockedAccountsRecorder) Del(_ context.Context, keys ...string) *redis.IntCmd {
	r.t.Fatalf("DEL is forbidden on the blocked-accounts SET, called with %v", keys)

	return nil
}

func newBlockedAccountsRepo(t *testing.T) (*RedisConsumerRepository, *blockedAccountsRecorder) {
	t.Helper()

	rec := &blockedAccountsRecorder{t: t}

	return &RedisConsumerRepository{conn: &staticRedisProvider{client: rec}}, rec
}

func blockedAccountsScope(t *testing.T) (uuid.UUID, uuid.UUID) {
	t.Helper()

	return uuid.Must(libCommons.GenerateUUIDv7()), uuid.Must(libCommons.GenerateUUIDv7())
}

func TestAddBlockedAccount_WritesCanonicalAccountIDWithSADD(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	organizationID, ledgerID := blockedAccountsScope(t)
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	require.NoError(t, repo.AddBlockedAccount(t.Context(), organizationID, ledgerID, accountID))

	require.Len(t, rec.saddCalls, 1)
	assert.Equal(t, utils.BlockedAccountsInternalKey(organizationID, ledgerID), rec.saddCalls[0].Key)
	assert.Equal(t, []any{accountID.String()}, rec.saddCalls[0].Members,
		"members must be canonical uuid.UUID.String(), the form every reader compares against")
	assert.Empty(t, rec.sremCalls)
}

func TestAddBlockedAccount_PropagatesRedisFailure(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	organizationID, ledgerID := blockedAccountsScope(t)

	sentinel := errors.New("redis down")
	rec.saddErr = sentinel

	err := repo.AddBlockedAccount(t.Context(), organizationID, ledgerID, uuid.Must(libCommons.GenerateUUIDv7()))

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel, "the block command must be able to see the underlying cause")
}

func TestRemoveBlockedAccount_WritesCanonicalAccountIDWithSREM(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	organizationID, ledgerID := blockedAccountsScope(t)
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	require.NoError(t, repo.RemoveBlockedAccount(t.Context(), organizationID, ledgerID, accountID))

	require.Len(t, rec.sremCalls, 1)
	assert.Equal(t, utils.BlockedAccountsInternalKey(organizationID, ledgerID), rec.sremCalls[0].Key)
	assert.Equal(t, []any{accountID.String()}, rec.sremCalls[0].Members)
	assert.Empty(t, rec.saddCalls, "unblock must not add anything to the index")
}

func TestRemoveBlockedAccount_PropagatesRedisFailure(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	organizationID, ledgerID := blockedAccountsScope(t)

	sentinel := errors.New("redis down")
	rec.sremErr = sentinel

	err := repo.RemoveBlockedAccount(t.Context(), organizationID, ledgerID, uuid.Must(libCommons.GenerateUUIDv7()))

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// TestIsHydratedAndBlocked_AsksTheSentinelInTheSameRoundTrip is the core
// fail-closed property of the read: hydration state and membership are decided
// from ONE SMISMEMBER, so no interleaving can answer "not blocked" from a SET
// that was emptied by a restart.
func TestIsHydratedAndBlocked_AsksTheSentinelInTheSameRoundTrip(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	organizationID, ledgerID := blockedAccountsScope(t)

	blocked := uuid.Must(libCommons.GenerateUUIDv7())
	free := uuid.Must(libCommons.GenerateUUIDv7())

	// sentinel present, first account blocked, second not.
	rec.smisResult = []bool{true, true, false}

	hydrated, got, err := repo.IsHydratedAndBlocked(t.Context(), organizationID, ledgerID, []uuid.UUID{blocked, free})
	require.NoError(t, err)

	require.Len(t, rec.smisCalls, 1, "hydration state and membership must cost exactly one round-trip")
	assert.Equal(t, utils.BlockedAccountsInternalKey(organizationID, ledgerID), rec.smisCalls[0].Key)
	assert.Equal(t,
		[]any{utils.BlockedAccountsHydratedMember, blocked.String(), free.String()},
		rec.smisCalls[0].Members,
		"the sentinel must be the FIRST member asked about, so its answer is positionally unambiguous")

	assert.True(t, hydrated)
	assert.Equal(t, []uuid.UUID{blocked}, got)
}

// TestIsHydratedAndBlocked_UnhydratedSetReportsNothingBlocked proves the read
// refuses to hand back a partial truth: with the sentinel absent the SET says
// nothing about any account, and the caller MUST hydrate rather than proceed.
func TestIsHydratedAndBlocked_UnhydratedSetReportsNothingBlocked(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	organizationID, ledgerID := blockedAccountsScope(t)

	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	// Sentinel absent, yet the account happens to be present: a half-written SET.
	rec.smisResult = []bool{false, true}

	hydrated, got, err := repo.IsHydratedAndBlocked(t.Context(), organizationID, ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)

	assert.False(t, hydrated)
	assert.Empty(t, got, "an unhydrated SET must yield no membership answer at all")
}

// TestIsHydratedAndBlocked_EmptyInputStillProbesHydration matters because the
// enrichment pre-read may hold no accounts yet still need to know whether the
// index is usable.
func TestIsHydratedAndBlocked_EmptyInputStillProbesHydration(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	organizationID, ledgerID := blockedAccountsScope(t)

	rec.smisResult = []bool{true}

	hydrated, got, err := repo.IsHydratedAndBlocked(t.Context(), organizationID, ledgerID, nil)
	require.NoError(t, err)

	require.Len(t, rec.smisCalls, 1)
	assert.Equal(t, []any{utils.BlockedAccountsHydratedMember}, rec.smisCalls[0].Members)
	assert.True(t, hydrated)
	assert.Empty(t, got)
}

func TestIsHydratedAndBlocked_PropagatesRedisFailure(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	organizationID, ledgerID := blockedAccountsScope(t)

	sentinel := errors.New("redis down")
	rec.smisErr = sentinel

	hydrated, got, err := repo.IsHydratedAndBlocked(t.Context(), organizationID, ledgerID, []uuid.UUID{uuid.Must(libCommons.GenerateUUIDv7())})

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.False(t, hydrated, "an errored probe must never read as hydrated")
	assert.Empty(t, got)
}

// TestIsHydratedAndBlocked_RejectsShortReply guards against a truncated
// BoolSlice: indexing it positionally would panic or, worse, silently shift the
// answers one account to the left.
func TestIsHydratedAndBlocked_RejectsShortReply(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	organizationID, ledgerID := blockedAccountsScope(t)

	rec.smisResult = []bool{true} // sentinel only, but two accounts were asked about

	hydrated, got, err := repo.IsHydratedAndBlocked(t.Context(), organizationID, ledgerID,
		[]uuid.UUID{uuid.Must(libCommons.GenerateUUIDv7()), uuid.Must(libCommons.GenerateUUIDv7())})

	require.Error(t, err)
	assert.False(t, hydrated)
	assert.Empty(t, got)
}

// TestHydrateBlockedAccounts_WritesSentinelLast is the invariant that makes a
// crashed hydration self-healing: members first, sentinel only after all of them
// landed, so an interrupted rebuild leaves an index that announces itself as
// incomplete.
func TestHydrateBlockedAccounts_WritesSentinelLast(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	organizationID, ledgerID := blockedAccountsScope(t)

	first := uuid.Must(libCommons.GenerateUUIDv7())
	second := uuid.Must(libCommons.GenerateUUIDv7())

	require.NoError(t, repo.HydrateBlockedAccounts(t.Context(), organizationID, ledgerID, []uuid.UUID{first, second}))

	require.Len(t, rec.saddCalls, 2, "one chunk of members, then the sentinel on its own")
	assert.Equal(t, []any{first.String(), second.String()}, rec.saddCalls[0].Members)
	assert.Equal(t, []any{utils.BlockedAccountsHydratedMember}, rec.saddCalls[1].Members)

	for _, call := range rec.saddCalls {
		assert.Equal(t, utils.BlockedAccountsInternalKey(organizationID, ledgerID), call.Key)
	}
}

// TestHydrateBlockedAccounts_DoesNotWriteSentinelWhenMembersFail keeps a failed
// rebuild honest: the SET must stay unhydrated so the next access retries.
func TestHydrateBlockedAccounts_DoesNotWriteSentinelWhenMembersFail(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	organizationID, ledgerID := blockedAccountsScope(t)

	sentinel := errors.New("redis down")
	rec.saddErr = sentinel

	err := repo.HydrateBlockedAccounts(t.Context(), organizationID, ledgerID, []uuid.UUID{uuid.Must(libCommons.GenerateUUIDv7())})

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	require.Len(t, rec.saddCalls, 1, "the sentinel must not be attempted after a member chunk failed")
}

// TestHydrateBlockedAccounts_EmptyLedgerIsStillHydrated: a ledger with zero
// blocked accounts is a legitimate, fully-known state. Skipping the sentinel
// there would make every transaction on that ledger re-hydrate forever.
func TestHydrateBlockedAccounts_EmptyLedgerIsStillHydrated(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	organizationID, ledgerID := blockedAccountsScope(t)

	require.NoError(t, repo.HydrateBlockedAccounts(t.Context(), organizationID, ledgerID, nil))

	require.Len(t, rec.saddCalls, 1)
	assert.Equal(t, []any{utils.BlockedAccountsHydratedMember}, rec.saddCalls[0].Members)
}

// TestHydrateBlockedAccounts_ChunksOversizedInput mirrors ScheduleBalanceSyncBatch:
// a ledger with a large blocked population must not be sent as one oversized
// command.
func TestHydrateBlockedAccounts_ChunksOversizedInput(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	organizationID, ledgerID := blockedAccountsScope(t)

	accountIDs := make([]uuid.UUID, maxRedisBatchSize+1)
	for i := range accountIDs {
		accountIDs[i] = uuid.Must(libCommons.GenerateUUIDv7())
	}

	require.NoError(t, repo.HydrateBlockedAccounts(t.Context(), organizationID, ledgerID, accountIDs))

	require.Len(t, rec.saddCalls, 3, "two member chunks plus the sentinel")
	assert.Len(t, rec.saddCalls[0].Members, maxRedisBatchSize)
	assert.Len(t, rec.saddCalls[1].Members, 1)
	assert.Equal(t, []any{utils.BlockedAccountsHydratedMember}, rec.saddCalls[2].Members)
}
