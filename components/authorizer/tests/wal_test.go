// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tests

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// testWALHMACKey is a 32-byte key shared across authorizer/tests package wal
// scenarios. Production WAL uses a key from AUTHORIZER_WAL_HMAC_KEY validated
// against the deny list and character-class policy in bootstrap/config.go.
var testWALHMACKey = []byte("integration-tests-wal-hmac-key!!")

// replaySkipObserver captures the reasons passed to ObserveWALReplaySkipped so
// tests can prove a skip metric actually fired (not just that NoError was
// returned -- which a silent drop would also satisfy).
type replaySkipObserver struct {
	reasons []string
}

func (o *replaySkipObserver) ObserveAuthorizeLockWait(_, _ int, _ time.Duration) {}
func (o *replaySkipObserver) ObserveAuthorizeLockHold(_, _ int, _ time.Duration) {}
func (o *replaySkipObserver) ObserveWALAppendFailure(_ error)                    {}

func (o *replaySkipObserver) ObserveWALReplaySkipped(reason, _ string, _ int) {
	o.reasons = append(o.reasons, reason)
}

func TestWALAppendAndRecovery(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "authorizer.wal")

	writer, err := wal.NewRingBufferWriter(walPath, 1024, time.Millisecond, testWALHMACKey)
	require.NoError(t, err)

	router := shard.NewRouter(8)
	liveEngine := engine.New(router, writer)

	defer liveEngine.Close()

	liveEngine.UpsertBalances(seedRecoveryBalances())

	resp, err := liveEngine.Authorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-recovery",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 1000, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "1#@bob#default", AccountAlias: "@bob", BalanceKey: "default", Amount: 1000, Scale: 2, Operation: constant.CREDIT},
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Authorized)

	require.NoError(t, writer.Close())

	entries, err := wal.Replay(walPath, [][]byte{testWALHMACKey}, nil)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	recoveryEngine := engine.New(router, wal.NewNoopWriter())

	defer recoveryEngine.Close()

	recoveryEngine.UpsertBalances(seedRecoveryBalances())

	require.NoError(t, recoveryEngine.ReplayEntries(entries))

	alice, ok := recoveryEngine.GetBalance("org", "ledger", "@alice", "default")
	require.True(t, ok)
	require.Equal(t, int64(9000), alice.Available)
	require.Equal(t, uint64(2), alice.Version)

	bob, ok := recoveryEngine.GetBalance("org", "ledger", "@bob", "default")
	require.True(t, ok)
	require.Equal(t, int64(1000), bob.Available)
	require.Equal(t, uint64(2), bob.Version)

	require.NoError(t, recoveryEngine.ReplayEntries(entries))

	aliceAfterSecondReplay, ok := recoveryEngine.GetBalance("org", "ledger", "@alice", "default")
	require.True(t, ok)
	require.Equal(t, int64(9000), aliceAfterSecondReplay.Available)
	require.Equal(t, uint64(2), aliceAfterSecondReplay.Version)
}

func TestReplayEntriesSkipsVersionMismatchWithoutMutation(t *testing.T) {
	router := shard.NewRouter(8)
	recoveryEngine := engine.New(router, wal.NewNoopWriter())

	defer recoveryEngine.Close()

	observer := &replaySkipObserver{}
	recoveryEngine.SetObserver(observer)
	recoveryEngine.ConfigureReplayPolicy(2048, 2048, false)
	recoveryEngine.UpsertBalances(seedRecoveryBalances())

	err := recoveryEngine.ReplayEntries([]wal.Entry{{
		TransactionID:  "tx-mismatch",
		OrganizationID: "org",
		LedgerID:       "ledger",
		Mutations: []wal.BalanceMutation{
			{
				AccountAlias:    "@alice",
				BalanceKey:      constant.DefaultBalanceKey,
				Available:       123,
				OnHold:          0,
				PreviousVersion: 999,
				NextVersion:     1000,
			},
		},
	}})
	require.NoError(t, err)

	// STRONGER ASSERTION: the skip must be observable through the metrics
	// pipeline. A silent drop (no observer fire) would also satisfy
	// require.NoError above, which is why the bare NoError assertion was
	// insufficient for detecting regressions.
	require.Equal(t, []string{"version_mismatch"}, observer.reasons,
		"version mismatch must emit exactly one skip metric")

	alice, ok := recoveryEngine.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(10000), alice.Available)
	require.Equal(t, int64(0), alice.OnHold)
	require.Equal(t, uint64(1), alice.Version)

	resp, err := recoveryEngine.Authorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-after-replay-mismatch",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 1000, Scale: 2, Operation: constant.DEBIT},
		},
	})
	require.NoError(t, err)
	require.True(t, resp.GetAuthorized())
}

func TestReplayEntriesSkipsMissingBalanceEntry(t *testing.T) {
	router := shard.NewRouter(8)
	recoveryEngine := engine.New(router, wal.NewNoopWriter())

	defer recoveryEngine.Close()

	recoveryEngine.ConfigureReplayPolicy(2048, 2048, false)
	recoveryEngine.UpsertBalances(seedRecoveryBalances())

	err := recoveryEngine.ReplayEntries([]wal.Entry{{
		TransactionID:  "tx-missing",
		OrganizationID: "org",
		LedgerID:       "ledger",
		Mutations: []wal.BalanceMutation{
			{
				AccountAlias:    "@alice",
				BalanceKey:      constant.DefaultBalanceKey,
				Available:       9999,
				OnHold:          0,
				PreviousVersion: 1,
				NextVersion:     2,
			},
			{
				AccountAlias:    "@missing",
				BalanceKey:      constant.DefaultBalanceKey,
				Available:       10,
				OnHold:          0,
				PreviousVersion: 1,
				NextVersion:     2,
			},
		},
	}})
	require.NoError(t, err)

	alice, ok := recoveryEngine.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(10000), alice.Available)
	require.Equal(t, uint64(1), alice.Version)
}

func TestReplayEntriesVersionMismatchSkipsWholeEntry(t *testing.T) {
	router := shard.NewRouter(8)
	recoveryEngine := engine.New(router, wal.NewNoopWriter())

	defer recoveryEngine.Close()

	recoveryEngine.ConfigureReplayPolicy(2048, 2048, false)
	recoveryEngine.UpsertBalances(seedRecoveryBalances())

	err := recoveryEngine.ReplayEntries([]wal.Entry{{
		TransactionID:  "tx-partial-mismatch",
		OrganizationID: "org",
		LedgerID:       "ledger",
		Mutations: []wal.BalanceMutation{
			{
				AccountAlias:    "@alice",
				BalanceKey:      constant.DefaultBalanceKey,
				Available:       9000,
				OnHold:          0,
				PreviousVersion: 1,
				NextVersion:     2,
			},
			{
				AccountAlias:    "@bob",
				BalanceKey:      constant.DefaultBalanceKey,
				Available:       500,
				OnHold:          0,
				PreviousVersion: 999,
				NextVersion:     1000,
			},
		},
	}})
	require.NoError(t, err)

	alice, ok := recoveryEngine.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(10000), alice.Available)
	require.Equal(t, uint64(1), alice.Version)

	bob, ok := recoveryEngine.GetBalance("org", "ledger", "@bob", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(0), bob.Available)
	require.Equal(t, uint64(1), bob.Version)
}

func seedRecoveryBalances() []*engine.Balance {
	return []*engine.Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			Available:      10000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
		{
			ID:             "b2",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@bob",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			Available:      0,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	}
}
