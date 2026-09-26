// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// expectOpenSeedAdmissionOf programs one successful seed admission over an account
// that was never closed, and returns the acquisition call so a caller can order a
// read after it.
func expectOpenSeedAdmissionOf(m *admissionMocks, accountID uuid.UUID) *gomock.Call {
	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), admissionOrgID, admissionLedgerID, accountID).
		Return("", false, nil)

	acquire := m.redis.EXPECT().AcquireAccountSeedAdmission(gomock.Any(), admissionOrgID, admissionLedgerID, accountID, gomock.Any()).
		Return(true, redis.AccountAdminHolderNone, nil)

	m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), admissionOrgID, admissionLedgerID, accountID).
		Return(time.Time{}, false, nil)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), admissionOrgID, admissionLedgerID, []uuid.UUID{accountID}).
		Return(map[uuid.UUID]*time.Time{accountID: nil}, nil)

	return acquire
}

// expectSeedHydrationAndRebuild programs the reads that follow a seed load that
// was fully admitted.
func expectSeedHydrationAndRebuild(m *admissionMocks) {
	m.account.EXPECT().ListAccountsByIDs(gomock.Any(), admissionOrgID, admissionLedgerID, gomock.Any()).
		Return([]*mmodel.Account{}, nil)
	m.uc.OperationRepo.(*operation.MockRepository).EXPECT().
		ListLatestByBalances(gomock.Any(), admissionOrgID, admissionLedgerID, gomock.Any()).
		Return(map[string]*operation.Operation{}, nil)
}

func expectSeedAdmissionRelease(m *admissionMocks, accountID uuid.UUID) {
	m.redis.EXPECT().ReleaseAccountSeedAdmission(gomock.Any(), admissionOrgID, admissionLedgerID, accountID, gomock.Any()).
		Return(true, nil)
}

func expectSeedRead(m *admissionMocks, rows ...*mmodel.Balance) *gomock.Call {
	return m.balance.EXPECT().ListByAliasesWithKeys(gomock.Any(), admissionOrgID, admissionLedgerID, []string{admissionAlias}).
		Return(rows, nil)
}

func servedIDs(balances []*mmodel.Balance) []string {
	ids := make([]string, 0, len(balances))
	for _, b := range balances {
		ids = append(ids, b.ID)
	}

	return ids
}

// TestGetBalances_ReReadNamingANewAccountExtendsTheAdmission covers a balance row
// that appears between the resolution read and the read under the admission, such
// as an overdraft companion a concurrent settings update just created. Its account
// was never checked against a closing, so the admission is extended to it, the
// account is proven open, and the rows are read again: the load succeeds instead
// of refusing, and only rows read under the complete admission are served.
func TestGetBalances_ReReadNamingANewAccountExtendsTheAdmission(t *testing.T) {
	m := newAdmissionMocks(t)
	appeared := uuid.New()

	m.redis.EXPECT().Get(gomock.Any(), utils.BalanceInternalKey(admissionOrgID, admissionLedgerID, admissionAlias)).
		Return("", nil)
	resolve := expectSeedRead(m, admissionSeedRow(admissionAccountID, 100))

	first := expectOpenSeedAdmissionOf(m, admissionAccountID)
	first.After(resolve)

	unprotected := admissionSeedRow(appeared, 7)
	reread := expectSeedRead(m, admissionSeedRow(admissionAccountID, 100), unprotected).After(first)

	extension := expectOpenSeedAdmissionOf(m, appeared)
	extension.After(reread)

	servedPrimary, servedAppeared := admissionSeedRow(admissionAccountID, 100), admissionSeedRow(appeared, 7)
	expectSeedRead(m, servedPrimary, servedAppeared).After(extension)

	expectSeedHydrationAndRebuild(m)
	expectSeedAdmissionRelease(m, admissionAccountID)
	expectSeedAdmissionRelease(m, appeared)

	balances, err := m.uc.GetBalances(context.Background(), admissionOrgID, admissionLedgerID, []string{admissionAlias})

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{servedPrimary.ID, servedAppeared.ID}, servedIDs(balances),
		"only rows read after the admission covered every account may be served")
	assert.NotContains(t, servedIDs(balances), unprotected.ID)
}

