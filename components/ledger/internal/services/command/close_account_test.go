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
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	midazpkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

var (
	closeOrgID     = uuid.MustParse("dddddddd-0000-0000-0000-000000000001")
	closeLedgerID  = uuid.MustParse("dddddddd-0000-0000-0000-000000000002")
	closeAccountID = uuid.MustParse("dddddddd-0000-0000-0000-000000000003")
	closeBalanceID = uuid.MustParse("dddddddd-0000-0000-0000-000000000004")

	closeTransactionID = uuid.MustParse("dddddddd-0000-0000-0000-000000000005")

	closeInstant = time.Date(2026, 9, 18, 10, 30, 0, 0, time.UTC)
)

type closeAccountMocks struct {
	t           *testing.T
	uc          *UseCase
	account     *account.MockRepository
	balance     *balance.MockRepository
	operation   *operation.MockRepository
	transaction *transaction.MockRepository
	redis       *txRedis.MockRedisRepository

	// token is the attempt token the closing installed its marker with, captured
	// when the marker is programmed so every later step can be matched against it.
	token string
}

func newCloseAccountMocks(t *testing.T) *closeAccountMocks {
	t.Helper()

	ctrl := gomock.NewController(t)

	mocks := &closeAccountMocks{
		t:           t,
		account:     account.NewMockRepository(ctrl),
		balance:     balance.NewMockRepository(ctrl),
		operation:   operation.NewMockRepository(ctrl),
		transaction: transaction.NewMockRepository(ctrl),
		redis:       txRedis.NewMockRedisRepository(ctrl),
	}

	mocks.uc = &UseCase{
		AccountRepo:          mocks.account,
		BalanceRepo:          mocks.balance,
		OperationRepo:        mocks.operation,
		TransactionRepo:      mocks.transaction,
		TransactionRedisRepo: mocks.redis,
	}

	return mocks
}

// closeAccountEntity builds the account row the closing reads.
func closeAccountEntity(accountType string, closedAt *time.Time) *mmodel.Account {
	return &mmodel.Account{
		ID:             closeAccountID.String(),
		OrganizationID: closeOrgID.String(),
		LedgerID:       closeLedgerID.String(),
		Type:           accountType,
		ClosedAt:       closedAt,
	}
}

// expectAccountRead programs the authoritative read of the account under test.
func (m *closeAccountMocks) expectAccountRead(acc *mmodel.Account, err error) {
	m.account.EXPECT().Find(gomock.Any(), closeOrgID, closeLedgerID, nil, closeAccountID, mmodel.HolderOffV1).
		Return(acc, err)
}

// expectMarkerInstalled programs the installation of the closing marker, the
// first protection write of an attempt, and captures the token it carries.
func (m *closeAccountMocks) expectMarkerInstalled() *gomock.Call {
	return m.redis.EXPECT().AcquireAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, token string) (bool, error) {
			m.token = token

			return true, nil
		})
}

// expectOwnMarkerRead programs the read through which the attempt recognizes the
// marker it just installed.
func (m *closeAccountMocks) expectOwnMarkerRead() *gomock.Call {
	return m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (string, bool, error) {
			return m.token, true, nil
		})
}

// attemptToken matches the token the attempt installed its marker with.
func (m *closeAccountMocks) attemptToken() gomock.Matcher {
	return gomock.Cond(func(token string) bool { return token != "" && token == m.token })
}

// expectProtectionTaken programs the protection of an attempt in its order: the
// closing marker first, then the attempt's own marker recognized, then the
// ownership under the same token.
func (m *closeAccountMocks) expectProtectionTaken() {
	gomock.InOrder(
		m.expectMarkerInstalled(),
		m.expectOwnMarkerRead(),
		m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, m.attemptToken()).
			Return(true, nil),
	)
}

// expectProtectionReleased programs the cleanup of a resolved attempt: the
// ownership leaves before the closing marker, so the attempt never holds an
// ownership without its marker, and both are conditional on the attempt's token.
func (m *closeAccountMocks) expectProtectionReleased() {
	gomock.InOrder(
		m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, m.attemptToken()).
			Return(true, nil),
		m.redis.EXPECT().ReleaseAccountClosingAttempt(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, m.attemptToken()).
			Return(true, nil),
	)
}

// expectClosingFinalized programs the finalization of a confirmed closing: the
// write intent, the eviction of every verified balance blob, the negative cache,
// then the ownership and last the closing marker.
func (m *closeAccountMocks) expectClosingFinalized(closedAt time.Time, evictions int) {
	m.redis.EXPECT().MarkAccountClosingWriteIssued(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, m.attemptToken()).
		Return(true, nil)
	m.redis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(evictions)

	gomock.InOrder(
		m.redis.EXPECT().SetAccountClosedMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, closedAt).Return(nil),
		m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, m.attemptToken()).
			Return(true, nil),
		m.redis.EXPECT().ReleaseAccountClosingAttempt(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, m.attemptToken()).
			Return(true, nil),
	)
}

