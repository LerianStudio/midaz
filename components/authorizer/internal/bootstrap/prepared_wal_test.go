// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// recordingWALWriter captures every Append call for test inspection. This is
// a simpler substitute for a full ringbuffer + replay roundtrip — the
// prepared-intent round-trip is validated at the logical level (entry is
// produced with the expected fields; replay consumes it correctly).
type recordingWALWriter struct {
	entries []wal.Entry
}

func (r *recordingWALWriter) Append(entry wal.Entry) error {
	r.entries = append(r.entries, entry)
	return nil
}

func (r *recordingWALWriter) Close() error { return nil }

// TestPreparedTxPersistedOnPrepare proves every successful PrepareAuthorize
// emits a WAL entry carrying a non-nil PreparedIntent with the prepared_tx_id
// and a serialized copy of the original request. This is the write side of
// the D1 audit finding #2 fix.
func TestPreparedTxPersistedOnPrepare(t *testing.T) {
	recorder := &recordingWALWriter{}

	eng := engine.New(shard.NewRouter(8), recorder)
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-persist",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	}

	ptx, resp, err := eng.PrepareAuthorize(req)
	require.NoError(t, err)
	require.NotNil(t, ptx)
	require.True(t, resp.GetAuthorized())

	err = persistPreparedIntent(eng, ptx, mustInitLogger(t))
	require.NoError(t, err)

	// Exactly one prepared-intent entry was appended.
	require.Len(t, recorder.entries, 1)
	entry := recorder.entries[0]
	require.NotNil(t, entry.PreparedIntent, "prepared intent field must be populated")
	require.Equal(t, ptx.ID, entry.PreparedIntent.PreparedTxID)
	require.NotEmpty(t, entry.PreparedIntent.RequestJSON)
	require.Empty(t, entry.Mutations, "prepared-intent entries must NOT carry mutations")
	require.False(t, entry.PreparedIntent.PreparedAt.IsZero())
}

// TestPreparedTxPersistedAndReplayedOnRestart simulates the crash/recovery
// cycle: prepare a tx, capture its WAL entry, discard the engine, rebuild
// a fresh engine from the same balance state, replay the prepared-intent
// entry, and verify the original prepared_tx_id is restored — so a
// coordinator's post-restart CommitPrepared call still works.
func TestPreparedTxPersistedAndReplayedOnRestart(t *testing.T) {
	// Phase 1 — pre-crash: prepare tx and capture WAL entries.
	recorder := &recordingWALWriter{}
	engPre := engine.New(shard.NewRouter(8), recorder)

	seed := []*engine.Balance{{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}}
	engPre.UpsertBalances(seed)

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-replay",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	}

	ptx, _, err := engPre.PrepareAuthorize(req)
	require.NoError(t, err)

	originalPreparedTxID := ptx.ID

	err = persistPreparedIntent(engPre, ptx, mustInitLogger(t))
	require.NoError(t, err)

	// Simulate crash: discard the engine, keep the WAL entries.
	engPre.Close()

	capturedEntries := append([]wal.Entry(nil), recorder.entries...)

	// Phase 2 — post-crash: fresh engine from the same balance seed, then
	// run prepared-intent replay.
	engPost := engine.New(shard.NewRouter(8), &recordingWALWriter{})
	defer engPost.Close()

	engPost.UpsertBalances(seed)

	err = replayPreparedIntents(engPost, capturedEntries, mustInitLogger(t), true, 30*time.Second)
	require.NoError(t, err)

	// The restored PreparedTx MUST be keyed under the original ID so a
	// coordinator retry against originalPreparedTxID finds it.
	restored := engPost.LookupPreparedTxByID(originalPreparedTxID)
	require.NotNil(t, restored, "prepared tx %s must be restored by replay", originalPreparedTxID)
	require.Equal(t, originalPreparedTxID, restored.ID)

	// Commit via the same ID — this is what a 2PC coordinator would do.
	commitResp, err := engPost.CommitPrepared(originalPreparedTxID)
	require.NoError(t, err)
	require.True(t, commitResp.GetAuthorized())
}

// TestReplayPreparedIntents_SkipsExpiredIntents guards against leaking locks
// across restart: if the prepared intent is older than the prepare timeout,
// re-preparing would leave its locks held for another full timeout interval
// before the reaper releases them. We skip such intents instead.
func TestReplayPreparedIntents_SkipsExpiredIntents(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), &recordingWALWriter{})
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	// Craft a prepared intent with PreparedAt one hour ago — well past any
	// sensible prepare timeout.
	entry := wal.Entry{
		TransactionID:  "tx-expired",
		OrganizationID: "org",
		LedgerID:       "ledger",
		PreparedIntent: &wal.PreparedIntent{
			PreparedTxID: "ptx-expired",
			RequestJSON:  []byte(`{"transactionId":"tx-expired"}`),
			PreparedAt:   time.Now().Add(-1 * time.Hour),
		},
	}

	err := replayPreparedIntents(eng, []wal.Entry{entry}, mustInitLogger(t), true, 30*time.Second)
	require.NoError(t, err)

	require.Nil(t, eng.LookupPreparedTxByID("ptx-expired"),
		"expired prepared intent must NOT be restored")
}

// TestReplayPreparedIntents_SkipsCommittedIntents documents idempotency: if
// the WAL contains both a prepared-intent entry AND a subsequent committed
// entry for the same transaction_id, the commit wins — replay must NOT
// re-prepare, since doing so would over-count against the already-mutated
// balance state.
func TestReplayPreparedIntents_SkipsCommittedIntents(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), &recordingWALWriter{})
	defer eng.Close()

	eng.UpsertBalances([]*engine.Balance{{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      900, // post-commit balance
		Scale:          2,
		Version:        2,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	entries := []wal.Entry{
		{
			TransactionID: "tx-committed",
			PreparedIntent: &wal.PreparedIntent{
				PreparedTxID: "ptx-committed",
				RequestJSON:  []byte(`{"transactionId":"tx-committed"}`),
				PreparedAt:   time.Now(),
			},
		},
		{
			TransactionID: "tx-committed",
			Mutations: []wal.BalanceMutation{
				{AccountAlias: "@alice", BalanceKey: "default", Available: 900, PreviousVersion: 1, NextVersion: 2},
			},
		},
	}

	err := replayPreparedIntents(eng, entries, mustInitLogger(t), false, 30*time.Second)
	require.NoError(t, err)

	require.Nil(t, eng.LookupPreparedTxByID("ptx-committed"),
		"prepared intent with subsequent commit must NOT be re-prepared")
}

// TestReplayPreparedIntents_NoEntriesIsNoop documents the empty-input case:
// a pristine authorizer with no historical WAL entries proceeds without any
// replay-related error.
func TestReplayPreparedIntents_NoEntriesIsNoop(t *testing.T) {
	eng := engine.New(shard.NewRouter(8), &recordingWALWriter{})
	defer eng.Close()

	err := replayPreparedIntents(eng, nil, mustInitLogger(t), true, 30*time.Second)
	require.NoError(t, err)
}
