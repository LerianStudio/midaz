// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// The recovery fixtures are written for the scope the completion contract fixture
// uses, so an engine envelope and a legacy record describe the same account.
var (
	recoveryScopeOrgID     = uuid.MustParse("33333333-3333-4333-8333-333333333333")
	recoveryScopeLedgerID  = uuid.MustParse("44444444-4444-4444-8444-444444444444")
	recoveryScopeAccountID = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	recoveryOtherAccountID = uuid.MustParse("99999999-9999-4999-8999-999999999999")
)

type closingRecoveryMocks struct {
	uc    *UseCase
	redis *txRedis.MockRedisRepository
}

func newClosingRecoveryMocks(t *testing.T) *closingRecoveryMocks {
	t.Helper()

	ctrl := gomock.NewController(t)

	mocks := &closingRecoveryMocks{redis: txRedis.NewMockRedisRepository(ctrl)}
	mocks.uc = &UseCase{TransactionRedisRepo: mocks.redis}

	return mocks
}

// closingLegacyRecoveryRecord builds a legacy backup record naming one account.
func closingLegacyRecoveryRecord(t *testing.T, accountID uuid.UUID) string {
	t.Helper()

	record := mmodel.TransactionRedisQueue{
		TransactionID:  uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		OrganizationID: recoveryScopeOrgID,
		LedgerID:       recoveryScopeLedgerID,
		Action:         constant.ActionDirect,
		Balances:       []mmodel.BalanceRedis{{ID: "balance-1", AccountID: accountID.String(), Key: constant.DefaultBalanceKey}},
	}

	raw, err := json.Marshal(record)
	require.NoError(t, err)

	return string(raw)
}

func closingWriteBehindRecoveryEnvelope(t *testing.T, action string) string {
	t.Helper()

	record, err := DecodeTransactionCompletionRecord(recoveryInventoryEnvelope(t, action))
	require.NoError(t, err)

	raw, err := json.Marshal(TransactionWriteBehindEnvelope{
		FormatVersion:    TransactionWriteBehindFormatVersion,
		ApplicationState: TransactionApplicationConfirmed,
		ReplayState:      TransactionReplayReconstructible,
		DurabilityState:  TransactionDurabilityPending,
		Record:           *record,
		Dependencies:     []TransactionEvidenceReference{},
	})
	require.NoError(t, err)

	return string(raw)
}

// closingPage builds one scan page of one origin.
func closingPage(source txRedis.RecoveryQueueSource, cursor uint64, payloads ...string) txRedis.RecoveryScanPage {
	page := txRedis.RecoveryScanPage{Source: source, Cursor: cursor}

	for index, payload := range payloads {
		page.Records = append(page.Records, txRedis.RecoveryScanRecord{Field: uuid.NewString(), Payload: payload})
		page.Bytes += len(payload) + len(page.Records[index].Field)
	}

	return page
}

// expectTerminalScan programs one origin answering a single terminal page.
func (m *closingRecoveryMocks) expectTerminalScan(source txRedis.RecoveryQueueSource, payloads ...string) {
	m.redis.EXPECT().ScanRecoveryMessages(gomock.Any(), source, uint64(0), gomock.Any()).
		Return(closingPage(source, 0, payloads...), nil)
}

