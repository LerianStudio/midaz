// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/LerianStudio/lib-commons/v7/commons/secretsmanager"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

// m2mEpoch is the fixed instant every token-source test starts its clock at.
var m2mEpoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// testClock is a manually advanced clock shared by a token source and its test.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock { return &testClock{now: m2mEpoch} }

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

// jwtExpiringAt builds an unsigned-in-spirit JWT whose only meaningful claim is
// exp; the token source reads it unverified.
func jwtExpiringAt(t *testing.T, label string, exp time.Time) string {
	t.Helper()

	claims := jwt.MapClaims{"sub": label}
	if !exp.IsZero() {
		claims["exp"] = exp.Unix()
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-signing-key"))
	require.NoError(t, err)

	return token
}

// fakeMinter is a scripted TokenMinter. tokenFor returns the token to mint for a
// client id and the mint number (1-based) for that client id.
type fakeMinter struct {
	mu       sync.Mutex
	calls    map[string]int
	tokenFor func(clientID string, n int) (string, error)
	// gate, once armed, holds every mint until the test closes it; entered
	// receives one value per mint that reached the gate.
	gate    chan struct{}
	entered chan struct{}
}

// arm makes every later mint signal entered and wait for gate.
func (m *fakeMinter) arm() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.gate = make(chan struct{})
	m.entered = make(chan struct{}, 64)
}

func (m *fakeMinter) GetApplicationToken(_ context.Context, clientID, clientSecret string) (string, error) {
	if clientSecret == "" {
		return "", errors.New("fake minter: empty secret")
	}

	m.mu.Lock()
	if m.calls == nil {
		m.calls = map[string]int{}
	}

	m.calls[clientID]++
	n := m.calls[clientID]
	gate, entered := m.gate, m.entered
	m.mu.Unlock()

	if entered != nil {
		entered <- struct{}{}
	}

	if gate != nil {
		<-gate
	}

	return m.tokenFor(clientID, n)
}

func (m *fakeMinter) mints(clientID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.calls[clientID]
}

func (m *fakeMinter) totalMints() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	total := 0
	for _, n := range m.calls {
		total += n
	}

	return total
}

// labelledTokens mints "<clientID>-<n>" JWTs expiring lifetime after the clock.
func labelledTokens(t *testing.T, clock *testClock, lifetime time.Duration) func(string, int) (string, error) {
	return func(clientID string, n int) (string, error) {
		var exp time.Time
		if lifetime > 0 {
			exp = clock.Now().Add(lifetime)
		}

		return jwtExpiringAt(t, fmt.Sprintf("%s-%d", clientID, n), exp), nil
	}
}

func subjectOf(t *testing.T, token string) string {
	t.Helper()

	claims := jwt.MapClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(token, claims)
	require.NoError(t, err)

	sub, err := claims.GetSubject()
	require.NoError(t, err)

	return sub
}

// fakeCredentials is a CredentialProvider returning "client-<tenant>" pairs and
// recording the tenants it was asked for and the invalidations.
type fakeCredentials struct {
	mu          sync.Mutex
	requested   []string
	invalidated []string
	err         error
}

func (f *fakeCredentials) Credentials(_ context.Context, tenantID string) (Credentials, error) {
	f.mu.Lock()
	f.requested = append(f.requested, tenantID)
	f.mu.Unlock()

	if f.err != nil {
		return Credentials{}, f.err
	}

	return Credentials{ClientID: "client-" + tenantID, ClientSecret: "secret-" + tenantID}, nil
}

func (f *fakeCredentials) Invalidate(tenantID string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.invalidated = append(f.invalidated, tenantID)
}

func (f *fakeCredentials) requestedTenants() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.requested...)
}

func (f *fakeCredentials) invalidatedTenants() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.invalidated...)
}

func newTestTokenSource(t *testing.T, minter TokenMinter, creds CredentialProvider, clock *testClock, opts ...M2MTokenSourceOption) *M2MTokenSource {
	t.Helper()

	scheduler := &fakeScheduler{}

	all := append([]M2MTokenSourceOption{WithTokenClock(clock.Now), WithRefreshScheduler(scheduler.schedule)}, opts...)

	src, err := NewM2MTokenSource(minter, creds, all...)
	require.NoError(t, err)

	src.jitter = noJitter

	return src
}

// noJitter pins the failure-window jitter at zero, so a window is exactly its
// base duration.
func noJitter() float64 { return 0 }

func TestNewM2MTokenSource_RequiresCollaborators(t *testing.T) {
	t.Parallel()

	_, err := NewM2MTokenSource(nil, NewStaticCredentials("id", "secret"))
	require.Error(t, err)

	_, err = NewM2MTokenSource(&fakeMinter{}, nil)
	require.Error(t, err)
}

func TestM2MTokenSource_RefreshPoint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		lifetime      time.Duration
		justBefore    time.Duration
		refreshesAt   time.Duration
		withoutExpiry bool
	}{
		{name: "80 percent of a long lifetime", lifetime: 10 * time.Minute, justBefore: 8*time.Minute - time.Second, refreshesAt: 8 * time.Minute},
		{name: "80 percent of a short lifetime, still before its expiry", lifetime: 100 * time.Second, justBefore: 79 * time.Second, refreshesAt: 80 * time.Second},
		{name: "five minute lifetime when exp is absent", withoutExpiry: true, justBefore: 4*time.Minute - time.Second, refreshesAt: 4 * time.Minute},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			clock := newTestClock()
			lifetime := tc.lifetime

			if tc.withoutExpiry {
				lifetime = 0
			}

			minter := &fakeMinter{tokenFor: labelledTokens(t, clock, lifetime)}
			src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock)

			assert.Zero(t, minter.totalMints(), "the token is minted lazily, on first use")

			first, err := src.Token(context.Background())
			require.NoError(t, err)
			assert.Equal(t, "ledger-1", subjectOf(t, first))

			clock.Advance(tc.justBefore)

			reused, err := src.Token(context.Background())
			require.NoError(t, err)
			assert.Equal(t, first, reused, "the token is reused until its refresh point")
			assert.Equal(t, 1, minter.mints("ledger"))

			clock.Advance(tc.refreshesAt - tc.justBefore)

			require.Eventually(t, func() bool {
				token, tokenErr := src.Token(context.Background())

				return tokenErr == nil && subjectOf(t, token) == "ledger-2"
			}, 5*time.Second, time.Millisecond, "the refresh point mints a new token")
			assert.Equal(t, 2, minter.mints("ledger"))
		})
	}
}

