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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// =============================================================================
// BLOCK GATE — VERDICT CLASSIFICATION AND INDEX REPAIR (UNIT)
// =============================================================================

// stubBlockedAccountsSource stands in for the durable source of truth the index
// is rebuilt from.
type stubBlockedAccountsSource struct {
	accountIDs []uuid.UUID
	err        error
	calls      int
}

func (s *stubBlockedAccountsSource) ListBlockedAccountIDs(_ context.Context, _, _ uuid.UUID) ([]uuid.UUID, error) {
	s.calls++

	return s.accountIDs, s.err
}

// TestClassifyBlockGateReply pins how the script's structured verdicts are read
// back. The gate answers with an ordinary value rather than an error reply, so
// misreading one as "proceed" is a silent fail-open — which is why the balance
// JSON and the verdicts are separated here rather than at the call site.
func TestClassifyBlockGateReply(t *testing.T) {
	t.Parallel()

	blockedID := uuid.NewString()

	tests := []struct {
		name          string
		reply         any
		wantVerdict   blockGateVerdict
		wantAccountID string
	}{
		{
			name:        "balance payload proceeds",
			reply:       `{"before":[],"after":[]}`,
			wantVerdict: blockGateProceed,
		},
		{
			name:        "byte-slice balance payload proceeds",
			reply:       []byte(`{"before":[],"after":[]}`),
			wantVerdict: blockGateProceed,
		},
		{
			name:        "unhydrated index asks for a rebuild",
			reply:       "NEEDS_HYDRATION",
			wantVerdict: blockGateNeedsHydration,
		},
		{
			name:          "denial carries the account that caused it",
			reply:         "BLOCKED:" + blockedID,
			wantVerdict:   blockGateBlocked,
			wantAccountID: blockedID,
		},
		{
			name:          "byte-slice denial is read the same way",
			reply:         []byte("BLOCKED:" + blockedID),
			wantVerdict:   blockGateBlocked,
			wantAccountID: blockedID,
		},
		{
			name:        "unexpected type proceeds and is left to the decoder",
			reply:       42,
			wantVerdict: blockGateProceed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			verdict, accountID := classifyBlockGateReply(tc.reply)

			assert.Equal(t, tc.wantVerdict, verdict)
			assert.Equal(t, tc.wantAccountID, accountID)
		})
	}
}

// TestRehydrateBlockedAccounts_NilSourceFailsClosed is the invariant that keeps
// a misconfigured deployment from becoming a silent unblock: with no source
// there is no way to tell an empty index from a lost one, so the only safe
// answer is to refuse.
func TestRehydrateBlockedAccounts_NilSourceFailsClosed(t *testing.T) {
	t.Parallel()

	repo, _ := newBlockedAccountsRepo(t)
	repo.blockedAccountsSource = nil

	organizationID, ledgerID := blockedAccountsScope(t)

	err := repo.rehydrateBlockedAccounts(t.Context(), organizationID, ledgerID)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBlockedAccountsIndexUnavailable,
		"a missing source is unavailability, never an empty index")
}

// TestRehydrateBlockedAccounts_SourceFailureFailsClosed covers the same
// invariant when the source of truth is reachable but broken.
func TestRehydrateBlockedAccounts_SourceFailureFailsClosed(t *testing.T) {
	t.Parallel()

	repo, _ := newBlockedAccountsRepo(t)

	sourceErr := errors.New("postgres is down")
	repo.blockedAccountsSource = &stubBlockedAccountsSource{err: sourceErr}

	organizationID, ledgerID := blockedAccountsScope(t)

	err := repo.rehydrateBlockedAccounts(t.Context(), organizationID, ledgerID)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBlockedAccountsIndexUnavailable)
	assert.ErrorIs(t, err, sourceErr, "the underlying cause must survive the wrap")
}

// TestRehydrateBlockedAccounts_RebuildsFromTheSource covers the happy path: the
// source is read once and its members land in the SET, sentinel last.
func TestRehydrateBlockedAccounts_RebuildsFromTheSource(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)

	blocked := []uuid.UUID{
		uuid.Must(libCommons.GenerateUUIDv7()),
		uuid.Must(libCommons.GenerateUUIDv7()),
	}
	source := &stubBlockedAccountsSource{accountIDs: blocked}
	repo.blockedAccountsSource = source

	organizationID, ledgerID := blockedAccountsScope(t)

	require.NoError(t, repo.rehydrateBlockedAccounts(t.Context(), organizationID, ledgerID))

	assert.Equal(t, 1, source.calls, "the source of truth is read once per repair")
	require.Len(t, rec.saddCalls, 2, "one chunk of members, then the sentinel")
	assert.Equal(t, []any{blocked[0].String(), blocked[1].String()}, rec.saddCalls[0].Members)
	assert.Equal(t, []any{utils.BlockedAccountsHydratedMember}, rec.saddCalls[1].Members,
		"the sentinel must be the LAST write, so an interrupted rebuild stays detectable")
}

// =============================================================================
// BLOCK GATE — SELF-REPAIRING PRE-READ (UNIT)
// =============================================================================
// ResolveBlockedAccounts is the read the commit path uses to decide whether an
// account-exception enrichment is worth running. It exists so the sentinel, the
// repair and the re-probe stay INSIDE this adapter: the handler asks "which of
// these accounts are blocked" and gets either an answer or an outage, never a
// hydration protocol to drive.