// expectBalancesRead programs the balance list of the account and the live state
// of each row, which the cache does not hold.
func (m *closeAccountMocks) expectBalancesRead(balances ...*mmodel.Balance) {
	m.balance.EXPECT().ListByAccountID(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return(balances, nil)

	for range balances {
		m.redis.EXPECT().ListBalanceByKey(gomock.Any(), closeOrgID, closeLedgerID, gomock.Any()).
			Return(nil, redis.Nil)
	}
}

// expectRecoveryWalked programs both recovery origins returning an empty terminal
// page, which is the shape of an account with no completion left to run.
func (m *closeAccountMocks) expectRecoveryWalked() {
	m.redis.EXPECT().ScanRecoveryMessages(gomock.Any(), gomock.Any(), uint64(0), gomock.Any()).
		Return(txRedis.RecoveryScanPage{Cursor: 0}, nil).Times(2)
}

// expectPersistenceProven programs the high-water-mark read that answers the
// balances the cache does not hold. No mark is the shape of a balance that never
// moved.
func (m *closeAccountMocks) expectPersistenceProven() {
	m.operation.EXPECT().ListLatestByBalances(gomock.Any(), closeOrgID, closeLedgerID, gomock.Any()).
		Return(map[string]*operation.Operation{}, nil)
}

// expectPendingQuery programs the pending participation query.
func (m *closeAccountMocks) expectPendingQuery(found bool, err error) {
	m.transaction.EXPECT().HasPendingByAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return(found, err)
}

// closeEligibleBalance is a settled balance: every monetary component is exactly
// zero, so it may not hold the closing back.
func closeEligibleBalance() *mmodel.Balance {
	balance := closingBalance(closeBalanceID.String(), "default", "0", "0", "0", 1)
	balance.AccountID = closeAccountID.String()
	balance.OrganizationID = closeOrgID.String()
	balance.LedgerID = closeLedgerID.String()

	return balance
}

// TestCloseAccount_ClosesAnEligibleAccount covers AC-01: an account whose balances
// are settled, whose earlier work concluded and which holds no pending transaction
// closes, and the instant is the one the database generated.
func TestCloseAccount_ClosesAnEligibleAccount(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.expectProtectionTaken()
	m.expectBalancesRead(closeEligibleBalance())
	m.expectRecoveryWalked()
	m.expectPersistenceProven()
	m.expectPendingQuery(false, nil)

	m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).Return(closeInstant, nil)
	m.expectClosingFinalized(closeInstant, 1)

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	require.NoError(t, err)
	assert.Equal(t, closeInstant, closedAt)
}

// TestCloseAccount_ClosesABlockedAccount covers AC-10: blocking is a live control
// over movement, not a closing state, so a blocked account that satisfies every
// condition closes and nothing about its blocking is read or changed.
func TestCloseAccount_ClosesABlockedAccount(t *testing.T) {
	m := newCloseAccountMocks(t)

	blocked := closeAccountEntity("deposit", nil)
	blocked.Blocked = utils.BoolPtr(true)

	m.expectAccountRead(blocked, nil)
	m.expectProtectionTaken()
	m.expectBalancesRead(closeEligibleBalance())
	m.expectRecoveryWalked()
	m.expectPersistenceProven()
	m.expectPendingQuery(false, nil)

	m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).Return(closeInstant, nil)
	m.expectClosingFinalized(closeInstant, 1)

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	require.NoError(t, err)
	assert.Equal(t, closeInstant, closedAt)
}

