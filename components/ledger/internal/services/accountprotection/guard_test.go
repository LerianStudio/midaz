// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accountprotection

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// recordingContext wires an in-memory span recorder into the tracking context so a
// test can read back the class the guard recorded on its own span.
func recordingContext() (context.Context, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	return libObservability.ContextWithTracer(context.Background(), provider.Tracer("accountprotection-test")), recorder
}

// findSpan returns the first ended span with the given name, failing the test when
// the guard never opened it.
func findSpan(t *testing.T, recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, span := range recorder.Ended() {
		if span.Name() == name {
			return span
		}
	}

	t.Fatalf("span %q was not recorded", name)

	return nil
}

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

// fakeMarkers is an in-memory SeedAdmissionStore with the conditional semantics
// of the cache: one ownership key per account holds either one exclusive owner or
// any number of seed admissions, never both, and each is released only by its own
// token.
type fakeMarkers struct {
	mu sync.Mutex

	closing    map[markerKey]string
	closed     map[markerKey]time.Time
	ownership  map[markerKey]string
	admissions map[markerKey]map[string]struct{}

	closingErr error
	closedErr  error
	setErr     error
	acquireErr error

	// closingErrAfter fails the closing marker read once it was answered this many
	// times, so a test can break the read that follows a refused acquisition.
	closingErrAfter int
	closingReads    int

	acquireCalls int
	releaseCalls int
	setCalls     int

	// beforeAcquire runs outside the lock before each acquisition of either mode,
	// so a test can order two callers, or change the markers, without sleeping.
	beforeAcquire func(accountID uuid.UUID)
}

func newFakeMarkers() *fakeMarkers {
	return &fakeMarkers{
		closing:    map[markerKey]string{},
		closed:     map[markerKey]time.Time{},
		ownership:  map[markerKey]string{},
		admissions: map[markerKey]map[string]struct{}{},
	}
}

func (f *fakeMarkers) GetAccountClosingMarker(_ context.Context, organizationID, ledgerID, accountID uuid.UUID) (string, bool, error) {
	if f.closingErr != nil {
		return "", false, f.closingErr
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.closingReads++
	if f.closingErrAfter > 0 && f.closingReads > f.closingErrAfter {
		return "", false, errCacheUnavailable
	}

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

	if f.acquireErr != nil {
		return false, f.acquireErr
	}

	key := markerKey{organizationID, ledgerID, accountID}
	if _, owned := f.ownership[key]; owned || len(f.admissions[key]) > 0 {
		return false, nil
	}

	f.ownership[key] = token

	return true, nil
}

func (f *fakeMarkers) ReleaseAccountAdminOwnership(_ context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.releaseCalls++

	key := markerKey{organizationID, ledgerID, accountID}
	if f.ownership[key] != token {
		return false, nil
	}

	delete(f.ownership, key)

	return true, nil
}

func (f *fakeMarkers) AdmitAccountSeed(_ context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	if f.beforeAcquire != nil {
		f.beforeAcquire(accountID)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.acquireCalls++

	if f.acquireErr != nil {
		return false, f.acquireErr
	}

	key := markerKey{organizationID, ledgerID, accountID}
	if _, owned := f.ownership[key]; owned {
		return false, nil
	}

	if f.admissions[key] == nil {
		f.admissions[key] = map[string]struct{}{}
	}

	f.admissions[key][token] = struct{}{}

	return true, nil
}

func (f *fakeMarkers) ReleaseAccountSeed(_ context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.releaseCalls++

	key := markerKey{organizationID, ledgerID, accountID}
	if _, admitted := f.admissions[key][token]; !admitted {
		return false, nil
	}

	delete(f.admissions[key], token)

	if len(f.admissions[key]) == 0 {
		delete(f.admissions, key)
	}

	return true, nil
}

// ownedCount is the number of accounts whose ownership key is held, by an
// exclusive owner or by at least one seed admission.
func (f *fakeMarkers) ownedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.ownership) + len(f.admissions)
}

