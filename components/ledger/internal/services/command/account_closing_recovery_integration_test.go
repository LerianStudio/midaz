//go:build integration

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
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// errAccountClosingLostAnswer stands for an outcome that proves nothing: the
// statement may have committed with the answer lost on the way back. It is
// deliberately not a SQLSTATE, because a SQLSTATE would be a known refusal.
var errAccountClosingLostAnswer = errors.New("connection reset before the closing answer arrived")

// accountClosingLostWriteRepo performs the real conditional write and then reports
// that its answer never arrived, which is the only situation a protection may not
// be given back in.
type accountClosingLostWriteRepo struct {
	account.Repository

	lose bool
}

func (r *accountClosingLostWriteRepo) CloseAccount(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (time.Time, error) {
	closedAt, err := r.Repository.CloseAccount(ctx, organizationID, ledgerID, id)
	if err != nil || !r.lose {
		return closedAt, err
	}

	return time.Time{}, errAccountClosingLostAnswer
}

// accountClosingFailingEvictionRepo fails every cache eviction, which is the step
// that must keep the protection in place after the instant is already recorded.
type accountClosingFailingEvictionRepo struct {
	txRedis.RedisRepository

	fail bool
}

func (r *accountClosingFailingEvictionRepo) Del(ctx context.Context, key string) error {
	if r.fail {
		return errors.New("the cache refused the eviction")
	}

	return r.RedisRepository.Del(ctx, key)
}

// accountClosingCancellingBalanceRepo cancels the caller's request at the moment
// the closing starts reading its evidence, which is the shape of a client that
// hangs up after the protection is already installed.
type accountClosingCancellingBalanceRepo struct {
	balance.Repository

	cancel context.CancelFunc
}

func (r *accountClosingCancellingBalanceRepo) ListByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) ([]*mmodel.Balance, error) {
	r.cancel()

	return nil, context.Canceled
}

// TestIntegrationAccountClosingKeepsProtectionWhenTheWriteAnswerIsLost is AS-12: a
// conditional write whose answer never arrived may have committed, so the attempt
// keeps its protection instead of giving it back, and the reconciliation that runs
// afterwards finishes the very same closing with the instant the database holds.
func TestIntegrationAccountClosingKeepsProtectionWhenTheWriteAnswerIsLost(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	repo := &accountClosingLostWriteRepo{Repository: h.accountRepo, lose: true}
	h.uc.AccountRepo = repo

	accountID := h.seedAccount(t, "@closing-lost-answer", "deposit")
	balanceID := h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-lost-answer", key: "default"})
	cacheKey := h.cacheBalance(t, accountID, balanceID, accountClosingBalanceSeed{alias: "@closing-lost-answer", key: "default"})

	_, err := h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)

	protection := h.protection(accountID)
	require.Equal(t, int64(2), h.exists(t, protection.closing, protection.ownership),
		"an unresolved write never has its protection removed")

	recorded := h.closedAt(t, accountID)
	require.True(t, recorded.Valid, "the write did land; only its answer was lost")

	// The reconciliation resolves it against the authoritative row.
	repo.lose = false

	stats := h.uc.ReconcileAccountClosings(ctx)
	require.Equal(t, 1, stats.Scanned)
	require.Equal(t, 1, stats.Completed)
	require.Zero(t, stats.Retained)

	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.ownership, cacheKey),
		"the reconciliation finishes the eviction and removes the protection")
	require.Equal(t, int64(1), h.exists(t, protection.closed), "the negative cache is installed by the reconciliation")

	after := h.closedAt(t, accountID)
	require.True(t, after.Valid)
	require.True(t, recorded.Time.Equal(after.Time), "the reconciliation never moves the instant")
}