// TestGetBalances_ReReadAfterAnEmptyResolutionAdmitsTheAccount covers the load
// whose resolution read found no row, so it took no admission at all: a row the
// read under the admission then finds must still be admitted before it is served.
func TestGetBalances_ReReadAfterAnEmptyResolutionAdmitsTheAccount(t *testing.T) {
	m := newAdmissionMocks(t)
	appeared := uuid.New()

	m.redis.EXPECT().Get(gomock.Any(), utils.BalanceInternalKey(admissionOrgID, admissionLedgerID, admissionAlias)).
		Return("", nil)
	resolve := expectSeedRead(m)

	unprotected := admissionSeedRow(appeared, 7)
	reread := expectSeedRead(m, unprotected).After(resolve)

	extension := expectOpenSeedAdmissionOf(m, appeared)
	extension.After(reread)

	served := admissionSeedRow(appeared, 7)
	expectSeedRead(m, served).After(extension)

	expectSeedHydrationAndRebuild(m)
	expectSeedAdmissionRelease(m, appeared)

	balances, err := m.uc.GetBalances(context.Background(), admissionOrgID, admissionLedgerID, []string{admissionAlias})

	require.NoError(t, err)
	assert.Equal(t, []string{served.ID}, servedIDs(balances))
}

// TestGetBalances_ReReadNamingAClosedAccountIsRefused proves the extension applies
// the same proof as the first admission: an account that turns out closed refuses
// the load with 0519, and every admission the load took is given back.
func TestGetBalances_ReReadNamingAClosedAccountIsRefused(t *testing.T) {
	m := newAdmissionMocks(t)
	closed := uuid.New()
	closedAt := admissionClosedAt

	m.redis.EXPECT().Get(gomock.Any(), utils.BalanceInternalKey(admissionOrgID, admissionLedgerID, admissionAlias)).
		Return("", nil)
	resolve := expectSeedRead(m, admissionSeedRow(admissionAccountID, 100))
	expectOpenSeedAdmissionOf(m, admissionAccountID).After(resolve)
	expectSeedRead(m, admissionSeedRow(admissionAccountID, 100), admissionSeedRow(closed, 0))

	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), admissionOrgID, admissionLedgerID, closed).
		Return("", false, nil)
	m.redis.EXPECT().AcquireAccountSeedAdmission(gomock.Any(), admissionOrgID, admissionLedgerID, closed, gomock.Any()).
		Return(true, redis.AccountAdminHolderNone, nil)
	m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), admissionOrgID, admissionLedgerID, closed).
		Return(time.Time{}, false, nil)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), admissionOrgID, admissionLedgerID, []uuid.UUID{closed}).
		Return(map[uuid.UUID]*time.Time{closed: &closedAt}, nil)
	m.redis.EXPECT().SetAccountClosedMarker(gomock.Any(), admissionOrgID, admissionLedgerID, closed, closedAt).
		Return(nil)

	expectSeedAdmissionRelease(m, closed)
	expectSeedAdmissionRelease(m, admissionAccountID)

	_, err := m.uc.GetBalances(context.Background(), admissionOrgID, admissionLedgerID, []string{admissionAlias})

	var unprocessable pkg.UnprocessableOperationError
	require.True(t, errors.As(err, &unprocessable), "got %v", err)
	assert.Equal(t, constant.ErrAccountClosed.Error(), unprocessable.Code)
}

// TestGetBalances_ReReadNamingAHeldAccountIsRefused proves an extension the
// account's exclusive holder refuses answers like the first admission would, and
// still gives back the admission the load already held.
func TestGetBalances_ReReadNamingAHeldAccountIsRefused(t *testing.T) {
	m := newAdmissionMocks(t)
	held := uuid.New()

	m.redis.EXPECT().Get(gomock.Any(), utils.BalanceInternalKey(admissionOrgID, admissionLedgerID, admissionAlias)).
		Return("", nil)
	resolve := expectSeedRead(m, admissionSeedRow(admissionAccountID, 100))
	expectOpenSeedAdmissionOf(m, admissionAccountID).After(resolve)
	expectSeedRead(m, admissionSeedRow(admissionAccountID, 100), admissionSeedRow(held, 0))

	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), admissionOrgID, admissionLedgerID, held).
		Return("", false, nil).AnyTimes()
	m.redis.EXPECT().AcquireAccountSeedAdmission(gomock.Any(), admissionOrgID, admissionLedgerID, held, gomock.Any()).
		Return(false, redis.AccountAdminHolderExclusive, nil)

	expectSeedAdmissionRelease(m, admissionAccountID)

	_, err := m.uc.GetBalances(context.Background(), admissionOrgID, admissionLedgerID, []string{admissionAlias})

	var conflict pkg.EntityConflictError
	require.True(t, errors.As(err, &conflict), "got %v", err)
	assert.Equal(t, constant.ErrAccountAdministrativeOperationInProgress.Error(), conflict.Code)
}

