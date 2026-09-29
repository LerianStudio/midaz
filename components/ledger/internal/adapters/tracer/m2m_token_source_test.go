// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

var tokenSourceEpoch = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

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

// stubMinter scripts the identity provider. Calls numbered holdFrom or later
// block until release closes; entered receives each call number as it starts.
type stubMinter struct {
	calls    atomic.Int32
	mintFn   func(call int32) (string, error)
	holdFrom int32
	release  chan struct{}
	entered  chan int32
	onMint   func(ctx context.Context)
}

func (m *stubMinter) GetApplicationToken(ctx context.Context, clientID, clientSecret string) (string, error) {
	call := m.calls.Add(1)

	if clientID != "ledger-client" || clientSecret != "ledger-secret" {
		return "", errors.New("unexpected credentials")
	}

	if m.entered != nil {
		m.entered <- call
	}

	if m.release != nil && m.holdFrom > 0 && call >= m.holdFrom {
		<-m.release
	}

	if m.onMint != nil {
		m.onMint(ctx)
	}

	return m.mintFn(call)
}

func clockAt(at time.Time) func() time.Time {
	return func() time.Time { return at }
}

func signedToken(t *testing.T, exp time.Time, subject string) string {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": subject, "exp": exp.Unix()}).SignedString([]byte("test-signing-key"))
	require.NoError(t, err)

	return token
}

func newTestTokenSource(t *testing.T, minter *stubMinter, clk *testClock) *M2MTokenSource {
	t.Helper()

	source, err := NewM2MTokenSource(minter, "ledger-client", "ledger-secret", clk.Now)
	require.NoError(t, err)

	return source
}

// awaitCached waits for a background renewal to land want in the cache.
func awaitCached(t *testing.T, source *M2MTokenSource, want string) {
	t.Helper()

	require.Eventually(t, func() bool {
		state := source.snapshot()
		return state.fresh && state.token == want
	}, 5*time.Second, time.Millisecond)
}

// awaitFlight blocks until the renewal in flight, if any, has returned. Do
// joins a running flight and runs the no-op otherwise; the renewal registers
// its flight before Token returns, so no later call can join the finished one.
func awaitFlight(source *M2MTokenSource) {
	_, _, _ = source.flight.Do("token", func() (any, error) { return nil, nil })
}

func TestM2MTokenSource_CacheHit(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	token := signedToken(t, tokenSourceEpoch.Add(10*time.Minute), "first")
	minter := &stubMinter{mintFn: func(int32) (string, error) { return token, nil }}
	source := newTestTokenSource(t, minter, clk)

	first, err := source.Token(t.Context())
	require.NoError(t, err)

	clk.Advance(8 * time.Minute)

	second, err := source.Token(t.Context())
	require.NoError(t, err)

	assert.Equal(t, token, first)
	assert.Equal(t, token, second)
	assert.Equal(t, int32(1), minter.calls.Load())
}

func TestM2MTokenSource_RenewsInBackgroundSixtySecondsBeforeExpiry(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	exp := tokenSourceEpoch.Add(10 * time.Minute)
	tokens := []string{signedToken(t, exp, "first"), signedToken(t, exp.Add(10*time.Minute), "second")}
	minter := &stubMinter{mintFn: func(call int32) (string, error) { return tokens[call-1], nil }}
	source := newTestTokenSource(t, minter, clk)

	_, err := source.Token(t.Context())
	require.NoError(t, err)

	clk.Advance(9*time.Minute - time.Second)

	cached, err := source.Token(t.Context())
	require.NoError(t, err)
	assert.Equal(t, tokens[0], cached)
	assert.Equal(t, int32(1), minter.calls.Load())

	clk.Advance(time.Second)

	served, err := source.Token(t.Context())
	require.NoError(t, err)
	assert.Equal(t, tokens[0], served, "a valid token is served while the renewal runs")

	awaitCached(t, source, tokens[1])

	renewed, err := source.Token(t.Context())
	require.NoError(t, err)
	assert.Equal(t, tokens[1], renewed)
	assert.Equal(t, int32(2), minter.calls.Load())
}