// TestIntegrationAccountClosingKeepsProtectionWhenTheEvictionFails is AS-13: the
// transition is already durable, so an eviction that fails may not be answered by
// giving the protection back — a blob left behind is a balance a later load could
// still serve. The reconciliation resumes exactly that finalization.
func TestIntegrationAccountClosingKeepsProtectionWhenTheEvictionFails(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	cache := &accountClosingFailingEvictionRepo{RedisRepository: h.uc.TransactionRedisRepo, fail: true}
	h.uc.TransactionRedisRepo = cache

	accountID := h.seedAccount(t, "@closing-eviction", "deposit")
	balanceID := h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-eviction", key: "default"})
	cacheKey := h.cacheBalance(t, accountID, balanceID, accountClosingBalanceSeed{alias: "@closing-eviction", key: "default"})

	_, err := h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)

	protection := h.protection(accountID)
	require.Equal(t, int64(2), h.exists(t, protection.closing, protection.ownership),
		"an unfinished finalization keeps its protection")
	require.Equal(t, int64(0), h.exists(t, protection.closed), "the negative cache is installed only after the eviction")
	require.Equal(t, int64(1), h.exists(t, cacheKey))

	recorded := h.closedAt(t, accountID)
	require.True(t, recorded.Valid, "the account is closed from the write onwards")

	// A pass that still cannot evict leaves everything exactly where it was.
	stats := h.uc.ReconcileAccountClosings(ctx)
	require.Equal(t, 1, stats.Retained)
	require.Zero(t, stats.Completed)
	require.Equal(t, int64(2), h.exists(t, protection.closing, protection.ownership))

	// Once the dependency recovers the same finalization concludes.
	cache.fail = false

	stats = h.uc.ReconcileAccountClosings(ctx)
	require.Equal(t, 1, stats.Completed)
	require.Zero(t, stats.Retained)

	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.ownership, cacheKey))
	require.Equal(t, int64(1), h.exists(t, protection.closed))
	require.True(t, recorded.Time.Equal(h.closedAt(t, accountID).Time), "the instant is preserved across the retry")
}

// TestIntegrationAccountClosingCleansUpAfterACancelledRequest is AS-19 for the
// attempt that never reached its write: the caller hung up while the evidence was
// being read, and the cleanup that removes the protection runs on a context of its
// own — inheriting the cancellation would abandon exactly the marker it exists to
// release. Nothing else is touched, and the account is still closable afterwards.
func TestIntegrationAccountClosingCleansUpAfterACancelledRequest(t *testing.T) {
	h := newAccountClosingHarness(t)

	accountID := h.seedAccount(t, "@closing-cancelled", "deposit")
	h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-cancelled", key: "default"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	balances := h.uc.BalanceRepo
	h.uc.BalanceRepo = &accountClosingCancellingBalanceRepo{Repository: balances, cancel: cancel}

	_, err := h.close(ctx, accountID)
	require.Error(t, err)

	protection := h.protection(accountID)
	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.closed, protection.ownership),
		"a cancelled attempt that issued no write leaves no protection behind")
	require.False(t, h.closedAt(t, accountID).Valid)

	blocked := true
	require.NoError(t, h.onboardingDB.QueryRow(`SELECT blocked FROM account WHERE id = $1`, accountID).Scan(&blocked))
	require.False(t, blocked, "a refused closing preserves the blocking state of the account")

	// The account was never touched, so the normal route still closes it.
	h.uc.BalanceRepo = balances

	_, err = h.close(context.Background(), accountID)
	require.NoError(t, err, "regularization through the normal route stays available")
}