// TestGetBalances_ReReadThatKeepsNamingNewAccountsIsRefused bounds the extension:
// a load whose every read names another account never settles on a set it
// proved, so after the bound it refuses with 0520 instead of chasing the drift,
// and every admission it took is given back.
func TestGetBalances_ReReadThatKeepsNamingNewAccountsIsRefused(t *testing.T) {
	m := newAdmissionMocks(t)

	m.redis.EXPECT().Get(gomock.Any(), utils.BalanceInternalKey(admissionOrgID, admissionLedgerID, admissionAlias)).
		Return("", nil)

	var reads atomic.Int32

	m.balance.EXPECT().ListByAliasesWithKeys(gomock.Any(), admissionOrgID, admissionLedgerID, []string{admissionAlias}).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
			reads.Add(1)

			return []*mmodel.Balance{admissionSeedRow(uuid.New(), 1)}, nil
		}).AnyTimes()

	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", false, nil).AnyTimes()

	var admitted, released atomic.Int32

	m.redis.EXPECT().AcquireAccountSeedAdmission(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) (bool, redis.AccountAdminHolder, error) {
			admitted.Add(1)

			return true, redis.AccountAdminHolderNone, nil
		}).AnyTimes()
	m.redis.EXPECT().ReleaseAccountSeedAdmission(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) (bool, error) {
			released.Add(1)

			return true, nil
		}).AnyTimes()
	m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(time.Time{}, false, nil).AnyTimes()
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[uuid.UUID]*time.Time{}, nil).AnyTimes()

	_, err := m.uc.GetBalances(context.Background(), admissionOrgID, admissionLedgerID, []string{admissionAlias})

	var unavailable pkg.ServiceUnavailableError
	require.True(t, errors.As(err, &unavailable), "got %v", err)
	assert.Equal(t, constant.ErrAccountClosingProtectionIndeterminate.Error(), unavailable.Code)

	assert.Equal(t, int32(2+maxSeedAdmissionExtensions), reads.Load(),
		"the resolution read, the read under the first admission, and one read per extension")
	assert.Equal(t, int32(1+maxSeedAdmissionExtensions), admitted.Load())
	assert.Equal(t, admitted.Load(), released.Load(), "every admission the load took is given back")
}

// TestGetBalances_ExtendedAdmissionIsHandedToTheExecution proves an extension
// reaches the sink like the first admission: the engine can only admit the new
// account's seed if the execution holds a token for it.
func TestGetBalances_ExtendedAdmissionIsHandedToTheExecution(t *testing.T) {
	m := newAdmissionMocks(t)
	appeared := uuid.New()

	m.redis.EXPECT().Get(gomock.Any(), utils.BalanceInternalKey(admissionOrgID, admissionLedgerID, admissionAlias)).
		Return("", nil)
	resolve := expectSeedRead(m, admissionSeedRow(admissionAccountID, 100))
	first := expectOpenSeedAdmissionOf(m, admissionAccountID)
	first.After(resolve)
	reread := expectSeedRead(m, admissionSeedRow(admissionAccountID, 100), admissionSeedRow(appeared, 7)).After(first)
	extension := expectOpenSeedAdmissionOf(m, appeared)
	extension.After(reread)
	expectSeedRead(m, admissionSeedRow(admissionAccountID, 100), admissionSeedRow(appeared, 7)).After(extension)
	expectSeedHydrationAndRebuild(m)

	ctx, sink := accountprotection.ContextWithSink(context.Background())

	_, err := m.uc.GetBalances(ctx, admissionOrgID, admissionLedgerID, []string{admissionAlias})
	require.NoError(t, err)

	assert.NotEmpty(t, sink.TokenFor(admissionOrgID, admissionLedgerID, admissionAccountID))
	assert.NotEmpty(t, sink.TokenFor(admissionOrgID, admissionLedgerID, appeared),
		"the extension belongs to the execution that admits the new account's seed")
}
