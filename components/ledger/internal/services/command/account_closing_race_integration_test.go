//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	libPointers "github.com/LerianStudio/lib-commons/v7/commons/pointers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// accountClosingRefusalCode extracts the sentinel code a refusal carries, over the
// three typed errors the closing surface answers with.
func accountClosingRefusalCode(t *testing.T, err error) string {
	t.Helper()

	require.Error(t, err)

	var conflict pkg.EntityConflictError
	if errors.As(err, &conflict) {
		return conflict.Code
	}

	var unprocessable pkg.UnprocessableOperationError
	if errors.As(err, &unprocessable) {
		return unprocessable.Code
	}

	var unavailable pkg.ServiceUnavailableError
	require.Truef(t, errors.As(err, &unavailable), "unexpected error type: %T (%v)", err, err)

	return unavailable.Code
}

// TestIntegrationAccountClosingRefusesOverACreditAppliedBeforeTheProtection is
// AS-01 at the command boundary: a credit executed while the account was open, and
// not yet written back to the row, is the state the closing has to read. The
// persisted row still says zero, and closing on that stale zero would strand the
// money — so the refusal comes from the live state, the credit survives untouched
// and no closing instant is recorded.
func TestIntegrationAccountClosingRefusesOverACreditAppliedBeforeTheProtection(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-unsynced", "deposit")
	row := accountClosingBalanceSeed{alias: "@closing-unsynced", key: "default"}
	balanceID := h.seedBalance(t, accountID, row)

	live := accountClosingBalanceSeed{alias: "@closing-unsynced", key: "default", available: "10", version: 1}
	cacheKey := h.cacheBalance(t, accountID, balanceID, live)

	_, err := h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountBalanceNotZero)

	require.False(t, h.closedAt(t, accountID).Valid, "a refused closing records no instant")
	require.Equal(t, int64(1), h.exists(t, cacheKey), "the refusal never evicts the state it read")
	require.Equal(t, row.version, h.balanceRow(t, balanceID).version, "a refusal moves no money and no version")

	protection := h.protection(accountID)
	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.closed, protection.ownership),
		"a known refusal gives back the protection of its own attempt")
}

// TestIntegrationAccountClosingOrdersItselfAgainstABalanceCreation is AS-05: the
// administrative ownership is the single thing both operations contend on, so one
// of them is always the later one and is refused rather than interleaved.
//
// Both directions are proven with barriers instead of timing. A creation that
// arrives while a closing holds the account is refused and persists nothing; a
// creation that finished before the protection is part of the list the closing then
// validates, and its residual is what refuses the closing.
func TestIntegrationAccountClosingOrdersItselfAgainstABalanceCreation(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-creation", "deposit")
	h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-creation", key: "default"})

	// Barrier: a closing attempt holds the account exactly as the use case does.
	const attemptToken = "closing-attempt-token"

	installed, err := h.uc.TransactionRedisRepo.AcquireAccountClosingMarker(ctx, h.organizationID, h.ledgerID, accountID, attemptToken)
	require.NoError(t, err)
	require.True(t, installed)

	owned, err := h.uc.TransactionRedisRepo.AcquireAccountAdminOwnership(ctx, h.organizationID, h.ledgerID, accountID, attemptToken)
	require.NoError(t, err)
	require.True(t, owned)

	_, err = h.uc.CreateAdditionalBalance(ctx, h.organizationID, h.ledgerID, accountID, &mmodel.CreateAdditionalBalance{
		Key:       "savings",
		Direction: libPointers.String(constant.DirectionCredit),
	})
	requireClosingCode(t, err, constant.ErrAccountClosingInProgress)

	balances, err := h.uc.BalanceRepo.ListByAccountID(ctx, h.organizationID, h.ledgerID, accountID)
	require.NoError(t, err)
	require.Len(t, balances, 1, "a creation refused by the coordination persists no balance")

	// Barrier: the closing attempt gives its protection back.
	released, err := h.uc.TransactionRedisRepo.ReleaseAccountAdminOwnership(ctx, h.organizationID, h.ledgerID, accountID, attemptToken)
	require.NoError(t, err)
	require.True(t, released)

	released, err = h.uc.TransactionRedisRepo.ReleaseAccountClosingAttempt(ctx, h.organizationID, h.ledgerID, accountID, attemptToken)
	require.NoError(t, err)
	require.True(t, released)

	created, err := h.uc.CreateAdditionalBalance(ctx, h.organizationID, h.ledgerID, accountID, &mmodel.CreateAdditionalBalance{
		Key:       "savings",
		Direction: libPointers.String(constant.DirectionCredit),
	})
	require.NoError(t, err, "with the protection gone the creation follows its normal contract")

	// The balance that exists before the protection takes part in the validation:
	// a residual on it refuses the closing, which is what "the list is stable"
	// means in practice.
	_, err = h.transactionDB.Exec(`UPDATE balance SET available = 1 WHERE id = $1`, created.ID)
	require.NoError(t, err)

	_, err = h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountBalanceNotZero)
	require.False(t, h.closedAt(t, accountID).Valid)
}

