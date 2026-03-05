// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

type failingPreparedWALWriter struct{}

// capturingWALWriter records all appended WAL entries for test assertions.
type capturingWALWriter struct {
	entries []wal.Entry
}

func (c *capturingWALWriter) Append(entry wal.Entry) error {
	c.entries = append(c.entries, entry)
	return nil
}

func (c *capturingWALWriter) Close() error { return nil }

// requireChannelBlocked asserts that ch does not produce a value within duration d.
// NOTE: This helper is inherently timing-sensitive. The duration d (typically 150ms)
// is chosen as a reasonable compromise between false positives on slow CI runners
// and test execution speed. If this proves flaky, increase d or use runtime.Gosched
// barriers instead of wall-clock timeouts.
func requireChannelBlocked[T any](t *testing.T, ch <-chan T, d time.Duration, msg string) {
	t.Helper()

	select {
	case <-ch:
		t.Fatal(msg)
	case <-time.After(d):
	}
}

var errWALAppendFailed = errors.New("wal append failed")

func (f failingPreparedWALWriter) Append(_ wal.Entry) error {
	return errWALAppendFailed
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
	require.ErrorIs(t, err, constant.ErrPreparedTxCapacityExceeded)
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

func TestCommitPreparedReturnsDoubleFailureWhenPutBackRetryExhausted(t *testing.T) {
	eng := New(shard.NewRouter(8), failingPreparedWALWriter{})
	defer eng.Close()

	eng.prepStore.maxRetries = 1
	eng.UpsertBalances([]*Balance{{
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
	}})

	ptx, resp, err := eng.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-putback-retry-exhausted",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{{
			OperationAlias: "0#@alice#default",
			AccountAlias:   "@alice",
			BalanceKey:     "default",
			Amount:         100,
			Scale:          2,
			Operation:      constant.DEBIT,
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, ptx)
	require.True(t, resp.GetAuthorized())

	_, err = eng.CommitPrepared(ptx.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "WAL append failed")
	require.Contains(t, err.Error(), "also failed to preserve prepared state")
	require.ErrorIs(t, err, errWALAppendFailed)

	err = eng.AbortPrepared(ptx.ID)
	require.ErrorIs(t, err, ErrPreparedTxNotFound)
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
	attempted := make(chan struct{})

	go func() {
		close(attempted)

		ptx2, _, err2 := eng.PrepareAuthorize(req("tx-concurrent-2"))
		resultCh <- struct {
			ptx *PreparedTx
			err error
		}{ptx: ptx2, err: err2}
	}()

	<-attempted

	requireChannelBlocked(t, resultCh, 150*time.Millisecond, "second prepare should block while first prepared transaction holds lock")

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

func TestPrepareAuthorizeReverseOperationOrderDoesNotDeadlock(t *testing.T) {
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
			AssetCode:      "USD",
			Available:      10000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	makeReq := func(txID, firstAlias, secondAlias string) *authorizerv1.AuthorizeRequest {
		return &authorizerv1.AuthorizeRequest{
			TransactionId:     txID,
			OrganizationId:    "org",
			LedgerId:          "ledger",
			Pending:           false,
			TransactionStatus: constant.CREATED,
			Operations: []*authorizerv1.BalanceOperation{
				{OperationAlias: "0", AccountAlias: firstAlias, BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
				{OperationAlias: "1", AccountAlias: secondAlias, BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.CREDIT},
			},
		}
	}

	type prepareResult struct {
		ptx *PreparedTx
		err error
	}

	start := make(chan struct{})
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	firstCh := make(chan prepareResult, 1)
	secondCh := make(chan prepareResult, 1)

	go func() {
		<-start
		close(firstStarted)

		ptx, _, err := eng.PrepareAuthorize(makeReq("tx-r1", "@alice", "@bob"))
		firstCh <- prepareResult{ptx: ptx, err: err}
	}()

	go func() {
		<-start
		close(secondStarted)

		ptx, _, err := eng.PrepareAuthorize(makeReq("tx-r2", "@bob", "@alice"))
		secondCh <- prepareResult{ptx: ptx, err: err}
	}()

	close(start)
	<-firstStarted
	<-secondStarted

	var winner prepareResult

	var loserCh chan prepareResult

	select {
	case winner = <-firstCh:
		loserCh = secondCh
	case winner = <-secondCh:
		loserCh = firstCh
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first prepare result")
	}

	require.NoError(t, winner.err)
	require.NotNil(t, winner.ptx)

	requireChannelBlocked(t, loserCh, 150*time.Millisecond, "second prepare should block until first prepared tx releases locks")

	require.NoError(t, eng.AbortPrepared(winner.ptx.ID))

	select {
	case loser := <-loserCh:
		require.NoError(t, loser.err)
		require.NotNil(t, loser.ptx)
		require.NoError(t, eng.AbortPrepared(loser.ptx.ID))
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second prepare after first lock release")
	}

	resp, err := eng.Authorize(makeReq("tx-after", "@alice", "@bob"))
	require.NoError(t, err)
	require.True(t, resp.GetAuthorized())
}

func TestUpsertBalancesDoesNotReplaceLockedBalancePointer(t *testing.T) {
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
		OnHold:         50,
		Scale:          2,
		Version:        5,
		AllowSending:   true,
		AllowReceiving: true,
		AccountID:      "acc-1",
	}})

	ptx, resp, err := eng.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-prepare",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, ptx)
	require.True(t, resp.GetAuthorized())

	started := make(chan struct{})
	finished := make(chan int64, 1)

	go func() {
		close(started)

		finished <- eng.UpsertBalances([]*Balance{{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   "@alice",
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			Scale:          2,
			Version:        5,
			AllowSending:   true,
			AllowReceiving: true,
		}})
	}()

	<-started

	requireChannelBlocked(t, finished, 150*time.Millisecond, "upsert should wait while prepared transaction holds balance lock")

	_, err = eng.CommitPrepared(ptx.ID)
	require.NoError(t, err)

	select {
	case inserted := <-finished:
		require.EqualValues(t, 0, inserted)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upsert to finish after commit")
	}

	balance, ok := eng.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(9900), balance.Available)
	require.Equal(t, uint64(6), balance.Version)

	eng.UpsertBalances([]*Balance{{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      8800,
		Scale:          2,
		Version:        7,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	updated, ok := eng.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(8800), updated.Available)
	require.Equal(t, uint64(7), updated.Version)
}

func TestConfigurePreparedTxStoreSetsValues(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.ConfigurePreparedTxStore(5*time.Second, 42)

	eng.prepStore.mu.Lock()

	timeout := eng.prepStore.timeout
	maxPending := eng.prepStore.max

	eng.prepStore.mu.Unlock()

	require.Equal(t, 5*time.Second, timeout)
	require.Equal(t, 42, maxPending)
}

func TestConfigurePreparedTxStoreIgnoresInvalidValues(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.prepStore.mu.Lock()

	originalTimeout := eng.prepStore.timeout
	originalMax := eng.prepStore.max

	eng.prepStore.mu.Unlock()

	// Zero/negative values should be ignored.
	eng.ConfigurePreparedTxStore(0, -1)

	eng.prepStore.mu.Lock()

	timeout := eng.prepStore.timeout
	maxPending := eng.prepStore.max

	eng.prepStore.mu.Unlock()

	require.Equal(t, originalTimeout, timeout)
	require.Equal(t, originalMax, maxPending)
}

func TestConfigurePreparedTxRetentionSetsValues(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.ConfigurePreparedTxRetention(2*time.Hour, 7)

	eng.prepStore.mu.Lock()

	ttl := eng.prepStore.committedTTL
	retries := eng.prepStore.maxRetries

	eng.prepStore.mu.Unlock()

	require.Equal(t, 2*time.Hour, ttl)
	require.Equal(t, 7, retries)
}

func TestConfigurePreparedTxRetentionIgnoresInvalidValues(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.prepStore.mu.Lock()

	originalTTL := eng.prepStore.committedTTL
	originalRetries := eng.prepStore.maxRetries

	eng.prepStore.mu.Unlock()

	eng.ConfigurePreparedTxRetention(0, -1)

	eng.prepStore.mu.Lock()

	ttl := eng.prepStore.committedTTL
	retries := eng.prepStore.maxRetries

	eng.prepStore.mu.Unlock()

	require.Equal(t, originalTTL, ttl)
	require.Equal(t, originalRetries, retries)
}

func TestConfigurePreparedTxStoreNilEngineGuard(t *testing.T) {
	var eng *Engine

	// Must not panic on nil engine.
	require.NotPanics(t, func() {
		eng.ConfigurePreparedTxStore(5*time.Second, 100)
	})
}

func TestConfigurePreparedTxRetentionNilEngineGuard(t *testing.T) {
	var eng *Engine

	// Must not panic on nil engine.
	require.NotPanics(t, func() {
		eng.ConfigurePreparedTxRetention(5*time.Second, 3)
	})
}

func TestCommittedResponseTTLPruning(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	// Set a very short committed TTL.
	eng.prepStore.mu.Lock()
	eng.prepStore.committedTTL = 10 * time.Millisecond
	eng.prepStore.mu.Unlock()

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

	// Prepare and commit a transaction.
	ptx, resp, err := eng.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-ttl-prune",
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

	// Immediately replaying should return the cached committed response.
	replayResp, err := eng.CommitPrepared(ptx.ID)
	require.NoError(t, err)
	require.True(t, replayResp.GetAuthorized())

	// Wait for the committed TTL to expire.
	time.Sleep(50 * time.Millisecond)

	// After TTL expiration, TakeForCommit should no longer find the cached response,
	// resulting in ErrPreparedTxNotFound.
	_, err = eng.CommitPrepared(ptx.ID)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPreparedTxNotFound)
}

