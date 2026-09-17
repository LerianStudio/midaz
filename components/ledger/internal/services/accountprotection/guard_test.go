// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accountprotection

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// fixedClosedAt is the closing instant every test reads and writes. A fixed
// instant keeps the assertions independent of the clock.
var fixedClosedAt = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

var errCacheUnavailable = errors.New("cache unavailable")

// markerKey addresses one marker by the complete scope, the same way the cache
// adapter does, so a test that varies the ledger really tests isolation.
type markerKey struct {
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	accountID      uuid.UUID
}

// fakeMarkers is an in-memory MarkerStore with the conditional semantics of the
// cache: ownership is exclusive and released only by its own token.
type fakeMarkers struct {
	mu sync.Mutex

	closing   map[markerKey]string
	closed    map[markerKey]time.Time
	ownership map[markerKey]string

	closingErr error
	closedErr  error
	setErr     error

	acquireCalls int
	setCalls     int

	// beforeAcquire runs outside the lock before each ownership acquisition, so a
	// test can order two callers without sleeping.
	beforeAcquire func(accountID uuid.UUID)
}

func newFakeMarkers() *fakeMarkers {
	return &fakeMarkers{
		closing:   map[markerKey]string{},
		closed:    map[markerKey]time.Time{},
		ownership: map[markerKey]string{},
	}
}

func (f *fakeMarkers) GetAccountClosingMarker(_ context.Context, organizationID, ledgerID, accountID uuid.UUID) (string, bool, error) {
	if f.closingErr != nil {
		return "", false, f.closingErr
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	token, found := f.closing[markerKey{organizationID, ledgerID, accountID}]

	return token, found, nil
}

func (f *fakeMarkers) GetAccountClosedMarker(_ context.Context, organizationID, ledgerID, accountID uuid.UUID) (time.Time, bool, error) {
	if f.closedErr != nil {
		return time.Time{}, false, f.closedErr
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	closedAt, found := f.closed[markerKey{organizationID, ledgerID, accountID}]

	return closedAt, found, nil
}

func (f *fakeMarkers) SetAccountClosedMarker(_ context.Context, organizationID, ledgerID, accountID uuid.UUID, closedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.setCalls++

	if f.setErr != nil {
		return f.setErr
	}

	f.closed[markerKey{organizationID, ledgerID, accountID}] = closedAt

	return nil
}

func (f *fakeMarkers) AcquireAccountAdminOwnership(_ context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	if f.beforeAcquire != nil {
		f.beforeAcquire(accountID)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.acquireCalls++

	key := markerKey{organizationID, ledgerID, accountID}
	if _, owned := f.ownership[key]; owned {
		return false, nil
	}

	f.ownership[key] = token

	return true, nil
}

func (f *fakeMarkers) ReleaseAccountAdminOwnership(_ context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := markerKey{organizationID, ledgerID, accountID}
	if f.ownership[key] != token {
		return false, nil
	}

	delete(f.ownership, key)

	return true, nil
}

func (f *fakeMarkers) ownedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.ownership)
}

// fakeAccounts is an in-memory ClosingStateReader over the authoritative rows.
type fakeAccounts struct {
	mu sync.Mutex

	closedAt map[uuid.UUID]*time.Time
	err      error
	calls    int
	ids      [][]uuid.UUID
}

func newFakeAccounts() *fakeAccounts {
	return &fakeAccounts{closedAt: map[uuid.UUID]*time.Time{}}
}

func (f *fakeAccounts) ListClosedAtByIDs(_ context.Context, _, _ uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]*time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	f.ids = append(f.ids, ids)

	if f.err != nil {
		return nil, f.err
	}

	states := make(map[uuid.UUID]*time.Time, len(ids))

	for _, id := range ids {
		if closedAt, known := f.closedAt[id]; known {
			states[id] = closedAt
		}
	}

	return states, nil
}

type guardFixture struct {
	guard          *Guard
	markers        *fakeMarkers
	accounts       *fakeAccounts
	organizationID uuid.UUID
	ledgerID       uuid.UUID
}

