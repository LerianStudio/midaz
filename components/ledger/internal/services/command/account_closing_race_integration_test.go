//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	libPointers "github.com/LerianStudio/lib-commons/v7/commons/pointers"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// accountClosingRefusalCode extracts the sentinel code a refusal carries, over the
// three typed errors the closing surface answers with.
func accountClosingRefusalCode(t *testing.T, err error) string {
	t.Helper()

	require.Error(t, err)

	var conflict pkg.EntityConflictError
	if errors.As(err, &conflict) {
		return conflict.Code
	}

	var unprocessable pkg.UnprocessableOperationError
	if errors.As(err, &unprocessable) {
		return unprocessable.Code
	}

	var unavailable pkg.ServiceUnavailableError
	require.Truef(t, errors.As(err, &unavailable), "unexpected error type: %T (%v)", err, err)

	return unavailable.Code
}

// TestIntegrationAccountClosingRefusesOverACreditAppliedBeforeTheProtection is
// AS-01 at the command boundary: a credit executed while the account was open, and
// not yet written back to the row, is the state the closing has to read. The
// persisted row still says zero, and closing on that stale zero would strand the
// money — so the refusal comes from the live state, the credit survives untouched
// and no closing instant is recorded.
func TestIntegrationAccountClosingRefusesOverACreditAppliedBeforeTheProtection(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-unsynced", "deposit")
	row := accountClosingBalanceSeed{alias: "@closing-unsynced", key: "default"}
	balanceID := h.seedBalance(t, accountID, row)

	live := accountClosingBalanceSeed{alias: "@closing-unsynced", key: "default", available: "10", version: 1}
	cacheKey := h.cacheBalance(t, accountID, balanceID, live)

	_, err := h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountBalanceNotZero)

	require.False(t, h.closedAt(t, accountID).Valid, "a refused closing records no instant")
	require.Equal(t, int64(1), h.exists(t, cacheKey), "the refusal never evicts the state it read")
	require.Equal(t, row.version, h.balanceRow(t, balanceID).version, "a refusal moves no money and no version")

	protection := h.protection(accountID)
	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.closed, protection.ownership),
		"a known refusal gives back the protection of its own attempt")
}

// TestIntegrationAccountClosingOrdersItselfAgainstABalanceCreation is AS-05: the
// administrative ownership is the single thing both operations contend on, so one
// of them is always the later one and is refused rather than interleaved.
//
// Both directions are proven with barriers instead of timing. A creation that
// arrives while a closing holds the account is refused and persists nothing; a
// creation that finished before the protection is part of the list the closing then
// validates, and its residual is what refuses the closing.
func TestIntegrationAccountClosingOrdersItselfAgainstABalanceCreation(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-creation", "deposit")
	h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-creation", key: "default"})

	// Barrier: a closing attempt holds the account exactly as the use case does.
	const attemptToken = "closing-attempt-token"

	owned, err := h.uc.TransactionRedisRepo.AcquireAccountAdminOwnership(ctx, h.organizationID, h.ledgerID, accountID, attemptToken)
	require.NoError(t, err)
	require.True(t, owned)

	installed, err := h.uc.TransactionRedisRepo.AcquireAccountClosingMarker(ctx, h.organizationID, h.ledgerID, accountID, attemptToken)
	require.NoError(t, err)
	require.True(t, installed)

	_, err = h.uc.CreateAdditionalBalance(ctx, h.organizationID, h.ledgerID, accountID, &mmodel.CreateAdditionalBalance{
		Key:       "savings",
		Direction: libPointers.String(constant.DirectionCredit),
	})
	requireClosingCode(t, err, constant.ErrAccountClosingInProgress)

	balances, err := h.uc.BalanceRepo.ListByAccountID(ctx, h.organizationID, h.ledgerID, accountID)
	require.NoError(t, err)
	require.Len(t, balances, 1, "a creation refused by the coordination persists no balance")

	// Barrier: the closing attempt gives its protection back.
	released, err := h.uc.TransactionRedisRepo.ReleaseAccountClosingAttempt(ctx, h.organizationID, h.ledgerID, accountID, attemptToken)
	require.NoError(t, err)
	require.True(t, released)

	released, err = h.uc.TransactionRedisRepo.ReleaseAccountAdminOwnership(ctx, h.organizationID, h.ledgerID, accountID, attemptToken)
	require.NoError(t, err)
	require.True(t, released)

	created, err := h.uc.CreateAdditionalBalance(ctx, h.organizationID, h.ledgerID, accountID, &mmodel.CreateAdditionalBalance{
		Key:       "savings",
		Direction: libPointers.String(constant.DirectionCredit),
	})
	require.NoError(t, err, "with the protection gone the creation follows its normal contract")

	// The balance that exists before the protection takes part in the validation:
	// a residual on it refuses the closing, which is what "the list is stable"
	// means in practice.
	_, err = h.transactionDB.Exec(`UPDATE balance SET available = 1 WHERE id = $1`, created.ID)
	require.NoError(t, err)

	_, err = h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountBalanceNotZero)
	require.False(t, h.closedAt(t, accountID).Valid)
}