// TestIntegrationAccountClosingKeepsTheAccountClosedBeyondTheNegativeCache covers
// AS-06 and AS-16 together: once the instant is recorded, a writer that still
// carries an older view of the account is refused — first from the negative cache,
// and then, after that cache is gone, from the authoritative row, which recomposes
// the cache instead of admitting anything.
//
// The five-minute horizon is never waited for: an expired denial IS an absent key,
// so the key is removed.
func TestIntegrationAccountClosingKeepsTheAccountClosedBeyondTheNegativeCache(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-horizon", "deposit")
	balanceID := h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-horizon", key: "default"})
	cacheKey := h.cacheBalance(t, accountID, balanceID, accountClosingBalanceSeed{alias: "@closing-horizon", key: "default"})

	closedAt, err := h.close(ctx, accountID)
	require.NoError(t, err)

	protection := h.protection(accountID)
	require.Equal(t, int64(0), h.exists(t, cacheKey), "the finalization evicts the balances it proved settled")
	require.Equal(t, int64(1), h.exists(t, protection.closed), "the finalization installs the negative cache")
	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.ownership),
		"the protection leaves only after the eviction and the negative cache are confirmed")

	ttl, err := h.client.TTL(ctx, protection.closed).Result()
	require.NoError(t, err)
	require.Positive(t, ttl)
	require.LessOrEqual(t, ttl, 300*time.Second, "the negative cache is a horizon, not a state")

	// A writer answered from the negative cache alone.
	_, err = h.uc.CreateAdditionalBalance(ctx, h.organizationID, h.ledgerID, accountID, &mmodel.CreateAdditionalBalance{
		Key:       "savings",
		Direction: libPointers.String(constant.DirectionCredit),
	})
	requireClosingCode(t, err, constant.ErrAccountClosed)

	// The negative cache expires. The persisted instant is what still decides.
	require.NoError(t, h.client.Del(ctx, protection.closed).Err())

	_, err = h.uc.CreateAdditionalBalance(ctx, h.organizationID, h.ledgerID, accountID, &mmodel.CreateAdditionalBalance{
		Key:       "savings",
		Direction: libPointers.String(constant.DirectionCredit),
	})
	requireClosingCode(t, err, constant.ErrAccountClosed)

	require.Equal(t, int64(1), h.exists(t, protection.closed), "the refusal recomposes the negative cache")
	require.Equal(t, int64(0), h.exists(t, cacheKey), "no balance is admitted back into the cache")

	balances, err := h.uc.BalanceRepo.ListByAccountID(ctx, h.organizationID, h.ledgerID, accountID)
	require.NoError(t, err)
	require.Len(t, balances, 1, "the closing preserves the balance rows it evicted from the cache")

	// The repeat is answered from the authoritative row, not from the cache.
	stored := h.closedAt(t, accountID)
	require.True(t, stored.Valid)
	require.True(t, closedAt.Equal(stored.Time), "the returned instant is the one the database recorded")

	_, err = h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountAlreadyClosed)
	require.True(t, closedAt.Equal(h.closedAt(t, accountID).Time), "a repeat never moves the instant")
}

