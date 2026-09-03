// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// TestUnblockAccount mirrors TestBlockAccount in the opposite direction. The two
// commands share one state-transition helper, so the value carried through
// AccountRepo.Update and BalanceRepo.UpdateAccountBlockedByAccountID is the only
// thing that differs — and it is asserted on every arm.
func TestUnblockAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(f *blockStateFixture)
		expectErr   bool
		errContains string
		errCode     string
		assertOK    func(t *testing.T, f *blockStateFixture, acc *mmodel.Account)
	}{
		{
			name: "success - blocked account transitions to unblocked and propagates",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.accountWithBlocked(boolPtr(true)), nil)
				f.expectUpdate(false, nil)
				f.expectPropagate(false, nil)
				f.expectListBalances(f.balancesOfAccount(), nil)
				f.expectSetAccountBlocked(f.expectedCacheKeys(), false, nil)
			},
			assertOK: func(t *testing.T, f *blockStateFixture, acc *mmodel.Account) {
				require.NotNil(t, acc)
				require.NotNil(t, acc.Blocked)
				assert.False(t, *acc.Blocked, "returned account must carry the cleared block state")
				assert.Equal(t, f.accountID.String(), acc.ID)
			},
		},
		{
			name: "success - legacy account with nil blocked is written explicitly false",
			setup: func(f *blockStateFixture) {
				// A nil pre-state is NOT equal to false: the column was never
				// written, so the source of truth must be made explicit rather
				// than short-circuited.
				f.expectFind(f.accountWithBlocked(nil), nil)
				f.expectUpdate(false, nil)
				f.expectPropagate(false, nil)
				f.expectListBalances(f.balancesOfAccount(), nil)
				f.expectSetAccountBlocked(f.expectedCacheKeys(), false, nil)
			},
			assertOK: func(t *testing.T, f *blockStateFixture, acc *mmodel.Account) {
				require.NotNil(t, acc)
				require.NotNil(t, acc.Blocked)
				assert.False(t, *acc.Blocked)
			},
		},
		{
			name: "idempotent no-op - already unblocked still re-propagates and re-invalidates",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
				f.expectPropagate(false, nil)
				f.expectListBalances(f.balancesOfAccount(), nil)
				f.expectSetAccountBlocked(f.expectedCacheKeys(), false, nil)
			},
			assertOK: func(t *testing.T, f *blockStateFixture, acc *mmodel.Account) {
				require.NotNil(t, acc)
				require.NotNil(t, acc.Blocked)
				assert.False(t, *acc.Blocked)
			},
		},
		{
			name: "failure - account does not exist",
			setup: func(f *blockStateFixture) {
				f.expectFind(nil, nil)
			},
			expectErr: true,
			errCode:   "0052",
		},
		{
			name: "failure - unblocking an external account is forbidden",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.externalAccount(), nil)
			},
			expectErr:   true,
			errContains: "0074",
		},
		{
			name: "failure - repository reports the account row is gone",
			setup: func(f *blockStateFixture) {
				f.expectFind(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr:   true,
			errContains: "errDatabaseItemNotFound",
		},
		{
			name: "failure - propagation to balances errors without confirming",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.accountWithBlocked(boolPtr(true)), nil)
				f.expectUpdate(false, nil)
				f.expectPropagate(false, errors.New("propagation exploded"))
			},
			expectErr:   true,
			errContains: "propagation exploded",
		},
		{
			name: "failure - cache invalidation error is returned, never swallowed",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.accountWithBlocked(boolPtr(true)), nil)
				f.expectUpdate(false, nil)
				f.expectPropagate(false, nil)
				f.expectListBalances(f.balancesOfAccount(), nil)
				f.expectSetAccountBlocked(f.expectedCacheKeys(), false, errors.New("redis unavailable"))
			},
			expectErr:   true,
			errContains: "redis unavailable",
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newBlockStateFixture(t)
			tc.setup(f)

			got, err := f.uc.UnblockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)

			if tc.expectErr {
				require.Error(t, err)
				assert.Nil(t, got, "no account may be confirmed to the operator on failure")

				if tc.errCode != "" {
					var notFound pkg.EntityNotFoundError

					require.ErrorAs(t, err, &notFound, "a missing account must surface as a 404-mapped business error")
					assert.Equal(t, tc.errCode, notFound.Code, "the account-not-found catalog code must reach the operator")

					return
				}

				assert.Contains(t, err.Error(), tc.errContains)

				return
			}

			require.NoError(t, err)
			tc.assertOK(t, f, got)
		})
	}
}

// TestUnblockAccount_EmitsAccountUpdatedEvent proves the audit trail carries the
// cleared state, not just the fact that something changed.
func TestUnblockAccount_EmitsAccountUpdatedEvent(t *testing.T) {
	t.Parallel()

	f := newBlockStateFixture(t)
	f.expectFind(f.accountWithBlocked(boolPtr(true)), nil)
	f.expectUpdate(false, nil)
	f.expectPropagate(false, nil)
	f.expectListBalances(f.balancesOfAccount(), nil)
	f.expectSetAccountBlocked(f.expectedCacheKeys(), false, nil)

	_, err := f.uc.UnblockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)
	require.NoError(t, err)

	events := f.emitter.Events()
	require.Len(t, events, 1, "an unblock must publish exactly one audit event")

	pkgStreaming.AssertEventEmitted(t, f.emitter, "account", "updated")
	assert.Equal(t, f.accountID.String(), events[0].Subject)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(events[0].Payload, &payload))
	assert.Equal(t, false, payload["blocked"], "the emitted state must be the post-unblock state")
}

// TestBlockUnblockAccount_ConvergesUnderRetrySequence exercises the TRD F1
// failure mode end to end: propagation dies mid-flight, the operator retries the
// same call, and the second attempt converges account, balances and cache
// without a second source-of-truth write.
func TestBlockUnblockAccount_ConvergesUnderRetrySequence(t *testing.T) {
	t.Parallel()

	// Attempt 1: account row is written, propagation fails.
	first := newBlockStateFixture(t)
	first.expectFind(first.accountWithBlocked(boolPtr(false)), nil)
	first.expectUpdate(true, nil)
	first.expectPropagate(true, errors.New("propagation exploded"))

	_, err := first.uc.BlockAccount(context.Background(), first.organizationID, first.ledgerID, first.accountID, mmodel.HolderOffV1)
	require.Error(t, err, "a partial propagation must never be confirmed to the operator")

	// Attempt 2: the account already holds the target state; the retry must
	// still drive the balances and the cache to convergence.
	second := newBlockStateFixture(t)
	second.expectFind(second.accountWithBlocked(boolPtr(true)), nil)
	second.expectPropagate(true, nil)
	second.expectListBalances(second.balancesOfAccount(), nil)
	second.expectSetAccountBlocked(second.expectedCacheKeys(), true, nil)

	acc, err := second.uc.BlockAccount(context.Background(), second.organizationID, second.ledgerID, second.accountID, mmodel.HolderOffV1)
	require.NoError(t, err)
	require.NotNil(t, acc.Blocked)
	assert.True(t, *acc.Blocked)
}
