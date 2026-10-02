// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	jwt "github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/singleflight"
)

// TokenSource yields the Access Manager application token the ledger presents
// on the reservation seam for the tenant carried by ctx (no tenant in ctx is
// the single-tenant entry). Token never returns an empty token with a nil
// error and every failure marks the call as never sent: a credential that
// could not be obtained is ErrTracerCredentialUnavailable, and a caller that
// stopped waiting for a mint is plain ErrTracerUnavailable. Invalidate is told
// the tracer rejected a token and reports whether a retry would carry a
// different one: it drops the cached token of the tenant in ctx only while it
// is still the rejected one, so a late rejection of a token already replaced
// never discards its replacement.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
	Invalidate(ctx context.Context, rejected string) bool
}

// TokenMinter exchanges an M2M client credential pair for an access token. It
// is satisfied by lib-auth's *middleware.AuthClient.
type TokenMinter interface {
	GetApplicationToken(ctx context.Context, clientID, clientSecret string) (string, error)
}

// RefreshScheduler runs run once after the given delay and returns a function
// that cancels the run if it has not started.
type RefreshScheduler func(after time.Duration, run func()) (cancel func())

// M2MTokenSource mints application tokens through a TokenMinter and caches one
// per canonical tenant id. A cached token is fresh until its refresh point (80%
// of its lifetime) and valid until 30 seconds before its exp, or until 90% of
// its lifetime when that margin would not leave it valid past its refresh
// point; exp is read unverified from the JWT and a token without one is given
// a five-minute lifetime. A fresh token is served as it is; a valid one is
// served at once while a background mint replaces it (one at a time per
// tenant); only a caller with no valid token waits for a mint, and only as long
// as both its own context and the wait timeout allow.
//
// Every stored token also schedules its own refresh at its refresh point. A
// refresh that fails, or is deferred to one in flight or to an open failure
// window, is retried while the token is valid, with a backoff that starts at
// mintFailureWindow, doubles per consecutive failure, is capped at a quarter of
// the span between the refresh point and the expiry, never fires inside the
// failure window and never runs past the expiry. A tenant idle for idleRefreshHorizon is no longer refreshed: its valid
// token stays served until it expires, and the tenant is then forgotten unless
// it called again. Concurrent mints for one tenant collapse into one, which
// runs detached from any caller under its own timeout. A token whose exp is not
// after its mint is a failed mint, and a lifetime is capped at maxTokenLifetime.
// A failed mint opens a failure window in which no caller and no scheduled run
// mints for that tenant: mintFailureWindow after the first consecutive failure,
// doubling per failure up to maxMintFailureWindow, each widened by a random
// share of up to 1/failureJitterDivisor so tenants failing together spread out,
// and reset by a successful mint. An Access Manager refusal drops the
// credential it was attempted with, at most once per tenant per
// credentialRefusalMinInterval. A token the tracer rejects within
// rejectedTokenMinAge of its mint is not replaced, so a tracer that rejects
// every token costs at most one mint per tenant in that window.
type M2MTokenSource struct {
	minter   TokenMinter
	creds    CredentialProvider
	now      func() time.Time
	schedule RefreshScheduler
	// jitter yields the random share, in [0, 1), a failure window is widened by.
	jitter      func() float64
	logger      libLog.Logger
	mintTimeout time.Duration
	waitTimeout time.Duration

	mu      sync.Mutex
	closed  bool
	tenants map[string]*tenantState
	flight  singleflight.Group
}

// M2MTokenSourceOption configures an M2MTokenSource.
type M2MTokenSourceOption func(*M2MTokenSource)

const (
	// DefaultTokenWaitTimeout is how long a caller with no valid token waits for
	// a mint unless WithTokenWaitTimeout says otherwise.
	DefaultTokenWaitTimeout = 3 * time.Second
	// MaxTokenWaitTimeout is the longest wait timeout accepted: the mint it waits
	// for is abandoned at that point, so a longer wait could only wait for nothing.
	MaxTokenWaitTimeout = defaultMintTimeout
)