func TestM2MTokenSource_BackgroundRenewalDoesNotBlockCallers(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	exp := tokenSourceEpoch.Add(10 * time.Minute)
	tokens := []string{signedToken(t, exp, "first"), signedToken(t, exp.Add(10*time.Minute), "second")}
	minter := &stubMinter{
		mintFn:   func(call int32) (string, error) { return tokens[call-1], nil },
		holdFrom: 2,
		release:  make(chan struct{}),
		entered:  make(chan int32, 4),
	}
	source := newTestTokenSource(t, minter, clk)

	_, err := source.Token(t.Context())
	require.NoError(t, err)
	require.Equal(t, int32(1), <-minter.entered)

	clk.Advance(9*time.Minute + 30*time.Second)

	for range 3 {
		served, err := source.Token(t.Context())
		require.NoError(t, err)
		assert.Equal(t, tokens[0], served)
	}

	require.Equal(t, int32(2), <-minter.entered, "one renewal is in flight")
	close(minter.release)

	awaitCached(t, source, tokens[1])
	assert.Equal(t, int32(2), minter.calls.Load(), "callers during the renewal share it")
}

func TestM2MTokenSource_BackgroundRenewalIsDetachedAndBounded(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	exp := tokenSourceEpoch.Add(10 * time.Minute)
	tokens := []string{signedToken(t, exp, "first"), signedToken(t, exp.Add(10*time.Minute), "second")}
	observed := make(chan error, 2)
	deadlines := make(chan bool, 2)
	minter := &stubMinter{
		mintFn:   func(call int32) (string, error) { return tokens[call-1], nil },
		holdFrom: 2,
		release:  make(chan struct{}),
		onMint: func(ctx context.Context) {
			_, ok := ctx.Deadline()
			deadlines <- ok
			observed <- ctx.Err()
		},
	}
	source := newTestTokenSource(t, minter, clk)

	_, err := source.Token(t.Context())
	require.NoError(t, err)
	require.NoError(t, <-observed)
	require.True(t, <-deadlines)

	clk.Advance(9*time.Minute + 30*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	served, err := source.Token(ctx)
	require.NoError(t, err)
	assert.Equal(t, tokens[0], served)
	cancel()

	close(minter.release)
	require.NoError(t, <-observed, "the renewal must not inherit the caller's cancellation")
	require.True(t, <-deadlines, "the renewal carries its own deadline")
	awaitCached(t, source, tokens[1])
}

func TestM2MTokenSource_ShortLivedTokenRenewsAtHalfLife(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	tokens := []string{signedToken(t, tokenSourceEpoch.Add(time.Minute), "first"), signedToken(t, tokenSourceEpoch.Add(2*time.Minute), "second")}
	minter := &stubMinter{mintFn: func(call int32) (string, error) { return tokens[call-1], nil }}
	source := newTestTokenSource(t, minter, clk)

	_, err := source.Token(t.Context())
	require.NoError(t, err)

	clk.Advance(29 * time.Second)

	_, err = source.Token(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int32(1), minter.calls.Load(), "a 60s token is not re-minted before half its life")

	clk.Advance(time.Second)

	_, err = source.Token(t.Context())
	require.NoError(t, err)
	awaitCached(t, source, tokens[1])
	assert.Equal(t, int32(2), minter.calls.Load())
}

func TestM2MTokenSource_TokenWithoutExpiryAssumesSixtySeconds(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	minter := &stubMinter{mintFn: func(int32) (string, error) { return "opaque-token", nil }}
	source := newTestTokenSource(t, minter, clk)

	_, err := source.Token(t.Context())
	require.NoError(t, err)

	clk.Advance(29 * time.Second)

	_, err = source.Token(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int32(1), minter.calls.Load())

	clk.Advance(31 * time.Second)

	_, err = source.Token(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int32(2), minter.calls.Load(), "an expired token is renewed before it is served")
}

func TestM2MTokenSource_ConcurrentCallersWithoutTokenMintOnce(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	token := signedToken(t, tokenSourceEpoch.Add(10*time.Minute), "shared")
	minter := &stubMinter{
		mintFn:   func(int32) (string, error) { return token, nil },
		holdFrom: 1,
		release:  make(chan struct{}),
	}
	source := newTestTokenSource(t, minter, clk)

	const callers = 50

	var wg sync.WaitGroup

	var started sync.WaitGroup

	results := make([]string, callers)
	failures := make([]error, callers)

	for i := range callers {
		wg.Add(1)
		started.Add(1)

		go func() {
			defer wg.Done()

			started.Done()

			results[i], failures[i] = source.Token(context.Background())
		}()
	}

	// The minter holds the one renewal until every caller has started; a
	// caller arriving after it returns reads the cache, so the single mint
	// holds either way.
	started.Wait()
	close(minter.release)
	wg.Wait()

	for i := range callers {
		require.NoError(t, failures[i])
		assert.Equal(t, token, results[i])
	}

	assert.Equal(t, int32(1), minter.calls.Load())
}

func TestM2MTokenSource_FailedRenewalServesCacheAndBacksOff(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	token := signedToken(t, tokenSourceEpoch.Add(10*time.Minute), "cached")
	minter := &stubMinter{mintFn: func(call int32) (string, error) {
		if call == 1 {
			return token, nil
		}

		return "", errors.New("identity provider unreachable")
	}}
	source := newTestTokenSource(t, minter, clk)

	_, err := source.Token(t.Context())
	require.NoError(t, err)

	clk.Advance(9*time.Minute + 30*time.Second)

	served, err := source.Token(t.Context())
	require.NoError(t, err)
	assert.Equal(t, token, served)
	awaitFlight(source)
	assert.Equal(t, int32(2), minter.calls.Load())

	clk.Advance(tokenRenewBackoff - time.Second)

	served, err = source.Token(t.Context())
	require.NoError(t, err)
	assert.Equal(t, token, served)
	assert.Equal(t, int32(2), minter.calls.Load(), "no renewal is attempted during the backoff")

	clk.Advance(time.Second)

	_, err = source.Token(t.Context())
	require.NoError(t, err)
	awaitFlight(source)
	assert.Equal(t, int32(3), minter.calls.Load(), "renewal resumes after the backoff")
}

func TestM2MTokenSource_MintFailureWithoutValidTokenReturnsSentinel(t *testing.T) {
	t.Parallel()

	t.Run("no cached token", func(t *testing.T) {
		t.Parallel()

		clk := &testClock{now: tokenSourceEpoch}
		minter := &stubMinter{mintFn: func(int32) (string, error) { return "", errors.New("identity provider unreachable") }}
		source := newTestTokenSource(t, minter, clk)

		token, err := source.Token(t.Context())
		require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		assert.Empty(t, token)

		for range 10 {
			_, err = source.Token(t.Context())
			require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		}

		assert.Equal(t, int32(1), minter.calls.Load(), "callers during the pause fail fast without minting")

		clk.Advance(tokenRenewBackoff)

		for range 10 {
			_, err = source.Token(t.Context())
			require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		}

		assert.Equal(t, int32(2), minter.calls.Load(), "one mint per backoff window")
	})

	t.Run("cached token expired", func(t *testing.T) {
		t.Parallel()

		clk := &testClock{now: tokenSourceEpoch}
		expired := signedToken(t, tokenSourceEpoch.Add(2*time.Minute), "expired")
		minter := &stubMinter{mintFn: func(call int32) (string, error) {
			if call == 1 {
				return expired, nil
			}

			return "", errors.New("identity provider unreachable")
		}}
		source := newTestTokenSource(t, minter, clk)

		_, err := source.Token(t.Context())
		require.NoError(t, err)

		clk.Advance(2 * time.Minute)

		token, err := source.Token(t.Context())
		require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
		assert.Empty(t, token)
	})

	t.Run("auth disabled mints an empty token", func(t *testing.T) {
		t.Parallel()

		clk := &testClock{now: tokenSourceEpoch}
		minter := &stubMinter{mintFn: func(int32) (string, error) { return "", nil }}
		source := newTestTokenSource(t, minter, clk)

		_, err := source.Token(t.Context())
		require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
	})
}

func TestM2MTokenSource_InvalidateForcesFreshMint(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	exp := tokenSourceEpoch.Add(10 * time.Minute)
	tokens := []string{signedToken(t, exp, "first"), signedToken(t, exp, "second")}
	minter := &stubMinter{mintFn: func(call int32) (string, error) { return tokens[call-1], nil }}
	source := newTestTokenSource(t, minter, clk)

	first, err := source.Token(t.Context())
	require.NoError(t, err)

	assert.False(t, source.Invalidate(context.Background(), ""), "no token presented, nothing to replace")
	assert.True(t, source.Invalidate(context.Background(), "some-other-token"), "a token that is not cached is already replaced")

	same, err := source.Token(t.Context())
	require.NoError(t, err)
	assert.Equal(t, first, same, "a token that is not cached invalidates nothing")
	assert.Equal(t, int32(1), minter.calls.Load())

	assert.False(t, source.Invalidate(context.Background(), first), "a token minted within the backoff is kept")

	kept, err := source.Token(t.Context())
	require.NoError(t, err)
	assert.Equal(t, first, kept)
	assert.Equal(t, int32(1), minter.calls.Load())

	clk.Advance(tokenRenewBackoff)
	assert.True(t, source.Invalidate(context.Background(), first))

	fresh, err := source.Token(t.Context())
	require.NoError(t, err)
	assert.Equal(t, tokens[1], fresh)
	assert.Equal(t, int32(2), minter.calls.Load())

	assert.True(t, source.Invalidate(context.Background(), first))

	kept, err = source.Token(t.Context())
	require.NoError(t, err)
	assert.Equal(t, tokens[1], kept, "a stale rejection never discards its replacement")
}

func TestM2MTokenSource_InvalidateKeepsMintPause(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	token := signedToken(t, tokenSourceEpoch.Add(10*time.Minute), "cached")
	minter := &stubMinter{mintFn: func(call int32) (string, error) {
		if call == 1 {
			return token, nil
		}

		return "", errors.New("identity provider unreachable")
	}}
	source := newTestTokenSource(t, minter, clk)

	_, err := source.Token(t.Context())
	require.NoError(t, err)

	clk.Advance(9*time.Minute + 30*time.Second)

	_, err = source.Token(t.Context())
	require.NoError(t, err)
	awaitFlight(source)
	require.Equal(t, int32(2), minter.calls.Load())

	require.True(t, source.Invalidate(context.Background(), token))

	_, err = source.Token(t.Context())
	require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
	assert.Equal(t, int32(2), minter.calls.Load(), "invalidating a token does not lift the pause of a failed mint")
}

func TestM2MTokenSource_BlockedCallerCancellationDoesNotFailRenewal(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	token := signedToken(t, tokenSourceEpoch.Add(10*time.Minute), "detached")
	mintErr := make(chan error, 1)
	minter := &stubMinter{
		mintFn:   func(int32) (string, error) { return token, nil },
		holdFrom: 1,
		release:  make(chan struct{}),
		entered:  make(chan int32, 1),
		onMint:   func(ctx context.Context) { mintErr <- ctx.Err() },
	}
	source := newTestTokenSource(t, minter, clk)

	ctx, cancel := context.WithCancel(context.Background())

	first := make(chan error, 1)

	go func() {
		_, err := source.Token(ctx)
		first <- err
	}()

	<-minter.entered
	cancel()

	err := <-first
	require.ErrorIs(t, err, constant.ErrTracerTokenUnavailable)
	require.ErrorIs(t, err, context.Canceled)

	close(minter.release)
	require.NoError(t, <-mintErr, "the shared renewal must not inherit the first caller's cancellation")

	awaitCached(t, source, token)

	served, err := source.Token(t.Context())
	require.NoError(t, err)
	assert.Equal(t, token, served)
	assert.Equal(t, int32(1), minter.calls.Load())
}

func TestM2MTokenSource_BlockingRenewalHasItsOwnDeadline(t *testing.T) {
	t.Parallel()

	clk := &testClock{now: tokenSourceEpoch}
	deadlines := make(chan bool, 1)
	minter := &stubMinter{
		mintFn: func(int32) (string, error) { return "opaque-token", nil },
		onMint: func(ctx context.Context) {
			_, ok := ctx.Deadline()
			deadlines <- ok
		},
	}
	source := newTestTokenSource(t, minter, clk)

	_, err := source.Token(context.Background())
	require.NoError(t, err)
	assert.True(t, <-deadlines, "a caller without a deadline must still get a bounded renewal")
}

func TestTokenWindow(t *testing.T) {
	t.Parallel()

	for name, scenario := range map[string]struct {
		lifetime, margin time.Duration
	}{
		"long-lived token keeps a sixty second margin": {lifetime: 10 * time.Minute, margin: 60 * time.Second},
		"sixty second token renews at half life":       {lifetime: 60 * time.Second, margin: 30 * time.Second},
		"ten second token renews at half life":         {lifetime: 10 * time.Second, margin: 5 * time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			exp := tokenSourceEpoch.Add(scenario.lifetime)
			refreshAt, expiresAt := tokenWindow(signedToken(t, exp, "window"), tokenSourceEpoch)
			assert.True(t, exp.Equal(expiresAt), "expiresAt %s", expiresAt)
			assert.True(t, exp.Add(-scenario.margin).Equal(refreshAt), "refreshAt %s", refreshAt)
		})
	}
}

func TestNewM2MTokenSource_RequiresCredentials(t *testing.T) {
	t.Parallel()

	minter := &stubMinter{mintFn: func(int32) (string, error) { return "", nil }}

	for name, build := range map[string]func() (*M2MTokenSource, error){
		"nil minter": func() (*M2MTokenSource, error) {
			return NewM2MTokenSource(nil, "id", "secret", clockAt(tokenSourceEpoch))
		},
		"empty client": func() (*M2MTokenSource, error) {
			return NewM2MTokenSource(minter, " ", "secret", clockAt(tokenSourceEpoch))
		},
		"empty secret": func() (*M2MTokenSource, error) { return NewM2MTokenSource(minter, "id", "", clockAt(tokenSourceEpoch)) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			source, err := build()
			require.Error(t, err)
			assert.Nil(t, source)
		})
	}
}