// TestCloseAccount_RefusesAnExternalAccount covers AC-05: an external account is
// refused with the existing code, before any protection is taken.
func TestCloseAccount_RefusesAnExternalAccount(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectAccountRead(closeAccountEntity(constant.ExternalAccountType, nil), nil)

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrForbiddenExternalAccountManipulation)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_RefusesAnAccountAlreadyClosed covers AC-11: the repeat is a
// conflict, and the recorded instant is never rewritten.
func TestCloseAccount_RefusesAnAccountAlreadyClosed(t *testing.T) {
	m := newCloseAccountMocks(t)

	recorded := closeInstant
	m.expectAccountRead(closeAccountEntity("deposit", &recorded), nil)

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountAlreadyClosed)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_RefusesWhenAnotherAttemptOwnsTheAccount covers AC-11 on the
// other side of the race: the loser is refused as a decision in flight, which is
// not the same answer as a transition that already landed. Its marker write is
// the one that loses, so it holds nothing: the strict mocks fail the test on an
// ownership acquisition or a release.
func TestCloseAccount_RefusesWhenAnotherAttemptOwnsTheAccount(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	gomock.InOrder(
		m.redis.EXPECT().AcquireAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
			Return(false, nil),
		m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
			Return("another-attempt", true, nil),
	)

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingInProgress)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_RefusesAResidualBalance covers AC-06: a component that is not
// exactly zero refuses the closing, and no closing instant is written.
func TestCloseAccount_RefusesAResidualBalance(t *testing.T) {
	m := newCloseAccountMocks(t)

	residual := closeEligibleBalance()
	residual.Available = decimal.RequireFromString("0.01")

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.expectProtectionTaken()
	m.expectBalancesRead(residual)
	m.expectProtectionReleased()

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountBalanceNotZero)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_RefusesAPendingTransaction covers AC-08: every component reads
// zero and the account is still one commit away from moving.
func TestCloseAccount_RefusesAPendingTransaction(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.expectProtectionTaken()
	m.expectBalancesRead(closeEligibleBalance())
	m.expectRecoveryWalked()
	m.expectPersistenceProven()
	m.expectPendingQuery(true, nil)
	m.expectProtectionReleased()

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountHasPendingTransactions)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_RefusesWhileCompletionIsPending covers AS-08: a record still in
// recovery means the completer has work left on this account, so the refusal is
// the temporary one and the closing never takes that job over.
func TestCloseAccount_RefusesWhileCompletionIsPending(t *testing.T) {
	m := newCloseAccountMocks(t)

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

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingPersistencePending)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_RefusesWhenTheProtectionCannotBeRead covers AS-07: a control
// that cannot be read is refused technically, because reading a protection failure
// as absence would turn it into an authorization. The marker write lost to a key
// that is there, and what that key holds cannot be read, so the refusal is not
// the closing-in-progress one.
func TestCloseAccount_RefusesWhenTheProtectionCannotBeRead(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.redis.EXPECT().AcquireAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(false, nil)
	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return("", false, txRedis.ErrAccountProtectionMarkerUnreadable)

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_KeepsTheProtectionWhenTheMarkerWriteIsUnresolved covers AS-12
// at the very first write: the marker may have landed with its answer lost, so the
// account stays protected and no cleanup removes it. The ownership is never
// attempted: the strict mocks fail the test on it or on any release.
func TestCloseAccount_KeepsTheProtectionWhenTheMarkerWriteIsUnresolved(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.redis.EXPECT().AcquireAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(false, errors.New("cache unavailable"))

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_KeepsTheMarkerWhenTheOwnershipWriteIsUnresolved covers AS-12 at
// the second write: the ownership may have landed with its answer lost, so the
// marker stays as the anchor through which reconciliation gives both back. The
// strict mocks fail the test on any release.
func TestCloseAccount_KeepsTheMarkerWhenTheOwnershipWriteIsUnresolved(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	gomock.InOrder(
		m.expectMarkerInstalled(),
		m.expectOwnMarkerRead(),
		m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, m.attemptToken()).
			Return(false, errors.New("cache unavailable")),
	)

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_RefusesWhenItsMarkerIsNoLongerItsOwn covers an attempt whose
// marker was reclaimed, as an attempt that never wrote, between its installation
// and the ownership. The attempt takes no ownership. When another closing already
// installed its own marker in the meantime, that closing is what holds the
// account; when nothing is there, the attempt's protection is simply gone.
func TestCloseAccount_RefusesWhenItsMarkerIsNoLongerItsOwn(t *testing.T) {
	t.Run("another closing holds the account", func(t *testing.T) {
		m := newCloseAccountMocks(t)

		m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
		gomock.InOrder(
			m.expectMarkerInstalled(),
			m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
				Return("another-attempt", true, nil),
			// The conditional release leaves the other closing's marker in place.
			m.redis.EXPECT().ReleaseAccountClosingAttempt(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, m.attemptToken()).
				Return(false, nil),
		)

		_, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

		requireClosingCode(t, err, constant.ErrAccountClosingInProgress)
	})

	t.Run("nothing holds the account", func(t *testing.T) {
		m := newCloseAccountMocks(t)

		m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
		gomock.InOrder(
			m.expectMarkerInstalled(),
			m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
				Return("", false, nil),
		)

		_, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

		requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	})
}

// TestCloseAccount_RefusesAnAccountAbsentFromTheScope covers AC-03: the closing
// never reaches the protection of an account that is not in the authorized scope.
func TestCloseAccount_RefusesAnAccountAbsentFromTheScope(t *testing.T) {
	m := newCloseAccountMocks(t)

	m.expectAccountRead(nil, nil)

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	var notFound midazpkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, constant.ErrAccountIDNotFound.Error(), notFound.Code)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_ResolvesAZeroRowWriteWithTheAuthoritativeRow covers AS-12: the
// statement alone does not say why it matched nothing, so the row decides — a
// closing instant there is the repeat, not an absence.
func TestCloseAccount_ResolvesAZeroRowWriteWithTheAuthoritativeRow(t *testing.T) {
	m := newCloseAccountMocks(t)

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

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountAlreadyClosed)
	assert.True(t, closedAt.IsZero())
}

// legacyRecoveryPayloadForClose is one legacy recovery record naming the account
// under test, which is the shape of an execution still waiting for completion.
func legacyRecoveryPayloadForClose() string {
	return `{"transaction_id":"` + closeTransactionID.String() + `","organization_id":"` + closeOrgID.String() +
		`","ledger_id":"` + closeLedgerID.String() + `","balances":[{"id":"` + closeBalanceID.String() +
		`","accountId":"` + closeAccountID.String() + `","available":"0","onHold":"0","version":1}]}`
}
