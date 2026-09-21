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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// expectClosingVerified programs everything an eligible account answers before its
// closing is written, so a finalization case can start from a proven account.
func (m *closeAccountMocks) expectClosingVerified() {
	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.expectProtectionTaken()
	m.expectBalancesRead(closeEligibleBalance())
	m.expectRecoveryWalked()
	m.expectPersistenceProven()
	m.expectPendingQuery(false, nil)
}

// expectWriteIntentRecorded programs the marker phase this attempt records before
// issuing its closing write.
func (m *closeAccountMocks) expectWriteIntentRecorded() {
	m.redis.EXPECT().MarkAccountClosingWriteIssued(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(true, nil)
}

// TestCloseAccount_FinalizesInOrder covers AC-01 and AS-16: the cached balances go
// before the negative cache, and the closing marker leaves only after both, so no
// window exists in which the protection is gone and the cache still serves the
// account.
func TestCloseAccount_FinalizesInOrder(t *testing.T) {
	m := newCloseAccountMocks(t)
	m.expectClosingVerified()

	intent := m.redis.EXPECT().MarkAccountClosingWriteIssued(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(true, nil)

	write := m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return(closeInstant, nil).After(intent)

	evict := m.redis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).After(write)

	closed := m.redis.EXPECT().SetAccountClosedMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, closeInstant).
		Return(nil).After(evict)

	marker := m.redis.EXPECT().ReleaseAccountClosingAttempt(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(true, nil).After(closed)

	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(true, nil).After(marker)

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	require.NoError(t, err)
	assert.Equal(t, closeInstant, closedAt)
}

// TestCloseAccount_KeepsTheProtectionWhenTheWriteOutcomeIsUnknown covers AS-12: a
// lost answer does not prove the write failed, so the protection stays and no
// retry is issued.
func TestCloseAccount_KeepsTheProtectionWhenTheWriteOutcomeIsUnknown(t *testing.T) {
	m := newCloseAccountMocks(t)
	m.expectClosingVerified()
	m.expectWriteIntentRecorded()

	m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return(time.Time{}, context.DeadlineExceeded)

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_ReleasesTheProtectionWhenTheServerRefusedTheWrite covers AS-11
// on the other side of the same branch: an error the server answered with proves
// nothing landed, so the attempt gives its own protection back.
func TestCloseAccount_ReleasesTheProtectionWhenTheServerRefusedTheWrite(t *testing.T) {
	m := newCloseAccountMocks(t)
	m.expectClosingVerified()
	m.expectWriteIntentRecorded()

	refused := &pgconn.PgError{Code: "23505"}

	m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return(time.Time{}, refused)
	m.expectProtectionReleased()

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	require.ErrorIs(t, err, refused)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_FinishesFinalizationAfterCallerCancels covers Fix B: the
// PostgreSQL close write is authoritative once it lands, so a caller that gives
// up right after must not stop eviction and the closed marker from completing.
func TestCloseAccount_FinishesFinalizationAfterCallerCancels(t *testing.T) {
	m := newCloseAccountMocks(t)
	m.expectClosingVerified()
	m.expectWriteIntentRecorded()

	ctx, cancel := context.WithCancel(context.Background())

	m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (time.Time, error) {
			cancel()
			return closeInstant, nil
		})

	evict := m.redis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil)
	closed := m.redis.EXPECT().SetAccountClosedMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, closeInstant).
		Return(nil).After(evict)
	marker := m.redis.EXPECT().ReleaseAccountClosingAttempt(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(true, nil).After(closed)
	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(true, nil).After(marker)

	closedAt, err := m.uc.CloseAccount(ctx, closeOrgID, closeLedgerID, closeAccountID)

	require.NoError(t, err)
	assert.Equal(t, closeInstant, closedAt)
}

// TestCloseAccount_KeepsTheProtectionWhenAnEvictionFails covers AS-13: the account
// is closed and a blob a later load could still serve is left behind, so the
// marker and the ownership stay for the reconciliation to finish the same work.
func TestCloseAccount_KeepsTheProtectionWhenAnEvictionFails(t *testing.T) {
	m := newCloseAccountMocks(t)
	m.expectClosingVerified()
	m.expectWriteIntentRecorded()

	m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).Return(closeInstant, nil)
	m.redis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(errors.New("cache unavailable"))

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_KeepsTheProtectionWhenTheClosedMarkerFails covers AS-13 one step
// later: the eviction concluded but the negative cache did not, so the protection
// is not handed back on an unfinished finalization.
func TestCloseAccount_KeepsTheProtectionWhenTheClosedMarkerFails(t *testing.T) {
	m := newCloseAccountMocks(t)
	m.expectClosingVerified()
	m.expectWriteIntentRecorded()

	m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).Return(closeInstant, nil)
	m.redis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil)
	m.redis.EXPECT().SetAccountClosedMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, closeInstant).
		Return(errors.New("cache unavailable"))

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_KeepsTheOwnershipWhenTheMarkerRemovalIsLost covers AS-13 at the
// last step: the answer to the marker removal was lost, so the ownership is not
// released either — a half-finished finalization keeps its whole protection.
func TestCloseAccount_KeepsTheOwnershipWhenTheMarkerRemovalIsLost(t *testing.T) {
	m := newCloseAccountMocks(t)
	m.expectClosingVerified()
	m.expectWriteIntentRecorded()

	m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).Return(closeInstant, nil)
	m.redis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil)
	m.redis.EXPECT().SetAccountClosedMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, closeInstant).Return(nil)
	m.redis.EXPECT().ReleaseAccountClosingAttempt(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(false, errors.New("cache unavailable"))

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_RefusesWhenTheWriteIntentCannotBeRecorded covers AS-12 before
// the write exists: without the recorded intent a later reconciliation could not
// tell an unresolved write from an attempt that never wrote, so the closing does
// not proceed — and since nothing was written, the protection is given back.
func TestCloseAccount_RefusesWhenTheWriteIntentCannotBeRecorded(t *testing.T) {
	m := newCloseAccountMocks(t)
	m.expectClosingVerified()

	m.redis.EXPECT().MarkAccountClosingWriteIssued(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(false, errors.New("cache unavailable"))
	m.expectProtectionReleased()

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_RepeatKeepsTheRecordedInstant covers AC-11 and AC-13: a repeat
// over a confirmed closing is a conflict answered from the authoritative row, and
// it neither rewrites the instant nor touches a balance row.
func TestCloseAccount_RepeatKeepsTheRecordedInstant(t *testing.T) {
	m := newCloseAccountMocks(t)

	recorded := closeInstant
	m.expectAccountRead(closeAccountEntity("deposit", &recorded), nil)

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountAlreadyClosed)
	assert.True(t, closedAt.IsZero())
	assert.Equal(t, closeInstant, recorded)
}

// TestEvictAccountClosingBalances_DropsEveryVerifiedBalance pins that the eviction
// covers the whole verified list rather than the default balance alone, and that
// it stops at a failure instead of reporting a partial cleanup as complete.
func TestEvictAccountClosingBalances_DropsEveryVerifiedBalance(t *testing.T) {
	m := newCloseAccountMocks(t)

	additional := closeEligibleBalance()
	additional.ID = closeTransactionID.String()
	additional.Key = "savings"

	states := []accountClosingBalanceState{
		{Persisted: closeEligibleBalance()},
		{Persisted: additional},
	}

	attempt := &accountClosingAttempt{
		organizationID: closeOrgID,
		ledgerID:       closeLedgerID,
		accountID:      closeAccountID,
		token:          "attempt-token",
	}

	m.redis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(2)

	require.NoError(t, m.uc.evictAccountClosingBalances(context.Background(), attempt, states))
}