func TestBalanceCloneIsIndependentSnapshot(t *testing.T) {
	balance := &Balance{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1000,
		OnHold:         300,
		Scale:          2,
		Version:        42,
		AccountType:    "checking",
		AllowSending:   true,
		AllowReceiving: true,
		AccountID:      "acc-1",
	}

	clone := balance.clone()
	require.NotNil(t, clone)
	require.Equal(t, balance.Available, clone.Available)
	require.Equal(t, balance.OnHold, clone.OnHold)
	require.Equal(t, balance.Version, clone.Version)

	clone.Available = 1
	clone.OnHold = 2
	clone.Version = 99

	require.Equal(t, int64(1000), balance.Available)
	require.Equal(t, int64(300), balance.OnHold)
	require.Equal(t, uint64(42), balance.Version)
}

func TestPrepareAuthorizeSameShardDifferentBalancesDoNotBlock(t *testing.T) {
	router := shard.NewRouter(8)
	firstAlias, secondAlias, ok := findAliasesSharingShard(router)
	require.True(t, ok, "expected to find two aliases sharing a shard")

	eng := New(router, wal.NewNoopWriter())
	defer eng.Close()

	eng.UpsertBalances([]*Balance{
		{
			ID:             "b1",
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   firstAlias,
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
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
			AccountAlias:   secondAlias,
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      10000,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	})

	req := func(txID, alias string) *authorizerv1.AuthorizeRequest {
		return &authorizerv1.AuthorizeRequest{
			TransactionId:     txID,
			OrganizationId:    "org",
			LedgerId:          "ledger",
			Pending:           false,
			TransactionStatus: constant.CREATED,
			Operations: []*authorizerv1.BalanceOperation{
				{OperationAlias: txID, AccountAlias: alias, BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			},
		}
	}

	ptx1, _, err := eng.PrepareAuthorize(req("tx-a", firstAlias))
	require.NoError(t, err)
	require.NotNil(t, ptx1)

	resultCh := make(chan struct {
		ptx *PreparedTx
		err error
	}, 1)

	go func() {
		ptx2, _, err2 := eng.PrepareAuthorize(req("tx-b", secondAlias))
		resultCh <- struct {
			ptx *PreparedTx
			err error
		}{ptx: ptx2, err: err2}
	}()

	select {
	case result := <-resultCh:
		require.NoError(t, result.err)
		require.NotNil(t, result.ptx)
		require.NoError(t, eng.AbortPrepared(result.ptx.ID))
	case <-time.After(2 * time.Second):
		t.Fatal("prepare on different balance in same shard should not block")
	}

	require.NoError(t, eng.AbortPrepared(ptx1.ID))
}

func TestUpsertBalancesSkipsStaleVersionUpdate(t *testing.T) {
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
		OnHold:         100,
		Scale:          2,
		Version:        10,
		AllowSending:   true,
		AllowReceiving: true,
	}})

	eng.UpsertBalances([]*Balance{{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      1,
		OnHold:         2,
		Scale:          2,
		Version:        9,
		AllowSending:   false,
		AllowReceiving: false,
	}})

	balance, ok := eng.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(10000), balance.Available)
	require.Equal(t, int64(100), balance.OnHold)
	require.Equal(t, uint64(10), balance.Version)
	require.True(t, balance.AllowSending)
	require.True(t, balance.AllowReceiving)
}

