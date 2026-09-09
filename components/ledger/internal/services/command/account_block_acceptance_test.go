// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// =============================================================================
// ACCOUNT-BLOCK ACCEPTANCE SUITE — command layer
// =============================================================================
// One named case per PRD acceptance criterion decided by a write use case: the
// block/unblock command (RF-02) and the exception minting command (RF-05, RF-07,
// RF-09). Enforcement criteria live in the mtransaction and redis suites of the
// same name — nothing here decides whether a transaction passes.

// blockPatchFixture wires the four collaborators an account PATCH carrying the
// blocked flag touches. The balance repo and the Redis repo are strict mocks with
// no default expectations, so a use case that reached them when it should not
// fails the test rather than passing quietly.
type blockPatchFixture struct {
	uc       *UseCase
	account  *account.MockRepository
	balance  *balance.MockRepository
	redis    *txRedis.MockRedisRepository
	metadata *mongodb.MockRepository
}

func newBlockPatchFixture(t *testing.T) *blockPatchFixture {
	t.Helper()

	ctrl := gomock.NewController(t)

	accountRepo := account.NewMockRepository(ctrl)
	balanceRepo := balance.NewMockRepository(ctrl)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	metadataRepo.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	metadataRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	return &blockPatchFixture{
		uc: &UseCase{
			AccountRepo:            accountRepo,
			BalanceRepo:            balanceRepo,
			TransactionRedisRepo:   redisRepo,
			OnboardingMetadataRepo: metadataRepo,
		},
		account:  accountRepo,
		balance:  balanceRepo,
		redis:    redisRepo,
		metadata: metadataRepo,
	}
}

// expectStoredBlocked arms the read/write pair of one PATCH, with the account row
// reporting the blocked state it ALREADY had before the patch.
func (f *blockPatchFixture) expectStoredBlocked(accountID uuid.UUID, storedBlocked, patchedBlocked bool) {
	f.account.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), mmodel.HolderOffV1).
		Return(&mmodel.Account{ID: accountID.String(), Type: "internal", Blocked: &storedBlocked}, nil)

	f.account.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Account{ID: accountID.String(), Name: "acc", Blocked: &patchedBlocked}, nil)
}

// TestRF02_UnblockingAnUnblockedAccountIsAnIdempotentNoOp covers the second RF-02
// criterion: the command does not care what the account's previous state was.
// Unblocking one that was never blocked runs the same path, reports success, and
// its only side effect is a cache rewrite that writes the value already there.
//
// Repeating it changes nothing, which is what makes the operation safe to retry
// after a timeout — the caller cannot tell the second call from the first.
func TestRF02_UnblockingAnUnblockedAccountIsAnIdempotentNoOp(t *testing.T) {
	t.Parallel()

	organizationID, ledgerID, accountID := uuid.New(), uuid.New(), uuid.New()

	f := newBlockPatchFixture(t)

	const passes = 2

	f.expectStoredBlocked(accountID, false, false)
	f.expectStoredBlocked(accountID, false, false)

	f.balance.EXPECT().
		ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
		Return([]*mmodel.Balance{{ID: uuid.New().String(), Alias: "@never_blocked", Key: "default"}}, nil).
		Times(passes)

	f.redis.EXPECT().
		UpdateBalanceCacheBlocked(gomock.Any(), organizationID, ledgerID, []string{"@never_blocked#default"}, false).
		Return(nil).
		Times(passes)

	unblock := false

	for pass := range passes {
		result, err := f.uc.UpdateAccount(context.Background(), organizationID, ledgerID, nil, accountID,
			&mmodel.UpdateAccountInput{Blocked: &unblock}, mmodel.HolderOffV1)

		require.NoErrorf(t, err, "unblocking an account that is not blocked must not error (pass %d)", pass)
		require.NotNil(t, result)
		require.NotNil(t, result.Blocked)
		assert.False(t, *result.Blocked, "the account stays unblocked")
	}
}

// TestRF05_MintingExceptionsLeavesAccountAndBalanceStateAlone covers the RF-05
// criterion that registering exceptions changes nothing about the accounts it
// names: it neither blocks nor unblocks them and it does not touch their balances.
//
// The assertion is structural. The use case is handed a BalanceRepo and an
// AccountRepo whose only armed call is the read-only alias lookup, so any write —
// an account Update, any balance call at all — fails the test on an unexpected
// call rather than on an assertion after the fact.
func TestRF05_MintingExceptionsLeavesAccountAndBalanceStateAlone(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	accountRepo := account.NewMockRepository(ctrl)
	balanceRepo := balance.NewMockRepository(ctrl)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	uc := &UseCase{AccountRepo: accountRepo, BalanceRepo: balanceRepo, TransactionRedisRepo: redisRepo}

	organizationID, ledgerID := uuid.New(), uuid.New()
	blocked := true
	alias := "@fraud_account"
	stored := &mmodel.Account{ID: uuid.NewString(), Alias: &alias, Blocked: &blocked}

	accountRepo.EXPECT().
		ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{alias}).
		Return([]*mmodel.Account{stored}, nil)

	redisRepo.EXPECT().
		CreateAccountBlockExceptions(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil)

	out, err := uc.CreateAccountBlockExceptions(context.Background(), organizationID, ledgerID,
		&mmodel.CreateAccountBlockExceptionsInput{
			Exceptions: []mmodel.CreateAccountBlockExceptionInput{{AccountAlias: alias, Amount: "150.00"}},
		})

	require.NoError(t, err)
	require.NotNil(t, out)
	require.Len(t, out.Exceptions, 1)

	require.NotNil(t, stored.Blocked)
	assert.True(t, *stored.Blocked, "minting an exception must not change the account's blocked state")
}