func TestM2MTokenSource_RefreshAhead(t *testing.T) {
	t.Parallel()

	t.Run("a stale but valid token is served at once while one background mint replaces it", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock)

		first, err := src.Token(context.Background())
		require.NoError(t, err)

		minter.arm()
		clock.Advance(9 * time.Minute)

		const callers = 16

		start := make(chan struct{})
		tokens := make([]string, callers)
		errs := make([]error, callers)

		var wg sync.WaitGroup

		for i := range callers {
			wg.Add(1)

			go func() {
				defer wg.Done()

				<-start

				tokens[i], errs[i] = src.Token(context.Background())
			}()
		}

		close(start)
		wg.Wait()

		for i := range callers {
			require.NoError(t, errs[i])
			assert.Equal(t, first, tokens[i], "every caller is served the cached token without waiting for the mint")
		}

		<-minter.entered

		select {
		case <-minter.entered:
			t.Fatal("a second mint started while the background refresh was in flight")
		case <-time.After(50 * time.Millisecond):
		}

		close(minter.gate)

		require.Eventually(t, func() bool {
			token, tokenErr := src.Token(context.Background())

			return tokenErr == nil && subjectOf(t, token) == "ledger-2"
		}, 5*time.Second, time.Millisecond)

		assert.Equal(t, 2, minter.mints("ledger"), "the refresh is exactly one background mint")
	})

	t.Run("a token past its expiry blocks the caller until a new one is minted", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)

		minter.arm()
		clock.Advance(10*time.Minute - tokenExpiryMargin)

		done := make(chan string, 1)

		go func() {
			token, tokenErr := src.Token(context.Background())
			if tokenErr != nil {
				done <- tokenErr.Error()

				return
			}

			done <- subjectOf(t, token)
		}()

		<-minter.entered

		select {
		case got := <-done:
			t.Fatalf("the caller returned %q before the mint finished", got)
		case <-time.After(50 * time.Millisecond):
		}

		close(minter.gate)

		assert.Equal(t, "ledger-2", <-done)
	})

	t.Run("a failed background refresh keeps serving the valid token", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()

		var fail atomic.Bool

		minter := &fakeMinter{tokenFor: func(clientID string, n int) (string, error) {
			if fail.Load() {
				return "", errors.New("access manager unreachable")
			}

			return labelledTokens(t, clock, 10*time.Minute)(clientID, n)
		}}
		src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock)

		first, err := src.Token(context.Background())
		require.NoError(t, err)

		fail.Store(true)

		clock.Advance(9 * time.Minute)

		token, err := src.Token(context.Background())
		require.NoError(t, err)
		assert.Equal(t, first, token)

		require.Eventually(t, func() bool { return minter.totalMints() == 2 }, 5*time.Second, time.Millisecond)
		require.Eventually(t, func() bool { return refreshFailed(src, "") }, 5*time.Second, time.Millisecond)

		token, err = src.Token(context.Background())
		require.NoError(t, err)
		assert.Equal(t, first, token, "the token stays served until its expiry")

		clock.Advance(mintFailureWindow - time.Millisecond)

		for range 4 {
			token, err = src.Token(context.Background())
			require.NoError(t, err)
			assert.Equal(t, first, token)
		}

		assert.True(t, refreshSettled(src, ""), "no refresh is attempted inside the failure window")
		assert.Equal(t, 2, minter.totalMints(), "no refresh is attempted inside the failure window")

		clock.Advance(time.Millisecond)

		token, err = src.Token(context.Background())
		require.NoError(t, err)
		assert.Equal(t, first, token)

		require.Eventually(t, func() bool { return minter.totalMints() == 3 && refreshFailed(src, "") }, 5*time.Second, time.Millisecond)

		_, err = src.Token(context.Background())
		require.NoError(t, err)
		assert.True(t, refreshSettled(src, ""), "exactly one refresh follows the window")
		assert.Equal(t, 3, minter.totalMints(), "exactly one refresh follows the window")
	})
}

// refreshSettled reports whether no refresh of the tenant is in flight.
func refreshSettled(src *M2MTokenSource, key string) bool {
	src.mu.Lock()
	defer src.mu.Unlock()

	st, ok := src.tenants[key]

	return !ok || !st.refreshing
}

// refreshFailed reports whether the tenant's last refresh has finished with a
// recorded failure.
func refreshFailed(src *M2MTokenSource, key string) bool {
	src.mu.Lock()
	defer src.mu.Unlock()

	st, ok := src.tenants[key]

	return ok && !st.refreshing && st.failure.err != nil
}

// tracked reports whether the source still holds any state for the tenant.
func tracked(src *M2MTokenSource, key string) bool {
	src.mu.Lock()
	defer src.mu.Unlock()

	_, ok := src.tenants[key]

	return ok
}

// toggledMinter mints labelled tokens of lifetime until fail is set, and fails
// every mint while it is.
func toggledMinter(t *testing.T, clock *testClock, lifetime time.Duration) (*fakeMinter, *atomic.Bool) {
	t.Helper()

	fail := &atomic.Bool{}
	tokens := labelledTokens(t, clock, lifetime)

	return &fakeMinter{tokenFor: func(clientID string, n int) (string, error) {
		if fail.Load() {
			return "", errors.New("access manager unreachable")
		}

		return tokens(clientID, n)
	}}, fail
}

func TestM2MTokenSource_SingleFlight(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
	minter.arm()
	src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock)

	const callers = 32

	start := make(chan struct{})
	tokens := make([]string, callers)
	errs := make([]error, callers)

	var wg sync.WaitGroup

	for i := range callers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			tokens[i], errs[i] = src.Token(context.Background())
		}()
	}

	close(start)
	<-minter.entered

	select {
	case <-minter.entered:
		t.Fatal("a second mint started while the first was in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(minter.gate)
	wg.Wait()

	for i := range callers {
		require.NoError(t, errs[i])
		assert.Equal(t, tokens[0], tokens[i], "every concurrent caller gets the one minted token")
	}

	assert.Equal(t, 1, minter.totalMints(), "concurrent callers collapse into one mint")
}

func TestM2MTokenSource_CanonicalTenantKey(t *testing.T) {
	t.Parallel()

	t.Run("a dashed and a dashless tenant id share one entry and one mint", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		creds := &fakeCredentials{}
		src := newTestTokenSource(t, minter, creds, clock)

		dashed, err := src.Token(tmcore.ContextWithTenantID(context.Background(), "0198A1B2-C3D4-7E5F-8A9B-0C1D2E3F4A5B"))
		require.NoError(t, err)

		dashless, err := src.Token(tmcore.ContextWithTenantID(context.Background(), "0198a1b2c3d47e5f8a9b0c1d2e3f4a5b"))
		require.NoError(t, err)

		assert.Equal(t, dashed, dashless)
		assert.Equal(t, 1, minter.totalMints())
		assert.Equal(t, []string{"0198a1b2c3d47e5f8a9b0c1d2e3f4a5b"}, creds.requestedTenants(),
			"the credential is resolved under the canonical tenant id")
	})

	t.Run("a tenant id that fails canonicalization is never sent", func(t *testing.T) {
		t.Parallel()

		minter := &fakeMinter{tokenFor: func(string, int) (string, error) { return "never", nil }}
		creds := &fakeCredentials{}
		src := newTestTokenSource(t, minter, creds, newTestClock())

		token, err := src.Token(tmcore.ContextWithTenantID(context.Background(), "tenant with spaces"))
		require.Error(t, err)
		assert.Empty(t, token)
		assert.ErrorIs(t, err, ErrTracerUnavailable)
		assert.ErrorIs(t, err, ErrTracerCredentialUnavailable)
		assert.ErrorIs(t, err, tmcore.ErrInvalidTenantIDFormat)
		assert.Zero(t, minter.totalMints())
		assert.Empty(t, creds.requestedTenants())
	})
}

