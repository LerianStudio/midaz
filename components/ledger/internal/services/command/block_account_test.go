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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// blockStateFixture wires one isolated UseCase per subtest so every case owns
// its own gomock controller and can run under t.Parallel. Sharing a controller
// across parallel subtests races on the expectation set.
type blockStateFixture struct {
	t           *testing.T
	uc          *UseCase
	accountRepo *account.MockRepository
	balanceRepo *balance.MockRepository
	redisRepo   *redis.MockRedisRepository
	emitter     *pkgStreaming.MockEmitter

	organizationID uuid.UUID
	ledgerID       uuid.UUID
	accountID      uuid.UUID
}

func newBlockStateFixture(t *testing.T) *blockStateFixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	f := &blockStateFixture{
		t:              t,
		accountRepo:    account.NewMockRepository(ctrl),
		balanceRepo:    balance.NewMockRepository(ctrl),
		redisRepo:      redis.NewMockRedisRepository(ctrl),
		emitter:        pkgStreaming.NewMockEmitter(),
		organizationID: uuid.New(),
		ledgerID:       uuid.New(),
		accountID:      uuid.New(),
	}

	f.uc = &UseCase{
		AccountRepo:          f.accountRepo,
		BalanceRepo:          f.balanceRepo,
		TransactionRedisRepo: f.redisRepo,
		Streaming:            f.emitter,
	}

	return f
}