const (
	// defaultTokenLifetime is the lifetime assumed for a token whose exp cannot
	// be read.
	defaultTokenLifetime = 5 * time.Minute
	// tokenExpiryMargin is how long before exp a token stops being served.
	tokenExpiryMargin = 30 * time.Second
	// tokenRefreshNumerator / tokenRefreshDenominator is the share of a token's
	// lifetime served before it is refreshed (80%).
	tokenRefreshNumerator   = 4
	tokenRefreshDenominator = 5
	// tokenShortExpiryNumerator / tokenShortExpiryDenominator is the share of
	// its lifetime a token too short for tokenExpiryMargin is served (90%).
	tokenShortExpiryNumerator   = 9
	tokenShortExpiryDenominator = 10
	// defaultMintTimeout bounds one credential read plus token mint.
	defaultMintTimeout = 5 * time.Second
	// mintFailureWindow is how long a tenant's first failed mint in a row is
	// answered from memory before another is attempted.
	mintFailureWindow = time.Second
	// maxMintFailureWindow caps the failure window consecutive failures widen.
	maxMintFailureWindow = 15 * time.Second
	// failureJitterDivisor bounds the random widening of a failure window at
	// this share of it.
	failureJitterDivisor = 4
	// maxTokenLifetime caps the lifetime read off a token's exp.
	maxTokenLifetime = 24 * time.Hour
	// rejectedTokenMinAge is how old a token must be before a rejection of it
	// replaces it. A younger token was minted moments ago; the tracer refusing
	// it says the credential is not accepted, which a new mint cannot fix.
	rejectedTokenMinAge = 5 * time.Second
	// idleRefreshHorizon is how long after a tenant's last seam call its token
	// keeps being refreshed proactively.
	idleRefreshHorizon = 30 * time.Minute
	// credentialRefusalMinInterval is the shortest gap between two drops of a
	// tenant's credential after an Access Manager refusal. lib-auth reports every
	// non-2xx answer as a refusal, so a 5xx or 429 burst must not re-read the
	// credential on every mint.
	credentialRefusalMinInterval = 30 * time.Second
	// retryBackoffCeilingDivisor caps a refresh retry's backoff at this share of
	// the span between the token's refresh point and its expiry.
	retryBackoffCeilingDivisor = 4
	// panicComponent names the ledger in recovered-panic signals.
	panicComponent = "ledger"
)

var (
	errTokenMintFailed   = errors.New("tracer seam token mint failed")
	errTokenMintPanicked = errors.New("tracer seam token mint panicked")
	errEmptyToken        = errors.New("tracer seam token mint returned an empty token")
	// errTokenAlreadyExpired marks a minted token whose exp is not after its
	// mint, which can never be served.
	errTokenAlreadyExpired = errors.New("tracer seam token mint returned an already expired token")
	// errTokenWaitAbandoned marks a call never sent because its caller's
	// context ended while it waited for a token mint. The credential did not
	// fail, so it is plain ErrTracerUnavailable.
	errTokenWaitAbandoned = errors.New("tracer seam call not sent: the caller stopped waiting for its token")
)

type cachedToken struct {
	token     string
	mintedAt  time.Time
	refreshAt time.Time
	expiresAt time.Time
}

type cachedFailure struct {
	err   error
	until time.Time
}

// tenantState is everything the source holds for one tenant, so forgetting a
// tenant is a single delete.
type tenantState struct {
	// token is the cached token; an empty token.token means none.
	token cachedToken
	// failure is the last failed mint; a nil failure.err means none.
	failure cachedFailure
	// failureStreak counts the consecutive failed mints; a successful one
	// resets it.
	failureStreak int
	refreshing    bool
	// cancel stops the pending scheduled run, if any; timerSeq identifies it, so
	// a run cancelled too late to stop recognizes itself as stale.
	cancel       func()
	timerSeq     uint64
	lastUsed     time.Time
	retryBackoff time.Duration
	// credentialDroppedAt is when a refusal last dropped the credential.
	credentialDroppedAt time.Time
}

func (st *tenantState) hasToken() bool { return st.token.token != "" }

func (st *tenantState) failingAt(now time.Time) bool {
	return st.failure.err != nil && now.Before(st.failure.until)
}

// WithTokenClock overrides the clock the token source measures refresh points
// with. A nil clock is ignored.
func WithTokenClock(now func() time.Time) M2MTokenSourceOption {
	return func(s *M2MTokenSource) {
		if now != nil {
			s.now = now
		}
	}
}

