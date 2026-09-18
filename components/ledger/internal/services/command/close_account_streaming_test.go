// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// withEmitter attaches a capturing emitter to the use case under test and returns
// it, so a test body asserts on what the closing published without repeating the
// wiring.
func (m *closeAccountMocks) withEmitter() *pkgStreaming.MockEmitter {
	emitter := pkgStreaming.NewMockEmitter()
	m.uc.Streaming = emitter

	return emitter
}

// programEligibleClosing programs every step of a closing that finishes: the
// account read, the protection, the verification and the finalization of the
// confirmed write.
func (m *closeAccountMocks) programEligibleClosing() {
	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.expectProtectionTaken()
	m.expectBalancesRead(closeEligibleBalance())
	m.expectRecoveryWalked()
	m.expectPersistenceProven()
	m.expectPendingQuery(false, nil)

	m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).Return(closeInstant, nil)
	m.expectClosingFinalized(closeInstant, 1)
}

// TestCloseAccount_EmitsAccountClosedAfterFinalization covers AC-14: a closing
// that finished publishes exactly one account.closed, carrying the scope, the
// account and the instant the database recorded.
func TestCloseAccount_EmitsAccountClosedAfterFinalization(t *testing.T) {
	m := newCloseAccountMocks(t)
	emitter := m.withEmitter()

	m.programEligibleClosing()

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	require.NoError(t, err)
	assert.Equal(t, closeInstant, closedAt)

	emitted := emitter.Events()
	require.Len(t, emitted, 1, "a finished closing publishes exactly one event")

	pkgStreaming.AssertEventEmitted(t, emitter, "account", "closed")

	event := emitted[0]

	assert.Equal(t, "account.closed", event.DefinitionKey)
	assert.Equal(t, pkgStreaming.DefaultTenantID, event.TenantID)
	assert.Equal(t, closeAccountID.String(), event.Subject, "the subject is the account that closed")
	assert.Equal(t, closeInstant, event.Timestamp, "the timestamp is the persisted instant, never a local clock")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(event.Payload, &payload))

	assert.Equal(t, map[string]any{
		"id":             closeAccountID.String(),
		"organizationId": closeOrgID.String(),
		"ledgerId":       closeLedgerID.String(),
		"closedAt":       closeInstant.Format(time.RFC3339),
	}, payload)
}

// TestCloseAccount_EmitsTheSameRequestForTheSameClosing covers AC-16 on the side
// midaz owns: the request a closing hands the producer is a function of the
// closing alone — same key, subject, timestamp and payload — so a transport that
// repeats it repeats one event rather than describing a second closing. The
// ce-id that deduplication reads is stamped by lib-streaming on that one request
// and is preserved across its own retries.
func TestCloseAccount_EmitsTheSameRequestForTheSameClosing(t *testing.T) {
	first := newCloseAccountMocks(t)
	firstEmitter := first.withEmitter()
	first.programEligibleClosing()

	_, err := first.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)
	require.NoError(t, err)

	second := newCloseAccountMocks(t)
	secondEmitter := second.withEmitter()
	second.programEligibleClosing()

	_, err = second.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)
	require.NoError(t, err)

	firstEvents := firstEmitter.Events()
	secondEvents := secondEmitter.Events()

	require.Len(t, firstEvents, 1)
	require.Len(t, secondEvents, 1)

	assert.Equal(t, firstEvents[0], secondEvents[0], "the same closing builds the same request")
}

// TestCloseAccount_KeepsSuccessWhenThePublicationFails covers AC-15: the broker is
// unreachable after the closing finished, so the failure is recorded and the
// caller still gets the closing instant — a publication never undoes a transition.
func TestCloseAccount_KeepsSuccessWhenThePublicationFails(t *testing.T) {
	m := newCloseAccountMocks(t)
	emitter := m.withEmitter()
	emitter.SetError(errors.New("broker unavailable"))

	m.programEligibleClosing()

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	require.NoError(t, err, "a failed publication must not convert success into an error")
	assert.Equal(t, closeInstant, closedAt)
	assert.Empty(t, emitter.Events(), "a failed publication captures nothing")
}

// TestCloseAccount_ClosesWithoutAnEmitterConfigured pins the disabled-streaming
// deployment: a nil emitter is not a failure of the closing.
func TestCloseAccount_ClosesWithoutAnEmitterConfigured(t *testing.T) {
	m := newCloseAccountMocks(t)

	require.Nil(t, m.uc.Streaming)

	m.programEligibleClosing()

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	require.NoError(t, err)
	assert.Equal(t, closeInstant, closedAt)
}

// TestCloseAccount_PublishesNothingOnARepeat covers AC-11 and AC-16: the second
// command over a closed account is refused before anything runs, so repeating the
// command never starts another emission.
func TestCloseAccount_PublishesNothingOnARepeat(t *testing.T) {
	m := newCloseAccountMocks(t)
	emitter := m.withEmitter()

	recorded := closeInstant
	m.expectAccountRead(closeAccountEntity("deposit", &recorded), nil)

	_, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountAlreadyClosed)
	assert.Empty(t, emitter.Events(), "a refused repeat publishes nothing")
}

