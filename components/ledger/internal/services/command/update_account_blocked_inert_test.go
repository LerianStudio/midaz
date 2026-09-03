// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
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
)

// =============================================================================
// PATCH /accounts — THE blocked FIELD IS INERT
// =============================================================================
// The blocked field on UpdateAccountInput is retained ONLY so clients that still
// send it keep getting a 200 (the body decoder rejects unknown keys, so deleting
// the field would turn `"blocked": true` into a 400). It carries no effect.
//
// That is a security property, not a cosmetic one: PATCH is governed by the
// ("accounts", "patch") authz tuple, while block/unblock are governed by the
// dedicated ("account-blocks", "post") tuple. An effectful blocked field on the
// PATCH body would let an operator granted only account edit rights freeze — or
// worse, release — an account they were never granted power over.
//
// So these tests assert ABSENCE: no durable block write, no enforcement-index
// write, no legacy projection write, in either direction.

// patchBlockFixture wires the UpdateAccount surface plus every port a block
// transition WOULD reach. All of them are left unarmed on purpose: the gomock
// controller fails the test if the update path touches any of them.
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

// emittedBlocked reads the blocked value the single account.updated carried.
func (f *patchBlockFixture) emittedBlocked() any {
	f.t.Helper()

	events := f.emitter.Events()
	require.Len(f.t, events, 1, "a PATCH must publish exactly one account.updated")

	var payload map[string]any
	require.NoError(f.t, json.Unmarshal(events[0].Payload, &payload))

	return payload["blocked"]
}

// TestUpdateAccount_BlockedFieldCannotBlockAnAccount is the RBAC-bypass closure:
// a PATCH carrying blocked=true on an unblocked account must leave it unblocked
// everywhere — the account row, the enforcement index and the legacy projection.
func TestUpdateAccount_BlockedFieldCannotBlockAnAccount(t *testing.T) {
	t.Parallel()

	f := newPatchBlockFixture(t, boolPtr(false))

	// Nothing is armed on redisRepo or balanceRepo: any AddBlockedAccount,
	// UpdateAccountBlockedByAccountID, ListByAccountID or SetAccountBlockedMany
	// call fails the gomock controller.

	updated, err := f.uc.UpdateAccount(context.Background(), f.organizationID, f.ledgerID, nil, f.accountID,
		&mmodel.UpdateAccountInput{Name: "Renamed", Blocked: boolPtr(true)}, mmodel.HolderOffV1)

	require.NoError(t, err, "a client still sending the retired field must not be rejected")
	require.NotNil(t, updated)

	f.mu.Lock()
	defer f.mu.Unlock()

	assert.Empty(t, f.blockedWrites, "the retired field must never reach the account row")
	assert.Equal(t, 1, f.plainWrites, "the generic field update must still happen")

	require.NotNil(t, updated.Blocked)
	assert.False(t, *updated.Blocked, "the response must report the UNCHANGED block state")
	assert.Equal(t, false, f.emittedBlocked(), "the audit event must not claim a block that did not happen")
}

// TestUpdateAccount_BlockedFieldCannotUnblockAnAccount is the dangerous
// direction: releasing an account is exactly what the dedicated authz tuple
// exists to gate, so a PATCH must never be able to do it.
func TestUpdateAccount_BlockedFieldCannotUnblockAnAccount(t *testing.T) {
	t.Parallel()

	f := newPatchBlockFixture(t, boolPtr(true))

	updated, err := f.uc.UpdateAccount(context.Background(), f.organizationID, f.ledgerID, nil, f.accountID,
		&mmodel.UpdateAccountInput{Name: "Renamed", Blocked: boolPtr(false)}, mmodel.HolderOffV1)

	require.NoError(t, err)
	require.NotNil(t, updated)

	f.mu.Lock()
	defer f.mu.Unlock()

	assert.Empty(t, f.blockedWrites, "no PATCH may release a blocked account")

	require.NotNil(t, updated.Blocked)
	assert.True(t, *updated.Blocked, "the account must still be reported as blocked")
	assert.Equal(t, true, f.emittedBlocked())
}

// TestUpdateAccount_BlockedFieldEqualToCurrentStateIsAlsoInert covers the arm
// where the requested value matches reality: still no write, still no
// propagation. There is no "harmless" branch that reaches the block path.
func TestUpdateAccount_BlockedFieldEqualToCurrentStateIsAlsoInert(t *testing.T) {
	t.Parallel()

	f := newPatchBlockFixture(t, boolPtr(true))

	updated, err := f.uc.UpdateAccount(context.Background(), f.organizationID, f.ledgerID, nil, f.accountID,
		&mmodel.UpdateAccountInput{Blocked: boolPtr(true)}, mmodel.HolderOffV1)

	require.NoError(t, err)
	require.NotNil(t, updated)

	f.mu.Lock()
	defer f.mu.Unlock()

	assert.Empty(t, f.blockedWrites)
	assert.Equal(t, 1, f.plainWrites)
}

// TestUpdateAccount_WithoutBlockedDoesNotPropagate pins the baseline: a PATCH
// that omits the field behaves identically to one that carries it, which is what
// "inert" means.
func TestUpdateAccount_WithoutBlockedDoesNotPropagate(t *testing.T) {
	t.Parallel()

	f := newPatchBlockFixture(t, boolPtr(false))

	updated, err := f.uc.UpdateAccount(context.Background(), f.organizationID, f.ledgerID, nil, f.accountID,
		&mmodel.UpdateAccountInput{Name: "Renamed only"}, mmodel.HolderOffV1)

	require.NoError(t, err)
	require.NotNil(t, updated)

	f.mu.Lock()
	defer f.mu.Unlock()

	assert.Empty(t, f.blockedWrites)
	assert.Len(t, f.emitter.Events(), 1)
}
