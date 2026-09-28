// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

const reconcileToken = "attempt-token"

// reconcileScope is the account one discovered marker names.
var reconcileScope = txRedis.AccountProtectionScope{
	OrganizationID: closeOrgID,
	LedgerID:       closeLedgerID,
	AccountID:      closeAccountID,
}

// expectMarkerDiscovered programs a terminal page carrying the account under test,
// followed by the empty ownership walk that closes the pass.
func (m *closeAccountMocks) expectMarkerDiscovered() {
	m.redis.EXPECT().ScanAccountClosingMarkers(gomock.Any(), uint64(0), gomock.Any()).
		Return(txRedis.AccountProtectionScanPage{Scopes: []txRedis.AccountProtectionScope{reconcileScope}}, nil)
	m.redis.EXPECT().ScanAccountAdminOwnerships(gomock.Any(), uint64(0), gomock.Any()).
		Return(txRedis.AccountProtectionScanPage{}, nil)
}

// expectAttemptRead programs the attempt the discovered marker carries.
func (m *closeAccountMocks) expectAttemptRead(writeIssued bool) {
	m.redis.EXPECT().ReadAccountClosingAttempt(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return(txRedis.AccountClosingAttempt{Token: reconcileToken, WriteIssued: writeIssued}, true, nil)
}

// TestReconcileAccountClosings_FinishesAConfirmedClosing covers AS-13 and AS-19:
// the instant is recorded, so the pass runs the finalization the interrupted
// attempt owed — eviction, negative cache, marker — with the instant the database
// already holds and no write of its own.
func TestReconcileAccountClosings_FinishesAConfirmedClosing(t *testing.T) {
	m := newCloseAccountMocks(t)

	recorded := closeInstant

	m.expectMarkerDiscovered()
	m.expectAttemptRead(true)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), closeOrgID, closeLedgerID, []uuid.UUID{closeAccountID}).
		Return(map[uuid.UUID]*time.Time{closeAccountID: &recorded}, nil)

	m.balance.EXPECT().ListByAccountID(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return([]*mmodel.Balance{closeEligibleBalance()}, nil)

	m.redis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil)
	m.redis.EXPECT().SetAccountClosedMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, recorded).Return(nil)
	m.redis.EXPECT().ReleaseAccountClosingAttempt(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, reconcileToken).
		Return(true, nil)
	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, reconcileToken).
		Return(true, nil)

	stats := m.uc.ReconcileAccountClosings(context.Background())

	assert.Equal(t, 1, stats.Completed)
	assert.Zero(t, stats.Released)
	assert.Zero(t, stats.Retained)
}

// TestReconcileAccountClosings_KeepsAnUnresolvedWrite covers AS-12: the column
// reads NULL and the attempt had already issued its write, so the write may still
// land and the protection stays — never released because time passed.
func TestReconcileAccountClosings_KeepsAnUnresolvedWrite(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectMarkerDiscovered()
	m.expectAttemptRead(true)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), closeOrgID, closeLedgerID, []uuid.UUID{closeAccountID}).
		Return(map[uuid.UUID]*time.Time{closeAccountID: nil}, nil)

	stats := m.uc.ReconcileAccountClosings(context.Background())

	assert.Equal(t, 1, stats.Retained)
	assert.Zero(t, stats.Released)
	assert.Zero(t, stats.Completed)
}

// TestReconcileAccountClosings_ReleasesAnAbortedAttempt covers AS-19: an attempt
// that never issued its write cannot have closed anything, so its marker and its
// ownership are given back — conditionally on that exact phase, which is what also
// stops the owner from writing afterwards.
func TestReconcileAccountClosings_ReleasesAnAbortedAttempt(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectMarkerDiscovered()
	m.expectAttemptRead(false)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), closeOrgID, closeLedgerID, []uuid.UUID{closeAccountID}).
		Return(map[uuid.UUID]*time.Time{closeAccountID: nil}, nil)

	m.redis.EXPECT().ReleaseAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, reconcileToken).
		Return(true, nil)
	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, reconcileToken).
		Return(true, nil)

	stats := m.uc.ReconcileAccountClosings(context.Background())

	assert.Equal(t, 1, stats.Released)
	assert.Zero(t, stats.Retained)
}