func TestCommitPreparedReleasesLocksForSubsequentAuthorization(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	req := seedPreparedReleaseScenario(t, eng)

	ptx, _, err := eng.PrepareAuthorize(protoAuthorizeRequest(t, req, "tx-commit"))
	require.NoError(t, err)
	require.NotNil(t, ptx)

	_, err = eng.CommitPrepared(ptx.ID)
	require.NoError(t, err)

	resp, err := eng.Authorize(protoAuthorizeRequest(t, req, "tx-after-commit"))
	require.NoError(t, err)
	require.True(t, resp.GetAuthorized())
}

func TestAbortPreparedReleasesLocksForSubsequentAuthorization(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	req := seedPreparedReleaseScenario(t, eng)

	ptx, _, err := eng.PrepareAuthorize(protoAuthorizeRequest(t, req, "tx-abort"))
	require.NoError(t, err)
	require.NotNil(t, ptx)

	require.NoError(t, eng.AbortPrepared(ptx.ID))

	resp, err := eng.Authorize(protoAuthorizeRequest(t, req, "tx-after-abort"))
	require.NoError(t, err)
	require.True(t, resp.GetAuthorized())
}

func TestPreparedReaperReleasesLocksForSubsequentAuthorization(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	eng.prepStore.timeout = 10 * time.Millisecond

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

	req := &authorizerv1.AuthorizeRequest{
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	}

	ptxExpire, _, err := eng.PrepareAuthorize(protoAuthorizeRequest(t, req, "tx-expire"))
	require.NoError(t, err)
	require.NotNil(t, ptxExpire)

	require.Eventually(t, func() bool {
		return eng.prepStore.Len() == 0
	}, 3*time.Second, 50*time.Millisecond)

	resp, err := eng.Authorize(protoAuthorizeRequest(t, req, "tx-final"))
	require.NoError(t, err)
	require.True(t, resp.GetAuthorized())
}