// TestIntegrationAccountClosingReconcilesOnlyWhatItCanResolve covers AS-19 across a
// restart and AS-14's isolation in one pass: the reconciliation discovers the
// markers by scanning the namespace, not by remembering them, and each one is
// resolved against the authoritative row of its own scope.
func TestIntegrationAccountClosingReconcilesOnlyWhatItCanResolve(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	// An attempt that died before issuing its write: the protection is releasable.
	aborted := h.seedAccount(t, "@closing-aborted", "deposit")
	h.seedBalance(t, aborted, accountClosingBalanceSeed{alias: "@closing-aborted", key: "default"})
	h.installAbandonedAttempt(t, aborted, "aborted-attempt-token", false)

	// An attempt that recorded its intent and whose row still reads NULL: nothing
	// proves that write can no longer land, so the protection stays.
	unresolved := h.seedAccount(t, "@closing-unresolved", "deposit")
	h.seedBalance(t, unresolved, accountClosingBalanceSeed{alias: "@closing-unresolved", key: "default"})
	h.installAbandonedAttempt(t, unresolved, "unresolved-attempt-token", true)

	// A neighbouring organization and ledger carry a closing of their own.
	neighbour := h.newNeighbourScope(t)
	neighbourAccount := neighbour.seedAccount(t, "@closing-neighbour", "deposit")
	neighbourBalance := neighbour.seedBalance(t, neighbourAccount, accountClosingBalanceSeed{alias: "@closing-neighbour", key: "default"})
	neighbourCache := neighbour.cacheBalance(t, neighbourAccount, neighbourBalance, accountClosingBalanceSeed{alias: "@closing-neighbour", key: "default"})
	neighbour.installAbandonedAttempt(t, neighbourAccount, "neighbour-attempt-token", true)

	_, err := neighbour.onboardingDB.Exec(`UPDATE account SET closed_at = $1 WHERE id = $2`, accountClosingFixedInstant, neighbourAccount)
	require.NoError(t, err)

	stats := h.uc.ReconcileAccountClosings(ctx)
	require.Equal(t, 3, stats.Scanned)
	require.Equal(t, 1, stats.Released, "only the attempt that never wrote is given back")
	require.Equal(t, 1, stats.Completed, "the confirmed closing is finished with its own instant")
	require.Equal(t, 1, stats.Retained, "an unresolved write is left protected")

	abortedKeys := h.protection(aborted)
	require.Equal(t, int64(0), h.exists(t, abortedKeys.closing, abortedKeys.ownership))

	unresolvedKeys := h.protection(unresolved)
	require.Equal(t, int64(2), h.exists(t, unresolvedKeys.closing, unresolvedKeys.ownership))
	require.False(t, h.closedAt(t, unresolved).Valid, "the reconciliation never closes an account itself")

	neighbourKeys := neighbour.protection(neighbourAccount)
	require.Equal(t, int64(0), h.exists(t, neighbourKeys.closing, neighbourKeys.ownership, neighbourCache))
	require.Equal(t, int64(1), h.exists(t, neighbourKeys.closed))

	stored := neighbour.closedAt(t, neighbourAccount)
	require.True(t, stored.Valid)
	require.True(t, accountClosingFixedInstant.Equal(stored.Time), "the reconciliation preserves the recorded instant")

	// The accounts of the other two scopes are untouched by the neighbour's closing.
	require.False(t, h.closedAt(t, aborted).Valid)
}

// TestIntegrationAccountClosingReconcilesAMarkerWithoutItsOwnership covers an
// attempt that lost its process between installing its closing marker and taking
// its ownership. It never issued its write, so reconciliation gives the marker
// back; there is no ownership to release, and releasing it anyway changes nothing.
func TestIntegrationAccountClosingReconcilesAMarkerWithoutItsOwnership(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-marker-only", "deposit")
	h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-marker-only", key: "default"})

	installed, err := h.uc.TransactionRedisRepo.AcquireAccountClosingMarker(ctx, h.organizationID, h.ledgerID, accountID, "marker-only-attempt-token")
	require.NoError(t, err)
	require.True(t, installed)

	stats := h.uc.ReconcileAccountClosings(ctx)
	require.Equal(t, 1, stats.Scanned)
	require.Equal(t, 1, stats.Released)
	require.Zero(t, stats.Retained)
	require.Zero(t, stats.Ownerships)

	protection := h.protection(accountID)
	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.closed, protection.ownership))
	require.False(t, h.closedAt(t, accountID).Valid)

	_, err = h.close(ctx, accountID)
	require.NoError(t, err, "the released account closes through the normal route")
}

