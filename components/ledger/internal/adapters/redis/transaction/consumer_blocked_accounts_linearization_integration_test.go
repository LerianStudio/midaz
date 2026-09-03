//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// =============================================================================
// BLOCKED ACCOUNTS — LINEARIZATION UNDER CONCURRENCY
// =============================================================================
// This is the test the whole refactor exists for.
//
// The projection this feature replaced enforced blocking from a per-BALANCE
// flag, copied to every balance of the account by a separate command, and read
// by Go validators BEFORE the atomic script ran. That is two windows wide open:
// the copy is not atomic across balances, and the validator's answer is stale by
// the time the script mutates. An account with several balances, transacting
// while an operator blocks it, could therefore post money after the block.
//
// The gate now lives inside the script, which Redis executes serially. That
// gives a single total order over blocks and transactions, and this file
// asserts what that order buys:
//
//	(a) once a worker sees a denial, it NEVER succeeds again — a success after
//	    an observed block would be a linearization violation;
//	(b) every transaction that ran before the block completed in full;
//	(c) no transaction landed PARTIALLY: all balances of the account move in
//	    lock-step, so a denial that mutated some of them would show up as a
//	    divergence between them;
//	(d) after the SADD RETURNS, not one ungranted transaction commits — the
//	    zero-window claim, asserted in real time rather than statistically.

const (
	// linearizationWorkers transact concurrently against the same account.
	linearizationWorkers = 8

	// linearizationRounds is how many transactions each worker attempts. The
	// block lands partway through, so every worker crosses it.
	linearizationRounds = 40

	// linearizationBalances is how many balances the account owns. More than one
	// is the point: it is the shape the old per-balance projection could not
	// keep consistent, and it is what makes a partial mutation observable.
	linearizationBalances = 4

	// linearizationDebit is the amount each transaction takes from EVERY balance
	// of the account, so all of them stay arithmetically identical.
	linearizationDebit = 1

	// linearizationSeed keeps every balance far from zero, so no overdraft or
	// insufficient-funds branch can reject a transaction. The only rejection
	// this test may observe is the block gate.
	linearizationSeed = 1_000_000
)

// linearizationOps builds one transaction: a DEBIT on every balance of the
// account, in a single atomic invocation.
func linearizationOps(orgID, ledgerID, accountID uuid.UUID, aliases []string) []mmodel.BalanceOperation {
	ops := make([]mmodel.BalanceOperation, 0, len(aliases))

	for _, alias := range aliases {
		balanceKey := alias + "#" + constant.DefaultBalanceKey

		ops = append(ops, mmodel.BalanceOperation{
			Balance: &mmodel.Balance{
				ID:             uuid.New().String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      accountID.String(),
				Alias:          alias,
				Key:            balanceKey,
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(linearizationSeed),
				OnHold:         decimal.Zero,
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				Direction:      constant.DirectionCredit,
				OverdraftUsed:  decimal.Zero,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
			Alias: alias,
			Amount: mtransaction.Amount{
				Asset:     "USD",
				Value:     decimal.NewFromInt(linearizationDebit),
				Operation: constant.DEBIT,
			},
			InternalKey: utils.BalanceInternalKey(orgID, ledgerID, balanceKey),
		})
	}

	return ops
}

// TestIntegration_BlockedAccountsLinearization_NoTransactionCommitsAfterTheBlock
// drives real concurrent traffic against a real Redis while the account is
// blocked underneath it.
func TestIntegration_BlockedAccountsLinearization_NoTransactionCommitsAfterTheBlock(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID, ledgerID := uuid.New(), uuid.New()
	accountID := uuid.New()

	aliases := make([]string, 0, linearizationBalances)
	for i := range linearizationBalances {
		aliases = append(aliases, "@linearization-"+uuid.New().String()[:8]+"-"+string(rune('a'+i)))
	}

	// The index starts hydrated and empty: this ledger has no blocked account
	// yet, which is what lets the early transactions through.
	require.NoError(t, infra.repo.HydrateBlockedAccounts(ctx, orgID, ledgerID, nil))

	// Seed every balance through the script itself, so the workers below all
	// take the cache-hit path and no one races the first SET NX.
	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, linearizationOps(orgID, ledgerID, accountID, aliases))
	require.NoError(t, err)

	baseline := readCachedBalance(t, infra, utils.BalanceInternalKey(orgID, ledgerID, aliases[0]+"#"+constant.DefaultBalanceKey)).Available

	var (
		accepted atomic.Int64
		denied   atomic.Int64
		// blockObserved closes as soon as ANY worker is denied, which is the
		// signal the blocker goroutine used to decide it is done.
		violations = make(chan string, linearizationWorkers*linearizationRounds)
		blockLanded = make(chan struct{})
		wg          sync.WaitGroup
	)

	// The blocker: it blocks the account partway through the run. SADD is the
	// enforcement act — the Postgres write that follows it in the real command
	// is irrelevant to what the script sees.
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer close(blockLanded)

		time.Sleep(25 * time.Millisecond)

		if err := infra.repo.AddBlockedAccount(ctx, orgID, ledgerID, accountID); err != nil {
			violations <- "block failed: " + err.Error()
		}
	}()

	for range linearizationWorkers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			sawDenial := false

			for range linearizationRounds {
				_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
					uuid.New(), constant.APPROVED, false,
					linearizationOps(orgID, ledgerID, accountID, aliases))

				var blockedErr AccountBlockedError

				switch {
				case err == nil:
					// (a) A success AFTER this worker already observed the block
					// means the script answered from a state older than one it
					// had already served — the definition of a non-linearizable
					// read.
					if sawDenial {
						violations <- "a transaction committed after the same worker had already been denied"

						return
					}

					accepted.Add(1)
				case errors.As(err, &blockedErr):
					sawDenial = true

					denied.Add(1)
				default:
					violations <- "unexpected failure: " + err.Error()

					return
				}
			}
		}()
	}

	wg.Wait()
	close(violations)

	for violation := range violations {
		t.Error(violation)
	}

	require.Positive(t, accepted.Load(),
		"the run is only meaningful if traffic flowed before the block")
	require.Positive(t, denied.Load(),
		"the run is only meaningful if the block took effect during it")

	// (d) The block has long since returned, so from here the answer is not a
	// race at all: every ungranted transaction MUST be refused.
	<-blockLanded

	for range 5 {
		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.APPROVED, false, linearizationOps(orgID, ledgerID, accountID, aliases))

		var blockedErr AccountBlockedError
		require.ErrorAs(t, err, &blockedErr,
			"after the SADD returned there is no window left: nothing without a grant may commit")
	}

	// (b) + (c) Arithmetic closes exactly. Every accepted transaction debited
	// every balance once, and nothing else touched them — so any partially
	// applied denial, or any lost update, shows up here.
	expected := decimal.RequireFromString(baseline).
		Sub(decimal.NewFromInt(accepted.Load() * linearizationDebit))

	for _, alias := range aliases {
		got := readCachedBalance(t, infra, utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+constant.DefaultBalanceKey)).Available

		assert.Equal(t, expected.String(), got,
			"balance %q must reflect exactly the accepted transactions: no partial mutation, no lost update", alias)
	}
}