// TestRF07_EachAuthorizedTransactionGetsItsOwnIdentifier covers the
// multi-transaction criterion of RF-07: a batch that authorizes N transactions on
// the SAME account for the SAME amount hands back N DISTINCT identifiers, one per
// transaction, never one identifier reused N times.
//
// Same alias and same amount is the case that would collapse under any
// content-derived identifier scheme, which is exactly why it is the case pinned
// here: single use is only meaningful if two authorizations are two identifiers.
func TestRF07_EachAuthorizedTransactionGetsItsOwnIdentifier(t *testing.T) {
	t.Parallel()

	f := newBlockExceptionFixture(t)

	organizationID, ledgerID := uuid.New(), uuid.New()
	alias := "@fraud_account"

	const authorized = 3

	items := make([]mmodel.CreateAccountBlockExceptionInput, 0, authorized)
	for range authorized {
		items = append(items, mmodel.CreateAccountBlockExceptionInput{AccountAlias: alias, Amount: "150.00"})
	}

	f.account.EXPECT().
		ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{alias}).
		Return([]*mmodel.Account{accountWithAlias(alias)}, nil)

	var written []txRedis.AccountBlockException

	f.redis.EXPECT().
		CreateAccountBlockExceptions(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, exceptions []txRedis.AccountBlockException) error {
			written = exceptions

			return nil
		})

	out, err := f.uc.CreateAccountBlockExceptions(context.Background(), organizationID, ledgerID,
		&mmodel.CreateAccountBlockExceptionsInput{Exceptions: items})

	require.NoError(t, err)
	require.NotNil(t, out)
	require.Len(t, out.Exceptions, authorized, "one identifier per authorized transaction")
	require.Len(t, written, authorized)

	seen := make(map[string]struct{}, authorized)
	for _, minted := range out.Exceptions {
		_, duplicate := seen[minted.AccountBlockExceptionID]
		require.False(t, duplicate, "identifiers must be distinct even for identical items")

		seen[minted.AccountBlockExceptionID] = struct{}{}
	}
}

// TestRF09_DefaultAndCustomValidity covers the two RF-09 criteria the minting
// command decides: an item that declares no validity gets the five-minute
// default, and an item that declares one gets it verbatim.
func TestRF09_DefaultAndCustomValidity(t *testing.T) {
	t.Parallel()

	f := newBlockExceptionFixture(t)

	organizationID, ledgerID := uuid.New(), uuid.New()
	customTTL := 900

	f.account.EXPECT().
		ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"@default_ttl", "@custom_ttl"}).
		Return([]*mmodel.Account{accountWithAlias("@default_ttl"), accountWithAlias("@custom_ttl")}, nil)

	var written []txRedis.AccountBlockException

	f.redis.EXPECT().
		CreateAccountBlockExceptions(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, exceptions []txRedis.AccountBlockException) error {
			written = exceptions

			return nil
		})

	_, err := f.uc.CreateAccountBlockExceptions(context.Background(), organizationID, ledgerID,
		&mmodel.CreateAccountBlockExceptionsInput{
			Exceptions: []mmodel.CreateAccountBlockExceptionInput{
				{AccountAlias: "@default_ttl", Amount: "10"},
				{AccountAlias: "@custom_ttl", Amount: "10", TTL: &customTTL},
			},
		})

	require.NoError(t, err)
	require.Len(t, written, 2)

	assert.Equal(t, 5*time.Minute, written[0].TTL, "an omitted validity is five minutes")
	assert.Equal(t, time.Duration(customTTL)*time.Second, written[1].TTL, "a declared validity is applied verbatim")
}

// TestAccountBlockCommands_LeaveNoGoroutines is the leak guard for the write
// paths the feature added: minting a batch of exceptions and propagating a
// block flip to the cache. Neither is allowed to spawn background work — the
// cache rewrite in particular is best-effort but SYNCHRONOUS, and moving it to a
// detached goroutine would leak one per PATCH under load while still passing
// every behavioral test in this package.
//
// IgnoreCurrent snapshots the goroutines already running when this test starts,
// so the assertion is about what these two calls leave behind, not about what
// earlier tests in the package did.
func TestAccountBlockCommands_LeaveNoGoroutines(t *testing.T) {
	ignoreExisting := goleak.IgnoreCurrent()

	organizationID, ledgerID, accountID := uuid.New(), uuid.New(), uuid.New()
	alias := "@leak_check"

	mint := newBlockExceptionFixture(t)
	mint.account.EXPECT().
		ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{alias}).
		Return([]*mmodel.Account{accountWithAlias(alias)}, nil)
	mint.redis.EXPECT().
		CreateAccountBlockExceptions(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil)

	_, err := mint.uc.CreateAccountBlockExceptions(context.Background(), organizationID, ledgerID,
		&mmodel.CreateAccountBlockExceptionsInput{
			Exceptions: []mmodel.CreateAccountBlockExceptionInput{{AccountAlias: alias, Amount: "10"}},
		})
	require.NoError(t, err)

	patch := newBlockPatchFixture(t)
	patch.expectStoredBlocked(accountID, false, true)
	patch.balance.EXPECT().
		ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
		Return([]*mmodel.Balance{{ID: uuid.New().String(), Alias: alias, Key: "default"}}, nil)
	patch.redis.EXPECT().
		UpdateBalanceCacheBlocked(gomock.Any(), organizationID, ledgerID, []string{alias + "#default"}, true).
		Return(nil)

	block := true

	_, err = patch.uc.UpdateAccount(context.Background(), organizationID, ledgerID, nil, accountID,
		&mmodel.UpdateAccountInput{Blocked: &block}, mmodel.HolderOffV1)
	require.NoError(t, err)

	goleak.VerifyNone(t, ignoreExisting)
}
