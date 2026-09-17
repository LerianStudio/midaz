// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	midazpkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

var (
	closingOrgID     = uuid.MustParse("cccccccc-0000-0000-0000-000000000001")
	closingLedgerID  = uuid.MustParse("cccccccc-0000-0000-0000-000000000002")
	closingAccountID = uuid.MustParse("cccccccc-0000-0000-0000-000000000003")
)

type closingBalanceMocks struct {
	uc        *UseCase
	balance   *balance.MockRepository
	operation *operation.MockRepository
	redis     *txRedis.MockRedisRepository
}

func newClosingBalanceMocks(t *testing.T) *closingBalanceMocks {
	t.Helper()

	ctrl := gomock.NewController(t)

	mocks := &closingBalanceMocks{
		balance:   balance.NewMockRepository(ctrl),
		operation: operation.NewMockRepository(ctrl),
		redis:     txRedis.NewMockRedisRepository(ctrl),
	}

	mocks.uc = &UseCase{
		BalanceRepo:          mocks.balance,
		OperationRepo:        mocks.operation,
		TransactionRedisRepo: mocks.redis,
	}

	return mocks
}

// closingBalance builds one persisted balance row of the account under test.
func closingBalance(id, key, available, onHold, overdraftUsed string, version int64) *mmodel.Balance {
	return &mmodel.Balance{
		ID:             id,
		AccountID:      closingAccountID.String(),
		OrganizationID: closingOrgID.String(),
		LedgerID:       closingLedgerID.String(),
		Alias:          "@closing",
		Key:            key,
		AssetCode:      "USD",
		Available:      decimal.RequireFromString(available),
		OnHold:         decimal.RequireFromString(onHold),
		OverdraftUsed:  decimal.RequireFromString(overdraftUsed),
		Version:        version,
	}
}

// closingHighWaterMark builds the operation holding a balance's high-water mark.
func closingHighWaterMark(available, onHold, overdraftUsed string, version int64) *operation.Operation {
	availableDecimal := decimal.RequireFromString(available)
	onHoldDecimal := decimal.RequireFromString(onHold)

	return &operation.Operation{
		BalanceAfter: operation.Balance{Available: &availableDecimal, OnHold: &onHoldDecimal, Version: &version},
		Snapshot:     mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: overdraftUsed},
	}
}

func requireClosingCode(t *testing.T, err error, sentinel error) {
	t.Helper()

	require.Error(t, err)

	var conflict midazpkg.EntityConflictError
	if errors.As(err, &conflict) {
		assert.Equal(t, sentinel.Error(), conflict.Code)

		return
	}

	var unprocessable midazpkg.UnprocessableOperationError
	if errors.As(err, &unprocessable) {
		assert.Equal(t, sentinel.Error(), unprocessable.Code)

		return
	}

	var unavailable midazpkg.ServiceUnavailableError
	require.Truef(t, errors.As(err, &unavailable), "unexpected error type: %T (%v)", err, err)
	assert.Equal(t, sentinel.Error(), unavailable.Code)
}

// TestIsAccountClosingIndeterminateSeparatesTheRefusalClasses pins how the span
// class of a refusal is decided: pkg.ValidateBusinessError maps a sentinel onto a
// typed error that does not wrap it, so errors.Is against the sentinel matches
// neither refusal and the mapped type plus its code is what separates the
// technical 0520 from the business 0518.
func TestIsAccountClosingIndeterminateSeparatesTheRefusalClasses(t *testing.T) {
	indeterminate := midazpkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
	pending := midazpkg.ValidateBusinessError(constant.ErrAccountClosingPersistencePending, constant.EntityAccount)

	assert.False(t, errors.Is(indeterminate, constant.ErrAccountClosingProtectionIndeterminate))
	assert.False(t, errors.Is(pending, constant.ErrAccountClosingPersistencePending))

	assert.True(t, isAccountClosingIndeterminate(indeterminate))
	assert.False(t, isAccountClosingIndeterminate(pending))
	assert.False(t, isAccountClosingIndeterminate(errors.New("unrelated")))
	assert.False(t, isAccountClosingIndeterminate(nil))
}