// WithRefreshScheduler overrides how the token source schedules a token's
// proactive refresh (time.AfterFunc by default). A nil scheduler is ignored.
func WithRefreshScheduler(schedule RefreshScheduler) M2MTokenSourceOption {
	return func(s *M2MTokenSource) {
		if schedule != nil {
			s.schedule = schedule
		}
	}
}

// WithTokenWaitTimeout sets how long a caller with no valid token waits for a
// mint, independently of any deadline the caller sets for the call that needs
// the token. A non-positive value keeps DefaultTokenWaitTimeout; a value above
// MaxTokenWaitTimeout is refused by NewM2MTokenSource.
func WithTokenWaitTimeout(d time.Duration) M2MTokenSourceOption {
	return func(s *M2MTokenSource) {
		if d > 0 {
			s.waitTimeout = d
		}
	}
}

// WithTokenLogger sets the logger a proactive refresh reports a failure to. A
// nil logger is ignored.
func WithTokenLogger(logger libLog.Logger) M2MTokenSourceOption {
	return func(s *M2MTokenSource) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// NewM2MTokenSource builds a token source over minter and creds. Both are
// required: a half-wired source would fail every seam call.
func NewM2MTokenSource(minter TokenMinter, creds CredentialProvider, opts ...M2MTokenSourceOption) (*M2MTokenSource, error) {
	if minter == nil {
		return nil, errors.New("tracer seam token source requires a token minter")
	}

	if creds == nil {
		return nil, errors.New("tracer seam token source requires a credential provider")
	}

	src := &M2MTokenSource{
		minter:      minter,
		creds:       creds,
		now:         time.Now,
		schedule:    afterFuncScheduler,
		jitter:      rand.Float64,
		logger:      &libLog.NopLogger{},
		mintTimeout: defaultMintTimeout,
		waitTimeout: DefaultTokenWaitTimeout,
		tenants:     make(map[string]*tenantState),
	}

	for _, opt := range opts {
		opt(src)
	}

	if src.waitTimeout > src.mintTimeout {
		return nil, fmt.Errorf("tracer seam token wait timeout %s exceeds the %s mint timeout", src.waitTimeout, src.mintTimeout)
	}

	return src, nil
}

// Token returns the token for the tenant in ctx: the cached one while it is
// valid, refreshing it in the background once it passed its refresh point, and
// a freshly minted one otherwise.
func (s *M2MTokenSource) Token(ctx context.Context) (string, error) {
	key, err := tokenKey(ctx)
	if err != nil {
		return "", credentialNotSent(err)
	}

	now := s.now()

	s.mu.Lock()
	st := s.stateLocked(key)
	st.lastUsed = now
	entry, cached := st.token, st.hasToken()
	failure, failing := st.failure, st.failingAt(now)
	refresh := cached && !now.Before(entry.refreshAt) && now.Before(entry.expiresAt) && !failing && !st.refreshing

	if refresh {
		st.refreshing = true
	}
	s.mu.Unlock()

	switch {
	case cached && now.Before(entry.refreshAt):
		return entry.token, nil
	case cached && now.Before(entry.expiresAt):
		if refresh {
			logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
			s.refreshInBackground(ctx, logger, key, entry.token)
		}

		return entry.token, nil
	case failing:
		return "", credentialNotSent(failure.err)
	}

	result := s.startMint(ctx, key)

	waitCtx, cancel := context.WithTimeout(ctx, s.waitTimeout)
	defer cancel()

	select {
	case res := <-result:
		if res.Err != nil {
			return "", credentialNotSent(res.Err)
		}

		token, _ := res.Val.(string)

		return token, nil
	case <-waitCtx.Done():
		return "", fmt.Errorf("%w: %w: %w", ErrTracerUnavailable, errTokenWaitAbandoned, waitCtx.Err())
	}
}

// Invalidate drops the cached token of the tenant in ctx when it is still the
// rejected one and is at least rejectedTokenMinAge old, and reports whether a
// retry would carry a different token. The credential stays: a rejected token
// says nothing about the credential it was minted with.
func (s *M2MTokenSource) Invalidate(ctx context.Context, rejected string) bool {
	key, err := tokenKey(ctx)
	if err != nil {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.tenants[key]
	if !ok || st.token.token != rejected {
		return true
	}

	if s.now().Sub(st.token.mintedAt) < rejectedTokenMinAge {
		return false
	}

	st.token = cachedToken{}
	st.retryBackoff = 0
	s.cancelTimerLocked(st)

	return true
}

// Warm mints the token for the tenant in ctx in the background, so the first
// seam call does not wait for the Access Manager. It never blocks; a failure is
// logged at Warn and the next call mints as usual.
func (s *M2MTokenSource) Warm(ctx context.Context, logger libLog.Logger) {
	key, err := tokenKey(ctx)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Tracer seam token not warmed: invalid tenant id", libLog.Err(err))

		return
	}

	result := s.startMint(ctx, key)

	libRuntime.SafeGoWithContextAndComponent(context.WithoutCancel(ctx), logger, panicComponent, "tracer_seam_token_warm", libRuntime.KeepRunning,
		func(ctx context.Context) {
			if res := <-result; res.Err != nil {
				logger.Log(ctx, libLog.LevelWarn, "Tracer seam token not warmed at boot; the first reservation call mints it", libLog.Err(res.Err))
			}
		})
}

// Close cancels every scheduled refresh and schedules no more. Tokens already
// cached stay served.
func (s *M2MTokenSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true

	for _, st := range s.tenants {
		s.cancelTimerLocked(st)
	}

	return nil
}

// refreshInBackground replaces the tenant's token without blocking the caller.
// Each failed replacement is logged at Warn and retried while token, the one
// being replaced, is still cached and valid; it stays served until it expires.
// The caller has marked the tenant as refreshing.
func (s *M2MTokenSource) refreshInBackground(ctx context.Context, logger libLog.Logger, key, token string) {
	result := s.startMint(ctx, key)

	libRuntime.SafeGoWithContextAndComponent(context.WithoutCancel(ctx), logger, panicComponent, "tracer_seam_token_refresh", libRuntime.KeepRunning,
		func(ctx context.Context) {
			res := <-result
			if res.Err == nil {
				return
			}

			logger.Log(ctx, libLog.LevelWarn, "Tracer seam token refresh failed; serving the cached token until it expires",
				libLog.String("tenant_id", key), libLog.Err(res.Err))

			s.scheduleRetry(key, token)
		})
}

// scheduleRetry arms the next retry of a failed refresh of token, backing off
// per consecutive failure, unless the source is closed or token was replaced,
// dropped or expired.
func (s *M2MTokenSource) scheduleRetry(key, token string) {
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.tenants[key]
	if s.closed || !ok || st.token.token != token || !now.Before(st.token.expiresAt) {
		return
	}

	st.retryBackoff = nextRetryBackoff(st.retryBackoff, st.token)
	at := now.Add(st.retryBackoff)

	if st.failingAt(now) && st.failure.until.After(at) {
		at = st.failure.until
	}

	s.scheduleLocked(key, st, at)
}

// proactiveRefresh is the tenant's scheduled run. A stale run (Close retires
// every run) or one for a dropped token does nothing. An expired token forgets a
// tenant idle past idleRefreshHorizon. A valid one is refreshed, unless it is
// not yet due, the tenant is idle past idleRefreshHorizon (the run moves to the
// expiry), or a refresh in flight or an open failure window defers it (the run
// is retried).
func (s *M2MTokenSource) proactiveRefresh(key string, seq uint64) {
	now := s.now()

	s.mu.Lock()

	st, ok := s.tenants[key]
	if !ok || st.timerSeq != seq || !st.hasToken() {
		s.mu.Unlock()

		return
	}

	st.cancel = nil
	entry := st.token
	idle := now.Sub(st.lastUsed) > idleRefreshHorizon

	switch {
	case !now.Before(entry.expiresAt):
		if idle {
			delete(s.tenants, key)
		}
	case now.Before(entry.refreshAt):
		s.scheduleLocked(key, st, entry.refreshAt)
	case idle:
		s.scheduleLocked(key, st, entry.expiresAt)
	case st.refreshing || st.failingAt(now):
		at := now.Add(mintFailureWindow)
		if st.failingAt(now) && st.failure.until.After(at) {
			at = st.failure.until
		}

		s.scheduleLocked(key, st, at)
	default:
		st.refreshing = true
		logger := s.logger
		s.mu.Unlock()

		s.refreshInBackground(libObservability.ContextWithLogger(context.Background(), logger), logger, key, entry.token)

		return
	}

	s.mu.Unlock()
}

// startMint joins or starts the tenant's single mint. The mint runs detached
// from ctx's cancellation, so a caller that gives up does not abort the mint
// others are waiting on, and a panic inside it is recovered into an error.
func (s *M2MTokenSource) startMint(ctx context.Context, key string) <-chan singleflight.Result {
	mintCtx := context.WithoutCancel(ctx)

	return s.flight.DoChan(key, func() (any, error) {
		defer s.refreshDone(key)

		boundedCtx, cancel := context.WithTimeout(mintCtx, s.mintTimeout)
		defer cancel()

		token, err := s.mintRecovering(boundedCtx, key)

		return token, err
	})
}

func (s *M2MTokenSource) mintRecovering(ctx context.Context, key string) (token string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
			libRuntime.HandlePanicValue(ctx, logger, recovered, panicComponent, "tracer_seam_token_mint")

			token, err = "", errTokenMintPanicked
			s.recordFailure(key, err)
		}
	}()

	return s.mint(ctx, key)
}