// TestAccountClosingRecoveryRecordAttributionReadsBothPersistedFormats proves the
// filter reads the records as they are persisted — the versioned engine envelope
// and the legacy backup record — and answers strictly on the full scope.
func TestAccountClosingRecoveryRecordAttributionReadsBothPersistedFormats(t *testing.T) {
	engineRecord := string(recoveryInventoryEnvelope(t, constant.ActionDirect))
	writeBehindRecord := closingWriteBehindRecoveryEnvelope(t, constant.ActionDirect)

	tests := []struct {
		name        string
		raw         string
		allowLegacy bool
		accountID   uuid.UUID
		ledgerID    uuid.UUID
		touches     bool
		fails       bool
	}{
		{name: "write-behind envelope naming the account", raw: writeBehindRecord, accountID: recoveryScopeAccountID, ledgerID: recoveryScopeLedgerID, touches: true},
		{name: "write-behind envelope naming another account", raw: writeBehindRecord, accountID: recoveryOtherAccountID, ledgerID: recoveryScopeLedgerID},
		{name: "write-behind envelope of another ledger", raw: writeBehindRecord, accountID: recoveryScopeAccountID, ledgerID: recoveryOtherAccountID},
		{name: "bare engine record naming the account", raw: engineRecord, accountID: recoveryScopeAccountID, ledgerID: recoveryScopeLedgerID, touches: true},
		{
			name: "legacy record naming the account", raw: closingLegacyRecoveryRecord(t, recoveryScopeAccountID),
			allowLegacy: true, accountID: recoveryScopeAccountID, ledgerID: recoveryScopeLedgerID, touches: true,
		},
		{
			name: "legacy record naming another account", raw: closingLegacyRecoveryRecord(t, recoveryOtherAccountID),
			allowLegacy: true, accountID: recoveryScopeAccountID, ledgerID: recoveryScopeLedgerID,
		},
		{
			name: "legacy record where only version 2 is persisted", raw: closingLegacyRecoveryRecord(t, recoveryScopeAccountID),
			accountID: recoveryScopeAccountID, ledgerID: recoveryScopeLedgerID, fails: true,
		},
		{name: "a record that is not an object", raw: `"payload"`, allowLegacy: true, accountID: recoveryScopeAccountID, ledgerID: recoveryScopeLedgerID, fails: true},
		{name: "a versioned record that does not decode", raw: `{"formatVersion":2}`, accountID: recoveryScopeAccountID, ledgerID: recoveryScopeLedgerID, fails: true},
		{
			name: "a legacy record with no identity", raw: `{"transaction_id":"00000000-0000-0000-0000-000000000000"}`,
			allowLegacy: true, accountID: recoveryScopeAccountID, ledgerID: recoveryScopeLedgerID, fails: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			touches, err := recoveryRecordTouchesAccount([]byte(test.raw), test.allowLegacy, recoveryScopeOrgID, test.ledgerID, test.accountID)

			if test.fails {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.touches, touches)
		})
	}
}

// TestAccountClosingRecoveryWalksEveryCursorBeforeConcludingAbsence covers AS-08
// and the cursor rule of D4: an empty page is not absence, so the walk of each
// origin continues until that origin returns its terminal cursor.
func TestAccountClosingRecoveryWalksEveryCursorBeforeConcludingAbsence(t *testing.T) {
	m := newClosingRecoveryMocks(t)

	unrelated := closingLegacyRecoveryRecord(t, recoveryOtherAccountID)

	gomock.InOrder(
		m.redis.EXPECT().ScanRecoveryMessages(gomock.Any(), txRedis.RecoveryQueueSourceLegacyBackup, uint64(0), gomock.Any()).
			Return(closingPage(txRedis.RecoveryQueueSourceLegacyBackup, 17), nil),
		m.redis.EXPECT().ScanRecoveryMessages(gomock.Any(), txRedis.RecoveryQueueSourceLegacyBackup, uint64(17), gomock.Any()).
			Return(closingPage(txRedis.RecoveryQueueSourceLegacyBackup, 0, unrelated), nil),
	)

	m.expectTerminalScan(txRedis.RecoveryQueueSourceEngineRecover)

	require.NoError(t, m.uc.verifyNoAccountClosingRecoveryPending(context.Background(), recoveryScopeOrgID, recoveryScopeLedgerID, recoveryScopeAccountID))
}

// TestAccountClosingRecoveryRefusesWhileACompletionIsStillOwed covers AS-08: an
// execution of this account still carried by the engine recovery hash refuses the
// closing temporarily, without the record being read as anything but evidence.
func TestAccountClosingRecoveryRefusesWhileACompletionIsStillOwed(t *testing.T) {
	m := newClosingRecoveryMocks(t)

	m.expectTerminalScan(txRedis.RecoveryQueueSourceLegacyBackup)
	m.expectTerminalScan(txRedis.RecoveryQueueSourceEngineRecover, string(recoveryInventoryEnvelope(t, constant.ActionCommit)))

	err := m.uc.verifyNoAccountClosingRecoveryPending(context.Background(), recoveryScopeOrgID, recoveryScopeLedgerID, recoveryScopeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingPersistencePending)
}

