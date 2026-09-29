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
	"time"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/sync/singleflight"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TokenSource supplies the bearer token the REST reservation seam presents to
// the Tracer. An error means no usable token exists and wraps
// constant.ErrTracerTokenUnavailable.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// TokenInvalidator is implemented by a TokenSource that caches. Invalidate
// reports a token the Tracer rejected and answers whether presenting the next
// Token may succeed: true when rejected was discarded or already replaced,
// false when no different token can be obtained yet.
type TokenInvalidator interface {
	Invalidate(ctx context.Context, rejected string) bool
}

// CredentialRejector is implemented by a TokenSource whose client credentials
// can themselves be stale. RejectCredentials reports a token the Tracer still
// refused after renewal, so the credentials it was minted from are read again.
type CredentialRejector interface {
	RejectCredentials(ctx context.Context, rejected string)
}

// TokenMinter issues an application (client-credentials) token. It is
// satisfied by the lib-auth AuthClient, which does not cache.
type TokenMinter = declaration.TokenMinter

const (
	// tokenRefreshMargin is the longest a cached token is renewed ahead of
	// `exp`; a token lives at least twice as long as its margin, so a
	// short-lived token is not re-minted on every call.
	tokenRefreshMargin = 60 * time.Second

	// tokenAssumedLifetime is the lifetime assumed for a token without `exp`.
	tokenAssumedLifetime = 60 * time.Second

	// tokenRenewTimeout bounds one shared renewal against the identity provider.
	tokenRenewTimeout = 5 * time.Second

	// tokenRenewBackoff is how long a failed mint suppresses every further mint,
	// and the minimum age a rejected token must reach before it is discarded
	// and replaced. Either way the identity provider sees at most one mint per
	// window, however many requests fail.
	tokenRenewBackoff = 5 * time.Second
)

type clock func() time.Time

// M2MTokenSource caches one client-credentials token and renews it at most
// once concurrently. The secret and the token are never logged.
type M2MTokenSource struct {
	minter   TokenMinter
	clientID string
	secret   string
	now      clock
	flight   singleflight.Group

	mu         sync.RWMutex
	token      string
	mintedAt   time.Time
	refreshAt  time.Time
	expiresAt  time.Time
	renewAfter time.Time
	// renewErr is the failure that started the current mint pause.
	renewErr error
}

// NewM2MTokenSource builds a caching token source over minter.
func NewM2MTokenSource(minter TokenMinter, clientID, secret string, clk clock) (*M2MTokenSource, error) {
	if minter == nil || strings.TrimSpace(clientID) == "" || secret == "" {
		return nil, errors.New("tracer M2M token source requires a minter and client credentials")
	}

	if clk == nil {
		clk = time.Now
	}

	return &M2MTokenSource{minter: minter, clientID: clientID, secret: secret, now: clk}, nil
}

// Token never waits on the identity provider while an unexpired token is
// cached: inside the refresh margin it serves that token and starts one
// background renewal. Only a caller without a valid token blocks, and a
// failure then wraps constant.ErrTracerTokenUnavailable. A failed mint
// suppresses every mint for tokenRenewBackoff; a caller without a valid token
// during that pause fails fast with the failure that started it, instead of
// reaching the identity provider.
//
// The renewal is shared by every concurrent caller, so it runs detached from
// the caller that started it, bounded by tokenRenewTimeout: one caller's
// deadline or cancellation must not fail the renewal the others wait on.
func (s *M2MTokenSource) Token(ctx context.Context) (string, error) {
	state := s.snapshot()
	if state.fresh {
		return state.token, nil
	}

	if state.valid {
		if !state.backingOff {
			s.renewDetached(ctx)
		}

		return state.token, nil
	}

	if state.backingOff {
		if state.renewErr != nil {
			return "", fmt.Errorf("%w: renewal paused after a failed mint: %w", constant.ErrTracerTokenUnavailable, state.renewErr)
		}

		return "", fmt.Errorf("%w: renewal paused after a failed mint", constant.ErrTracerTokenUnavailable)
	}

	results := s.renewDetached(ctx)

	select {
	case result := <-results:
		if result.Err != nil {
			return "", result.Err
		}

		token, ok := result.Val.(string)
		if !ok {
			return "", constant.ErrTracerTokenUnavailable
		}

		return token, nil
	case <-ctx.Done():
		if cached := s.snapshot(); cached.valid {
			return cached.token, nil
		}

		return "", fmt.Errorf("%w: %w", constant.ErrTracerTokenUnavailable, ctx.Err())
	}
}

