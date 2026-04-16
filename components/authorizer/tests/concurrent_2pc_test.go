// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tests

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// TestAuthorizeConcurrentCrossShardSameBalance fires 100 goroutines all
// attempting to debit the SAME pair of balances that live on DIFFERENT
// shards. Under -race, this exercises:
//
//  1. Deterministic per-balance lock ordering (same pair → same order).
//  2. No deadlock (reverse order by a racing goroutine would cause one).
//  3. No double-commit (version must increment exactly once per successful
//     Authorize).
//  4. Correct final balance (sum of successful debits == starting balance
//     minus funds held by insufficient-funds rejections).
//
// The single-engine variant is the unit-of-isolation we can exercise here
// without a second-authorizer bufconn harness; the PrepareAuthorize +
// CommitPrepared path still goes through the same per-balance locking
// machinery that 2PC uses across instances. The cross-instance coordination
// layer is covered separately by cross_shard_test.go in the bootstrap
// package.
// seedTwoBalancesOnDistinctShards seeds the engine with two balances that
// hash to different shards, each with the given starting balance.
func seedTwoBalancesOnDistinctShards(
	e *engine.Engine, aliasA, aliasB string, startingBalance int64,
) {
	e.UpsertBalances([]*engine.Balance{
		{
			ID: "balance-a", OrganizationID: "org", LedgerID: "ledger",
			AccountAlias: aliasA, BalanceKey: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: startingBalance, Scale: 2, Version: 1,
			AllowSending: true, AllowReceiving: true,
		},
		{
			ID: "balance-b", OrganizationID: "org", LedgerID: "ledger",
			AccountAlias: aliasB, BalanceKey: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: startingBalance, Scale: 2, Version: 1,
			AllowSending: true, AllowReceiving: true,
		},
	})
}

// concurrentAuthorizeCounts aggregates the outcome counters returned by
// runConcurrentCrossShardDebit.
type concurrentAuthorizeCounts struct {
	approvals int64
	rejects   int64
	errCount  int64
}

// runConcurrentCrossShardDebit fires goroutines*txPerGoroutine Authorize
// calls in parallel and returns the aggregated outcome counts.
func runConcurrentCrossShardDebit(
	e *engine.Engine, aliasA, aliasB string,
	goroutines, txPerGoroutine int, amount int64,
) concurrentAuthorizeCounts {
	var (
		wg        sync.WaitGroup
		approvals atomic.Int64
		rejects   atomic.Int64
		errCount  atomic.Int64
	)

	start := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			<-start

			for j := 0; j < txPerGoroutine; j++ {
				txID := fmt.Sprintf("tx-g%d-%d", id, j)

				resp, err := e.Authorize(&authorizerv1.AuthorizeRequest{
					TransactionId: txID, OrganizationId: "org", LedgerId: "ledger",
					TransactionStatus: constant.CREATED,
					Operations: []*authorizerv1.BalanceOperation{
						{OperationAlias: "0#" + aliasA + "#default", AccountAlias: aliasA, BalanceKey: "default", Amount: amount, Scale: 2, Operation: constant.DEBIT},
						{OperationAlias: "1#" + aliasB + "#default", AccountAlias: aliasB, BalanceKey: "default", Amount: amount, Scale: 2, Operation: constant.CREDIT},
					},
				})

				switch {
				case err != nil:
					errCount.Add(1)
				case resp.GetAuthorized():
					approvals.Add(1)
				default:
					rejects.Add(1)
				}
			}
		}(i)
	}

	close(start)
	wg.Wait()

	return concurrentAuthorizeCounts{
		approvals: approvals.Load(),
		rejects:   rejects.Load(),
		errCount:  errCount.Load(),
	}
}

func TestAuthorizeConcurrentCrossShardSameBalance(t *testing.T) {
	t.Parallel()

	router := shard.NewRouter(16)
	aliasA, aliasB := findAliasesOnDistinctShards(t, router)

	const (
		startingBalance = int64(1_000_000)
		goroutines      = 100
		txPerGoroutine  = 3
		debitAmount     = int64(100)
	)

	e := engine.New(router, wal.NewNoopWriter())

	defer e.Close()

	seedTwoBalancesOnDistinctShards(e, aliasA, aliasB, startingBalance)

	counts := runConcurrentCrossShardDebit(e, aliasA, aliasB, goroutines, txPerGoroutine, debitAmount)

	require.Equal(t, int64(0), counts.errCount,
		"no goroutine must see an internal error under concurrent cross-shard debit/credit")

	totalAttempts := int64(goroutines * txPerGoroutine)
	require.Equal(t, totalAttempts, counts.approvals+counts.rejects,
		"every authorization attempt must reach a terminal outcome")

	finalA, ok := e.GetBalance("org", "ledger", aliasA, "default")
	require.True(t, ok)

	finalB, ok := e.GetBalance("org", "ledger", aliasB, "default")
	require.True(t, ok)

	require.Equal(t, startingBalance-counts.approvals*debitAmount, finalA.Available,
		"balance A final must equal starting - approvals*debit (no double-commit, no lost commit)")
	require.Equal(t, startingBalance+counts.approvals*debitAmount, finalB.Available,
		"balance B final must equal starting + approvals*credit")

	require.Equal(t, uint64(1)+uint64(counts.approvals), finalA.Version,
		"balance A version must increment exactly once per approval")
	require.Equal(t, uint64(1)+uint64(counts.approvals), finalB.Version,
		"balance B version must increment exactly once per approval")

	t.Logf("concurrent 2PC: approvals=%d rejects=%d (totalAttempts=%d) finalA=%d finalB=%d",
		counts.approvals, counts.rejects, totalAttempts, finalA.Available, finalB.Available)
}