// admittedTokens returns the live seed admissions of one account.
func (f *fakeMarkers) admittedTokens(key markerKey) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	tokens := make([]string, 0, len(f.admissions[key]))
	for token := range f.admissions[key] {
		tokens = append(tokens, token)
	}

	return tokens
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
		guard:          NewSeedAdmissionGuard(accounts, markers),
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

// acquisitionMode names one of the two ways an operation takes an account, so a
// guarantee both modes share is asserted once per mode.
type acquisitionMode struct {
	name    string
	acquire func(g *Guard, ctx context.Context, organizationID, ledgerID uuid.UUID, accountIDs []uuid.UUID) (*Admission, error)
	span    string
}

var acquisitionModes = []acquisitionMode{
	{name: "seed admission", acquire: (*Guard).AcquireSeedAdmission, span: "exec.acquire_account_seed_admission"},
	{name: "exclusive ownership", acquire: (*Guard).AcquireExclusive, span: "exec.acquire_account_admission"},
}

// TestAcquireSeedAdmission_ConcurrentLoadsShareTheAccount proves two cache-miss
// loads of the same account are compatible, so both are admitted, each under its
// own token, and each release gives back only its own admission.
func TestAcquireSeedAdmission_ConcurrentLoadsShareTheAccount(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()
	ctx := context.Background()

	first, err := f.guard.AcquireSeedAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)

	second, err := f.guard.AcquireSeedAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err, "a second load of the same account must not be refused by the first")

	require.NotEmpty(t, first.Token())
	require.NotEmpty(t, second.Token())
	assert.NotEqual(t, first.Token(), second.Token(), "each admission carries its own token")
	assert.ElementsMatch(t, []string{first.Token(), second.Token()}, f.markers.admittedTokens(f.key(accountID)))

	first.Release(ctx)
	assert.Equal(t, []string{second.Token()}, f.markers.admittedTokens(f.key(accountID)),
		"a release gives back only its own admission")

	second.Release(ctx)
	assert.Zero(t, f.markers.ownedCount())
}

// TestAcquireExclusive_ContentionRefusesTheSecondCaller keeps the administrative
// operations serialized among themselves: the winner keeps the account and the
// loser is refused as busy, because no closing marker names a closing.
func TestAcquireExclusive_ContentionRefusesTheSecondCaller(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()
	ctx := context.Background()

	first, err := f.guard.AcquireExclusive(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)

	_, err = f.guard.AcquireExclusive(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.Error(t, err)
	assertErrorCode(t, err, constant.ErrAccountAdministrativeOperationInProgress.Error())

	first.Release(ctx)
	assert.Zero(t, f.markers.ownedCount())

	second, err := f.guard.AcquireExclusive(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err, "the account is free once the winner released it")
	second.Release(ctx)
}

// TestAcquireExclusive_LiveSeedAdmissionRefusesAsBusy proves a closing, a
// balance creation or a deletion never runs beside a load that is about to seed
// the account. The refusal is the busy code, not a closing, and it leaves the
// load's admission in place.
func TestAcquireExclusive_LiveSeedAdmissionRefusesAsBusy(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()

	seed, err := f.guard.AcquireSeedAdmission(context.Background(), f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)

	ctx, recorder := recordingContext()

	_, err = f.guard.AcquireExclusive(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})

	require.Error(t, err)
	assertErrorCode(t, err, constant.ErrAccountAdministrativeOperationInProgress.Error())
	assert.Equal(t, []string{seed.Token()}, f.markers.admittedTokens(f.key(accountID)), "the load keeps its admission")

	span := findSpan(t, recorder, "exec.acquire_account_admission")
	assert.NotEqual(t, codes.Error, span.Status().Code, "a busy account is a business refusal")
}

