// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tests

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
	"github.com/stretchr/testify/require"
)

func TestWALAppendAndRecovery(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "authorizer.wal")

	writer, err := wal.NewRingBufferWriter(walPath, 1024, time.Millisecond)
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

	entries, err := wal.Replay(walPath)
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