// TestReconcileAccountClosings_ReleasesAMarkerWhoseAttemptNeverTookItsOwnership
// covers an attempt that stopped between its two protection writes: its marker is
// there, its ownership never was. The ownership release answers that it had
// nothing to remove, which is not a failure, and the marker is still given back.
func TestReconcileAccountClosings_ReleasesAMarkerWhoseAttemptNeverTookItsOwnership(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectMarkerDiscovered()
	m.expectAttemptRead(false)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), closeOrgID, closeLedgerID, []uuid.UUID{closeAccountID}).
		Return(map[uuid.UUID]*time.Time{closeAccountID: nil}, nil)

	gomock.InOrder(
		m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, reconcileToken).
			Return(false, nil),
		m.redis.EXPECT().ReleaseAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, reconcileToken).
			Return(true, nil),
	)

	stats := m.uc.ReconcileAccountClosings(context.Background())

	assert.Equal(t, 1, stats.Released)
	assert.Zero(t, stats.Retained)
}

// TestReconcileAccountClosings_KeepsAnAttemptThatMovedOn covers the same branch
// from the owner's side: the conditional removal did not apply because the owner
// advanced between the read and the release, so nothing was taken from it.
func TestReconcileAccountClosings_KeepsAnAttemptThatMovedOn(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectMarkerDiscovered()
	m.expectAttemptRead(false)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), closeOrgID, closeLedgerID, []uuid.UUID{closeAccountID}).
		Return(map[uuid.UUID]*time.Time{closeAccountID: nil}, nil)

	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, reconcileToken).
		Return(false, nil)
	m.redis.EXPECT().ReleaseAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, reconcileToken).
		Return(false, nil)

	stats := m.uc.ReconcileAccountClosings(context.Background())

	assert.Equal(t, 1, stats.Retained)
	assert.Zero(t, stats.Released)
}

// TestReconcileAccountClosings_KeepsTheMarkerWhenOwnershipReleaseFails covers Fix
// D: the ownership carries no TTL, so if it cannot be released the closing marker
// stays as the retry anchor — the pass reports Retained, never Completed, and never
// removes the marker.
func TestReconcileAccountClosings_KeepsTheMarkerWhenOwnershipReleaseFails(t *testing.T) {
	m := newCloseAccountMocks(t)

	recorded := closeInstant

	m.expectMarkerDiscovered()
	m.expectAttemptRead(true)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), closeOrgID, closeLedgerID, []uuid.UUID{closeAccountID}).
		Return(map[uuid.UUID]*time.Time{closeAccountID: &recorded}, nil)
	m.balance.EXPECT().ListByAccountID(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return([]*mmodel.Balance{closeEligibleBalance()}, nil)
	m.redis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil)
	m.redis.EXPECT().SetAccountClosedMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, recorded).Return(nil)
	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, reconcileToken).
		Return(false, errors.New("ownership store unavailable"))

	stats := m.uc.ReconcileAccountClosings(context.Background())

	assert.Equal(t, 1, stats.Retained)
	assert.Zero(t, stats.Completed)
}

// TestReconcileAccountClosings_KeepsTheMarkerWhenAnAbortedOwnershipReleaseFails
// covers Fix D on the abort path: the same no-TTL ownership risk applies before an
// attempt ever issued its write, so the marker also stays there.
func TestReconcileAccountClosings_KeepsTheMarkerWhenAnAbortedOwnershipReleaseFails(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectMarkerDiscovered()
	m.expectAttemptRead(false)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), closeOrgID, closeLedgerID, []uuid.UUID{closeAccountID}).
		Return(map[uuid.UUID]*time.Time{closeAccountID: nil}, nil)
	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, reconcileToken).
		Return(false, errors.New("ownership store unavailable"))

	stats := m.uc.ReconcileAccountClosings(context.Background())

	assert.Equal(t, 1, stats.Retained)
	assert.Zero(t, stats.Released)
}