// TestAcquisition_RefusedByAnotherHolderNamesWhoHoldsIt proves, in either mode, a
// refused acquisition reads the closing marker again, so a closing that owns the
// account answers as a closing and any other holder answers as busy.
func TestAcquisition_RefusedByAnotherHolderNamesWhoHoldsIt(t *testing.T) {
	t.Parallel()

	for _, mode := range acquisitionModes {
		for _, closing := range []bool{false, true} {
			want := constant.ErrAccountAdministrativeOperationInProgress
			if closing {
				want = constant.ErrAccountClosingInProgress
			}

			t.Run(fmt.Sprintf("%s, closing marker installed=%t", mode.name, closing), func(t *testing.T) {
				t.Parallel()

				f := newGuardFixture()
				accountID := uuid.New()
				f.markers.ownership[f.key(accountID)] = uuid.NewString()

				// A closing takes the ownership before it installs its marker, so the
				// marker can appear between the check that precedes the acquisition
				// and the refusal.
				f.markers.beforeAcquire = func(uuid.UUID) {
					if closing {
						f.markers.mu.Lock()
						f.markers.closing[f.key(accountID)] = uuid.NewString()
						f.markers.mu.Unlock()
					}
				}

				ctx, recorder := recordingContext()

				_, err := mode.acquire(f.guard, ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})

				require.Error(t, err)
				assertErrorCode(t, err, want.Error())
				assert.Empty(t, f.markers.admittedTokens(f.key(accountID)), "a refusal takes nothing")

				span := findSpan(t, recorder, mode.span)
				assert.NotEqual(t, codes.Error, span.Status().Code, "another holder is a business refusal")
			})
		}
	}
}

// TestAcquisition_ClosingMarkerRefusesBeforeAnyAcquisition proves a closing in
// flight refuses every operation it coordinates with, before anything is taken,
// and the refusal is the business outcome the coordination exists to produce.
func TestAcquisition_ClosingMarkerRefusesBeforeAnyAcquisition(t *testing.T) {
	t.Parallel()

	for _, mode := range acquisitionModes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()

			f := newGuardFixture()
			accountID := uuid.New()
			f.markers.closing[f.key(accountID)] = uuid.NewString()

			ctx, recorder := recordingContext()

			_, err := mode.acquire(f.guard, ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})

			require.Error(t, err)
			assertErrorCode(t, err, constant.ErrAccountClosingInProgress.Error())
			assert.Zero(t, f.markers.acquireCalls, "nothing is attempted over a closing")
			assert.Zero(t, f.markers.ownedCount(), "a refusal must take no ownership")

			span := findSpan(t, recorder, mode.span)
			assert.NotEqual(t, codes.Error, span.Status().Code, "a business refusal keeps the span green")
		})
	}
}

// TestAcquisition_UnreadableProtectionIsTechnical proves every state the guard
// cannot read refuses as an indeterminate protection instead of being read as
// absence, and that the refusal is recorded as the technical failure it is: the
// cache could not answer, which is not the same as another operation holding the
// account.
func TestAcquisition_UnreadableProtectionIsTechnical(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		break_ func(f *guardFixture, accountID uuid.UUID)
	}{
		{
			name:   "the closing marker cannot be read",
			break_: func(f *guardFixture, _ uuid.UUID) { f.markers.closingErr = errCacheUnavailable },
		},
		{
			name:   "the acquisition fails",
			break_: func(f *guardFixture, _ uuid.UUID) { f.markers.acquireErr = errCacheUnavailable },
		},
		{
			name: "the closing marker cannot be read after a refusal",
			break_: func(f *guardFixture, accountID uuid.UUID) {
				f.markers.ownership[f.key(accountID)] = uuid.NewString()
				f.markers.closingErrAfter = 1
			},
		},
	}

	for _, mode := range acquisitionModes {
		for _, tt := range tests {
			t.Run(mode.name+", "+tt.name, func(t *testing.T) {
				t.Parallel()

				f := newGuardFixture()
				accountID := uuid.New()
				tt.break_(f, accountID)

				ownedBefore := f.markers.ownedCount()

				ctx, recorder := recordingContext()

				_, err := mode.acquire(f.guard, ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})

				require.Error(t, err)
				assertErrorCode(t, err, constant.ErrAccountClosingProtectionIndeterminate.Error())
				assert.Equal(t, ownedBefore, f.markers.ownedCount(), "a refusal must take no ownership")

				span := findSpan(t, recorder, mode.span)
				assert.Equal(t, codes.Error, span.Status().Code, "an unreadable protection surface is a technical failure")
			})
		}
	}
}

