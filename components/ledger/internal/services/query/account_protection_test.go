// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// expectOpenAccountAdmission programs the protection of an account that was never
// closed: no marker exists, the ownership is free, and the authoritative read
// reports no closing. It is the baseline every pre-existing cache-miss test runs
// under, so those tests keep asserting what they were written for.
func expectOpenAccountAdmission(redisMock *redis.MockRedisRepository, accountMock *account.MockRepository) {
	redisMock.EXPECT().GetAccountClosingMarker(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", false, nil).AnyTimes()
	redisMock.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true, nil).AnyTimes()
	redisMock.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true, nil).AnyTimes()
	redisMock.EXPECT().GetAccountClosedMarker(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(time.Time{}, false, nil).AnyTimes()
	accountMock.EXPECT().ListClosedAtByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[uuid.UUID]*time.Time{}, nil).AnyTimes()
}

type admissionMocks struct {
	uc      *UseCase
	balance *balance.MockRepository
	account *account.MockRepository
	redis   *redis.MockRedisRepository
}

var (
	admissionOrgID     = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
	admissionLedgerID  = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000002")
	admissionAccountID = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000003")
	admissionClosedAt  = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
)

const admissionAlias = "@closing_account#default"

func newAdmissionMocks(t *testing.T) *admissionMocks {
	t.Helper()

	ctrl := gomock.NewController(t)

	mocks := &admissionMocks{
		balance: balance.NewMockRepository(ctrl),
		account: account.NewMockRepository(ctrl),
		redis:   redis.NewMockRedisRepository(ctrl),
	}

	mocks.uc = &UseCase{
		BalanceRepo:          mocks.balance,
		AccountRepo:          mocks.account,
		TransactionRedisRepo: mocks.redis,
		OperationRepo:        operation.NewMockRepository(ctrl),
	}

	return mocks
}

// expectCacheMiss makes the cache read miss and the database return one balance of
// the account under test, which is the only path that reaches the protection.
func (m *admissionMocks) expectCacheMiss() {
	m.redis.EXPECT().Get(gomock.Any(), utils.BalanceInternalKey(admissionOrgID, admissionLedgerID, admissionAlias)).
		Return("", nil)
	m.balance.EXPECT().ListByAliasesWithKeys(gomock.Any(), admissionOrgID, admissionLedgerID, []string{admissionAlias}).
		Return([]*mmodel.Balance{{
			ID:        uuid.NewString(),
			AccountID: admissionAccountID.String(),
			Alias:     "@closing_account",
			Key:       constant.DefaultBalanceKey,
		}}, nil)
}

// TestGetBalances_CacheMissRefusesAClosedAccount covers AS-06 and AS-17: a load
// that finds the account closed refuses before the seed can be used, no matter
// whether the negative cache is still there.
func TestGetBalances_CacheMissRefusesAClosedAccount(t *testing.T) {
	tests := []struct {
		name   string
		expect func(m *admissionMocks)
	}{
		{
			name: "the negative cache still holds the closing",
			expect: func(m *admissionMocks) {
				m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID).
					Return(admissionClosedAt, true, nil)
			},
		},
		{
			name: "the negative cache expired and the authoritative row answers",
			expect: func(m *admissionMocks) {
				m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID).
					Return(time.Time{}, false, nil)
				m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), admissionOrgID, admissionLedgerID, []uuid.UUID{admissionAccountID}).
					Return(map[uuid.UUID]*time.Time{admissionAccountID: &admissionClosedAt}, nil)
				m.redis.EXPECT().SetAccountClosedMarker(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID, admissionClosedAt).
					Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newAdmissionMocks(t)
			m.expectCacheMiss()

			m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID).
				Return("", false, nil)
			m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID, gomock.Any()).
				Return(true, nil)
			m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID, gomock.Any()).
				Return(true, nil)

			tt.expect(m)

			balances, err := m.uc.GetBalances(context.Background(), admissionOrgID, admissionLedgerID, []string{admissionAlias})

			require.Error(t, err)
			assert.Nil(t, balances, "a closed account must yield no seed at all")

			var unprocessable pkg.UnprocessableOperationError
			require.True(t, errors.As(err, &unprocessable))
			assert.Equal(t, constant.ErrAccountClosed.Error(), unprocessable.Code)
		})
	}
}