// TestCloseAccount_PublishesNothingOnARepeatResolvedByTheWrite covers the same
// repeat arriving one step later: the conditional write matched no row and the
// authoritative row named it a conflict, which is still not a closing of its own.
func TestCloseAccount_PublishesNothingOnARepeatResolvedByTheWrite(t *testing.T) {
	m := newCloseAccountMocks(t)
	emitter := m.withEmitter()

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.expectProtectionTaken()
	m.expectBalancesRead(closeEligibleBalance())
	m.expectRecoveryWalked()
	m.expectPersistenceProven()
	m.expectPendingQuery(false, nil)

	recorded := closeInstant

	m.redis.EXPECT().MarkAccountClosingWriteIssued(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(true, nil)
	m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return(time.Time{}, account.ErrAccountCloseNotApplied)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), closeOrgID, closeLedgerID, []uuid.UUID{closeAccountID}).
		Return(map[uuid.UUID]*time.Time{closeAccountID: &recorded}, nil)

	m.expectProtectionReleased()

	_, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountAlreadyClosed)
	assert.Empty(t, emitter.Events())
}

// TestCloseAccount_PublishesNothingOnARefusedClosing covers the refusal classes
// that answer before the write: a residual balance and a pending transaction both
// leave the account open, so neither announces a closing.
func TestCloseAccount_PublishesNothingOnARefusedClosing(t *testing.T) {
	t.Run("residual balance", func(t *testing.T) {
		m := newCloseAccountMocks(t)
		emitter := m.withEmitter()

		residual := closeEligibleBalance()
		residual.Available = decimal.RequireFromString("0.01")

		m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
		m.expectProtectionTaken()
		m.expectBalancesRead(residual)
		m.expectProtectionReleased()

		_, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

		requireClosingCode(t, err, constant.ErrAccountBalanceNotZero)
		assert.Empty(t, emitter.Events())
	})

	t.Run("pending transaction", func(t *testing.T) {
		m := newCloseAccountMocks(t)
		emitter := m.withEmitter()

		m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
		m.expectProtectionTaken()
		m.expectBalancesRead(closeEligibleBalance())
		m.expectRecoveryWalked()
		m.expectPersistenceProven()
		m.expectPendingQuery(true, nil)
		m.expectProtectionReleased()

		_, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

		requireClosingCode(t, err, constant.ErrAccountHasPendingTransactions)
		assert.Empty(t, emitter.Events())
	})

	t.Run("persistence in progress", func(t *testing.T) {
		m := newCloseAccountMocks(t)
		emitter := m.withEmitter()

		m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
		m.expectProtectionTaken()
		m.expectBalancesRead(closeEligibleBalance())

		m.redis.EXPECT().ScanRecoveryMessages(gomock.Any(), txRedis.RecoveryQueueSourceLegacyBackup, uint64(0), gomock.Any()).
			Return(txRedis.RecoveryScanPage{
				Source:  txRedis.RecoveryQueueSourceLegacyBackup,
				Cursor:  0,
				Records: []txRedis.RecoveryScanRecord{{Field: "tx", Payload: legacyRecoveryPayloadForClose()}},
			}, nil)

		m.expectProtectionReleased()

		_, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

		requireClosingCode(t, err, constant.ErrAccountClosingPersistencePending)
		assert.Empty(t, emitter.Events())
	})
}

// TestCloseAccount_PublishesNothingWhenTheWriteOutcomeIsUnknown pins the boundary
// the event must never cross: the closing write left no answer, so the protection
// stays and nothing is announced about a transition nobody can confirm.
func TestCloseAccount_PublishesNothingWhenTheWriteOutcomeIsUnknown(t *testing.T) {
	m := newCloseAccountMocks(t)
	emitter := m.withEmitter()

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.expectProtectionTaken()
	m.expectBalancesRead(closeEligibleBalance())
	m.expectRecoveryWalked()
	m.expectPersistenceProven()
	m.expectPendingQuery(false, nil)

	m.redis.EXPECT().MarkAccountClosingWriteIssued(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(true, nil)
	m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return(time.Time{}, context.DeadlineExceeded)

	_, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	assert.Empty(t, emitter.Events(), "an unresolved write announces nothing")
}

// TestCloseAccount_PublishesNothingWhenTheFinalizationDoesNotFinish pins the other
// half of that boundary: the instant is recorded but the cache still holds state
// the account may no longer serve, so the closing is not finished and the event
// belongs to the command that finishes it — never to this one.
func TestCloseAccount_PublishesNothingWhenTheFinalizationDoesNotFinish(t *testing.T) {
	m := newCloseAccountMocks(t)
	emitter := m.withEmitter()

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.expectProtectionTaken()
	m.expectBalancesRead(closeEligibleBalance())
	m.expectRecoveryWalked()
	m.expectPersistenceProven()
	m.expectPendingQuery(false, nil)

	m.redis.EXPECT().MarkAccountClosingWriteIssued(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(true, nil)
	m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).Return(closeInstant, nil)
	m.redis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(errors.New("cache unavailable"))

	_, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	assert.Empty(t, emitter.Events())
}

// TestReconcileAccountClosings_PublishesNothing pins the deliberate gap of D6: the
// reconciliation finishes a closing another attempt left interrupted, and it does
// so without an event. There is no outbox behind account.closed, so a process that
// died after the write loses its event rather than having a later pass invent one
// — the closing state stays queryable, which is what consumers fall back on.
func TestReconcileAccountClosings_PublishesNothing(t *testing.T) {
	m := newCloseAccountMocks(t)
	emitter := m.withEmitter()

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

	require.Equal(t, 1, stats.Completed)
	assert.Empty(t, emitter.Events(), "reconciliation finishes a closing without announcing it")
}