// mint resolves the tenant's credential and exchanges it for a token. A token
// cached by a mint that finished after the caller's cache miss is reused.
func (s *M2MTokenSource) mint(ctx context.Context, key string) (string, error) {
	if token, ok := s.fresh(key); ok {
		return token, nil
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.m2m_token_source.mint")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.tenant_id", key))

	creds, err := s.creds.Credentials(ctx, key)
	if err != nil {
		credErr := fmt.Errorf("resolve tracer seam credential: %w", err)
		libOpentelemetry.HandleSpanError(span, "Failed to resolve the tracer seam credential", credErr)
		s.recordFailure(key, credErr)

		return "", credErr
	}

	mintedAt := s.now()

	token, err := s.minter.GetApplicationToken(ctx, creds.ClientID, creds.ClientSecret)
	if err == nil && token == "" {
		err = errEmptyToken
	}

	var refreshAt, expiresAt time.Time
	if err == nil {
		refreshAt, expiresAt, err = tokenDeadlines(token, mintedAt)
	}

	if err != nil {
		mintErr := fmt.Errorf("%w: %w", errTokenMintFailed, err)
		libOpentelemetry.HandleSpanError(span, "Failed to mint the tracer seam token", mintErr)

		if credentialRefused(err) && s.claimCredentialDrop(key) {
			s.creds.Invalidate(key)
		}

		s.recordFailure(key, mintErr)

		return "", mintErr
	}

	s.store(key, cachedToken{token: token, mintedAt: mintedAt, refreshAt: refreshAt, expiresAt: expiresAt})

	return token, nil
}