func TestM2MTokenSource_MintFailureWindow(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: func(string, int) (string, error) { return "", errors.New("access manager unreachable") }}
	creds := &fakeCredentials{}
	src := newTestTokenSource(t, minter, creds, clock)

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

	_, err := src.Token(ctx)
	require.Error(t, err)

	for range 10 {
		_, err = src.Token(ctx)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTracerCredentialUnavailable)
		assert.Contains(t, err.Error(), "access manager unreachable", "the window returns the last mint error")
	}

	assert.Equal(t, 1, minter.totalMints(), "a failed mint is not repeated inside the failure window")

	_, err = src.Token(tmcore.ContextWithTenantID(context.Background(), "tenant-b"))
	require.Error(t, err)
	assert.Equal(t, 2, minter.totalMints(), "the window is per tenant")

	clock.Advance(mintFailureWindow)

	_, err = src.Token(ctx)
	require.Error(t, err)
	assert.Equal(t, 3, minter.totalMints(), "the window closes and the next call mints again")

	assert.Empty(t, creds.invalidatedTenants(), "an unreachable Access Manager says nothing about the credential")
}

func TestM2MTokenSource_PanicInMintIsRecovered(t *testing.T) {
	t.Parallel()

	minter := &fakeMinter{tokenFor: func(string, int) (string, error) { panic("minter exploded") }}
	src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), newTestClock())

	token, err := src.Token(context.Background())
	require.Error(t, err)
	assert.Empty(t, token)
	assert.ErrorIs(t, err, ErrTracerUnavailable)
	assert.ErrorIs(t, err, ErrTracerCredentialUnavailable)
}

func TestM2MTokenSource_Warm(t *testing.T) {
	t.Parallel()

	t.Run("warming mints the token once in the background", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock)

		src.Warm(context.Background(), &recordingLogger{})

		require.Eventually(t, func() bool { return minter.totalMints() == 1 }, 5*time.Second, time.Millisecond)
		require.Eventually(t, func() bool {
			token, err := src.Token(context.Background())

			return err == nil && subjectOf(t, token) == "ledger-1"
		}, 5*time.Second, time.Millisecond)
		assert.Equal(t, 1, minter.totalMints(), "the first call is served from the warmed cache")
	})

	t.Run("warming never blocks and a failure is logged at warn", func(t *testing.T) {
		t.Parallel()

		minter := &fakeMinter{tokenFor: func(string, int) (string, error) { return "", errors.New("access manager unreachable") }}
		minter.arm()
		src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), newTestClock())
		logger := &recordingLogger{}

		src.Warm(context.Background(), logger)

		<-minter.entered
		close(minter.gate)

		require.Eventually(t, func() bool { return len(logger.atLevel(libLog.LevelWarn)) == 1 }, 5*time.Second, time.Millisecond)
		assert.NotContains(t, logger.atLevel(libLog.LevelWarn)[0], "s3cret")
	})
}

func TestM2MTokenSource_OneEntryPerTenant(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
	src := newTestTokenSource(t, minter, &fakeCredentials{}, clock)

	ctxA := tmcore.ContextWithTenantID(context.Background(), "tenant-a")
	ctxB := tmcore.ContextWithTenantID(context.Background(), "tenant-b")

	tokenA, err := src.Token(ctxA)
	require.NoError(t, err)

	tokenB, err := src.Token(ctxB)
	require.NoError(t, err)

	assert.Equal(t, "client-tenant-a-1", subjectOf(t, tokenA))
	assert.Equal(t, "client-tenant-b-1", subjectOf(t, tokenB))

	againA, err := src.Token(ctxA)
	require.NoError(t, err)
	assert.Equal(t, tokenA, againA, "a tenant never receives another tenant's token")

	assert.Equal(t, 1, minter.mints("client-tenant-a"))
	assert.Equal(t, 1, minter.mints("client-tenant-b"))
}

func TestM2MTokenSource_InvalidateDropsOnlyTheRejectedToken(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
	creds := &fakeCredentials{}
	src := newTestTokenSource(t, minter, creds, clock)

	ctxA := tmcore.ContextWithTenantID(context.Background(), "tenant-a")
	ctxB := tmcore.ContextWithTenantID(context.Background(), "tenant-b")

	tokenA, err := src.Token(ctxA)
	require.NoError(t, err)

	tokenB, err := src.Token(ctxB)
	require.NoError(t, err)

	assert.True(t, src.Invalidate(ctxA, "a-token-already-replaced"), "the cached token is already another one")

	stillA, err := src.Token(ctxA)
	require.NoError(t, err)
	assert.Equal(t, tokenA, stillA, "a rejection of an older token never deletes the newer one")
	assert.Equal(t, 1, minter.mints("client-tenant-a"))

	assert.False(t, src.Invalidate(ctxA, tokenA), "a token rejected moments after its mint is kept")

	clock.Advance(rejectedTokenMinAge)

	assert.True(t, src.Invalidate(ctxA, tokenA))

	refreshed, err := src.Token(ctxA)
	require.NoError(t, err)
	assert.Equal(t, "client-tenant-a-2", subjectOf(t, refreshed))

	stillB, err := src.Token(ctxB)
	require.NoError(t, err)
	assert.Equal(t, tokenB, stillB, "invalidating one tenant leaves the others cached")

	assert.Empty(t, creds.invalidatedTenants(), "a rejected token never drops the credential")
}