// Invalidate discards rejected when it is the cached token. A cached token
// minted less than tokenRenewBackoff ago is kept and reported false: a
// rejection that soon is not about the token's age, and replacing it would
// cost the identity provider a mint per rejected request. The mint pause of an
// earlier failure is preserved.
func (s *M2MTokenSource) Invalidate(_ context.Context, rejected string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rejected == "" {
		return false
	}

	if s.token != rejected {
		return true
	}

	if s.now().Before(s.mintedAt.Add(tokenRenewBackoff)) {
		return false
	}

	s.token, s.mintedAt, s.refreshAt, s.expiresAt = "", time.Time{}, time.Time{}, time.Time{}

	return true
}

func (s *M2MTokenSource) renewDetached(ctx context.Context) <-chan singleflight.Result {
	detached := context.WithoutCancel(ctx)

	return s.flight.DoChan("token", func() (any, error) {
		renewCtx, cancel := context.WithTimeout(detached, tokenRenewTimeout)
		defer cancel()

		return s.renew(renewCtx)
	})
}

func (s *M2MTokenSource) renew(ctx context.Context) (string, error) {
	cached := s.snapshot()
	if cached.fresh {
		return cached.token, nil
	}

	token, err := s.minter.GetApplicationToken(ctx, s.clientID, s.secret)
	if err == nil && token == "" {
		err = errors.New("empty application token")
	}

	now := s.now()

	var refreshAt, expiresAt time.Time
	if err == nil {
		refreshAt, expiresAt = tokenWindow(token, now)
		if !now.Before(expiresAt) {
			err = errors.New("application token already expired")
		}
	}

	if err != nil {
		s.mu.Lock()
		s.renewAfter, s.renewErr = now.Add(tokenRenewBackoff), err
		s.mu.Unlock()

		// Re-read: the cached token may have been invalidated during the mint.
		if current := s.snapshot(); current.valid {
			return current.token, nil
		}

		return "", fmt.Errorf("%w: %w", constant.ErrTracerTokenUnavailable, err)
	}

	s.mu.Lock()
	s.token, s.mintedAt, s.refreshAt, s.expiresAt, s.renewAfter, s.renewErr = token, now, refreshAt, expiresAt, time.Time{}, nil
	s.mu.Unlock()

	return token, nil
}

type tokenState struct {
	token string
	// fresh: outside the refresh margin. valid: not yet expired.
	fresh, valid bool
	// backingOff: a failed mint suppresses every mint.
	backingOff bool
	renewErr   error
}

func (s *M2MTokenSource) snapshot() tokenState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := s.now()
	state := tokenState{backingOff: now.Before(s.renewAfter)}

	if state.backingOff {
		state.renewErr = s.renewErr
	}

	if s.token == "" {
		return state
	}

	state.token, state.fresh, state.valid = s.token, now.Before(s.refreshAt), now.Before(s.expiresAt)

	return state
}

// tokenWindow reads `exp` without verifying the signature: the Tracer
// verifies the token, the ledger only schedules renewal. The refresh margin is
// min(tokenRefreshMargin, lifetime/2).
func tokenWindow(token string, now time.Time) (time.Time, time.Time) {
	expiresAt := now.Add(tokenAssumedLifetime)

	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err == nil {
		if exp, err := claims.GetExpirationTime(); err == nil && exp != nil {
			expiresAt = exp.Time
		}
	}

	margin := min(tokenRefreshMargin, expiresAt.Sub(now)/2)

	return expiresAt.Add(-margin), expiresAt
}