// TestAccountClosingBalancesZeroedChecksEveryComponentIndividually covers AC-06,
// AC-07 and AC-09: every component of every balance must read exactly zero, with
// no compensation across components or balances and no rounding — while an unused
// overdraft limit is left alone, because a permission to owe is not a debt.
func TestAccountClosingBalancesZeroedChecksEveryComponentIndividually(t *testing.T) {
	limit := "500"

	tests := []struct {
		name   string
		states []accountClosingBalanceState
		closes bool
	}{
		{
			name: "default, additional and internal balances all zero",
			states: []accountClosingBalanceState{
				{Persisted: closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 3)},
				{Persisted: closingBalance("b-savings", "savings", "0", "0", "0", 1)},
				{Persisted: closingBalance("b-internal", "internal", "0", "0", "0", 0)},
			},
			closes: true,
		},
		{
			name: "an unused overdraft limit is not a debt",
			states: func() []accountClosingBalanceState {
				b := closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 4)
				b.Settings = &mmodel.BalanceSettings{AllowOverdraft: true, OverdraftLimitEnabled: true, OverdraftLimit: &limit}

				return []accountClosingBalanceState{{Persisted: b}}
			}(),
			closes: true,
		},
		{
			name:   "a positive residual on the smallest precision refuses",
			states: []accountClosingBalanceState{{Persisted: closingBalance("b-default", constant.DefaultBalanceKey, "0.01", "0", "0", 1)}},
		},
		{
			name:   "a negative residual refuses",
			states: []accountClosingBalanceState{{Persisted: closingBalance("b-default", constant.DefaultBalanceKey, "-0.01", "0", "0", 1)}},
		},
		{
			name:   "funds on hold refuse",
			states: []accountClosingBalanceState{{Persisted: closingBalance("b-default", constant.DefaultBalanceKey, "0", "0.01", "0", 1)}},
		},
		{
			name:   "used overdraft refuses",
			states: []accountClosingBalanceState{{Persisted: closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0.01", 1)}},
		},
		{
			name: "a residual on an additional balance is not hidden by a zero default",
			states: []accountClosingBalanceState{
				{Persisted: closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 1)},
				{Persisted: closingBalance("b-savings", "savings", "0.01", "0", "0", 1)},
			},
		},
		{
			name: "opposite residuals on two balances are never compensated",
			states: []accountClosingBalanceState{
				{Persisted: closingBalance("b-default", constant.DefaultBalanceKey, "10.00", "0", "0", 1)},
				{Persisted: closingBalance("b-savings", "savings", "-10.00", "0", "0", 1)},
			},
		},
		{
			name: "the live state decides over a stale row",
			states: []accountClosingBalanceState{{
				Persisted: closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 1),
				Live:      closingBalance("b-default", constant.DefaultBalanceKey, "10.00", "0", "0", 2),
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := verifyAccountClosingBalancesZeroed(test.states)

			if test.closes {
				require.NoError(t, err)

				return
			}

			requireClosingCode(t, err, constant.ErrAccountBalanceNotZero)
		})
	}
}

// TestLoadAccountClosingBalanceStatesReadsEveryBalanceAndItsLiveState proves the
// whole balance list is read from the primary and paired with the cache, and that
// a cache miss is carried as an absence of live state rather than as a zero.
func TestLoadAccountClosingBalanceStatesReadsEveryBalanceAndItsLiveState(t *testing.T) {
	m := newClosingBalanceMocks(t)

	rows := []*mmodel.Balance{
		closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 2),
		closingBalance("b-savings", "savings", "0", "0", "0", 1),
	}

	m.balance.EXPECT().ListByAccountID(gomock.Any(), closingOrgID, closingLedgerID, closingAccountID).Return(rows, nil)
	m.redis.EXPECT().ListBalanceByKey(gomock.Any(), closingOrgID, closingLedgerID, "@closing#"+constant.DefaultBalanceKey).
		Return(closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 2), nil)
	m.redis.EXPECT().ListBalanceByKey(gomock.Any(), closingOrgID, closingLedgerID, "@closing#savings").
		Return(nil, redis.Nil)

	states, err := m.uc.loadAccountClosingBalanceStates(context.Background(), closingOrgID, closingLedgerID, closingAccountID)
	require.NoError(t, err)
	require.Len(t, states, 2)
	require.NotNil(t, states[0].Live)
	assert.Nil(t, states[1].Live)
}