// store caches a freshly minted token and schedules its proactive refresh.
func (s *M2MTokenSource) store(key string, entry cachedToken) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.stateLocked(key)
	st.token = entry
	st.failure = cachedFailure{}
	st.failureStreak = 0
	st.retryBackoff = 0

	if st.lastUsed.IsZero() {
		st.lastUsed = entry.mintedAt
	}

	s.scheduleLocked(key, st, entry.refreshAt)
}

// stateLocked returns the tenant's state, creating it when absent. The caller
// holds s.mu.
func (s *M2MTokenSource) stateLocked(key string) *tenantState {
	st, ok := s.tenants[key]
	if !ok {
		st = &tenantState{}
		s.tenants[key] = st
	}

	return st
}

// scheduleLocked replaces the tenant's scheduled run with one at at, never
// later than its token's expiry. The caller holds s.mu.
func (s *M2MTokenSource) scheduleLocked(key string, st *tenantState, at time.Time) {
	s.cancelTimerLocked(st)

	if s.closed {
		return
	}

	if at.After(st.token.expiresAt) {
		at = st.token.expiresAt
	}

	seq := st.timerSeq
	st.cancel = s.schedule(at.Sub(s.now()), func() { s.proactiveRefresh(key, seq) })
}

// cancelTimerLocked cancels the tenant's scheduled run, if any, and retires its
// sequence number. The caller holds s.mu.
func (s *M2MTokenSource) cancelTimerLocked(st *tenantState) {
	if st.cancel != nil {
		st.cancel()
		st.cancel = nil
	}

	st.timerSeq++
}