// TestGetBalances_CacheMissRefusesWhenAClosingOwnsTheAccount proves a closing in
// flight stops the admission before any ownership is taken.
func TestGetBalances_CacheMissRefusesWhenAClosingOwnsTheAccount(t *testing.T) {
	m := newAdmissionMocks(t)
	m.expectCacheMiss()

	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID).
		Return(uuid.NewString(), true, nil)

	_, err := m.uc.GetBalances(context.Background(), admissionOrgID, admissionLedgerID, []string{admissionAlias})

	require.Error(t, err)

	var conflict pkg.EntityConflictError
	require.True(t, errors.As(err, &conflict))
	assert.Equal(t, constant.ErrAccountClosingInProgress.Error(), conflict.Code)
}

// TestGetBalances_CacheMissRefusesWhenTheProtectionIsUnreadable covers AS-07: a
// protection that cannot be read is not an open account.
func TestGetBalances_CacheMissRefusesWhenTheProtectionIsUnreadable(t *testing.T) {
	m := newAdmissionMocks(t)
	m.expectCacheMiss()

	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID).
		Return("", false, redis.ErrAccountProtectionMarkerUnreadable)

	_, err := m.uc.GetBalances(context.Background(), admissionOrgID, admissionLedgerID, []string{admissionAlias})

	require.Error(t, err)

	var unavailable pkg.ServiceUnavailableError
	require.True(t, errors.As(err, &unavailable))
	assert.Equal(t, constant.ErrAccountClosingProtectionIndeterminate.Error(), unavailable.Code)
}

// admissionSeedRow builds one balance row of the account under test.
func admissionSeedRow(accountID uuid.UUID, available int64) *mmodel.Balance {
	return &mmodel.Balance{
		ID:        uuid.NewString(),
		AccountID: accountID.String(),
		Alias:     "@closing_account",
		Key:       constant.DefaultBalanceKey,
		Available: decimal.NewFromInt(available),
	}
}

// TestGetBalances_SeedIsReadUnderTheOwnership proves the widened window: the first
// read only names the accounts to own, the seed itself is read again under that
// ownership, and the ownership survives the hydration and the rebuild. The rows the
// pre-ownership read returned never reach the caller.
func TestGetBalances_SeedIsReadUnderTheOwnership(t *testing.T) {
	m := newAdmissionMocks(t)

	beforeOwnership := admissionSeedRow(admissionAccountID, 100)
	underOwnership := admissionSeedRow(admissionAccountID, 200)

	m.redis.EXPECT().Get(gomock.Any(), utils.BalanceInternalKey(admissionOrgID, admissionLedgerID, admissionAlias)).
		Return("", nil)

	resolve := m.balance.EXPECT().ListByAliasesWithKeys(gomock.Any(), admissionOrgID, admissionLedgerID, []string{admissionAlias}).
		Return([]*mmodel.Balance{beforeOwnership}, nil)

	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID).
		Return("", false, nil).After(resolve)
	acquire := m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID, gomock.Any()).
		Return(true, nil).After(resolve)
	m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID).
		Return(time.Time{}, false, nil)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), admissionOrgID, admissionLedgerID, []uuid.UUID{admissionAccountID}).
		Return(map[uuid.UUID]*time.Time{admissionAccountID: nil}, nil)

	reread := m.balance.EXPECT().ListByAliasesWithKeys(gomock.Any(), admissionOrgID, admissionLedgerID, []string{admissionAlias}).
		Return([]*mmodel.Balance{underOwnership}, nil).After(acquire)

	blocked := false
	hydrate := m.account.EXPECT().ListAccountsByIDs(gomock.Any(), admissionOrgID, admissionLedgerID, []uuid.UUID{admissionAccountID}).
		Return([]*mmodel.Account{{ID: admissionAccountID.String(), Blocked: &blocked}}, nil).After(reread)

	rebuild := m.uc.OperationRepo.(*operation.MockRepository).EXPECT().
		ListLatestByBalances(gomock.Any(), admissionOrgID, admissionLedgerID, gomock.Any()).
		Return(map[string]*operation.Operation{}, nil).After(hydrate)

	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID, gomock.Any()).
		Return(true, nil).After(rebuild)

	balances, err := m.uc.GetBalances(context.Background(), admissionOrgID, admissionLedgerID, []string{admissionAlias})

	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.Equal(t, underOwnership.ID, balances[0].ID, "the seed must be the row read under the ownership")
	assert.True(t, decimal.NewFromInt(200).Equal(balances[0].Available))
}