func newGuardFixture() *guardFixture {
	markers := newFakeMarkers()
	accounts := newFakeAccounts()

	return &guardFixture{
		guard:          NewGuard(accounts, markers),
		markers:        markers,
		accounts:       accounts,
		organizationID: uuid.New(),
		ledgerID:       uuid.New(),
	}
}

func (f *guardFixture) key(accountID uuid.UUID) markerKey {
	return markerKey{f.organizationID, f.ledgerID, accountID}
}

// TestEnsureOpen_OpenAccountWithoutMarkers covers AS-15: an account that was never
// closed owns no marker, and that absence alone never authorizes admission — the
// authoritative row is what answers.
func TestEnsureOpen_OpenAccountWithoutMarkers(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()
	f.accounts.closedAt[accountID] = nil

	require.NoError(t, f.guard.EnsureOpen(context.Background(), f.organizationID, f.ledgerID, []uuid.UUID{accountID}))

	assert.Equal(t, 1, f.accounts.calls, "the open account is decided by the authoritative read")
	assert.Empty(t, f.markers.closed, "an open account must acquire no marker")
	assert.Zero(t, f.markers.setCalls)
}

// TestEnsureOpen_UnknownAccountIsNotRefused keeps an account with no row in scope
// on the pre-existing behavior: it reports no closing, so nothing new refuses it.
func TestEnsureOpen_UnknownAccountIsNotRefused(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()

	require.NoError(t, f.guard.EnsureOpen(context.Background(), f.organizationID, f.ledgerID, []uuid.UUID{uuid.New()}))
}

// TestEnsureOpen_ClosedMarkerAnswersWithoutReadingTheDatabase proves the negative
// cache does its job: an account already proven closed costs no authoritative read.
func TestEnsureOpen_ClosedMarkerAnswersWithoutReadingTheDatabase(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()
	f.markers.closed[f.key(accountID)] = fixedClosedAt

	err := f.guard.EnsureOpen(context.Background(), f.organizationID, f.ledgerID, []uuid.UUID{accountID})

	closed, ok := AsClosedAccountError(err)
	require.True(t, ok, "a closed account must be reported as such")
	assert.Equal(t, accountID, closed.AccountID)
	assert.True(t, fixedClosedAt.Equal(closed.ClosedAt))
	assert.Zero(t, f.accounts.calls, "the negative cache exists to spare this read")
}

// TestEnsureOpen_ExpiredClosedMarkerRefusesAndRecomposes covers AS-16: once the
// five-minute negative cache expires, the authoritative row still refuses the
// account and the cache is recomposed. Expiry is a cache horizon, not a reopening.
func TestEnsureOpen_ExpiredClosedMarkerRefusesAndRecomposes(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()
	f.accounts.closedAt[accountID] = &fixedClosedAt

	err := f.guard.EnsureOpen(context.Background(), f.organizationID, f.ledgerID, []uuid.UUID{accountID})

	closed, ok := AsClosedAccountError(err)
	require.True(t, ok)
	assert.Equal(t, accountID, closed.AccountID)
	assert.Equal(t, 1, f.accounts.calls)

	recomposed, found := f.markers.closed[f.key(accountID)]
	require.True(t, found, "the refusal must recompose the negative cache")
	assert.True(t, fixedClosedAt.Equal(recomposed))
}

// TestEnsureOpen_RecomposeFailureStillRefuses keeps the refusal independent of the
// cache write: a closed account is refused even when the negative cache cannot be
// rewritten.
func TestEnsureOpen_RecomposeFailureStillRefuses(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()
	f.accounts.closedAt[accountID] = &fixedClosedAt
	f.markers.setErr = errCacheUnavailable

	_, ok := AsClosedAccountError(f.guard.EnsureOpen(context.Background(), f.organizationID, f.ledgerID, []uuid.UUID{accountID}))
	assert.True(t, ok)
}