// TestAcquireSeedAdmission_WithoutASharedSurfaceRefuses proves a guard built over
// a cache that cannot hold shared admissions never falls back to an exclusive one,
// which would make concurrent loads refuse each other again, and never admits a
// seed unprotected.
func TestAcquireSeedAdmission_WithoutASharedSurfaceRefuses(t *testing.T) {
	t.Parallel()

	markers := newFakeMarkers()
	guard := NewGuard(newFakeAccounts(), exclusiveOnly{markers})

	ctx, recorder := recordingContext()

	_, err := guard.AcquireSeedAdmission(ctx, uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})

	require.Error(t, err)
	assertErrorCode(t, err, constant.ErrAccountClosingProtectionIndeterminate.Error())
	assert.Zero(t, markers.acquireCalls)
	assert.Zero(t, markers.ownedCount())

	span := findSpan(t, recorder, "exec.acquire_account_seed_admission")
	assert.Equal(t, codes.Error, span.Status().Code)
}

// exclusiveOnly hides the shared admission of a fake, leaving the surface an
// exclusive-only deployment of the guard sees.
type exclusiveOnly struct{ MarkerStore }

// TestAcquisition_PartialAcquisitionIsReleased proves a refusal on the second
// account gives back the first, in either mode, and touches nothing it does not
// own.
func TestAcquisition_PartialAcquisitionIsReleased(t *testing.T) {
	t.Parallel()

	t.Run("seed admission refused by an exclusive owner", func(t *testing.T) {
		t.Parallel()

		f := newGuardFixture()
		first, second := orderedPair()

		rivalToken := uuid.NewString()
		f.markers.ownership[f.key(second)] = rivalToken

		_, err := f.guard.AcquireSeedAdmission(context.Background(), f.organizationID, f.ledgerID, []uuid.UUID{first, second})
		require.Error(t, err)
		assertErrorCode(t, err, constant.ErrAccountAdministrativeOperationInProgress.Error())

		assert.Empty(t, f.markers.admittedTokens(f.key(first)), "the admission already taken is given back")
		assert.Equal(t, 1, f.markers.ownedCount(), "only the rival's ownership survives")
		assert.Equal(t, rivalToken, f.markers.ownership[f.key(second)], "a key owned by another operation is never released")
	})

	t.Run("exclusive ownership refused by a live seed admission", func(t *testing.T) {
		t.Parallel()

		f := newGuardFixture()
		ctx := context.Background()
		first, second := orderedPair()

		seed, err := f.guard.AcquireSeedAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{second})
		require.NoError(t, err)

		_, err = f.guard.AcquireExclusive(ctx, f.organizationID, f.ledgerID, []uuid.UUID{first, second})
		require.Error(t, err)
		assertErrorCode(t, err, constant.ErrAccountAdministrativeOperationInProgress.Error())

		assert.NotContains(t, f.markers.ownership, f.key(first), "the ownership already taken is given back")
		assert.Equal(t, 1, f.markers.ownedCount(), "only the load's admission survives")
		assert.Equal(t, []string{seed.Token()}, f.markers.admittedTokens(f.key(second)))
	})
}

