// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// =============================================================================
// BLOCK / UNBLOCK — FAIL-CLOSED ORDERING
// =============================================================================
// The blocked-accounts Redis SET is the enforcement index the transactional hot
// path consults, so WHEN it is written decides whether a window exists in which
// a blocked account can still transact. The order is asymmetric on purpose:
//
//	block:   SADD first  — enforcement starts before the durable write, so a
//	                       failure afterwards leaves the account blocked, not free.
//	unblock: SREM last   — the durable write lands before enforcement is lifted,
//	                       so a failure leaves a residual block, which is safe.
//
// Every failure in between is returned WITHOUT compensation: rolling the SADD
// back would reopen exactly the window the ordering closes. Both directions are
// idempotent, so the operator's retry is what completes the transition.

// expectAddBlocked arms the SADD that makes the block effective.
func (f *blockStateFixture) expectAddBlocked(err error) *gomock.Call {
	return f.redisRepo.EXPECT().
		AddBlockedAccount(gomock.Any(), f.organizationID, f.ledgerID, f.accountID).
		Return(err).
		Times(1)
}

// expectRemoveBlocked arms the SREM that lifts the block.
func (f *blockStateFixture) expectRemoveBlocked(err error) *gomock.Call {
	return f.redisRepo.EXPECT().
		RemoveBlockedAccount(gomock.Any(), f.organizationID, f.ledgerID, f.accountID).
		Return(err).
		Times(1)
}

// TestBlockAccount_WritesEnforcementIndexBeforeSourceOfTruth pins the block
// order. Any later step failing is fine; the account is already unable to
// transact, which is the direction a partial block must fail in.
func TestBlockAccount_WritesEnforcementIndexBeforeSourceOfTruth(t *testing.T) {
	t.Parallel()

	f := newBlockStateFixture(t)

	find := f.accountRepo.EXPECT().
		Find(gomock.Any(), f.organizationID, f.ledgerID, nil, f.accountID, mmodel.HolderOffV1).
		Return(f.accountWithBlocked(boolPtr(false)), nil).
		Times(1)

	sadd := f.expectAddBlocked(nil)

	update := f.accountRepo.EXPECT().
		Update(gomock.Any(), f.organizationID, f.ledgerID, nil, f.accountID, gomock.Any()).
		Return(f.accountWithBlocked(boolPtr(true)), nil).
		Times(1)

	propagate := f.balanceRepo.EXPECT().
		UpdateAccountBlockedByAccountID(gomock.Any(), f.organizationID, f.ledgerID, f.accountID, true).
		Return(nil).
		Times(1)

	list := f.balanceRepo.EXPECT().
		ListByAccountID(gomock.Any(), f.organizationID, f.ledgerID, f.accountID).
		Return(f.balancesOfAccount(), nil).
		Times(1)

	cache := f.redisRepo.EXPECT().
		SetAccountBlockedMany(gomock.Any(), f.expectedCacheKeys(), true).
		Return(nil).
		Times(1)

	gomock.InOrder(find, sadd, update, propagate, list, cache)

	_, err := f.uc.BlockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)
	require.NoError(t, err)
}

// TestUnblockAccount_ClearsEnforcementIndexAfterSourceOfTruth pins the mirrored
// order: the durable state moves first, so a crash before the SREM leaves the
// account blocked in the index — an over-restriction the operator can retry
// away, never an unblock the source of truth does not back.
func TestUnblockAccount_ClearsEnforcementIndexAfterSourceOfTruth(t *testing.T) {
	t.Parallel()

	f := newBlockStateFixture(t)

	find := f.accountRepo.EXPECT().
		Find(gomock.Any(), f.organizationID, f.ledgerID, nil, f.accountID, mmodel.HolderOffV1).
		Return(f.accountWithBlocked(boolPtr(true)), nil).
		Times(1)

	update := f.accountRepo.EXPECT().
		Update(gomock.Any(), f.organizationID, f.ledgerID, nil, f.accountID, gomock.Any()).
		Return(f.accountWithBlocked(boolPtr(false)), nil).
		Times(1)

	srem := f.expectRemoveBlocked(nil)

	propagate := f.balanceRepo.EXPECT().
		UpdateAccountBlockedByAccountID(gomock.Any(), f.organizationID, f.ledgerID, f.accountID, false).
		Return(nil).
		Times(1)

	list := f.balanceRepo.EXPECT().
		ListByAccountID(gomock.Any(), f.organizationID, f.ledgerID, f.accountID).
		Return(f.balancesOfAccount(), nil).
		Times(1)

	cache := f.redisRepo.EXPECT().
		SetAccountBlockedMany(gomock.Any(), f.expectedCacheKeys(), false).
		Return(nil).
		Times(1)

	gomock.InOrder(find, update, srem, propagate, list, cache)

	_, err := f.uc.UnblockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)
	require.NoError(t, err)
}

// TestBlockAccount_EnforcementIndexFailureStopsBeforeAnyWrite: if the index
// cannot be written, the block is not effective, so nothing else may happen —
// least of all a durable account.blocked that the hot path would not honour.
func TestBlockAccount_EnforcementIndexFailureStopsBeforeAnyWrite(t *testing.T) {
	t.Parallel()

	f := newBlockStateFixture(t)
	f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
	f.expectAddBlocked(errors.New("redis unavailable"))
	// No Update, no propagation, no cache write: gomock fails the test on any.

	got, err := f.uc.BlockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "redis unavailable")
	assert.Empty(t, f.emitter.Events(), "a block that never became effective must not be audited as one")
}