// TestEnsureOpen_UnreadableProtectionIsNotAbsence covers AS-07: a marker that
// cannot be read, or an authoritative read that fails, refuses the operation
// instead of being interpreted as an open account.
func TestEnsureOpen_UnreadableProtectionIsNotAbsence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		break_ func(f *guardFixture)
	}{
		{
			name:   "the closed marker cannot be read",
			break_: func(f *guardFixture) { f.markers.closedErr = errCacheUnavailable },
		},
		{
			name:   "the authoritative state cannot be read",
			break_: func(f *guardFixture) { f.accounts.err = errCacheUnavailable },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newGuardFixture()
			tt.break_(f)

			err := f.guard.EnsureOpen(context.Background(), f.organizationID, f.ledgerID, []uuid.UUID{uuid.New()})

			require.Error(t, err)
			assertErrorCode(t, err, constant.ErrAccountClosingProtectionIndeterminate.Error())

			_, closed := AsClosedAccountError(err)
			assert.False(t, closed, "an unestablished state is not a closing")
		})
	}
}

// TestEnsureOpen_ScopeIsolation proves a closing recorded in one ledger never
// refuses the same account identifier in another.
func TestEnsureOpen_ScopeIsolation(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()
	f.markers.closed[markerKey{f.organizationID, uuid.New(), accountID}] = fixedClosedAt

	require.NoError(t, f.guard.EnsureOpen(context.Background(), f.organizationID, f.ledgerID, []uuid.UUID{accountID}))
}

// TestAcquireAdmission_ContentionRefusesTheSecondCaller covers AS-05: the winner
// keeps the ownership and the loser is refused instead of running beside it.
func TestAcquireAdmission_ContentionRefusesTheSecondCaller(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()
	ctx := context.Background()

	first, err := f.guard.AcquireAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)
	require.NotEmpty(t, first.Token())

	_, err = f.guard.AcquireAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.Error(t, err)
	assertErrorCode(t, err, constant.ErrAccountClosingInProgress.Error())

	first.Release(ctx)
	assert.Zero(t, f.markers.ownedCount())

	second, err := f.guard.AcquireAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err, "the account is free once the winner released it")
	second.Release(ctx)
}

// TestAcquireAdmission_ClosingAttemptRefusesAdmission proves a closing in flight
// blocks the operations it coordinates with, before any ownership is taken.
func TestAcquireAdmission_ClosingAttemptRefusesAdmission(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()
	f.markers.closing[f.key(accountID)] = uuid.NewString()

	_, err := f.guard.AcquireAdmission(context.Background(), f.organizationID, f.ledgerID, []uuid.UUID{accountID})

	require.Error(t, err)
	assertErrorCode(t, err, constant.ErrAccountClosingInProgress.Error())
	assert.Zero(t, f.markers.ownedCount(), "a refusal must take no ownership")
}

// TestAcquireAdmission_PartialAcquisitionIsReleased proves a refusal on the second
// account gives back the first, and touches nothing it does not own.
func TestAcquireAdmission_PartialAcquisitionIsReleased(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	ctx := context.Background()

	first, second := orderedPair()

	rivalToken := uuid.NewString()
	f.markers.ownership[f.key(second)] = rivalToken

	_, err := f.guard.AcquireAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{first, second})
	require.Error(t, err)

	assert.Equal(t, 1, f.markers.ownedCount(), "only the rival's ownership survives")
	assert.Equal(t, rivalToken, f.markers.ownership[f.key(second)], "a key owned by another operation is never released")
}