// TestLoadAccountClosingBalanceStatesRefusesAnUnreadableCache proves a cache read
// failure is never read as "no live state": it leaves the balance unknown, so the
// closing is refused as indeterminate.
func TestLoadAccountClosingBalanceStatesRefusesAnUnreadableCache(t *testing.T) {
	m := newClosingBalanceMocks(t)

	m.balance.EXPECT().ListByAccountID(gomock.Any(), closingOrgID, closingLedgerID, closingAccountID).
		Return([]*mmodel.Balance{closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 2)}, nil)
	m.redis.EXPECT().ListBalanceByKey(gomock.Any(), closingOrgID, closingLedgerID, gomock.Any()).
		Return(nil, errors.New("cache unavailable"))

	_, err := m.uc.loadAccountClosingBalanceStates(context.Background(), closingOrgID, closingLedgerID, closingAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
}

// TestLoadAccountClosingBalanceStatesRefusesAMisattributedCacheEntry proves a
// cached blob that does not describe the row it was read for is evidence that
// cannot be attributed, and is refused instead of compared.
func TestLoadAccountClosingBalanceStatesRefusesAMisattributedCacheEntry(t *testing.T) {
	m := newClosingBalanceMocks(t)

	other := closingBalance("b-other", constant.DefaultBalanceKey, "0", "0", "0", 2)

	m.balance.EXPECT().ListByAccountID(gomock.Any(), closingOrgID, closingLedgerID, closingAccountID).
		Return([]*mmodel.Balance{closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 2)}, nil)
	m.redis.EXPECT().ListBalanceByKey(gomock.Any(), closingOrgID, closingLedgerID, gomock.Any()).Return(other, nil)

	_, err := m.uc.loadAccountClosingBalanceStates(context.Background(), closingOrgID, closingLedgerID, closingAccountID)

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
}

// TestAccountClosingPersistenceRefusesWhileTheSyncWorkerOwesWork covers AS-08: a
// credit and a debit of 10.00 leave the live state at zero while the row is still
// two versions behind, so the closing is refused temporarily instead of closing on
// a zero the database has not seen.
func TestAccountClosingPersistenceRefusesWhileTheSyncWorkerOwesWork(t *testing.T) {
	m := newClosingBalanceMocks(t)

	states := []accountClosingBalanceState{{
		Persisted: closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 0),
		Live:      closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 2),
	}}

	require.NoError(t, verifyAccountClosingBalancesZeroed(states))

	err := m.uc.verifyAccountClosingBalancePersistence(context.Background(), closingOrgID, closingLedgerID, states)

	requireClosingCode(t, err, constant.ErrAccountClosingPersistencePending)
}

// TestAccountClosingPersistenceAcceptsAConvergedBalance proves the refusal is
// temporary: once the worker persisted the same version and components, the same
// verification passes without the closing having synced anything itself.
func TestAccountClosingPersistenceAcceptsAConvergedBalance(t *testing.T) {
	m := newClosingBalanceMocks(t)

	states := []accountClosingBalanceState{{
		Persisted: closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 2),
		Live:      closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 2),
	}}

	require.NoError(t, m.uc.verifyAccountClosingBalancePersistence(context.Background(), closingOrgID, closingLedgerID, states))
}

// TestAccountClosingPersistenceRefusesIrreconcilableEvidence proves a cached state
// behind its row, and one that disagrees with it at the same version, are both
// refused as indeterminate rather than resolved by preferring either side.
func TestAccountClosingPersistenceRefusesIrreconcilableEvidence(t *testing.T) {
	tests := []struct {
		name  string
		state accountClosingBalanceState
	}{
		{
			name: "the cached state is behind the row",
			state: accountClosingBalanceState{
				Persisted: closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 3),
				Live:      closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 2),
			},
		},
		{
			name: "the components disagree at the same version",
			state: accountClosingBalanceState{
				Persisted: closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0", 2),
				Live:      closingBalance("b-default", constant.DefaultBalanceKey, "0", "0", "0.01", 2),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newClosingBalanceMocks(t)

			err := m.uc.verifyAccountClosingBalancePersistence(context.Background(), closingOrgID, closingLedgerID,
				[]accountClosingBalanceState{test.state})

			requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
		})
	}
}