// TestGetBalances_SeedOutsideTheOwnedSetIsRefused proves the re-read cannot smuggle
// in an account the ownership never covered: such a seed was checked against no
// closing at all, so it is refused instead of served.
func TestGetBalances_SeedOutsideTheOwnedSetIsRefused(t *testing.T) {
	m := newAdmissionMocks(t)

	stranger := uuid.New()

	m.redis.EXPECT().Get(gomock.Any(), utils.BalanceInternalKey(admissionOrgID, admissionLedgerID, admissionAlias)).
		Return("", nil)
	m.balance.EXPECT().ListByAliasesWithKeys(gomock.Any(), admissionOrgID, admissionLedgerID, []string{admissionAlias}).
		Return([]*mmodel.Balance{admissionSeedRow(admissionAccountID, 100)}, nil)

	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID).
		Return("", false, nil)
	m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID, gomock.Any()).
		Return(true, nil)
	m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID).
		Return(time.Time{}, false, nil)
	m.account.EXPECT().ListClosedAtByIDs(gomock.Any(), admissionOrgID, admissionLedgerID, []uuid.UUID{admissionAccountID}).
		Return(map[uuid.UUID]*time.Time{admissionAccountID: nil}, nil)

	m.balance.EXPECT().ListByAliasesWithKeys(gomock.Any(), admissionOrgID, admissionLedgerID, []string{admissionAlias}).
		Return([]*mmodel.Balance{admissionSeedRow(stranger, 100)}, nil)

	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), admissionOrgID, admissionLedgerID, admissionAccountID, gomock.Any()).
		Return(true, nil)

	_, err := m.uc.GetBalances(context.Background(), admissionOrgID, admissionLedgerID, []string{admissionAlias})

	require.Error(t, err)

	var unavailable pkg.ServiceUnavailableError
	require.True(t, errors.As(err, &unavailable))
	assert.Equal(t, constant.ErrAccountClosingProtectionIndeterminate.Error(), unavailable.Code)
}

// TestGetBalances_CacheHitTouchesNoProtection is the hard performance constraint of
// the design: a cached balance acquires no ownership and reads no account row. The
// strict mocks fail the test if any of those calls happen.
func TestGetBalances_CacheHitTouchesNoProtection(t *testing.T) {
	m := newAdmissionMocks(t)

	cached := fmt.Sprintf(`{"ID":%q,"AccountID":%q,"AccountType":"deposit","AssetCode":"BRL",`+
		`"Alias":"@closing_account","Key":"default","Available":"120","OnHold":"0","Version":3,`+
		`"AllowSending":1,"AllowReceiving":1}`, uuid.NewString(), admissionAccountID.String())

	m.redis.EXPECT().Get(gomock.Any(), utils.BalanceInternalKey(admissionOrgID, admissionLedgerID, admissionAlias)).
		Return(cached, nil)

	balances, err := m.uc.GetBalances(context.Background(), admissionOrgID, admissionLedgerID, []string{admissionAlias})

	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.Equal(t, admissionAccountID.String(), balances[0].AccountID)
}
