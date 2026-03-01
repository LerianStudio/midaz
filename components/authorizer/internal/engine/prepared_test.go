// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
	"github.com/stretchr/testify/require"
)

type failingPreparedWALWriter struct{}

func (f failingPreparedWALWriter) Append(_ wal.Entry) error {
	return errors.New("wal append failed")
}

func (f failingPreparedWALWriter) Close() error {
	return nil
}

func TestPreparedTxStorePutEnforcesCapacity(t *testing.T) {
	store := newPreparedTxStore(DefaultPrepareTimeout, 1)

	err := store.Put(&PreparedTx{ID: "ptx-1"})
	require.NoError(t, err)

	err = store.Put(&PreparedTx{ID: "ptx-2"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "capacity exceeded")
}

func TestPreparedTxStoreNilReceiverGuards(t *testing.T) {
	var store *preparedTxStore

	require.Error(t, store.Put(&PreparedTx{ID: "ptx-1"}))
	require.Error(t, store.PutBack(&PreparedTx{ID: "ptx-1"}))

	ptx, committed, found := store.TakeForCommit("ptx-1")
	require.False(t, found)
	require.Nil(t, ptx)
	require.Nil(t, committed)

	ptxAbort, err := store.TakeForAbort("ptx-1")
	require.ErrorIs(t, err, ErrPreparedTxNotFound)
	require.Nil(t, ptxAbort)

	store.MarkCommitted("ptx-1", &authorizerv1.AuthorizeResponse{Authorized: true})
	require.Nil(t, store.Expired())
	require.Equal(t, 0, store.Len())
}

func TestCommitPreparedWALFailurePreservesPreparedState(t *testing.T) {
	eng := New(shard.NewRouter(8), failingPreparedWALWriter{})
	defer eng.Close()

	eng.UpsertBalances([]*Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			OnHold:         0,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	ptx, resp, err := eng.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-1",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.GetAuthorized())
	require.NotNil(t, ptx)

	_, err = eng.CommitPrepared(ptx.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "WAL append failed")

	err = eng.AbortPrepared(ptx.ID)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPreparedTxCommitDecided)

	eng.SetWALWriter(wal.NewNoopWriter())

	commitResp, err := eng.CommitPrepared(ptx.ID)
	require.NoError(t, err)
	require.True(t, commitResp.GetAuthorized())
}

func TestPrepareAuthorizeAndCommitPreparedSuccess(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			OnHold:         0,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	ptx, resp, err := eng.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-prepare-commit",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, ptx)
	require.True(t, resp.GetAuthorized())

	commitResp, err := eng.CommitPrepared(ptx.ID)
	require.NoError(t, err)
	require.True(t, commitResp.GetAuthorized())

	balance, ok := eng.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(9900), balance.Available)
	require.Equal(t, uint64(2), balance.Version)

	replayResp, err := eng.CommitPrepared(ptx.ID)
	require.NoError(t, err)
	require.True(t, replayResp.GetAuthorized())
	require.Equal(t, commitResp.GetBalances(), replayResp.GetBalances())
}

func TestCommitDecidedPreparedIsNotAutoExpired(t *testing.T) {
	eng := New(shard.NewRouter(8), failingPreparedWALWriter{})
	defer eng.Close()

	eng.prepStore.timeout = 100 * time.Millisecond

	eng.UpsertBalances([]*Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			OnHold:         0,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	ptx, _, err := eng.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-commit-decided-not-expired",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, ptx)

	_, err = eng.CommitPrepared(ptx.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "WAL append failed")

	time.Sleep(200 * time.Millisecond)

	eng.SetWALWriter(wal.NewNoopWriter())
	commitResp, err := eng.CommitPrepared(ptx.ID)
	require.NoError(t, err)
	require.True(t, commitResp.GetAuthorized())
}

func TestPreparedTxStoreExpiredMarksAndRemoves(t *testing.T) {
	store := newPreparedTxStore(5*time.Millisecond, 10)

	ptx := &PreparedTx{ID: "ptx-expired", createdAt: time.Now().Add(-10 * time.Millisecond)}
	require.NoError(t, store.Put(ptx))

	expired := store.Expired()
	require.Len(t, expired, 1)
	require.Equal(t, "ptx-expired", expired[0].ID)
	require.Equal(t, 0, store.Len())
}

func TestReapExpiredPreparedReleasesLocks(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.prepStore.timeout = 10 * time.Millisecond

	eng.UpsertBalances([]*Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			OnHold:         0,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	ptx, resp, err := eng.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-expire",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, ptx)
	require.True(t, resp.GetAuthorized())

	require.Eventually(t, func() bool {
		return eng.prepStore.Len() == 0
	}, 3*time.Second, 50*time.Millisecond)

	_, err = eng.CommitPrepared(ptx.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")

	finalResp, err := eng.Authorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-after-expire",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	})
	require.NoError(t, err)
	require.True(t, finalResp.GetAuthorized())
}

func TestPrepareAuthorizeConcurrentSameBalanceBlocksUntilRelease(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*Balance{{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      10000,
		Scale:          2,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	req := func(txID string) *authorizerv1.AuthorizeRequest {
		return &authorizerv1.AuthorizeRequest{
			TransactionId:     txID,
			OrganizationId:    "org",
			LedgerId:          "ledger",
			Pending:           false,
			TransactionStatus: constant.CREATED,
			Operations: []*authorizerv1.BalanceOperation{
				{OperationAlias: "0#@alice#default", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			},
		}
	}

	ptx1, resp1, err := eng.PrepareAuthorize(req("tx-concurrent-1"))
	require.NoError(t, err)
	require.NotNil(t, ptx1)
	require.True(t, resp1.GetAuthorized())

	resultCh := make(chan struct {
		ptx *PreparedTx
		err error
	}, 1)

	go func() {
		ptx2, _, err2 := eng.PrepareAuthorize(req("tx-concurrent-2"))
		resultCh <- struct {
			ptx *PreparedTx
			err error
		}{ptx: ptx2, err: err2}
	}()

	select {
	case <-resultCh:
		t.Fatal("second prepare should block while first prepared transaction holds lock")
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, eng.AbortPrepared(ptx1.ID))

	select {
	case result := <-resultCh:
		require.NoError(t, result.err)
		require.NotNil(t, result.ptx)
		require.NoError(t, eng.AbortPrepared(result.ptx.ID))
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second prepare after lock release")
	}
}