// TestAccountClosingRecoveryTreatsAnAcknowledgedRecordAsGone proves a record
// acknowledged while the walk is in flight — it simply stops being returned —
// leaves the walk able to conclude, which is what lets a retry close once the
// completer finished.
func TestAccountClosingRecoveryTreatsAnAcknowledgedRecordAsGone(t *testing.T) {
	m := newClosingRecoveryMocks(t)

	gomock.InOrder(
		m.redis.EXPECT().ScanRecoveryMessages(gomock.Any(), txRedis.RecoveryQueueSourceLegacyBackup, uint64(0), gomock.Any()).
			Return(closingPage(txRedis.RecoveryQueueSourceLegacyBackup, 5), nil),
		m.redis.EXPECT().ScanRecoveryMessages(gomock.Any(), txRedis.RecoveryQueueSourceLegacyBackup, uint64(5), gomock.Any()).
			Return(closingPage(txRedis.RecoveryQueueSourceLegacyBackup, 0), nil),
	)

	m.expectTerminalScan(txRedis.RecoveryQueueSourceEngineRecover)

	require.NoError(t, m.uc.verifyNoAccountClosingRecoveryPending(context.Background(), recoveryScopeOrgID, recoveryScopeLedgerID, recoveryScopeAccountID))
}

// TestAccountClosingRecoveryRefusesUnprovenAbsence covers AS-10: a scan that fails
// and a record that cannot be read both leave absence unproven, and unproven
// absence is refused technically instead of being read as "nothing pending".
func TestAccountClosingRecoveryRefusesUnprovenAbsence(t *testing.T) {
	t.Run("the scan fails", func(t *testing.T) {
		m := newClosingRecoveryMocks(t)

		m.redis.EXPECT().ScanRecoveryMessages(gomock.Any(), txRedis.RecoveryQueueSourceLegacyBackup, uint64(0), gomock.Any()).
			Return(txRedis.RecoveryScanPage{}, errors.New("cache unavailable"))

		err := m.uc.verifyNoAccountClosingRecoveryPending(context.Background(), recoveryScopeOrgID, recoveryScopeLedgerID, recoveryScopeAccountID)

		requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	})

	t.Run("a record cannot be read", func(t *testing.T) {
		m := newClosingRecoveryMocks(t)

		m.expectTerminalScan(txRedis.RecoveryQueueSourceLegacyBackup, `{"formatVersion":2,"payload":`)

		err := m.uc.verifyNoAccountClosingRecoveryPending(context.Background(), recoveryScopeOrgID, recoveryScopeLedgerID, recoveryScopeAccountID)

		requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	})

	t.Run("the byte budget runs out first", func(t *testing.T) {
		m := newClosingRecoveryMocks(t)

		oversized := closingPage(txRedis.RecoveryQueueSourceLegacyBackup, 3)
		oversized.Bytes = maxAccountClosingRecoveryBytes + 1

		m.redis.EXPECT().ScanRecoveryMessages(gomock.Any(), txRedis.RecoveryQueueSourceLegacyBackup, uint64(0), gomock.Any()).
			Return(oversized, nil)

		err := m.uc.verifyNoAccountClosingRecoveryPending(context.Background(), recoveryScopeOrgID, recoveryScopeLedgerID, recoveryScopeAccountID)

		requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	})

	t.Run("a conclusive record on the budget-breaking page still decides", func(t *testing.T) {
		m := newClosingRecoveryMocks(t)

		oversized := closingPage(txRedis.RecoveryQueueSourceLegacyBackup, 3, closingLegacyRecoveryRecord(t, recoveryScopeAccountID))
		oversized.Bytes = maxAccountClosingRecoveryBytes + 1

		m.redis.EXPECT().ScanRecoveryMessages(gomock.Any(), txRedis.RecoveryQueueSourceLegacyBackup, uint64(0), gomock.Any()).
			Return(oversized, nil)

		err := m.uc.verifyNoAccountClosingRecoveryPending(context.Background(), recoveryScopeOrgID, recoveryScopeLedgerID, recoveryScopeAccountID)

		requireClosingCode(t, err, constant.ErrAccountClosingPersistencePending)
	})
}

// TestAccountClosingRecoveryIsolatesTheScopeOfEachOrigin covers AS-14: a record of
// another organization is not this account's evidence, whichever hash carries it.
func TestAccountClosingRecoveryIsolatesTheScopeOfEachOrigin(t *testing.T) {
	m := newClosingRecoveryMocks(t)

	foreign := strings.ReplaceAll(closingLegacyRecoveryRecord(t, recoveryScopeAccountID),
		recoveryScopeOrgID.String(), recoveryOtherAccountID.String())

	m.expectTerminalScan(txRedis.RecoveryQueueSourceLegacyBackup, foreign)
	m.expectTerminalScan(txRedis.RecoveryQueueSourceEngineRecover)

	require.NoError(t, m.uc.verifyNoAccountClosingRecoveryPending(context.Background(), recoveryScopeOrgID, recoveryScopeLedgerID, recoveryScopeAccountID))
}
