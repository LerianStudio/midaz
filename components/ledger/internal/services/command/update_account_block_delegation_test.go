// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// The legacy PATCH /accounts blocked field used to write the account row and
// stop there, leaving every balance of the account carrying the old projection
// and every cached balance serving it. It now delegates to the same
// setAccountBlockState helper the dedicated block/unblock endpoints use, so
// there is exactly one code path with one set of convergence guarantees.

// patchBlockFixture wires the UpdateAccount surface plus the propagation and
// cache-eviction ports the delegation reaches.
type patchBlockFixture struct {
	t           *testing.T
	uc          *UseCase
	accountRepo *account.MockRepository
	balanceRepo *balance.MockRepository
	redisRepo   *redis.MockRedisRepository
	emitter     *pkgStreaming.MockEmitter

	organizationID uuid.UUID
	ledgerID       uuid.UUID
	accountID      uuid.UUID

	mu            sync.Mutex
	blockedWrites []bool
	plainWrites   int
}

func newPatchBlockFixture(t *testing.T, current *bool) *patchBlockFixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	f := &patchBlockFixture{
		t:              t,
		accountRepo:    account.NewMockRepository(ctrl),
		balanceRepo:    balance.NewMockRepository(ctrl),
		redisRepo:      redis.NewMockRedisRepository(ctrl),
		emitter:        pkgStreaming.NewMockEmitter(),
		organizationID: uuid.New(),
		ledgerID:       uuid.New(),
		accountID:      uuid.New(),
	}

	metadataRepo := mongodb.NewMockRepository(ctrl)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mongodb.Metadata{Data: map[string]any{}}, nil).AnyTimes()
	metadataRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	pre := &mmodel.Account{
		ID:             f.accountID.String(),
		OrganizationID: f.organizationID.String(),
		LedgerID:       f.ledgerID.String(),
		Name:           "Patchable Account",
		Type:           "deposit",
		AssetCode:      "USD",
		Blocked:        current,
		UpdatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	f.accountRepo.EXPECT().
		Find(gomock.Any(), f.organizationID, f.ledgerID, nil, f.accountID, mmodel.HolderOffV1).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, _ mmodel.HolderPolicy) (*mmodel.Account, error) {
			out := *pre
			return &out, nil
		}).AnyTimes()

	f.accountRepo.EXPECT().
		Update(gomock.Any(), f.organizationID, f.ledgerID, gomock.Any(), f.accountID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, in *mmodel.Account) (*mmodel.Account, error) {
			f.mu.Lock()
			defer f.mu.Unlock()

			if in.Blocked != nil {
				f.blockedWrites = append(f.blockedWrites, *in.Blocked)
			} else {
				f.plainWrites++
			}

			out := *in
			out.UpdatedAt = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

			return &out, nil
		}).AnyTimes()

	f.uc = &UseCase{
		AccountRepo:            f.accountRepo,
		BalanceRepo:            f.balanceRepo,
		TransactionRedisRepo:   f.redisRepo,
		OnboardingMetadataRepo: metadataRepo,
		Streaming:              f.emitter,
	}

	return f
}

// expectPropagation arms the balance-wide projection UPDATE and the atomic
// multi-key cache eviction the helper performs.
func (f *patchBlockFixture) expectPropagation(blocked bool) {
	balances := []*mmodel.Balance{
		{ID: uuid.New().String(), AccountID: f.accountID.String(), Alias: "@patchable", Key: "default"},
		{ID: uuid.New().String(), AccountID: f.accountID.String(), Alias: "@patchable", Key: "savings"},
	}

	keys := make([]string, 0, len(balances))
	for _, b := range balances {
		keys = append(keys, utils.BalanceInternalKey(f.organizationID, f.ledgerID, b.Alias+"#"+b.Key))
	}

	f.balanceRepo.EXPECT().
		UpdateAccountBlockedByAccountID(gomock.Any(), f.organizationID, f.ledgerID, f.accountID, blocked).
		Return(nil).
		Times(1)
	f.balanceRepo.EXPECT().
		ListByAccountID(gomock.Any(), f.organizationID, f.ledgerID, f.accountID).
		Return(balances, nil).
		Times(1)
	f.redisRepo.EXPECT().
		SetAccountBlockedMany(gomock.Any(), keys, blocked).
		Return(nil).
		Times(1)
}

// TestUpdateAccount_BlockedFieldDelegatesToBlockStatePath proves the PATCH now
// travels the dedicated helper: the account row moves, every balance is
// realigned and every cached balance key has its AccountBlocked flag flipped in
// one atomic mutation.
func TestUpdateAccount_BlockedFieldDelegatesToBlockStatePath(t *testing.T) {
	t.Parallel()

	f := newPatchBlockFixture(t, boolPtr(false))
	f.expectPropagation(true)

	updated, err := f.uc.UpdateAccount(context.Background(), f.organizationID, f.ledgerID, nil, f.accountID,
		&mmodel.UpdateAccountInput{Name: "Renamed", Blocked: boolPtr(true)}, mmodel.HolderOffV1)

	require.NoError(t, err)
	require.NotNil(t, updated)

	f.mu.Lock()
	defer f.mu.Unlock()

	require.Len(t, f.blockedWrites, 1, "the block transition must be written exactly once, by the shared helper")
	assert.True(t, f.blockedWrites[0])

	assert.Len(t, f.emitter.Events(), 1, "a single PATCH must not emit account.updated twice")
}

// TestUpdateAccount_WithoutBlockedDoesNotPropagate pins the negative: a PATCH
// that does not carry blocked must not touch the balance projection or the
// cache. The block mechanism is opt-in per request.
func TestUpdateAccount_WithoutBlockedDoesNotPropagate(t *testing.T) {
	t.Parallel()

	f := newPatchBlockFixture(t, boolPtr(false))

	// No propagation expectations are armed: any call to
	// UpdateAccountBlockedByAccountID, ListByAccountID or SetAccountBlockedMany
	// fails the gomock controller.

	updated, err := f.uc.UpdateAccount(context.Background(), f.organizationID, f.ledgerID, nil, f.accountID,
		&mmodel.UpdateAccountInput{Name: "Renamed only"}, mmodel.HolderOffV1)

	require.NoError(t, err)
	require.NotNil(t, updated)

	f.mu.Lock()
	defer f.mu.Unlock()

	assert.Empty(t, f.blockedWrites, "no blocked write may be issued when the PATCH omits the field")
	assert.Len(t, f.emitter.Events(), 1)
}

// TestUpdateAccount_BlockedEqualToCurrentBehavesAsHelperNoOp locks the helper's
// convergence semantics onto the PATCH: the source-of-truth write is skipped,
// but propagation and eviction still run so a retry after a partially failed
// attempt converges.
func TestUpdateAccount_BlockedEqualToCurrentBehavesAsHelperNoOp(t *testing.T) {
	t.Parallel()

	f := newPatchBlockFixture(t, boolPtr(true))
	f.expectPropagation(true)

	updated, err := f.uc.UpdateAccount(context.Background(), f.organizationID, f.ledgerID, nil, f.accountID,
		&mmodel.UpdateAccountInput{Blocked: boolPtr(true)}, mmodel.HolderOffV1)

	require.NoError(t, err)
	require.NotNil(t, updated)

	f.mu.Lock()
	defer f.mu.Unlock()

	assert.Empty(t, f.blockedWrites, "the account already holds the target state, so no blocked write is issued")
	assert.Len(t, f.emitter.Events(), 1)
}