// TestIntegrationAccountClosingLetsOnlyOneConcurrentAttemptThrough is AC-11 over
// the real coordination: several closings contend on the same eligible account and
// the closing marker decides. Exactly one records the instant; every other one is
// refused as a dispute in progress or as already closed, and no refusal writes a
// second timestamp.
func TestIntegrationAccountClosingLetsOnlyOneConcurrentAttemptThrough(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-dispute", "deposit")
	h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-dispute", key: "default"})

	type attempt struct {
		closedAt time.Time
		err      error
	}

	results := make([]attempt, 8)
	start := make(chan struct{})

	var wg sync.WaitGroup

	for index := range results {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			closedAt, err := h.close(ctx, accountID)
			results[index] = attempt{closedAt: closedAt, err: err}
		}()
	}

	close(start)
	wg.Wait()

	winners := 0

	for _, result := range results {
		if result.err == nil {
			winners++

			continue
		}

		// A closing takes its ownership only under its own marker, so a losing attempt
		// always meets the winner's marker or its recorded instant — never an
		// ownership that reads as some other operation holding the account.
		require.Contains(t,
			[]string{constant.ErrAccountClosingInProgress.Error(), constant.ErrAccountAlreadyClosed.Error()},
			accountClosingRefusalCode(t, result.err), "a losing attempt is refused as a dispute or as already closed")
	}

	require.Equal(t, 1, winners, "at most one attempt may record the transition")

	stored := h.closedAt(t, accountID)
	require.True(t, stored.Valid)

	for _, result := range results {
		if result.err == nil {
			require.True(t, result.closedAt.Equal(stored.Time), "the winner returns the instant the database holds")
		}
	}
}

// accountClosingHoldPoint is where a gated closing attempt is held.
type accountClosingHoldPoint int

const (
	// holdBeforeProtection holds the attempt at its first protection call, after it
	// read the account as open and before it holds anything.
	holdBeforeProtection accountClosingHoldPoint = iota
	// holdBetweenProtectionWrites holds the attempt once the first of its two
	// protection writes (the closing marker and the ownership) has landed.
	holdBetweenProtectionWrites
	// holdBetweenProtectionReleases holds the attempt once the first of its two
	// protection releases has landed.
	holdBetweenProtectionReleases
)

// accountClosingGate holds one attempt at one point until the test resumes it.
type accountClosingGate struct {
	reached chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func newAccountClosingGate() *accountClosingGate {
	return &accountClosingGate{reached: make(chan struct{}), resume: make(chan struct{})}
}

// hold stops the caller the first time it is reached and lets every later call
// through.
func (g *accountClosingGate) hold() {
	g.once.Do(func() {
		close(g.reached)
		<-g.resume
	})
}

// awaitReached fails the test when the attempt never reaches its hold point.
func (g *accountClosingGate) awaitReached(t *testing.T) {
	t.Helper()

	select {
	case <-g.reached:
	case <-time.After(30 * time.Second):
		t.Fatal("the closing attempt never reached its hold point")
	}
}

// accountClosingGatedCache is the real cache with one attempt held at a point of
// its protection protocol. It names the point by what the attempt holds, not by
// which call comes first, so the barrier describes the same window whatever order
// the protocol takes its steps in.
type accountClosingGatedCache struct {
	txRedis.RedisRepository

	gate  *accountClosingGate
	point accountClosingHoldPoint
}

func (c *accountClosingGatedCache) GetAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (string, bool, error) {
	c.holdBefore()

	return c.RedisRepository.GetAccountClosingMarker(ctx, organizationID, ledgerID, accountID)
}

func (c *accountClosingGatedCache) AcquireAccountClosingMarker(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	c.holdBefore()

	installed, err := c.RedisRepository.AcquireAccountClosingMarker(ctx, organizationID, ledgerID, accountID, token)
	c.holdAfter(holdBetweenProtectionWrites, installed, err)

	return installed, err
}

func (c *accountClosingGatedCache) AcquireAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	c.holdBefore()

	owned, err := c.RedisRepository.AcquireAccountAdminOwnership(ctx, organizationID, ledgerID, accountID, token)
	c.holdAfter(holdBetweenProtectionWrites, owned, err)

	return owned, err
}