// TestAuthorizePreparedCrossShardSameBalance_NoDeadlock proves the
// prepared-commit path of 2PC (PrepareAuthorize → CommitPrepared) does not
// deadlock under the same concurrent pressure. Unlike Authorize (which
// holds locks for microseconds), Prepare holds locks across a network
// round-trip in production — so deadlock risk is strictly higher.
func TestAuthorizePreparedCrossShardSameBalance_NoDeadlock(t *testing.T) {
	t.Parallel()

	router := shard.NewRouter(16)
	aliasA, aliasB := findAliasesOnDistinctShards(t, router)

	e := engine.New(router, wal.NewNoopWriter())

	defer e.Close()

	e.UpsertBalances([]*engine.Balance{
		{
			ID: "ba", OrganizationID: "org", LedgerID: "ledger",
			AccountAlias: aliasA, BalanceKey: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: 1_000_000, Scale: 2, Version: 1,
			AllowSending: true, AllowReceiving: true,
		},
		{
			ID: "bb", OrganizationID: "org", LedgerID: "ledger",
			AccountAlias: aliasB, BalanceKey: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: 1_000_000, Scale: 2, Version: 1,
			AllowSending: true, AllowReceiving: true,
		},
	})

	const goroutines = 50

	var (
		wg        sync.WaitGroup
		committed atomic.Int64
		aborted   atomic.Int64
	)

	start := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			<-start

			// Prepare a cross-shard tx with a deterministic tx id.
			ptx, resp, err := e.PrepareAuthorize(&authorizerv1.AuthorizeRequest{
				TransactionId:     fmt.Sprintf("ptx-%d", id),
				OrganizationId:    "org",
				LedgerId:          "ledger",
				TransactionStatus: constant.CREATED,
				Operations: []*authorizerv1.BalanceOperation{
					{OperationAlias: fmt.Sprintf("0#%s#default", aliasA), AccountAlias: aliasA, BalanceKey: "default", Amount: 10, Scale: 2, Operation: constant.DEBIT},
					{OperationAlias: fmt.Sprintf("1#%s#default", aliasB), AccountAlias: aliasB, BalanceKey: "default", Amount: 10, Scale: 2, Operation: constant.CREDIT},
				},
			})
			if err != nil {
				aborted.Add(1)
				return
			}

			if resp != nil && !resp.Authorized {
				aborted.Add(1)
				return
			}

			// Alternate commit vs abort to ensure both code paths run
			// concurrently under the same lock-ordering regime.
			if id%2 == 0 {
				_, err := e.CommitPrepared(ptx.ID)
				if err == nil {
					committed.Add(1)
				}
			} else {
				if err := e.AbortPrepared(ptx.ID); err == nil {
					aborted.Add(1)
				}
			}
		}(i)
	}

	// Impose a hard deadline; if the test can't finish in 10s with 50
	// goroutines and 16 shards, it's a deadlock not a slow test.
	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	close(start)

	select {
	case <-done:
		// ok
	case <-time.After(10 * time.Second):
		t.Fatal("prepared-commit concurrent test deadlocked — 50 goroutines failed to finish in 10s")
	}

	require.Equal(t, int64(goroutines), committed.Load()+aborted.Load(),
		"every goroutine must reach a terminal state (commit or abort)")

	// Half commit (even ids), half abort (odd ids). Final balances must
	// reflect only the commits: 10 debits × 25 commits = 250 moved.
	finalA, _ := e.GetBalance("org", "ledger", aliasA, "default")
	require.Equal(t, 1_000_000-committed.Load()*10, finalA.Available)
}

// findAliasesOnDistinctShards iterates until it finds two aliases the
// router maps to distinct shards. Fails the test if exhausting 4096
// candidates didn't yield a pair.
func findAliasesOnDistinctShards(t *testing.T, router *shard.Router) (string, string) {
	t.Helper()

	seen := map[int]string{}

	for i := 0; i < 4096; i++ {
		alias := fmt.Sprintf("@concurrent-%d", i)
		shardID := router.ResolveBalance(alias, constant.DefaultBalanceKey)

		if existing, ok := seen[shardID]; !ok {
			seen[shardID] = alias
		} else if existing != alias && len(seen) >= 2 {
			// We already have two shards populated; return any
			// existing + the current if they differ.
			for otherShard, otherAlias := range seen {
				if otherShard != shardID {
					return otherAlias, alias
				}
			}
		}

		if len(seen) >= 2 {
			break
		}
	}

	require.GreaterOrEqual(t, len(seen), 2, "must find aliases on at least 2 distinct shards")

	var shards [2]int

	var idx int

	for s := range seen {
		shards[idx] = s
		idx++

		if idx == 2 {
			break
		}
	}

	return seen[shards[0]], seen[shards[1]]
}