// TestReconcileAccountClosings_RetriesAFailedEviction covers AS-13: an eviction
// that failed leaves a blob a load could still serve, so the protection stays and
// the next pass resumes the same finalization rather than a movement slipping
// through a half-finished one.
func TestReconcileAccountClosings_RetriesAFailedEviction(t *testing.T) {
	m := newCloseAccountMocks(t)

	recorded := closeInstant

	m.expectMarkerDiscovered()
	m.expectAttemptRead(true)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), closeOrgID, closeLedgerID, []uuid.UUID{closeAccountID}).
		Return(map[uuid.UUID]*time.Time{closeAccountID: &recorded}, nil)
	m.balance.EXPECT().ListByAccountID(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return([]*mmodel.Balance{closeEligibleBalance()}, nil)
	m.redis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(errors.New("cache unavailable"))

	stats := m.uc.ReconcileAccountClosings(context.Background())

	assert.Equal(t, 1, stats.Retained)
	assert.Zero(t, stats.Completed)
}

// TestReconcileAccountClosings_KeepsAnIndeterminateAuthoritativeRead covers AS-10:
// a state that could not be read is not an open account, so nothing is released on
// the strength of a failed query.
func TestReconcileAccountClosings_KeepsAnIndeterminateAuthoritativeRead(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectMarkerDiscovered()
	m.expectAttemptRead(false)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), closeOrgID, closeLedgerID, []uuid.UUID{closeAccountID}).
		Return(nil, errors.New("database unavailable"))

	stats := m.uc.ReconcileAccountClosings(context.Background())

	assert.Equal(t, 1, stats.Retained)
	assert.Zero(t, stats.Released)
}

// TestReconcileAccountClosings_WalksEveryPage pins that one non-terminal page does
// not end the walk: only a terminal cursor does, so a marker in a later page is
// still reconciled.
func TestReconcileAccountClosings_WalksEveryPage(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.redis.EXPECT().ScanAccountClosingMarkers(gomock.Any(), uint64(0), gomock.Any()).
		Return(txRedis.AccountProtectionScanPage{Cursor: 7}, nil)
	m.redis.EXPECT().ScanAccountClosingMarkers(gomock.Any(), uint64(7), gomock.Any()).
		Return(txRedis.AccountProtectionScanPage{Scopes: []txRedis.AccountProtectionScope{reconcileScope}, Unreadable: 1}, nil)
	m.redis.EXPECT().ScanAccountAdminOwnerships(gomock.Any(), uint64(0), gomock.Any()).
		Return(txRedis.AccountProtectionScanPage{Scopes: []txRedis.AccountProtectionScope{reconcileScope}}, nil)

	m.expectAttemptRead(true)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), closeOrgID, closeLedgerID, []uuid.UUID{closeAccountID}).
		Return(map[uuid.UUID]*time.Time{closeAccountID: nil}, nil)

	stats := m.uc.ReconcileAccountClosings(context.Background())

	assert.Equal(t, 1, stats.Scanned)
	assert.Equal(t, 1, stats.Unreadable)
	assert.Equal(t, 1, stats.Ownerships)
}

// TestReconcileAccountClosings_IgnoresAMarkerThatIsAlreadyGone pins that a marker
// removed between the scan and the read is nothing to resolve, which is exactly
// what an attempt finishing its own work looks like.
func TestReconcileAccountClosings_IgnoresAMarkerThatIsAlreadyGone(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectMarkerDiscovered()
	m.redis.EXPECT().ReadAccountClosingAttempt(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return(txRedis.AccountClosingAttempt{}, false, nil)

	stats := m.uc.ReconcileAccountClosings(context.Background())

	assert.Equal(t, 1, stats.Scanned)
	assert.Zero(t, stats.Retained)
	assert.Zero(t, stats.Released)
	assert.Zero(t, stats.Completed)
}

// TestReconcileAccountClosings_StopsWithoutAProtectionSurface pins that a
// deployment without the cache or the account repository reconciles nothing rather
// than scanning a surface it does not have.
func TestReconcileAccountClosings_StopsWithoutAProtectionSurface(t *testing.T) {
	uc := &UseCase{}

	stats := uc.ReconcileAccountClosings(context.Background())

	require.Equal(t, AccountClosingReconciliationStats{}, stats)
}