func (c *accountClosingGatedCache) ReleaseAccountClosingAttempt(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	released, err := c.RedisRepository.ReleaseAccountClosingAttempt(ctx, organizationID, ledgerID, accountID, token)
	c.holdAfter(holdBetweenProtectionReleases, released, err)

	return released, err
}

func (c *accountClosingGatedCache) ReleaseAccountAdminOwnership(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	released, err := c.RedisRepository.ReleaseAccountAdminOwnership(ctx, organizationID, ledgerID, accountID, token)
	c.holdAfter(holdBetweenProtectionReleases, released, err)

	return released, err
}

func (c *accountClosingGatedCache) holdBefore() {
	if c.point == holdBeforeProtection {
		c.gate.hold()
	}
}

func (c *accountClosingGatedCache) holdAfter(point accountClosingHoldPoint, applied bool, err error) {
	if c.point == point && applied && err == nil {
		c.gate.hold()
	}
}

// gatedAttempt runs one closing of the account through its own use case, over the
// same dependencies as the harness but with the cache gated at point. The result
// arrives on the returned channel.
func (h *accountClosingHarness) gatedAttempt(ctx context.Context, accountID uuid.UUID, point accountClosingHoldPoint) (*accountClosingGate, <-chan error) {
	gate := newAccountClosingGate()

	uc := *h.uc
	uc.TransactionRedisRepo = &accountClosingGatedCache{RedisRepository: h.uc.TransactionRedisRepo, gate: gate, point: point}

	done := make(chan error, 1)

	go func() {
		_, err := uc.CloseAccount(ctx, h.organizationID, h.ledgerID, accountID)
		done <- err
	}()

	return gate, done
}

// awaitClosing fails the test when a gated attempt does not finish.
func awaitClosing(t *testing.T, done <-chan error) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("the closing attempt never finished")

		return nil
	}
}

// accountClosingSeedStore presents the harness cache as the shared seed admission
// surface a cache-miss load takes, the same translation the load itself uses.
type accountClosingSeedStore struct {
	txRedis.RedisRepository
}

func (s accountClosingSeedStore) AdmitAccountSeed(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	admitted, _, err := s.AcquireAccountSeedAdmission(ctx, organizationID, ledgerID, accountID, token)

	return admitted, err
}

func (s accountClosingSeedStore) ReleaseAccountSeed(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return s.ReleaseAccountSeedAdmission(ctx, organizationID, ledgerID, accountID, token)
}

// seedAdmissionGuard is the guard a cache-miss load builds over this harness.
func (h *accountClosingHarness) seedAdmissionGuard() *accountprotection.Guard {
	return accountprotection.NewSeedAdmissionGuard(h.accountRepo, accountClosingSeedStore{h.uc.TransactionRedisRepo})
}

// TestIntegrationAccountClosingRefusesARivalWhileTheWinnerTakesItsProtection holds
// the winning closing between its two protection writes, the window a rival could
// once meet as an ownership without a marker and read as some other operation.
// Every contender that arrives there — a second closing, a cache-miss seed
// admission, a balance creation — is told a closing holds the account, and none
// of them takes anything; the winner then closes normally.
func TestIntegrationAccountClosingRefusesARivalWhileTheWinnerTakesItsProtection(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-protection-window", "deposit")
	h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-protection-window", key: "default"})

	gate, winner := h.gatedAttempt(ctx, accountID, holdBetweenProtectionWrites)
	gate.awaitReached(t)

	protection := h.protection(accountID)
	require.Equal(t, int64(1), h.exists(t, protection.closing, protection.ownership),
		"the winner holds exactly one of its two protection keys")

	_, err := h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountClosingInProgress)

	_, err = h.seedAdmissionGuard().AcquireSeedAdmission(ctx, h.organizationID, h.ledgerID, []uuid.UUID{accountID})
	requireClosingCode(t, err, constant.ErrAccountClosingInProgress)

	_, err = h.uc.CreateAdditionalBalance(ctx, h.organizationID, h.ledgerID, accountID, &mmodel.CreateAdditionalBalance{
		Key:       "savings",
		Direction: libPointers.String(constant.DirectionCredit),
	})
	requireClosingCode(t, err, constant.ErrAccountClosingInProgress)

	require.Equal(t, int64(1), h.exists(t, protection.closing, protection.ownership), "no rival took a key of its own")
	require.False(t, h.closedAt(t, accountID).Valid)

	close(gate.resume)
	require.NoError(t, awaitClosing(t, winner), "the held closing concludes once resumed")

	require.True(t, h.closedAt(t, accountID).Valid)
	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.ownership))

	balances, err := h.uc.BalanceRepo.ListByAccountID(ctx, h.organizationID, h.ledgerID, accountID)
	require.NoError(t, err)
	require.Len(t, balances, 1, "the refused creation persisted nothing")
}

