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