func TestM2MTokenSource_FailuresAreUnavailableAndNotSent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		minter *fakeMinter
		creds  CredentialProvider
	}{
		{
			name:   "mint failure",
			minter: &fakeMinter{tokenFor: func(string, int) (string, error) { return "", errors.New("access manager unreachable") }},
			creds:  NewStaticCredentials("ledger", "s3cret"),
		},
		{
			name:   "empty token is a mint failure",
			minter: &fakeMinter{tokenFor: func(string, int) (string, error) { return "", nil }},
			creds:  NewStaticCredentials("ledger", "s3cret"),
		},
		{
			name:   "missing tenant credential",
			minter: &fakeMinter{tokenFor: func(string, int) (string, error) { return "never", nil }},
			creds:  &fakeCredentials{err: secretsmanager.ErrM2MCredentialsNotFound},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := newTestTokenSource(t, tc.minter, tc.creds, newTestClock())

			token, err := src.Token(context.Background())

			require.Error(t, err)
			assert.Empty(t, token)
			assert.ErrorIs(t, err, ErrTracerUnavailable)
			assert.ErrorIs(t, err, ErrTracerCredentialUnavailable)
			assert.NotErrorIs(t, err, ErrTracerNoAnswer)
			assert.NotContains(t, err.Error(), "s3cret", "no secret reaches the error")
		})
	}
}

func TestM2MTokenSource_CallerGivesUpWhileMintContinues(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
	minter.arm()
	src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)

	go func() {
		_, err := src.Token(ctx)
		done <- err
	}()

	<-minter.entered
	cancel()

	err := <-done
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTracerUnavailable)
	assert.NotErrorIs(t, err, ErrTracerCredentialUnavailable, "the credential did not fail")
	assert.NotErrorIs(t, err, ErrTracerNoAnswer, "the caller gave up before anything was sent")

	close(minter.gate)

	token, err := src.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ledger-1", subjectOf(t, token), "the detached mint finished and filled the cache")
	assert.Equal(t, 1, minter.totalMints())
}

// recordingLogger captures every log line rendered with its fields.
type recordingLogger struct {
	mu    sync.Mutex
	lines map[int][]string
}

func (l *recordingLogger) Log(_ context.Context, level int, msg string, fields ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lines == nil {
		l.lines = map[int][]string{}
	}

	parts := []string{msg}
	for _, f := range fields {
		parts = append(parts, fmt.Sprint(f))
	}

	l.lines[level] = append(l.lines[level], strings.Join(parts, " "))
}

func (l *recordingLogger) With(_ ...any) libLog.Logger      { return l }
func (l *recordingLogger) WithGroup(_ string) libLog.Logger { return l }
func (l *recordingLogger) Enabled(_ int) bool               { return true }
func (l *recordingLogger) Sync(_ context.Context) error     { return nil }

func (l *recordingLogger) atLevel(level int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return append([]string(nil), l.lines[level]...)
}

func TestTokenDeadlines_RefreshComesBeforeExpiry(t *testing.T) {
	t.Parallel()

	for _, lifetime := range []time.Duration{10 * time.Second, 31 * time.Second, 100 * time.Second, 149 * time.Second, 150 * time.Second, 10 * time.Minute} {
		t.Run(lifetime.String(), func(t *testing.T) {
			t.Parallel()

			token := jwtExpiringAt(t, "ledger", m2mEpoch.Add(lifetime))

			refreshAt, expiresAt, err := tokenDeadlines(token, m2mEpoch)
			require.NoError(t, err)

			assert.True(t, refreshAt.Before(expiresAt), "a token is refreshed while it is still served")
			assert.True(t, refreshAt.After(m2mEpoch))
			assert.False(t, expiresAt.After(m2mEpoch.Add(lifetime)), "a token is never served past its exp")
		})
	}
}

func TestTokenDeadlines(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		token       string
		wantRefresh time.Duration
		wantExpiry  time.Duration
		wantErr     error
	}{
		{name: "10 minute lifetime", token: jwtExpiringAt(t, "ledger", m2mEpoch.Add(10*time.Minute)), wantRefresh: 8 * time.Minute, wantExpiry: 10*time.Minute - tokenExpiryMargin},
		{name: "100 second lifetime", token: jwtExpiringAt(t, "ledger", m2mEpoch.Add(100*time.Second)), wantRefresh: 80 * time.Second, wantExpiry: 90 * time.Second},
		{name: "lifetime capped at 24 hours", token: jwtExpiringAt(t, "ledger", m2mEpoch.Add(48*time.Hour)), wantRefresh: 24 * time.Hour * 4 / 5, wantExpiry: 24*time.Hour - tokenExpiryMargin},
		{name: "far-future exp does not overflow", token: jwtExpiringAt(t, "ledger", time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)), wantRefresh: 24 * time.Hour * 4 / 5, wantExpiry: 24*time.Hour - tokenExpiryMargin},
		{name: "exp at the mint instant", token: jwtExpiringAt(t, "ledger", m2mEpoch), wantErr: errTokenAlreadyExpired},
		{name: "exp before the mint instant", token: jwtExpiringAt(t, "ledger", m2mEpoch.Add(-time.Minute)), wantErr: errTokenAlreadyExpired},
		{name: "opaque token gets the default lifetime", token: "opaque-token", wantRefresh: 4 * time.Minute, wantExpiry: 5*time.Minute - tokenExpiryMargin},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			refreshAt, expiresAt, err := tokenDeadlines(tc.token, m2mEpoch)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantRefresh, refreshAt.Sub(m2mEpoch))
			assert.Equal(t, tc.wantExpiry, expiresAt.Sub(m2mEpoch))
		})
	}
}

func TestM2MTokenSource_AlreadyExpiredTokenIsAMintFailure(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: func(clientID string, n int) (string, error) {
		return jwtExpiringAt(t, fmt.Sprintf("%s-%d", clientID, n), clock.Now()), nil
	}}
	creds := &fakeCredentials{}
	src := newTestTokenSource(t, minter, creds, clock)
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

	token, err := src.Token(ctx)
	require.Error(t, err)
	assert.Empty(t, token)
	assert.ErrorIs(t, err, ErrTracerCredentialUnavailable)
	assert.ErrorIs(t, err, errTokenAlreadyExpired)

	_, err = src.Token(ctx)
	require.ErrorIs(t, err, errTokenAlreadyExpired, "the failure window answers from memory")
	assert.Equal(t, 1, minter.totalMints(), "an expired token is never cached as valid and opens the failure window")
	assert.Empty(t, creds.invalidatedTenants(), "an expired token says nothing about the credential")
}