// TestIntegrationAccountClosingRefusesARivalWhileTheWinnerGivesItsProtectionBack
// is the other end of the winner's protection: its instant is recorded and it has
// given back one of its two keys. A rival that read the account open before that
// instant arrives now and is still told a closing holds the account.
func TestIntegrationAccountClosingRefusesARivalWhileTheWinnerGivesItsProtectionBack(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-release-window", "deposit")
	h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-release-window", key: "default"})

	rivalGate, rival := h.gatedAttempt(ctx, accountID, holdBeforeProtection)
	rivalGate.awaitReached(t)

	winnerGate, winner := h.gatedAttempt(ctx, accountID, holdBetweenProtectionReleases)
	winnerGate.awaitReached(t)

	protection := h.protection(accountID)
	require.True(t, h.closedAt(t, accountID).Valid, "the winner recorded its instant")
	require.Equal(t, int64(1), h.exists(t, protection.closing, protection.ownership),
		"the winner still holds exactly one of its two protection keys")

	close(rivalGate.resume)
	requireClosingCode(t, awaitClosing(t, rival), constant.ErrAccountClosingInProgress)

	close(winnerGate.resume)
	require.NoError(t, awaitClosing(t, winner))

	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.ownership))
	require.Equal(t, int64(1), h.exists(t, protection.closed))
}

// TestIntegrationAccountClosingIsRefusedByALiveSeedAdmission runs the closing over
// the real cache while a cache-miss load holds a shared seed admission on the
// account. The closing is refused as busy, gives back the closing marker it had
// installed, records no instant and leaves the admission exactly where it was; once
// the load gives its admission back, the account closes.
func TestIntegrationAccountClosingIsRefusedByALiveSeedAdmission(t *testing.T) {
	h := newAccountClosingHarness(t)
	ctx := context.Background()

	accountID := h.seedAccount(t, "@closing-seed-admission", "deposit")
	h.seedBalance(t, accountID, accountClosingBalanceSeed{alias: "@closing-seed-admission", key: "default"})

	admission, err := h.seedAdmissionGuard().AcquireSeedAdmission(ctx, h.organizationID, h.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)

	_, err = h.close(ctx, accountID)
	requireClosingCode(t, err, constant.ErrAccountAdministrativeOperationInProgress)

	protection := h.protection(accountID)
	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.closed), "the refused closing leaves no marker behind")
	require.False(t, h.closedAt(t, accountID).Valid)

	kind, err := h.client.Type(ctx, protection.ownership).Result()
	require.NoError(t, err)
	require.Equal(t, "zset", kind, "the ownership key still holds the shared admissions")

	_, err = h.client.ZScore(ctx, protection.ownership, admission.Token()).Result()
	require.NoError(t, err, "the load's admission is still a live member")

	admission.Release(ctx)

	_, err = h.close(ctx, accountID)
	require.NoError(t, err, "with the admission given back the closing proceeds")
	require.True(t, h.closedAt(t, accountID).Valid)
	require.Equal(t, int64(0), h.exists(t, protection.closing, protection.ownership))
}