func seedPreparedReleaseScenario(t *testing.T, eng *Engine) *authorizerv1.AuthorizeRequest {
	t.Helper()

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

	return &authorizerv1.AuthorizeRequest{
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "0", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
		},
	}
}

func TestPrepareAuthorizeSameBalanceOperations(t *testing.T) {
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

	req := &authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-same-balance",
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations: []*authorizerv1.BalanceOperation{
			{OperationAlias: "1", AccountAlias: "@alice", BalanceKey: "default", Amount: 100, Scale: 2, Operation: constant.DEBIT},
			{OperationAlias: "2", AccountAlias: "@alice", BalanceKey: "default", Amount: 200, Scale: 2, Operation: constant.CREDIT},
		},
	}

	ptx, resp, err := eng.PrepareAuthorize(req)
	require.NoError(t, err)
	require.NotNil(t, ptx)
	require.True(t, resp.GetAuthorized())

	_, err = eng.CommitPrepared(ptx.ID)
	require.NoError(t, err)

	balance, ok := eng.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(10100), balance.Available)
	require.Equal(t, uint64(3), balance.Version)
}

func TestUpsertBalancesEqualVersionRefreshesPolicyOnly(t *testing.T) {
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
		OnHold:         50,
		Scale:          2,
		Version:        5,
		AllowSending:   true,
		AllowReceiving: true,
		AccountID:      "acc-1",
	}})

	eng.UpsertBalances([]*Balance{{
		ID:             "b1",
		OrganizationID: "org",
		LedgerID:       "ledger",
		AccountAlias:   "@alice",
		BalanceKey:     constant.DefaultBalanceKey,
		AssetCode:      "BRL",
		Available:      9999, // Financial fields must not change on equal version.
		OnHold:         111,
		Scale:          4,
		Version:        5, // Equal version refreshes policy metadata only.
		AllowSending:   false,
		AllowReceiving: false,
		AccountType:    "blocked",
		AccountID:      "acc-2",
	}})

	balance, ok := eng.GetBalance("org", "ledger", "@alice", constant.DefaultBalanceKey)
	require.True(t, ok)
	require.Equal(t, int64(10000), balance.Available)
	require.Equal(t, int64(50), balance.OnHold)
	require.Equal(t, int32(2), balance.Scale)
	require.Equal(t, "USD", balance.AssetCode)
	require.Equal(t, "acc-1", balance.AccountID)
	require.Equal(t, uint64(5), balance.Version)
	require.False(t, balance.AllowSending)
	require.False(t, balance.AllowReceiving)
	require.Equal(t, "blocked", balance.AccountType)
}