// TestIntegrationAccountClosingAbortsWhenReconciliationReclaimsItsMarker covers a
// live attempt whose marker a reconciliation pass reclaims before the attempt took
// its ownership: the pass cannot tell it from an abandoned one, since neither
// issued a write. The attempt then refuses without taking the ownership, records
// no instant and leaves nothing behind.
func TestIntegrationAccountClosingAbortsWhenReconciliationReclaimsItsMarker(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-reclaimed", "deposit")
	h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-reclaimed", key: "default"})

	gate, attempt := h.gatedAttempt(ctx, accountID, holdBetweenProtectionWrites)
	gate.awaitReached(t)

	stats := h.uc.ReconcileAccountClosings(ctx)
	require.Equal(t, 1, stats.Released, "the attempt's marker is reclaimed as an attempt that never wrote")

	close(gate.resume)
	requireClosingCode(t, awaitClosing(t, attempt), constant.ErrAccountClosingProtectionIndeterminate)

	protection := h.protection(accountID)
	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.closed, protection.ownership),
		"the aborted attempt leaves no protection behind")
	require.False(t, h.closedAt(t, accountID).Valid)

	_, err := h.close(ctx, accountID)
	require.NoError(t, err)
}

// TestIntegrationAccountClosingRefusesWhenTheProtectionCannotBeRead is AS-07: a
// control that cannot be read is not an absence. The closing refuses technically,
// a writer over the same account refuses as well, and a recorded instant elsewhere
// is untouched by any of it.
func TestIntegrationAccountClosingRefusesWhenTheProtectionCannotBeRead(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-unreadable", "deposit")
	h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-unreadable", key: "default"})

	protection := h.protection(accountID)
	require.NoError(t, h.client.Set(ctx, protection.closing, "   ", 0).Err())

	_, err := h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)
	require.False(t, h.closedAt(t, accountID).Valid)

	_, err = h.uc.CreateAdditionalBalance(ctx, h.organizationID, h.ledgerID, accountID, &mmodel.CreateAdditionalBalance{Key: "savings"})
	requireClosingCode(t, err, constant.ErrAccountClosingProtectionIndeterminate)

	balances, err := h.uc.BalanceRepo.ListByAccountID(ctx, h.organizationID, h.ledgerID, accountID)
	require.NoError(t, err)
	require.Len(t, balances, 1, "an unreadable control authorizes nothing")

	// An open account that owns no control at all is the normal case and is not
	// affected by the neighbour's unreadable one.
	open := h.seedAccount(t, "@closing-open", "deposit")
	h.seedBalance(t, open, accountClosingBalanceSeed{alias: "@closing-open", key: "default"})

	openKeys := h.protection(open)
	require.Equal(t, int64(0), h.exists(t, openKeys.closing, openKeys.closed, openKeys.ownership),
		"an open account owns no key at all")

	_, err = h.close(ctx, open)
	require.NoError(t, err, "the normal absence of the controls is not a refusal")
}

// installAbandonedAttempt leaves behind the protection of an attempt that lost its
// process, optionally past the point where its write may already be in flight.
func (h *accountClosingHarness) installAbandonedAttempt(t *testing.T, accountID uuid.UUID, token string, writeIssued bool) {
	t.Helper()

	ctx := context.Background()

	installed, err := h.uc.TransactionRedisRepo.AcquireAccountClosingMarker(ctx, h.organizationID, h.ledgerID, accountID, token)
	require.NoError(t, err)
	require.True(t, installed)

	owned, err := h.uc.TransactionRedisRepo.AcquireAccountAdminOwnership(ctx, h.organizationID, h.ledgerID, accountID, token)
	require.NoError(t, err)
	require.True(t, owned)

	if !writeIssued {
		return
	}

	recorded, err := h.uc.TransactionRedisRepo.MarkAccountClosingWriteIssued(ctx, h.organizationID, h.ledgerID, accountID, token)
	require.NoError(t, err)
	require.True(t, recorded)
}

// newNeighbourScope returns a harness view over another organization and ledger,
// sharing the same databases and cache. Every control is addressed by the full
// scope, so what happens there must stay there.
func (h *accountClosingHarness) newNeighbourScope(t *testing.T) *accountClosingHarness {
	t.Helper()

	organizationID := pgtestutil.CreateTestOrganization(t, h.onboardingDB)
	ledgerID := pgtestutil.CreateTestLedger(t, h.onboardingDB, organizationID)

	neighbour := *h
	neighbour.organizationID = organizationID
	neighbour.ledgerID = ledgerID

	return &neighbour
}