func TestFailureWindow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		streak int
		jitter float64
		want   time.Duration
	}{
		{streak: 1, want: time.Second},
		{streak: 2, want: 2 * time.Second},
		{streak: 3, want: 4 * time.Second},
		{streak: 4, want: 8 * time.Second},
		{streak: 5, want: 15 * time.Second},
		{streak: 64, want: 15 * time.Second},
		{streak: 1, jitter: 0.5, want: 1125 * time.Millisecond},
		{streak: 5, jitter: 0.5, want: 15*time.Second + 1875*time.Millisecond},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("streak %d jitter %v", tc.streak, tc.jitter), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, failureWindow(tc.streak, tc.jitter))
		})
	}
}

func TestM2MTokenSource_ConsecutiveFailuresWidenTheWindowForEveryTenant(t *testing.T) {
	t.Parallel()

	clock := newTestClock()

	var fail atomic.Bool

	fail.Store(true)

	tokens := labelledTokens(t, clock, 10*time.Minute)
	minter := &fakeMinter{tokenFor: func(clientID string, n int) (string, error) {
		if fail.Load() {
			return "", errors.New("access manager unreachable")
		}

		return tokens(clientID, n)
	}}
	src := newTestTokenSource(t, minter, &fakeCredentials{}, clock)

	const tenants = 50

	ctxs := make([]context.Context, tenants)
	for i := range tenants {
		ctxs[i] = tmcore.ContextWithTenantID(context.Background(), fmt.Sprintf("tenant-%02d", i))
	}

	for elapsed := time.Duration(0); elapsed < time.Minute; elapsed += 250 * time.Millisecond {
		for _, ctx := range ctxs {
			_, err := src.Token(ctx)
			require.Error(t, err)
		}

		clock.Advance(250 * time.Millisecond)
	}

	for i := range tenants {
		assert.Equal(t, 7, minter.mints(fmt.Sprintf("client-tenant-%02d", i)),
			"attempts at 0s, 1s, 3s, 7s, 15s, 30s and 45s: the window doubles per failure and is capped")
	}

	fail.Store(false)
	clock.Advance(15 * time.Second)

	_, err := src.Token(ctxs[0])
	require.NoError(t, err)

	fail.Store(true)
	clock.Advance(10 * time.Minute)

	_, err = src.Token(ctxs[0])
	require.Error(t, err)

	clock.Advance(mintFailureWindow)

	_, err = src.Token(ctxs[0])
	require.Error(t, err)
	assert.Equal(t, 10, minter.mints("client-tenant-00"), "a successful mint resets the streak to the base window")
}

func TestM2MTokenSource_FailureWindowIsJittered(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: func(string, int) (string, error) { return "", errors.New("access manager unreachable") }}

	src, err := NewM2MTokenSource(minter, NewStaticCredentials("ledger", "s3cret"),
		WithTokenClock(clock.Now), WithRefreshScheduler((&fakeScheduler{}).schedule))
	require.NoError(t, err)

	src.jitter = func() float64 { return 0.5 }

	_, err = src.Token(context.Background())
	require.Error(t, err)

	clock.Advance(1125*time.Millisecond - time.Nanosecond)

	_, err = src.Token(context.Background())
	require.Error(t, err)
	assert.Equal(t, 1, minter.totalMints(), "the jittered window is still open")

	clock.Advance(time.Nanosecond)

	_, err = src.Token(context.Background())
	require.Error(t, err)
	assert.Equal(t, 2, minter.totalMints(), "the window closes at its jittered end")
}

func TestM2MTokenSource_ConcurrentMintsForTwoTenantsStayApart(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
	minter.arm()
	src := newTestTokenSource(t, minter, &fakeCredentials{}, clock)

	type result struct {
		token string
		err   error
	}

	results := map[string]chan result{"tenant-a": make(chan result, 1), "tenant-b": make(chan result, 1)}

	for tenant, out := range results {
		go func() {
			token, err := src.Token(tmcore.ContextWithTenantID(context.Background(), tenant))
			out <- result{token: token, err: err}
		}()
	}

	<-minter.entered
	<-minter.entered
	close(minter.gate)

	for tenant, out := range results {
		got := <-out
		require.NoError(t, got.err)
		assert.Equal(t, "client-"+tenant+"-1", subjectOf(t, got.token), "each tenant gets the token minted with its own credential")
	}

	assert.Equal(t, 1, minter.mints("client-tenant-a"))
	assert.Equal(t, 1, minter.mints("client-tenant-b"))
}

func TestM2MTokenSource_BackgroundRefreshFailureIsLoggedOnce(t *testing.T) {
	t.Parallel()

	clock := newTestClock()

	var fail atomic.Bool

	minter := &fakeMinter{tokenFor: func(clientID string, n int) (string, error) {
		if fail.Load() {
			return "", errors.New("access manager unreachable")
		}

		return labelledTokens(t, clock, 10*time.Minute)(clientID, n)
	}}
	src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock)
	logger := &recordingLogger{}
	ctx := libObservability.ContextWithLogger(context.Background(), logger)

	first, err := src.Token(ctx)
	require.NoError(t, err)

	fail.Store(true)
	clock.Advance(9 * time.Minute)

	for range 4 {
		token, tokenErr := src.Token(ctx)
		require.NoError(t, tokenErr)
		assert.Equal(t, first, token)
	}

	require.Eventually(t, func() bool { return len(logger.atLevel(libLog.LevelWarn)) == 1 }, 5*time.Second, time.Millisecond)

	warn := logger.atLevel(libLog.LevelWarn)[0]
	assert.Contains(t, warn, "Tracer seam token refresh failed; serving the cached token until it expires")
	assert.Contains(t, warn, "access manager unreachable")
	assert.NotContains(t, warn, first, "no token reaches the log")
	assert.NotContains(t, warn, "s3cret")
	assert.Empty(t, logger.atLevel(libLog.LevelError))
	assert.Equal(t, 2, minter.totalMints(), "the failure window holds the next refresh")
}

func TestM2MTokenSource_CallerDeadlineWhileWaitingIsNotACredentialFailure(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
	minter.arm()
	src := newTestTokenSource(t, minter, NewStaticCredentials("ledger", "s3cret"), clock)

	t.Cleanup(func() { close(minter.gate) })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	token, err := src.Token(ctx)
	require.Error(t, err)
	assert.Empty(t, token)
	assert.ErrorIs(t, err, ErrTracerUnavailable)
	assert.NotErrorIs(t, err, ErrTracerCredentialUnavailable, "the credential did not fail; the caller stopped waiting")
	assert.NotErrorIs(t, err, ErrTracerNoAnswer, "nothing was sent")

	mapped := mapGRPCError(err)
	assert.ErrorIs(t, mapped, ErrTracerUnavailable)
	assert.NotErrorIs(t, mapped, ErrTracerNoAnswer, "the client never reads an abandoned wait as an unanswered call")
	assert.NotErrorIs(t, mapped, ErrTracerCredentialUnavailable)
}