// TestAcquireExclusive_StableOrderAvoidsDeadlock runs two callers over the same
// two accounts in opposite request orders. A stable acquisition order means both
// contend on the same account first, so one of them always completes.
func TestAcquireExclusive_StableOrderAvoidsDeadlock(t *testing.T) {
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
		admission, err := f.guard.AcquireExclusive(ctx, f.organizationID, f.ledgerID, []uuid.UUID{first, second})
		if err == nil {
			admission.Release(ctx)
		}

		results <- err
	}()

	go func() {
		admission, err := f.guard.AcquireExclusive(ctx, f.organizationID, f.ledgerID, []uuid.UUID{second, first})
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

// TestAcquisition_DuplicateAccountsCollapse proves the external companion costs no
// second acquisition in either mode: it belongs to the same account, so it is
// already covered and never recurses.
func TestAcquisition_DuplicateAccountsCollapse(t *testing.T) {
	t.Parallel()

	for _, mode := range acquisitionModes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()

			f := newGuardFixture()
			accountID := uuid.New()
			ctx := context.Background()

			admission, err := mode.acquire(f.guard, ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID, accountID, uuid.Nil})
			require.NoError(t, err)

			assert.Equal(t, []uuid.UUID{accountID}, admission.Accounts())
			assert.Equal(t, 1, f.markers.acquireCalls)

			admission.Release(ctx)
			assert.Zero(t, f.markers.ownedCount())
		})
	}
}

// TestAdmission_IndeterminateOutcomeKeepsItsHold proves work whose result is
// unknown keeps its protection until reconciliation resolves it, in either mode.
// Nothing releases it on the way out and nothing releases it by age. A kept seed
// admission still refuses a closing, and it does not refuse other loads.
func TestAdmission_IndeterminateOutcomeKeepsItsHold(t *testing.T) {
	t.Parallel()

	for _, mode := range acquisitionModes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()

			f := newGuardFixture()
			accountID := uuid.New()
			ctx := context.Background()

			admission, err := mode.acquire(f.guard, ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
			require.NoError(t, err)

			admission.MarkIndeterminate()
			admission.Release(ctx)

			assert.Zero(t, f.markers.releaseCalls, "an unknown result is never released")
			assert.Equal(t, 1, f.markers.ownedCount())

			_, err = f.guard.AcquireExclusive(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
			assert.Error(t, err, "the account stays protected for reconciliation")
		})
	}

	t.Run("a kept seed admission does not refuse other loads", func(t *testing.T) {
		t.Parallel()

		f := newGuardFixture()
		accountID := uuid.New()
		ctx := context.Background()

		kept, err := f.guard.AcquireSeedAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
		require.NoError(t, err)

		kept.MarkIndeterminate()
		kept.Release(ctx)

		other, err := f.guard.AcquireSeedAdmission(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
		require.NoError(t, err)

		other.Release(ctx)
		assert.Equal(t, []string{kept.Token()}, f.markers.admittedTokens(f.key(accountID)))
	})
}

// TestAdmission_ReleaseIsIdempotent proves a second release neither fails nor
// drops an ownership taken afterwards by another operation.
func TestAdmission_ReleaseIsIdempotent(t *testing.T) {
	t.Parallel()

	f := newGuardFixture()
	accountID := uuid.New()
	ctx := context.Background()

	admission, err := f.guard.AcquireExclusive(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)

	admission.Release(ctx)

	successor, err := f.guard.AcquireExclusive(ctx, f.organizationID, f.ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)

	admission.Release(ctx)

	assert.Equal(t, successor.Token(), f.markers.ownership[f.key(accountID)],
		"a late release must not drop the successor's ownership")
}

// TestGuard_WithoutCacheIsInert proves a guard with no protection surface changes
// nothing: it owns nothing and refuses nothing, in either mode.
func TestGuard_WithoutCacheIsInert(t *testing.T) {
	t.Parallel()

	var guard *Guard

	ctx := context.Background()

	for _, mode := range acquisitionModes {
		admission, err := mode.acquire(guard, ctx, uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.NoError(t, err, mode.name)
		assert.Empty(t, admission.Accounts(), mode.name)

		admission.Release(ctx)
	}

	require.NoError(t, guard.EnsureOpen(ctx, uuid.New(), uuid.New(), []uuid.UUID{uuid.New()}))

	admission, err := NewSeedAdmissionGuard(nil, nil).AcquireSeedAdmission(ctx, uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
	require.NoError(t, err)
	assert.Empty(t, admission.Accounts())
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