// TestAccountClosingPersistenceAnswersACacheMissFromTheOperationTrail covers
// AS-08 and AS-10 on the cache-miss path: the trail decides, a row behind it is
// persistence still owed, a mark that carries no state is inconclusive, and no
// balance is warmed into the cache to find out.
func TestAccountClosingPersistenceAnswersACacheMissFromTheOperationTrail(t *testing.T) {
	balanceID := uuid.MustParse("cccccccc-0000-0000-0000-000000000010").String()
	emptyMark := &operation.Operation{Snapshot: mmodel.OperationSnapshot{OverdraftUsedAfter: "0"}}

	tests := []struct {
		name     string
		row      *mmodel.Balance
		mark     *operation.Operation
		sentinel error
	}{
		{
			name: "a balance that never moved has no mark and closes",
			row:  closingBalance(balanceID, constant.DefaultBalanceKey, "0", "0", "0", 0),
		},
		{
			name: "a row at the mark closes",
			row:  closingBalance(balanceID, constant.DefaultBalanceKey, "0", "0", "0", 2),
			mark: closingHighWaterMark("0", "0", "0", 2),
		},
		{
			name:     "a row behind the mark is persistence still owed",
			row:      closingBalance(balanceID, constant.DefaultBalanceKey, "0", "0", "0", 1),
			mark:     closingHighWaterMark("0", "0", "0", 2),
			sentinel: constant.ErrAccountClosingPersistencePending,
		},
		{
			name:     "a mark with no state to compare is inconclusive",
			row:      closingBalance(balanceID, constant.DefaultBalanceKey, "0", "0", "0", 2),
			mark:     emptyMark,
			sentinel: constant.ErrAccountClosingProtectionIndeterminate,
		},
		{
			name:     "a mark that disagrees with the row at the same version is inconclusive",
			row:      closingBalance(balanceID, constant.DefaultBalanceKey, "0", "0", "0", 2),
			mark:     closingHighWaterMark("10.00", "0", "0", 2),
			sentinel: constant.ErrAccountClosingProtectionIndeterminate,
		},
		{
			name:     "a debt the row does not carry is inconclusive",
			row:      closingBalance(balanceID, constant.DefaultBalanceKey, "0", "0", "0", 2),
			mark:     closingHighWaterMark("0", "0", "0.01", 2),
			sentinel: constant.ErrAccountClosingProtectionIndeterminate,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newClosingBalanceMocks(t)

			marks := map[string]*operation.Operation{}
			if test.mark != nil {
				marks[test.row.ID] = test.mark
			}

			m.operation.EXPECT().ListLatestByBalances(gomock.Any(), closingOrgID, closingLedgerID, gomock.Any()).Return(marks, nil)

			err := m.uc.verifyAccountClosingBalancePersistence(context.Background(), closingOrgID, closingLedgerID,
				[]accountClosingBalanceState{{Persisted: test.row}})

			if test.sentinel == nil {
				require.NoError(t, err)

				return
			}

			requireClosingCode(t, err, test.sentinel)
		})
	}
}

// TestAccountClosingPersistenceIgnoresTheCompanionOverdraftSnapshot proves the
// overdraft companion is compared on its own state only: its operations mirror the
// default balance's overdraft snapshot, which describes another balance.
func TestAccountClosingPersistenceIgnoresTheCompanionOverdraftSnapshot(t *testing.T) {
	m := newClosingBalanceMocks(t)

	companionID := uuid.MustParse("cccccccc-0000-0000-0000-000000000011").String()
	companion := closingBalance(companionID, constant.OverdraftBalanceKey, "0", "0", "0", 2)

	m.operation.EXPECT().ListLatestByBalances(gomock.Any(), closingOrgID, closingLedgerID, gomock.Any()).
		Return(map[string]*operation.Operation{companionID: closingHighWaterMark("0", "0", "40.00", 2)}, nil)

	require.NoError(t, m.uc.verifyAccountClosingBalancePersistence(context.Background(), closingOrgID, closingLedgerID,
		[]accountClosingBalanceState{{Persisted: companion}}))
}

// TestAccountClosingPersistenceRefusesAnUnreadableTrail proves an unavailable
// dependency is refused technically instead of being read as "nothing pending".
func TestAccountClosingPersistenceRefusesAnUnreadableTrail(t *testing.T) {
	m := newClosingBalanceMocks(t)

	m.operation.EXPECT().ListLatestByBalances(gomock.Any(), closingOrgID, closingLedgerID, gomock.Any()).
		Return(nil, errors.New("database unavailable"))

	err := m.uc.verifyAccountClosingBalancePersistence(context.Background(), closingOrgID, closingLedgerID,
		[]accountClosingBalanceState{{Persisted: closingBalance(uuid.NewString(), constant.DefaultBalanceKey, "0", "0", "0", 1)}})

	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
}