func findAliasesSharingShard(router *shard.Router) (string, string, bool) {
	if router == nil {
		return "", "", false
	}

	firstByShard := make(map[int]string, router.ShardCount())

	for i := 0; i < 5000; i++ {
		alias := fmt.Sprintf("@same-shard-%d", i)
		shardID := router.ResolveBalance(alias, constant.DefaultBalanceKey)

		if first, ok := firstByShard[shardID]; ok && first != alias {
			return first, alias, true
		}

		firstByShard[shardID] = alias
	}

	return "", "", false
}

func protoAuthorizeRequest(t *testing.T, base *authorizerv1.AuthorizeRequest, txID string) *authorizerv1.AuthorizeRequest {
	t.Helper()
	require.NotNil(t, base)

	clonedReq, ok := proto.Clone(base).(*authorizerv1.AuthorizeRequest)
	require.True(t, ok)
	require.NotNil(t, clonedReq)

	clonedReq.TransactionId = txID
	clonedReq.Operations = make([]*authorizerv1.BalanceOperation, 0, len(base.GetOperations()))

	for _, op := range base.GetOperations() {
		if op == nil {
			continue
		}

		clonedOp, opOK := proto.Clone(op).(*authorizerv1.BalanceOperation)
		require.True(t, opOK)
		require.NotNil(t, clonedOp)

		clonedReq.Operations = append(clonedReq.Operations, clonedOp)
	}

	return clonedReq
}

func TestTagCrossShardOnPreparedTx(t *testing.T) {
	cw := &capturingWALWriter{}
	eng := New(shard.NewRouter(8), cw)

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

	ptx, resp, err := eng.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
		TransactionId:     "tx-tag-cross-shard",
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

	participants := []wal.WALParticipant{
		{InstanceAddr: "authorizer-1:50051", PreparedTxID: ptx.ID, IsLocal: true},
		{InstanceAddr: "authorizer-2:50051", PreparedTxID: "ptx-remote", IsLocal: false},
	}

	tagged := eng.TagCrossShard(ptx.ID, participants)
	require.True(t, tagged, "TagCrossShard should return true for a valid prepared tx")

	// Commit and verify the WAL entry carries cross-shard metadata.
	_, err = eng.CommitPrepared(ptx.ID)
	require.NoError(t, err)

	require.Len(t, cw.entries, 1, "expected exactly one WAL entry after commit")

	entry := cw.entries[0]
	require.True(t, entry.CrossShard, "WAL entry should have CrossShard=true after TagCrossShard")
	require.Len(t, entry.Participants, 2, "WAL entry should carry all participants")
	require.Equal(t, "authorizer-1:50051", entry.Participants[0].InstanceAddr)
	require.True(t, entry.Participants[0].IsLocal)
	require.Equal(t, "authorizer-2:50051", entry.Participants[1].InstanceAddr)
	require.False(t, entry.Participants[1].IsLocal)
	require.Equal(t, "ptx-remote", entry.Participants[1].PreparedTxID)
}

func TestTagCrossShardReturnsFalseForUnknownTx(t *testing.T) {
	eng := New(shard.NewRouter(8), wal.NewNoopWriter())
	defer eng.Close()

	participants := []wal.WALParticipant{
		{InstanceAddr: "authorizer-1:50051", PreparedTxID: "nonexistent", IsLocal: true},
	}

	tagged := eng.TagCrossShard("nonexistent-tx-id", participants)
	require.False(t, tagged, "TagCrossShard should return false for unknown tx")
}

func TestTagCrossShardReturnsFalseForNilEngine(t *testing.T) {
	var eng *Engine

	tagged := eng.TagCrossShard("any-id", []wal.WALParticipant{
		{InstanceAddr: "localhost:50051", PreparedTxID: "ptx-1", IsLocal: true},
	})
	require.False(t, tagged, "TagCrossShard should return false for nil engine")
}