func (s *M2MTokenSource) fresh(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.tenants[key]
	if !ok || !st.hasToken() || !s.now().Before(st.token.refreshAt) {
		return "", false
	}

	return st.token.token, true
}

// refreshDone clears the tenant's background-refresh mark once its mint ends.
func (s *M2MTokenSource) refreshDone(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if st, ok := s.tenants[key]; ok {
		st.refreshing = false
	}
}

func (s *M2MTokenSource) recordFailure(key string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.stateLocked(key)
	st.failureStreak++
	st.failure = cachedFailure{err: err, until: s.now().Add(failureWindow(st.failureStreak, s.jitter()))}
}

// claimCredentialDrop reports whether a refusal may drop the tenant's
// credential now, and records the drop when it may.
func (s *M2MTokenSource) claimCredentialDrop(key string) bool {
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.stateLocked(key)
	if !st.credentialDroppedAt.IsZero() && now.Sub(st.credentialDroppedAt) < credentialRefusalMinInterval {
		return false
	}

	st.credentialDroppedAt = now

	return true
}

// failureWindow is how long the streak-th consecutive failed mint is answered
// from memory: mintFailureWindow doubled per earlier failure, capped at
// maxMintFailureWindow, then widened by jitter, in [0, 1), times a
// 1/failureJitterDivisor share.
func failureWindow(streak int, jitter float64) time.Duration {
	window := mintFailureWindow
	for range streak - 1 {
		if window >= maxMintFailureWindow {
			break
		}

		window *= 2
	}

	window = min(window, maxMintFailureWindow)

	return window + time.Duration(jitter*float64(window/failureJitterDivisor))
}

// nextRetryBackoff is the delay before the next retry of a failed refresh of
// entry, given the previous one (zero for the first failure).
func nextRetryBackoff(previous time.Duration, entry cachedToken) time.Duration {
	next := mintFailureWindow
	if previous > 0 {
		next = previous * 2
	}

	if ceiling := entry.expiresAt.Sub(entry.refreshAt) / retryBackoffCeilingDivisor; ceiling > 0 && next > ceiling {
		next = ceiling
	}

	return next
}

// afterFuncScheduler is the production RefreshScheduler.
func afterFuncScheduler(after time.Duration, run func()) func() {
	timer := time.AfterFunc(after, run)

	return func() { timer.Stop() }
}

// credentialRefused reports whether a mint failed because the Access Manager
// refused the credential (lib-auth answers a non-2xx as a commons.Response) or
// minted nothing with it. Only then is the credential worth reading again; a
// transport failure says nothing about it.
func credentialRefused(err error) bool {
	var refusal commons.Response

	return errors.As(err, &refusal) || errors.Is(err, errEmptyToken)
}

// tokenKey is the cache key of the tenant in ctx: its canonical id, or empty in
// single-tenant mode.
func tokenKey(ctx context.Context) (string, error) {
	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		return "", nil
	}

	return tmcore.CanonicalTenantID(tenantID)
}

// tokenDeadlines returns the refresh point and the expiry of a token minted at
// mintedAt, or errTokenAlreadyExpired when its exp is not after mintedAt. The
// refresh point comes before the expiry whenever the lifetime can tell them
// apart.
func tokenDeadlines(token string, mintedAt time.Time) (time.Time, time.Time, error) {
	lifetime := defaultTokenLifetime

	if exp, ok := tokenExpiry(token); ok {
		if !exp.After(mintedAt) {
			return time.Time{}, time.Time{}, errTokenAlreadyExpired
		}

		lifetime = min(exp.Sub(mintedAt), maxTokenLifetime)
	}

	serve := lifetime * tokenRefreshNumerator / tokenRefreshDenominator
	valid := lifetime - tokenExpiryMargin

	if valid <= serve {
		valid = lifetime * tokenShortExpiryNumerator / tokenShortExpiryDenominator
	}

	return mintedAt.Add(serve), mintedAt.Add(valid), nil
}

// tokenExpiry reads exp off a JWT without verifying it. The value only decides
// when to re-mint a token this ledger just minted; it is never a trust decision.
func tokenExpiry(token string) (time.Time, bool) {
	claims := jwt.MapClaims{}

	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return time.Time{}, false
	}

	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return time.Time{}, false
	}

	return exp.Time, true
}