// TestIntegration_BlockedAccountsLinearization_GrantedTrafficIsNeverBlocked is
// the control. The same race, but every operation carries an exception grant:
// the block must be invisible to it, or the gate would be denying traffic it
// was told to let through.
func TestIntegration_BlockedAccountsLinearization_GrantedTrafficIsNeverBlocked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	orgID, ledgerID := uuid.New(), uuid.New()
	accountID := uuid.New()
	alias := "@linearization-granted-" + uuid.New().String()[:8]

	require.NoError(t, infra.repo.HydrateBlockedAccounts(ctx, orgID, ledgerID, nil))

	grantedOps := func() []mmodel.BalanceOperation {
		ops := linearizationOps(orgID, ledgerID, accountID, []string{alias})
		ops[0].Amount.GrantedExceptionID = uuid.NewString()

		return ops
	}

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, grantedOps())
	require.NoError(t, err)

	baseline := readCachedBalance(t, infra, utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+constant.DefaultBalanceKey)).Available

	var (
		accepted   atomic.Int64
		violations = make(chan string, linearizationWorkers*linearizationRounds)
		wg         sync.WaitGroup
	)

	wg.Add(1)

	go func() {
		defer wg.Done()

		time.Sleep(25 * time.Millisecond)

		if err := infra.repo.AddBlockedAccount(ctx, orgID, ledgerID, accountID); err != nil {
			violations <- "block failed: " + err.Error()
		}
	}()

	for range linearizationWorkers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range linearizationRounds {
				_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
					uuid.New(), constant.APPROVED, false, grantedOps())
				if err != nil {
					violations <- "granted traffic was refused: " + err.Error()

					return
				}

				accepted.Add(1)
			}
		}()
	}

	wg.Wait()
	close(violations)

	for violation := range violations {
		t.Error(violation)
	}

	assert.Equal(t, int64(linearizationWorkers*linearizationRounds), accepted.Load(),
		"a grant must survive a block landing mid-flight")

	expected := decimal.RequireFromString(baseline).
		Sub(decimal.NewFromInt(accepted.Load() * linearizationDebit))

	assert.Equal(t, expected.String(),
		readCachedBalance(t, infra, utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+constant.DefaultBalanceKey)).Available,
		"every granted transaction must have applied exactly once")
}