// TestBlockAccount_KeepsIndexEntryWhenSourceOfTruthFails is the deliberate
// fail-closed asymmetry: after the SADD the account is already denied, and a
// failure of the durable write MUST NOT roll it back. An over-restriction is
// recoverable by retry; a rollback would reopen the window.
func TestBlockAccount_KeepsIndexEntryWhenSourceOfTruthFails(t *testing.T) {
	t.Parallel()

	f := newBlockStateFixture(t)
	f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
	f.expectAddBlocked(nil)
	f.expectUpdate(true, errors.New("connection reset by peer"))
	// No RemoveBlockedAccount is armed: a compensating SREM here would fail OPEN.

	_, err := f.uc.BlockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection reset by peer")
}

// TestBlockAccount_KeepsIndexEntryWhenLegacyPropagationFails extends the same
// rule to the legacy projection kept in parallel during the strangling: a
// failure there is reported, and the SADD stays.
func TestBlockAccount_KeepsIndexEntryWhenLegacyPropagationFails(t *testing.T) {
	t.Parallel()

	f := newBlockStateFixture(t)
	f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
	f.expectAddBlocked(nil)
	f.expectUpdate(true, nil)
	f.expectPropagate(true, errors.New("propagation exploded"))

	_, err := f.uc.BlockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "propagation exploded")
	assert.Empty(t, f.emitter.Events())
}

// TestUnblockAccount_KeepsIndexEntryWhenSourceOfTruthFails: the durable write
// failed, so the block must remain enforced. Reaching the SREM anyway would lift
// enforcement for a state the database never accepted.
func TestUnblockAccount_KeepsIndexEntryWhenSourceOfTruthFails(t *testing.T) {
	t.Parallel()

	f := newBlockStateFixture(t)
	f.expectFind(f.accountWithBlocked(boolPtr(true)), nil)
	f.expectUpdate(false, errors.New("connection reset by peer"))
	// No RemoveBlockedAccount is armed: gomock fails the test if it is called.

	_, err := f.uc.UnblockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection reset by peer")
}

// TestUnblockAccount_ReportsIndexFailureWithoutPropagating: a residual entry in
// the index is safe (the account stays blocked), but the operator has to learn
// the unblock did not take effect, so the error is returned and the legacy
// propagation does not run on top of an inconsistent state.
func TestUnblockAccount_ReportsIndexFailureWithoutPropagating(t *testing.T) {
	t.Parallel()

	f := newBlockStateFixture(t)
	f.expectFind(f.accountWithBlocked(boolPtr(true)), nil)
	f.expectUpdate(false, nil)
	f.expectRemoveBlocked(errors.New("redis unavailable"))

	got, err := f.uc.UnblockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "redis unavailable")
	assert.Empty(t, f.emitter.Events(), "an unblock that did not take effect must not be audited as one")
}

// TestBlockAccount_IsIdempotentOnRepeatedCalls proves the retry path: the second
// block finds the account already blocked, skips the durable write, yet still
// re-asserts the index entry and re-propagates, so a half-finished first attempt
// converges.
func TestBlockAccount_IsIdempotentOnRepeatedCalls(t *testing.T) {
	t.Parallel()

	first := newBlockStateFixture(t)
	first.expectFind(first.accountWithBlocked(boolPtr(false)), nil)
	first.expectAddBlocked(nil)
	first.expectUpdate(true, nil)
	first.expectPropagate(true, nil)
	first.expectListBalances(first.balancesOfAccount(), nil)
	first.expectSetAccountBlocked(first.expectedCacheKeys(), true, nil)

	_, err := first.uc.BlockAccount(context.Background(), first.organizationID, first.ledgerID, first.accountID, mmodel.HolderOffV1)
	require.NoError(t, err)

	second := newBlockStateFixture(t)
	second.expectFind(second.accountWithBlocked(boolPtr(true)), nil)
	second.expectAddBlocked(nil) // SADD on an existing member is a no-op that still reports success.
	second.expectPropagate(true, nil)
	second.expectListBalances(second.balancesOfAccount(), nil)
	second.expectSetAccountBlocked(second.expectedCacheKeys(), true, nil)

	acc, err := second.uc.BlockAccount(context.Background(), second.organizationID, second.ledgerID, second.accountID, mmodel.HolderOffV1)
	require.NoError(t, err)
	require.NotNil(t, acc.Blocked)
	assert.True(t, *acc.Blocked)
}

// TestUnblockAccount_IsIdempotentOnRepeatedCalls mirrors the above for the
// unblock direction.
func TestUnblockAccount_IsIdempotentOnRepeatedCalls(t *testing.T) {
	t.Parallel()

	f := newBlockStateFixture(t)
	f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
	// Already unblocked: no durable write, but the index is still cleared so a
	// residue from a failed earlier unblock is swept.
	f.expectRemoveBlocked(nil)
	f.expectPropagate(false, nil)
	f.expectListBalances(f.balancesOfAccount(), nil)
	f.expectSetAccountBlocked(f.expectedCacheKeys(), false, nil)

	acc, err := f.uc.UnblockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)
	require.NoError(t, err)
	require.NotNil(t, acc.Blocked)
	assert.False(t, *acc.Blocked)
}

// TestBlockAccount_GuardRejectionNeverTouchesIndex keeps the 0074 external
// account guard where it is: before anything is written anywhere.
func TestBlockAccount_GuardRejectionNeverTouchesIndex(t *testing.T) {
	t.Parallel()

	f := newBlockStateFixture(t)
	f.expectFind(f.externalAccount(), nil)
	// No AddBlockedAccount is armed: the guard fires straight after the read.

	_, err := f.uc.BlockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "0074")
}
