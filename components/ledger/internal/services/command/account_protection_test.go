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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	midazpkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

var (
	protectionOrgID     = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000001")
	protectionLedgerID  = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000002")
	protectionAccountID = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000003")
	protectionClosedAt  = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
)

type protectionMocks struct {
	uc      *UseCase
	balance *balance.MockRepository
	account *account.MockRepository
	redis   *redis.MockRedisRepository
}

func newProtectionMocks(t *testing.T) *protectionMocks {
	t.Helper()

	ctrl := gomock.NewController(t)

	mocks := &protectionMocks{
		balance: balance.NewMockRepository(ctrl),
		account: account.NewMockRepository(ctrl),
		redis:   redis.NewMockRedisRepository(ctrl),
	}

	mocks.uc = &UseCase{
		BalanceRepo:          mocks.balance,
		AccountRepo:          mocks.account,
		TransactionRedisRepo: mocks.redis,
	}

	return mocks
}

// expectOwnershipRoundTrip programs one successful acquisition and its release for
// the account under test.
func (m *protectionMocks) expectOwnershipRoundTrip() {
	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return("", false, nil)
	m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, gomock.Any()).
		Return(true, nil)
	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID, gomock.Any()).
		Return(true, nil)
}

// TestCreateAdditionalBalance_RefusesAClosedAccount covers AS-05: a creation that
// arrives after the closing is refused, and no balance is persisted.
func TestCreateAdditionalBalance_RefusesAClosedAccount(t *testing.T) {
	m := newProtectionMocks(t)
	m.expectOwnershipRoundTrip()

	m.redis.EXPECT().GetAccountClosedMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return(protectionClosedAt, true, nil)

	_, err := m.uc.CreateAdditionalBalance(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID,
		&mmodel.CreateAdditionalBalance{Key: "savings"})

	require.Error(t, err)

	var unprocessable midazpkg.UnprocessableOperationError
	require.True(t, errors.As(err, &unprocessable))
	assert.Equal(t, constant.ErrAccountIneligibility.Error(), unprocessable.Code)
}

// TestCreateAdditionalBalance_RefusesWhileAClosingOwnsTheAccount proves the
// creation and the closing coordinate through the same ownership, so a closing in
// flight validates a balance list that cannot grow under it.
func TestCreateAdditionalBalance_RefusesWhileAClosingOwnsTheAccount(t *testing.T) {
	m := newProtectionMocks(t)

	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return(uuid.NewString(), true, nil)

	_, err := m.uc.CreateAdditionalBalance(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID,
		&mmodel.CreateAdditionalBalance{Key: "savings"})

	require.Error(t, err)

	var conflict midazpkg.EntityConflictError
	require.True(t, errors.As(err, &conflict))
	assert.Equal(t, constant.ErrAccountClosingInProgress.Error(), conflict.Code)
}

// TestCreateAdditionalBalance_RejectedBeforeProtection proves the reserved-key
// guard still answers first: a payload that can never be valid costs no ownership.
func TestCreateAdditionalBalance_RejectedBeforeProtection(t *testing.T) {
	m := newProtectionMocks(t)

	_, err := m.uc.CreateAdditionalBalance(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID,
		&mmodel.CreateAdditionalBalance{Key: constant.OverdraftBalanceKey})

	require.Error(t, err)
}

// TestDeleteAllBalancesByAccountID_RefusedWhileAClosingOwnsTheAccount proves the
// delete takes the same ownership, so it cannot change the balance list a closing
// is validating.
func TestDeleteAllBalancesByAccountID_RefusedWhileAClosingOwnsTheAccount(t *testing.T) {
	m := newProtectionMocks(t)

	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return(uuid.NewString(), true, nil)

	err := m.uc.DeleteAllBalancesByAccountID(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID, uuid.NewString())

	require.Error(t, err)

	var conflict midazpkg.EntityConflictError
	require.True(t, errors.As(err, &conflict))
	assert.Equal(t, constant.ErrAccountClosingInProgress.Error(), conflict.Code)
}

// TestDeleteAllBalancesByAccountID_ReleasesOwnershipWhenThereIsNothingToDelete
// proves the ownership is given back as soon as the operation's result is known.
func TestDeleteAllBalancesByAccountID_ReleasesOwnershipWhenThereIsNothingToDelete(t *testing.T) {
	m := newProtectionMocks(t)
	m.expectOwnershipRoundTrip()

	m.balance.EXPECT().ListByAccountID(gomock.Any(), protectionOrgID, protectionLedgerID, protectionAccountID).
		Return([]*mmodel.Balance{}, nil)

	require.NoError(t, m.uc.DeleteAllBalancesByAccountID(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID, uuid.NewString()))
}

// TestAccountProtection_WithoutRepositoriesIsInert proves a use case wired without
// the protection surface behaves exactly as before.
func TestAccountProtection_WithoutRepositoriesIsInert(t *testing.T) {
	uc := &UseCase{}

	admission, err := uc.acquireAccountAdmission(context.Background(), protectionOrgID, protectionLedgerID, protectionAccountID)
	require.NoError(t, err)

	admission.Release(context.Background())

	require.NoError(t, uc.ensureAccountsNotClosed(context.Background(), protectionOrgID, protectionLedgerID,
		constant.ErrAccountIneligibility, protectionAccountID))
}