// accountWithBlocked builds the pre-state record AccountRepo.Find returns.
// A nil blocked models a legacy row where the column was never written.
func (f *blockStateFixture) accountWithBlocked(blocked *bool) *mmodel.Account {
	return &mmodel.Account{
		ID:             f.accountID.String(),
		OrganizationID: f.organizationID.String(),
		LedgerID:       f.ledgerID.String(),
		Name:           "Blockable Account",
		Type:           "deposit",
		AssetCode:      "USD",
		Blocked:        blocked,
		UpdatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// externalAccount builds the pre-state record for an account of type external.
// Those accounts are the traceability anchor for value entering and leaving the
// ledger, so mutating them is forbidden on every command path.
func (f *blockStateFixture) externalAccount() *mmodel.Account {
	acc := f.accountWithBlocked(boolPtr(false))
	acc.Type = "external"
	acc.Name = "External Account"

	return acc
}

// balancesOfAccount returns two balances on distinct keys so the cache
// invalidation assertion can prove BOTH keys travel in the same
// SetAccountBlockedMany call.
func (f *blockStateFixture) balancesOfAccount() []*mmodel.Balance {
	return []*mmodel.Balance{
		{
			ID:        uuid.New().String(),
			AccountID: f.accountID.String(),
			Alias:     "@blockable",
			Key:       "default",
		},
		{
			ID:        uuid.New().String(),
			AccountID: f.accountID.String(),
			Alias:     "@blockable",
			Key:       "savings",
		},
	}
}

func (f *blockStateFixture) expectedCacheKeys() []string {
	balances := f.balancesOfAccount()
	keys := make([]string, 0, len(balances))

	for _, b := range balances {
		keys = append(keys, utils.BalanceInternalKey(f.organizationID, f.ledgerID, b.Alias+"#"+b.Key))
	}

	return keys
}

// expectFind arms the pre-state read on the account, which is the first step of
// the block state transition and the source of the 404 semantics.
func (f *blockStateFixture) expectFind(acc *mmodel.Account, err error) {
	f.accountRepo.EXPECT().
		Find(gomock.Any(), f.organizationID, f.ledgerID, nil, f.accountID, mmodel.HolderOffV1).
		Return(acc, err).
		Times(1)
}

// expectUpdate arms the source-of-truth write. blocked is asserted on the
// account payload so a command that forgets to set it fails loudly.
func (f *blockStateFixture) expectUpdate(blocked bool, err error) {
	f.accountRepo.EXPECT().
		Update(gomock.Any(), f.organizationID, f.ledgerID, nil, f.accountID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error) {
			require.NotNil(f.t, acc, "Update must receive the account payload")
			require.NotNil(f.t, acc.Blocked, "Update must carry the new block state")
			assert.Equal(f.t, blocked, *acc.Blocked, "Update must carry the requested block state")

			if err != nil {
				return nil, err
			}

			out := *acc
			out.UpdatedAt = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

			return &out, nil
		}).
		Times(1)
}

func (f *blockStateFixture) expectPropagate(blocked bool, err error) {
	f.balanceRepo.EXPECT().
		UpdateAccountBlockedByAccountID(gomock.Any(), f.organizationID, f.ledgerID, f.accountID, blocked).
		Return(err).
		Times(1)
}

func (f *blockStateFixture) expectListBalances(balances []*mmodel.Balance, err error) {
	f.balanceRepo.EXPECT().
		ListByAccountID(gomock.Any(), f.organizationID, f.ledgerID, f.accountID).
		Return(balances, err).
		Times(1)
}

func (f *blockStateFixture) expectSetAccountBlocked(keys []string, blocked bool, err error) {
	f.redisRepo.EXPECT().
		SetAccountBlockedMany(gomock.Any(), keys, blocked).
		Return(err).
		Times(1)
}

// TestBlockAccount covers the full F1 sequence of the block direction: read,
// idempotent short-circuit that still re-propagates, source-of-truth write,
// balance propagation and invalidate-first cache eviction. Every failure after
// the read must surface to the caller so the operator retries.
func TestBlockAccount(t *testing.T) {
	t.Parallel()

	propagationErr := errors.New("propagation exploded")
	cacheErr := errors.New("redis unavailable")

	tests := []struct {
		name        string
		setup       func(f *blockStateFixture)
		expectErr   bool
		errContains string
		errCode     string
		assertOK    func(t *testing.T, f *blockStateFixture, acc *mmodel.Account)
	}{
		{
			name: "success - unblocked account transitions to blocked and propagates",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
				f.expectUpdate(true, nil)
				f.expectPropagate(true, nil)
				f.expectListBalances(f.balancesOfAccount(), nil)
				f.expectSetAccountBlocked(f.expectedCacheKeys(), true, nil)
			},
			assertOK: func(t *testing.T, f *blockStateFixture, acc *mmodel.Account) {
				require.NotNil(t, acc)
				require.NotNil(t, acc.Blocked)
				assert.True(t, *acc.Blocked, "returned account must carry the new block state")
				assert.Equal(t, f.accountID.String(), acc.ID)
				assert.Equal(t, "Blockable Account", acc.Name, "untouched fields must survive the merge")
			},
		},
		{
			name: "success - legacy account with nil blocked transitions to blocked",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.accountWithBlocked(nil), nil)
				f.expectUpdate(true, nil)
				f.expectPropagate(true, nil)
				f.expectListBalances(f.balancesOfAccount(), nil)
				f.expectSetAccountBlocked(f.expectedCacheKeys(), true, nil)
			},
			assertOK: func(t *testing.T, f *blockStateFixture, acc *mmodel.Account) {
				require.NotNil(t, acc)
				require.NotNil(t, acc.Blocked)
				assert.True(t, *acc.Blocked)
			},
		},
		{
			name: "idempotent no-op - already blocked still re-propagates and re-invalidates",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.accountWithBlocked(boolPtr(true)), nil)
				// No AccountRepo.Update: the source of truth already holds the
				// target state. Propagation and eviction MUST still run so a
				// retry after a partial failure converges.
				f.expectPropagate(true, nil)
				f.expectListBalances(f.balancesOfAccount(), nil)
				f.expectSetAccountBlocked(f.expectedCacheKeys(), true, nil)
			},
			assertOK: func(t *testing.T, f *blockStateFixture, acc *mmodel.Account) {
				require.NotNil(t, acc)
				require.NotNil(t, acc.Blocked)
				assert.True(t, *acc.Blocked)
			},
		},
		{
			name: "success - account with no balances still succeeds",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
				f.expectUpdate(true, nil)
				f.expectPropagate(true, nil)
				f.expectListBalances([]*mmodel.Balance{}, nil)
				// No DEL is issued for an empty key set.
			},
			assertOK: func(t *testing.T, f *blockStateFixture, acc *mmodel.Account) {
				require.NotNil(t, acc)
				require.NotNil(t, acc.Blocked)
				assert.True(t, *acc.Blocked)
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
			name: "failure - blocking an external account is forbidden",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.externalAccount(), nil)
				// Nothing is written, propagated or evicted: the guard fires
				// straight after the load.
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
			name: "failure - source-of-truth update rejects with not found",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
				f.expectUpdate(true, services.ErrDatabaseItemNotFound)
			},
			expectErr: true,
			errCode:   "0052",
		},
		{
			name: "failure - source-of-truth update fails technically",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
				f.expectUpdate(true, errors.New("connection reset by peer"))
				// Nothing propagates: the source of truth never moved.
			},
			expectErr:   true,
			errContains: "connection reset by peer",
		},
		{
			name: "failure - propagation to balances errors without confirming",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
				f.expectUpdate(true, nil)
				f.expectPropagate(true, propagationErr)
				// Cache is never touched: the read model did not converge.
			},
			expectErr:   true,
			errContains: "propagation exploded",
		},
		{
			name: "failure - listing balances for invalidation errors",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
				f.expectUpdate(true, nil)
				f.expectPropagate(true, nil)
				f.expectListBalances(nil, errors.New("list balances exploded"))
			},
			expectErr:   true,
			errContains: "list balances exploded",
		},
		{
			name: "failure - cache invalidation error is returned, never swallowed",
			setup: func(f *blockStateFixture) {
				f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
				f.expectUpdate(true, nil)
				f.expectPropagate(true, nil)
				f.expectListBalances(f.balancesOfAccount(), nil)
				f.expectSetAccountBlocked(f.expectedCacheKeys(), true, cacheErr)
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

			got, err := f.uc.BlockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)

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

// TestBlockAccount_EmitsAccountUpdatedEvent proves the audit trail: a
// successful block publishes exactly one account.updated carrying blocked=true.
func TestBlockAccount_EmitsAccountUpdatedEvent(t *testing.T) {
	t.Parallel()

	f := newBlockStateFixture(t)
	f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
	f.expectUpdate(true, nil)
	f.expectPropagate(true, nil)
	f.expectListBalances(f.balancesOfAccount(), nil)
	f.expectSetAccountBlocked(f.expectedCacheKeys(), true, nil)

	_, err := f.uc.BlockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)
	require.NoError(t, err)

	events := f.emitter.Events()
	require.Len(t, events, 1, "a block must publish exactly one audit event")

	pkgStreaming.AssertEventEmitted(t, f.emitter, "account", "updated")
	assert.Equal(t, f.accountID.String(), events[0].Subject)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(events[0].Payload, &payload))
	assert.Equal(t, true, payload["blocked"], "the emitted state must be the post-block state")
}