func TestM2MTokenSource_MintFailureDropsTheCredentialOnlyOnARefusal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		mintErr   error
		wantDrops bool
	}{
		{name: "an Access Manager refusal drops it", mintErr: commons.Response{Code: "AUT-1004", Message: "invalid client"}, wantDrops: true},
		{name: "an empty token drops it", mintErr: nil, wantDrops: true},
		{name: "a transport failure keeps it", mintErr: errors.New("dial tcp: connection refused")},
		{name: "a deadline keeps it", mintErr: fmt.Errorf("failed to make request: %w", context.DeadlineExceeded)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			minter := &fakeMinter{tokenFor: func(string, int) (string, error) { return "", tc.mintErr }}
			creds := &fakeCredentials{}
			src := newTestTokenSource(t, minter, creds, newTestClock())

			_, err := src.Token(tmcore.ContextWithTenantID(context.Background(), "tenant-a"))
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrTracerCredentialUnavailable)

			if tc.wantDrops {
				assert.Equal(t, []string{"tenant-a"}, creds.invalidatedTenants())
			} else {
				assert.Empty(t, creds.invalidatedTenants(), "a transient failure says nothing about the credential")
			}
		})
	}
}

// fakeTimer is one refresh a fakeScheduler holds until the test fires it.
type fakeTimer struct {
	after     time.Duration
	run       func()
	cancelled atomic.Bool
}

// fakeScheduler is a RefreshScheduler that never fires on its own.
type fakeScheduler struct {
	mu     sync.Mutex
	timers []*fakeTimer
}

func (f *fakeScheduler) schedule(after time.Duration, run func()) func() {
	timer := &fakeTimer{after: after, run: run}

	f.mu.Lock()
	f.timers = append(f.timers, timer)
	f.mu.Unlock()

	return func() { timer.cancelled.Store(true) }
}

// pending returns the scheduled refreshes neither cancelled nor fired.
func (f *fakeScheduler) pending() []*fakeTimer {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []*fakeTimer

	for _, timer := range f.timers {
		if !timer.cancelled.Load() {
			out = append(out, timer)
		}
	}

	return out
}

// fireOnly fires the single pending refresh and marks it fired.
func (f *fakeScheduler) fireOnly(t *testing.T) time.Duration {
	t.Helper()

	pending := f.pending()
	require.Len(t, pending, 1, "exactly one refresh is scheduled per tenant")

	timer := pending[0]
	timer.cancelled.Store(true)
	timer.run()

	return timer.after
}

func newScheduledTokenSource(t *testing.T, minter TokenMinter, clock *testClock, opts ...M2MTokenSourceOption) (*M2MTokenSource, *fakeScheduler) {
	t.Helper()

	scheduler := &fakeScheduler{}

	src, err := NewM2MTokenSource(minter, NewStaticCredentials("ledger", "s3cret"),
		append([]M2MTokenSourceOption{WithTokenClock(clock.Now), WithRefreshScheduler(scheduler.schedule)}, opts...)...)
	require.NoError(t, err)

	src.jitter = noJitter

	t.Cleanup(func() { _ = src.Close() })

	return src, scheduler
}