// TestResolveBlockedAccounts_HydratedIndexAnswersInOneProbe is the common path:
// a healthy index costs exactly one SMISMEMBER and never touches Postgres.
func TestResolveBlockedAccounts_HydratedIndexAnswersInOneProbe(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)

	source := &stubBlockedAccountsSource{}
	repo.blockedAccountsSource = source

	organizationID, ledgerID := blockedAccountsScope(t)

	blocked := uuid.Must(libCommons.GenerateUUIDv7())
	free := uuid.Must(libCommons.GenerateUUIDv7())

	rec.smisResult = []bool{true, true, false}

	got, err := repo.ResolveBlockedAccounts(t.Context(), organizationID, ledgerID, []uuid.UUID{blocked, free})
	require.NoError(t, err)

	assert.Equal(t, []uuid.UUID{blocked}, got)
	assert.Len(t, rec.smisCalls, 1, "a hydrated index must not cost a second round-trip")
	assert.Equal(t, 0, source.calls, "the source of truth is read only to repair, never to answer")
}

// TestResolveBlockedAccounts_EmptyInputCostsNothing keeps the pre-read free for
// the transactions that have no account to ask about.
func TestResolveBlockedAccounts_EmptyInputCostsNothing(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	repo.blockedAccountsSource = &stubBlockedAccountsSource{}

	organizationID, ledgerID := blockedAccountsScope(t)

	got, err := repo.ResolveBlockedAccounts(t.Context(), organizationID, ledgerID, nil)

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Empty(t, rec.smisCalls, "nothing to ask about must cost no round-trip")
}

// TestResolveBlockedAccounts_RepairsUnhydratedIndexThenAnswers is the whole
// point of the method: an index lost to a restart is rebuilt and re-probed here,
// so the caller never learns that a sentinel exists.
func TestResolveBlockedAccounts_RepairsUnhydratedIndexThenAnswers(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)

	blocked := uuid.Must(libCommons.GenerateUUIDv7())

	source := &stubBlockedAccountsSource{accountIDs: []uuid.UUID{blocked}}
	repo.blockedAccountsSource = source

	organizationID, ledgerID := blockedAccountsScope(t)

	// First probe: sentinel absent. Second probe, after the rebuild: hydrated,
	// and the account is there.
	rec.smisResults = [][]bool{
		{false, false},
		{true, true},
	}

	got, err := repo.ResolveBlockedAccounts(t.Context(), organizationID, ledgerID, []uuid.UUID{blocked})
	require.NoError(t, err)

	assert.Equal(t, []uuid.UUID{blocked}, got,
		"the repaired index must answer, and it must not answer 'not blocked'")
	assert.Len(t, rec.smisCalls, 2, "exactly one re-probe after the repair")
	assert.Equal(t, 1, source.calls)
	require.Len(t, rec.saddCalls, 2, "one chunk of members, then the sentinel")
	assert.Equal(t, []any{utils.BlockedAccountsHydratedMember}, rec.saddCalls[1].Members)
}

// TestResolveBlockedAccounts_StillUnhydratedAfterRepairIsUnavailable: a rebuild
// that did not stick is an outage, never a licence to read the index as empty.
func TestResolveBlockedAccounts_StillUnhydratedAfterRepairIsUnavailable(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	repo.blockedAccountsSource = &stubBlockedAccountsSource{}

	organizationID, ledgerID := blockedAccountsScope(t)

	rec.smisResults = [][]bool{
		{false, false},
		{false, false},
	}

	got, err := repo.ResolveBlockedAccounts(t.Context(), organizationID, ledgerID,
		[]uuid.UUID{uuid.Must(libCommons.GenerateUUIDv7())})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBlockedAccountsIndexUnavailable)
	assert.Empty(t, got)
	assert.Len(t, rec.smisCalls, 2, "the index is re-probed exactly once, never in a loop")
}

// TestResolveBlockedAccounts_ProbeFailureIsUnavailable: an unreachable Redis is
// an infrastructure failure the caller must surface as one — collapsing it into
// "nothing is blocked" is the fail-open this design refuses, and collapsing it
// into a denial would answer an outage with a business error.
func TestResolveBlockedAccounts_ProbeFailureIsUnavailable(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	repo.blockedAccountsSource = &stubBlockedAccountsSource{}

	organizationID, ledgerID := blockedAccountsScope(t)

	probeErr := errors.New("redis down")
	rec.smisErr = probeErr

	got, err := repo.ResolveBlockedAccounts(t.Context(), organizationID, ledgerID,
		[]uuid.UUID{uuid.Must(libCommons.GenerateUUIDv7())})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBlockedAccountsIndexUnavailable)
	assert.ErrorIs(t, err, probeErr, "the underlying cause must survive the wrap")
	assert.Empty(t, got)
}

// TestResolveBlockedAccounts_NilSourceFailsClosed mirrors the atomic gate: a
// repository that cannot repair the index cannot read a lost one as empty.
func TestResolveBlockedAccounts_NilSourceFailsClosed(t *testing.T) {
	t.Parallel()

	repo, rec := newBlockedAccountsRepo(t)
	repo.blockedAccountsSource = nil

	organizationID, ledgerID := blockedAccountsScope(t)

	rec.smisResult = []bool{false, false}

	got, err := repo.ResolveBlockedAccounts(t.Context(), organizationID, ledgerID,
		[]uuid.UUID{uuid.Must(libCommons.GenerateUUIDv7())})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBlockedAccountsIndexUnavailable)
	assert.Empty(t, got)
}

// emptyBlockedAccountsSource is the source the test infrastructures install.
//
// It stands in for the command layer, which in production hands the repository
// a Postgres-backed source: these ledgers simply have no blocked account to
// hydrate. It is NOT a convenience default — a nil source is deliberately
// fail-closed, so a test that drives the atomic script has to say out loud that
// its ledger is unblocked.
func emptyBlockedAccountsSource() BlockedAccountsSource {
	return &stubBlockedAccountsSource{}
}