// TestIntegrationAccountClosingKeepsTheAccountClosedBeyondTheNegativeCache covers
// AS-06 and AS-16 together: once the instant is recorded, a writer that still
// carries an older view of the account is refused — first from the negative cache,
// and then, after that cache is gone, from the authoritative row, which recomposes
// the cache instead of admitting anything.
//
// The five-minute horizon is never waited for: an expired denial IS an absent key,
// so the key is removed.
func TestIntegrationAccountClosingKeepsTheAccountClosedBeyondTheNegativeCache(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-horizon", "deposit")
	balanceID := h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-horizon", key: "default"})
	cacheKey := h.cacheBalance(t, accountID, balanceID, accountClosingBalanceSeed{alias: "@closing-horizon", key: "default"})

	closedAt, err := h.close(ctx, accountID)
	require.NoError(t, err)

	protection := h.protection(accountID)
	require.Equal(t, int64(0), h.exists(t, cacheKey), "the finalization evicts the balances it proved settled")
	require.Equal(t, int64(1), h.exists(t, protection.closed), "the finalization installs the negative cache")
	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.ownership),
		"the protection leaves only after the eviction and the negative cache are confirmed")

	ttl, err := h.client.TTL(ctx, protection.closed).Result()
	require.NoError(t, err)
	require.Positive(t, ttl)
	require.LessOrEqual(t, ttl, 300*time.Second, "the negative cache is a horizon, not a state")

	// A writer answered from the negative cache alone.
	_, err = h.uc.CreateAdditionalBalance(ctx, h.organizationID, h.ledgerID, accountID, &mmodel.CreateAdditionalBalance{
		Key:       "savings",
		Direction: libPointers.String(constant.DirectionCredit),
	})
	requireClosingCode(t, err, constant.ErrAccountClosed)

	// The negative cache expires. The persisted instant is what still decides.
	require.NoError(t, h.client.Del(ctx, protection.closed).Err())

	_, err = h.uc.CreateAdditionalBalance(ctx, h.organizationID, h.ledgerID, accountID, &mmodel.CreateAdditionalBalance{
		Key:       "savings",
		Direction: libPointers.String(constant.DirectionCredit),
	})
	requireClosingCode(t, err, constant.ErrAccountClosed)

	require.Equal(t, int64(1), h.exists(t, protection.closed), "the refusal recomposes the negative cache")
	require.Equal(t, int64(0), h.exists(t, cacheKey), "no balance is admitted back into the cache")

	balances, err := h.uc.BalanceRepo.ListByAccountID(ctx, h.organizationID, h.ledgerID, accountID)
	require.NoError(t, err)
	require.Len(t, balances, 1, "the closing preserves the balance rows it evicted from the cache")

	// The repeat is answered from the authoritative row, not from the cache.
	stored := h.closedAt(t, accountID)
	require.True(t, stored.Valid)
	require.True(t, closedAt.Equal(stored.Time), "the returned instant is the one the database recorded")

	_, err = h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountAlreadyClosed)
	require.True(t, closedAt.Equal(h.closedAt(t, accountID).Time), "a repeat never moves the instant")
}

// TestIntegrationAccountClosingLetsOnlyOneConcurrentAttemptThrough is AC-11 over
// the real coordination: two closings contend on the same eligible account and the
// ownership decides. Exactly one records the instant; the other is refused as a
// dispute in progress or as already closed, and neither outcome writes a second
// timestamp.
func TestIntegrationAccountClosingLetsOnlyOneConcurrentAttemptThrough(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-dispute", "deposit")
	h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-dispute", key: "default"})

	type attempt struct {
		closedAt time.Time
		err      error
	}

	results := make([]attempt, 2)
	start := make(chan struct{})

	var wg sync.WaitGroup

	for index := range results {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			closedAt, err := h.close(ctx, accountID)
			results[index] = attempt{closedAt: closedAt, err: err}
		}()
	}

	close(start)
	wg.Wait()

	winners := 0

	for _, result := range results {
		if result.err == nil {
			winners++

			continue
		}

		require.Contains(t,
			[]string{constant.ErrAccountClosingInProgress.Error(), constant.ErrAccountAlreadyClosed.Error()},
			accountClosingRefusalCode(t, result.err), "a losing attempt is refused as a dispute or as already closed")
	}

	require.Equal(t, 1, winners, "at most one attempt may record the transition")

	stored := h.closedAt(t, accountID)
	require.True(t, stored.Valid)

	for _, result := range results {
		if result.err == nil {
			require.True(t, result.closedAt.Equal(stored.Time), "the winner returns the instant the database holds")
		}
	}
}