func TestM2MTokenSource_ProactiveRefresh(t *testing.T) {
	t.Parallel()

	t.Run("an idle tenant's token is refreshed at its refresh point and never goes cold", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)

		clock.Advance(8 * time.Minute)
		assert.Equal(t, 8*time.Minute, scheduler.fireOnly(t), "the refresh is scheduled at 80% of the lifetime")

		require.Eventually(t, func() bool { return minter.totalMints() == 2 }, 5*time.Second, time.Millisecond)
		require.Eventually(t, func() bool { return len(scheduler.pending()) == 1 }, 5*time.Second, time.Millisecond)
		assert.Equal(t, 8*time.Minute, scheduler.pending()[0].after, "the new token schedules its own refresh")

		clock.Advance(2 * time.Minute)

		token, err := src.Token(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "ledger-2", subjectOf(t, token), "past the first token's expiry the caller is served without waiting")
		assert.Equal(t, 2, minter.totalMints())
	})

	t.Run("a tenant idle past the horizon stops being refreshed and is let go at expiry", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)

		for mints := 2; ; mints++ {
			clock.Advance(8 * time.Minute)
			scheduler.fireOnly(t)

			if clock.Now().Sub(m2mEpoch) > idleRefreshHorizon {
				break
			}

			require.Eventually(t, func() bool { return minter.totalMints() == mints }, 5*time.Second, time.Millisecond)
			require.Eventually(t, func() bool { return len(scheduler.pending()) == 1 && refreshSettled(src, "") }, 5*time.Second, time.Millisecond)
		}

		assert.True(t, refreshSettled(src, ""), "no refresh past the idle horizon")
		assert.Equal(t, 4, minter.totalMints(), "no refresh past the idle horizon")

		cleanup := scheduler.pending()
		require.Len(t, cleanup, 1, "the still-valid token is kept until its expiry")
		assert.Equal(t, 90*time.Second, cleanup[0].after, "the cleanup runs at the token's expiry")

		clock.Advance(cleanup[0].after)
		scheduler.fireOnly(t)

		assert.False(t, tracked(src, ""), "an idle tenant is forgotten once its token expires")
		assert.Empty(t, scheduler.pending())

		token, err := src.Token(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "ledger-5", subjectOf(t, token), "the tenant's next call mints again")
	})

	t.Run("a tenant idle past the horizon is still served its valid token", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, time.Hour)}
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)

		clock.Advance(48 * time.Minute)
		assert.Equal(t, 48*time.Minute, scheduler.fireOnly(t))
		assert.True(t, refreshSettled(src, ""))
		assert.Equal(t, 1, minter.totalMints(), "an idle tenant's token is not refreshed")

		clock.Advance(2 * time.Minute)

		token, err := src.Token(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "ledger-1", subjectOf(t, token), "the valid token is served from the cache, without waiting for a mint")

		require.Eventually(t, func() bool { return minter.totalMints() == 2 && refreshSettled(src, "") }, 5*time.Second, time.Millisecond,
			"a tenant calling again past the refresh point is refreshed again")
	})

	t.Run("a tenant that keeps calling is refreshed past the horizon", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)

		for mints := 2; clock.Now().Sub(m2mEpoch) <= idleRefreshHorizon; mints++ {
			clock.Advance(7 * time.Minute)

			_, err = src.Token(context.Background())
			require.NoError(t, err)

			clock.Advance(time.Minute)
			scheduler.fireOnly(t)

			require.Eventually(t, func() bool {
				return minter.totalMints() == mints && len(scheduler.pending()) == 1 && refreshSettled(src, "")
			}, 5*time.Second, time.Millisecond)
		}

		assert.Equal(t, 32*time.Minute, clock.Now().Sub(m2mEpoch))
		assert.Equal(t, 5, minter.totalMints(), "a tenant calling within the horizon is refreshed past it")
		assert.Len(t, scheduler.pending(), 1, "and its next refresh stays scheduled")
	})

	t.Run("a failed scheduled refresh is retried and the idle tenant keeps a valid token", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter, fail := toggledMinter(t, clock, 10*time.Minute)
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)

		fail.Store(true)
		clock.Advance(8 * time.Minute)
		scheduler.fireOnly(t)

		require.Eventually(t, func() bool { return refreshFailed(src, "") && len(scheduler.pending()) == 1 }, 5*time.Second, time.Millisecond,
			"a failed refresh arms a retry while the token is valid")

		retry := scheduler.pending()[0]
		assert.Equal(t, mintFailureWindow, retry.after)

		fail.Store(false)
		clock.Advance(retry.after)
		scheduler.fireOnly(t)

		require.Eventually(t, func() bool {
			return minter.totalMints() == 3 && len(scheduler.pending()) == 1 && refreshSettled(src, "")
		}, 5*time.Second, time.Millisecond)

		clock.Advance(2 * time.Minute)

		token, err := src.Token(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "ledger-3", subjectOf(t, token), "past the first token's expiry the retried token is served")
		assert.Equal(t, 3, minter.totalMints())
		assert.False(t, refreshFailed(src, ""), "a successful mint clears the failure")

		fail.Store(true)
		clock.Advance(scheduler.pending()[0].after - 2*time.Minute)
		scheduler.fireOnly(t)

		require.Eventually(t, func() bool { return refreshFailed(src, "") && len(scheduler.pending()) == 1 }, 5*time.Second, time.Millisecond)
		assert.Equal(t, mintFailureWindow, scheduler.pending()[0].after, "a successful mint resets the backoff")
	})

	t.Run("a scheduled refresh deferred by one in flight arms a retry", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)

		minter.arm()
		clock.Advance(9 * time.Minute)

		_, err = src.Token(context.Background())
		require.NoError(t, err)

		<-minter.entered

		scheduler.fireOnly(t)

		retry := scheduler.pending()
		require.Len(t, retry, 1, "the deferred refresh keeps the chain alive")
		assert.Equal(t, mintFailureWindow, retry[0].after)

		close(minter.gate)

		require.Eventually(t, func() bool {
			pending := scheduler.pending()

			return minter.totalMints() == 2 && len(pending) == 1 && pending[0].after == 8*time.Minute && refreshSettled(src, "")
		}, 5*time.Second, time.Millisecond, "the refresh in flight replaces the retry with the new token's refresh")
	})

	t.Run("a persistent outage retries with a bounded backoff until the token expires", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter, fail := toggledMinter(t, clock, 10*time.Minute)
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)

		fail.Store(true)
		clock.Advance(8 * time.Minute)
		scheduler.fireOnly(t)

		expiresAt := m2mEpoch.Add(10*time.Minute - tokenExpiryMargin)

		var delays []time.Duration

		for clock.Now().Before(expiresAt) {
			require.Eventually(t, func() bool { return len(scheduler.pending()) == 1 && refreshSettled(src, "") }, 5*time.Second, time.Millisecond)
			require.Less(t, len(delays), 20, "the retries are bounded")

			delays = append(delays, scheduler.pending()[0].after)
			clock.Advance(delays[len(delays)-1])
			scheduler.fireOnly(t)
		}

		assert.Equal(t, []time.Duration{
			time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second,
			22500 * time.Millisecond, 22500 * time.Millisecond, 14 * time.Second,
		}, delays, "the backoff doubles, is capped at a quarter of the refresh-to-expiry span, and never passes the expiry")
		assert.Equal(t, 9, minter.totalMints(), "one mint per retry, none at the expiry")
		assert.True(t, refreshSettled(src, ""))
		assert.Empty(t, scheduler.pending(), "nothing is retried once the token expired")
	})

	t.Run("a tenant idle for exactly the horizon is still refreshed", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, time.Hour)}
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)

		clock.Advance(48*time.Minute - idleRefreshHorizon)

		_, err = src.Token(context.Background())
		require.NoError(t, err)

		clock.Advance(idleRefreshHorizon)
		scheduler.fireOnly(t)

		require.Eventually(t, func() bool { return minter.totalMints() == 2 && refreshSettled(src, "") }, 5*time.Second, time.Millisecond)
	})

	t.Run("a run that fires before the refresh point is moved back to it", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)

		clock.Advance(time.Minute)
		scheduler.fireOnly(t)

		assert.True(t, refreshSettled(src, ""))
		assert.Equal(t, 1, minter.totalMints(), "a fresh token is not refreshed")

		pending := scheduler.pending()
		require.Len(t, pending, 1)
		assert.Equal(t, 7*time.Minute, pending[0].after)
	})

	t.Run("a run inside the failure window is deferred", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter, fail := toggledMinter(t, clock, 10*time.Minute)
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)

		fail.Store(true)
		clock.Advance(8 * time.Minute)
		scheduler.fireOnly(t)

		require.Eventually(t, func() bool { return refreshFailed(src, "") && len(scheduler.pending()) == 1 }, 5*time.Second, time.Millisecond)

		clock.Advance(mintFailureWindow / 2)
		scheduler.fireOnly(t)

		assert.True(t, refreshSettled(src, ""))
		assert.Equal(t, 2, minter.totalMints(), "no mint inside the failure window")

		pending := scheduler.pending()
		require.Len(t, pending, 1)
		assert.Equal(t, mintFailureWindow, pending[0].after, "the run is retried a failure window later")
	})

	t.Run("a short-lived token's retry never fires inside the failure window", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter, fail := toggledMinter(t, clock, 20*time.Second)
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)

		fail.Store(true)
		clock.Advance(16 * time.Second)
		scheduler.fireOnly(t)

		require.Eventually(t, func() bool { return refreshFailed(src, "") && len(scheduler.pending()) == 1 }, 5*time.Second, time.Millisecond)
		assert.Equal(t, mintFailureWindow, scheduler.pending()[0].after,
			"a backoff capped below the failure window waits for the window to close")
	})

	t.Run("a refresh failing for a token already dropped arms nothing", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter, fail := toggledMinter(t, clock, 10*time.Minute)
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		token, err := src.Token(context.Background())
		require.NoError(t, err)

		fail.Store(true)
		minter.arm()
		clock.Advance(9 * time.Minute)

		_, err = src.Token(context.Background())
		require.NoError(t, err)

		<-minter.entered
		require.True(t, src.Invalidate(context.Background(), token))
		close(minter.gate)

		require.Eventually(t, func() bool { return refreshFailed(src, "") }, 5*time.Second, time.Millisecond)
		require.Eventually(t, func() bool { return minter.totalMints() == 2 }, 5*time.Second, time.Millisecond)
		assert.Never(t, func() bool { return len(scheduler.pending()) > 0 }, 50*time.Millisecond, time.Millisecond,
			"a failure for a token no longer cached schedules no retry")
	})

	t.Run("a refresh landing after close schedules nothing", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)

		minter.arm()
		clock.Advance(9 * time.Minute)

		_, err = src.Token(context.Background())
		require.NoError(t, err)

		<-minter.entered
		require.NoError(t, src.Close())
		close(minter.gate)

		require.Eventually(t, func() bool { return minter.totalMints() == 2 && refreshSettled(src, "") }, 5*time.Second, time.Millisecond)
		assert.Empty(t, scheduler.pending(), "a closed source schedules no refresh for the token it stored")
	})

	t.Run("a replaced token's refresh is cancelled", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(context.Background())
		require.NoError(t, err)
		require.Len(t, scheduler.pending(), 1)

		first := scheduler.pending()[0]

		clock.Advance(9 * time.Minute)

		_, err = src.Token(context.Background())
		require.NoError(t, err)

		require.Eventually(t, func() bool { return minter.totalMints() == 2 }, 5*time.Second, time.Millisecond)
		require.Eventually(t, first.cancelled.Load, 5*time.Second, time.Millisecond)
		assert.Len(t, scheduler.pending(), 1)
	})

	t.Run("an invalidated token's refresh is cancelled", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		token, err := src.Token(context.Background())
		require.NoError(t, err)

		clock.Advance(rejectedTokenMinAge)
		require.True(t, src.Invalidate(context.Background(), token))
		assert.Empty(t, scheduler.pending())
	})

	t.Run("close cancels every refresh and a late one does nothing", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()
		minter := &fakeMinter{tokenFor: labelledTokens(t, clock, 10*time.Minute)}
		src, scheduler := newScheduledTokenSource(t, minter, clock)

		_, err := src.Token(tmcore.ContextWithTenantID(context.Background(), "tenant-a"))
		require.NoError(t, err)

		_, err = src.Token(tmcore.ContextWithTenantID(context.Background(), "tenant-b"))
		require.NoError(t, err)

		require.Len(t, scheduler.pending(), 2)

		late := scheduler.pending()[0]

		require.NoError(t, src.Close())
		assert.Empty(t, scheduler.pending())

		clock.Advance(8 * time.Minute)
		late.run()

		assert.True(t, refreshSettled(src, "tenant-a"), "a closed source mints nothing on its own")
		assert.True(t, refreshSettled(src, "tenant-b"), "a closed source mints nothing on its own")
		assert.Equal(t, 2, minter.totalMints(), "a closed source mints nothing on its own")
	})

	t.Run("a failed refresh is logged once at warn", func(t *testing.T) {
		t.Parallel()

		clock := newTestClock()

		var fail atomic.Bool

		minter := &fakeMinter{tokenFor: func(clientID string, n int) (string, error) {
			if fail.Load() {
				return "", errors.New("access manager unreachable")
			}

			return labelledTokens(t, clock, 10*time.Minute)(clientID, n)
		}}
		logger := &recordingLogger{}
		src, scheduler := newScheduledTokenSource(t, minter, clock, WithTokenLogger(logger))

		first, err := src.Token(context.Background())
		require.NoError(t, err)

		fail.Store(true)
		clock.Advance(8 * time.Minute)
		scheduler.fireOnly(t)

		require.Eventually(t, func() bool { return len(logger.atLevel(libLog.LevelWarn)) == 1 }, 5*time.Second, time.Millisecond)
		assert.Contains(t, logger.atLevel(libLog.LevelWarn)[0], "Tracer seam token refresh failed")
		assert.NotContains(t, logger.atLevel(libLog.LevelWarn)[0], first)

		token, err := src.Token(context.Background())
		require.NoError(t, err)
		assert.Equal(t, first, token, "the cached token stays served")
	})
}