// TestBlockAccount_EmitsAuditEventOnIdempotentRetry pins the convergence
// contract: when a previous attempt persisted the account but died before
// propagating, the retry short-circuits the write yet MUST still emit, or the
// operator action would never reach the audit stream.
func TestBlockAccount_EmitsAuditEventOnIdempotentRetry(t *testing.T) {
	t.Parallel()

	f := newBlockStateFixture(t)
	f.expectFind(f.accountWithBlocked(boolPtr(true)), nil)
	f.expectPropagate(true, nil)
	f.expectListBalances(f.balancesOfAccount(), nil)
	f.expectSetAccountBlocked(f.expectedCacheKeys(), true, nil)

	_, err := f.uc.BlockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)
	require.NoError(t, err)

	events := f.emitter.Events()
	require.Len(t, events, 1, "the converging retry must still reach the audit stream")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(events[0].Payload, &payload))
	assert.Equal(t, true, payload["blocked"])
}

// TestBlockAccount_NoEventWhenPropagationFails guards the ordering rule: the
// event is the LAST step, so a failed propagation must not leave an audit
// record claiming a state the read model never reached.
func TestBlockAccount_NoEventWhenPropagationFails(t *testing.T) {
	t.Parallel()

	f := newBlockStateFixture(t)
	f.expectFind(f.accountWithBlocked(boolPtr(false)), nil)
	f.expectUpdate(true, nil)
	f.expectPropagate(true, errors.New("propagation exploded"))

	_, err := f.uc.BlockAccount(context.Background(), f.organizationID, f.ledgerID, f.accountID, mmodel.HolderOffV1)
	require.Error(t, err)

	assert.Empty(t, f.emitter.Events(), "no audit event may claim a state the balances never reached")
}