// TestAcquireAdmission_StableOrderAvoidsDeadlock runs two callers over the same
// two accounts in opposite request orders. A stable acquisition order means both
// contend on the same account first, so one of them always completes.
func TestAcquireAdmission_StableOrderAvoidsDeadlock(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	ctx := context.Background()
	first, second := orderedPair()

	// Both callers reach the ownership of the lower account before either takes the
	// higher one, which is exactly the interleaving a crossing order would deadlock on.
	var (
		barrier sync.WaitGroup
		once    sync.Once
	)

	barrier.Add(2)

	f.markers.beforeAcquire = func(accountID uuid.UUID) {
		if accountID == first {
			once.Do(func() {})
			barrier.Done()
			barrier.Wait()
		}
	}

	results := make(chan error, 2)

	go func() {
		admission, err := f.guard.AcquireAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{first, second})
		if err == nil {
			admission.Release(ctx)
		}

		results <- err
	}()

	go func() {
		admission, err := f.guard.AcquireAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{second, first})
		if err == nil {
			admission.Release(ctx)
		}

		results <- err
	}()

	failures := 0

	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				failures++
			}
		case <-time.After(10 * time.Second):
			t.Fatal("acquisition deadlocked")
		}
	}

	assert.LessOrEqual(t, failures, 1, "at least one caller must complete")
	assert.Zero(t, f.markers.ownedCount(), "every acquisition was released")
}

// TestAcquireAdmission_DuplicateAccountsCollapse proves the external companion
// costs no second acquisition: it belongs to the same account, so it is already
// covered and never recurses.
func TestAcquireAdmission_DuplicateAccountsCollapse(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()
	ctx := context.Background()

	admission, err := f.guard.AcquireAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID, accountID, uuid.Nil})
	require.NoError(t, err)

	assert.Equal(t, []uuid.UUID{accountID}, admission.Accounts())
	assert.Equal(t, 1, f.markers.acquireCalls)

	admission.Release(ctx)
}

// TestAdmission_IndeterminateOutcomeKeepsOwnership covers D5: work whose result is
// unknown keeps its protection until reconciliation resolves it. Nothing releases
// it on the way out, and nothing releases it by age.
func TestAdmission_IndeterminateOutcomeKeepsOwnership(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()
	ctx := context.Background()

	admission, err := f.guard.AcquireAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)

	admission.MarkIndeterminate()
	admission.Release(ctx)

	assert.Equal(t, 1, f.markers.ownedCount(), "an unknown result is never released")

	_, err = f.guard.AcquireAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	assert.Error(t, err, "the account stays protected for reconciliation")
}

// TestAdmission_ReleaseIsIdempotent proves a second release neither fails nor
// drops an ownership taken afterwards by another operation.
func TestAdmission_ReleaseIsIdempotent(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()
	ctx := context.Background()

	admission, err := f.guard.AcquireAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)

	admission.Release(ctx)

	successor, err := f.guard.AcquireAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)

	admission.Release(ctx)

	assert.Equal(t, successor.Token(), f.markers.ownership[f.key(accountID)],
		"a late release must not drop the successor's ownership")
}

// TestGuard_WithoutCacheIsInert proves a guard with no protection surface changes
// nothing: it owns nothing and refuses nothing.
func TestGuard_WithoutCacheIsInert(t *testing.T) {
	t.Parallel()

	var guard *Guard

	ctx := context.Background()

	admission, err := guard.AcquireAdmission(ctx, uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
	require.NoError(t, err)
	assert.Empty(t, admission.Accounts())

	require.NoError(t, guard.EnsureOpen(ctx, uuid.New(), uuid.New(), []uuid.UUID{uuid.New()}))

	admission.Release(ctx)
}

// assertErrorCode checks the business code a refusal carries, which is the part of
// the error the API contract pins.
func assertErrorCode(t *testing.T, err error, code string) {
	t.Helper()

	var conflict pkg.EntityConflictError
	if errors.As(err, &conflict) {
		assert.Equal(t, code, conflict.Code)

		return
	}

	var unavailable pkg.ServiceUnavailableError
	if errors.As(err, &unavailable) {
		assert.Equal(t, code, unavailable.Code)

		return
	}

	t.Fatalf("error %v carries no business code", err)
}

// orderedPair returns two account identifiers whose acquisition order is known, so
// a test can name the first one the guard reaches.
func orderedPair() (uuid.UUID, uuid.UUID) {
	for {
		left, right := uuid.New(), uuid.New()
		if left.String() < right.String() {
			return left, right
		}
	}
}