func TestM2MTokenSource_CredentialRefusalDropsTheCredentialAtMostOncePerInterval(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter := &fakeMinter{tokenFor: func(string, int) (string, error) {
		return "", commons.Response{Code: "AUT-1004", Message: "invalid client"}
	}}
	creds := &fakeCredentials{}
	src := newTestTokenSource(t, minter, creds, clock)
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

	_, err := src.Token(ctx)
	require.Error(t, err)
	assert.Equal(t, []string{"tenant-a"}, creds.invalidatedTenants())

	clock.Advance(mintFailureWindow)

	_, err = src.Token(ctx)
	require.Error(t, err)
	assert.Equal(t, 2, minter.totalMints())
	assert.Equal(t, []string{"tenant-a"}, creds.invalidatedTenants(), "a refusal inside the interval keeps the credential")

	_, err = src.Token(tmcore.ContextWithTenantID(context.Background(), "tenant-b"))
	require.Error(t, err)
	assert.Equal(t, []string{"tenant-a", "tenant-b"}, creds.invalidatedTenants(), "the interval is per tenant")

	clock.Advance(credentialRefusalMinInterval - mintFailureWindow)

	_, err = src.Token(ctx)
	require.Error(t, err)
	assert.Equal(t, []string{"tenant-a", "tenant-b", "tenant-a"}, creds.invalidatedTenants(), "the interval elapsed, so the credential is dropped again")
}

func TestM2MTokenSource_RefreshTelemetryCarriesTheTenant(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	minter, fail := toggledMinter(t, clock, 10*time.Minute)
	src := newTestTokenSource(t, minter, &fakeCredentials{}, clock)
	logger := &recordingLogger{}

	tracingCtx, recorder := recordingContext(t)
	ctx := tmcore.ContextWithTenantID(libObservability.ContextWithLogger(tracingCtx, logger), "tenant-a")

	_, err := src.Token(ctx)
	require.NoError(t, err)

	mintSpan := endedSpan(t, recorder, "tracer.m2m_token_source.mint")
	assert.Contains(t, mintSpan.Attributes(), attribute.String("app.request.tenant_id", "tenant-a"))

	fail.Store(true)
	clock.Advance(9 * time.Minute)

	_, err = src.Token(ctx)
	require.NoError(t, err)

	require.Eventually(t, func() bool { return len(logger.atLevel(libLog.LevelWarn)) == 1 }, 5*time.Second, time.Millisecond)
	assert.Contains(t, logger.atLevel(libLog.LevelWarn)[0], fmt.Sprint(libLog.String("tenant_id", "tenant-a")))
}
