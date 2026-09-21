// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// TestCloseAccount_CleansUpAfterACancelledRequest covers AS-19: the cleanup runs
// on a context decoupled from the request, because a cancelled request is one of
// the reasons a marker exists — inheriting that cancellation would abandon exactly
// the protection the cleanup was installed to release.
func TestCloseAccount_CleansUpAfterACancelledRequest(t *testing.T) {
	m := newCloseAccountMocks(t)

	ctx, cancel := context.WithCancel(context.Background())

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.expectProtectionTaken()

	m.balance.EXPECT().ListByAccountID(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) ([]*mmodel.Balance, error) {
			cancel()

			return nil, context.Canceled
		})

	m.redis.EXPECT().ReleaseAccountClosingAttempt(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		DoAndReturn(func(cleanupCtx context.Context, _, _, _ uuid.UUID, _ string) (bool, error) {
			require.NoError(t, cleanupCtx.Err(), "the cleanup context must not inherit the request cancellation")

			return true, nil
		})
	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		DoAndReturn(func(cleanupCtx context.Context, _, _, _ uuid.UUID, _ string) (bool, error) {
			require.NoError(t, cleanupCtx.Err(), "the ownership release must not inherit the request cancellation")

			return true, nil
		})

	closedAt, err := m.uc.CloseAccount(ctx, closeOrgID, closeLedgerID, closeAccountID)

	require.Error(t, err)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_CleanupFailureDoesNotMaskTheRefusal covers AS-11: a cleanup
// that could not run leaves its keys for reconciliation, and the caller still
// receives the refusal that brought the attempt there.
func TestCloseAccount_CleanupFailureDoesNotMaskTheRefusal(t *testing.T) {
	m := newCloseAccountMocks(t)

	residual := closeEligibleBalance()
	residual.Available = decimal.RequireFromString("0.01")

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.expectProtectionTaken()
	m.expectBalancesRead(residual)

	m.redis.EXPECT().ReleaseAccountClosingAttempt(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(false, errors.New("cache unavailable"))
	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(false, errors.New("cache unavailable"))

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	requireClosingCode(t, err, constant.ErrAccountBalanceNotZero)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_CleanupRemovesOnlyItsOwnProtection covers AS-14: the removal
// carries the attempt's own token, so a marker or ownership another attempt
// installed is reported as not owned and left exactly where it is.
func TestCloseAccount_CleanupRemovesOnlyItsOwnProtection(t *testing.T) {
	m := newCloseAccountMocks(t)

	var token string

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)

	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return("", false, nil)
	m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, acquired string) (bool, error) {
			token = acquired

			return true, nil
		})
	m.redis.EXPECT().AcquireAccountClosingMarker(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		Return(true, nil)

	m.balance.EXPECT().ListByAccountID(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).
		Return(nil, errors.New("database unavailable"))

	m.redis.EXPECT().ReleaseAccountClosingAttempt(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, released string) (bool, error) {
			assert.Equal(t, token, released)

			// A foreign marker answers exactly like this, and the cleanup treats it as
			// nothing to do rather than as a failure.
			return false, nil
		})
	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, released string) (bool, error) {
			assert.Equal(t, token, released)

			return false, nil
		})

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	require.Error(t, err)
	assert.True(t, closedAt.IsZero())
}

// TestCloseAccount_RegularizationClosesAfterARefusal covers AS-11 end to end: the
// refused attempt left nothing behind, so once the residual is settled the very
// next attempt takes the protection again and closes.
func TestCloseAccount_RegularizationClosesAfterARefusal(t *testing.T) {
	m := newCloseAccountMocks(t)

	residual := closeEligibleBalance()
	residual.Available = decimal.RequireFromString("0.01")

	m.expectAccountRead(closeAccountEntity("deposit", nil), nil)
	m.expectProtectionTaken()
	m.expectBalancesRead(residual)
	m.expectProtectionReleased()

	_, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)
	requireClosingCode(t, err, constant.ErrAccountBalanceNotZero)

	m.expectClosingVerified()
	m.account.EXPECT().CloseAccount(gomock.Any(), closeOrgID, closeLedgerID, closeAccountID).Return(closeInstant, nil)
	m.expectClosingFinalized(closeInstant, 1)

	closedAt, err := m.uc.CloseAccount(context.Background(), closeOrgID, closeLedgerID, closeAccountID)

	require.NoError(t, err)
	assert.Equal(t, closeInstant, closedAt)
}
