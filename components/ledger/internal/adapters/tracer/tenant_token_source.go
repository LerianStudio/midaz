// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/LerianStudio/lib-commons/v7/commons/secretsmanager"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	// tenantTokenSourceLimit bounds how many tenants hold a token source at
	// once; the least recently used one is dropped to admit another.
	tenantTokenSourceLimit = 1024

	// tenantTokenSourceIdleTTL drops a tenant's token source after this long
	// without a call, together with the token it caches.
	tenantTokenSourceIdleTTL = 30 * time.Minute
)

// ErrTenantIdentityUnprovisioned reports a tenant the ledger holds no usable
// identity for toward the Tracer: its credentials are absent, denied,
// malformed or refused by plugin-auth. It is wrapped with
// ErrTracerRequestRejected, because a request cannot be authenticated for the
// tenant until the tenant is provisioned, whatever the transaction.
var ErrTenantIdentityUnprovisioned = errors.New("tenant has no usable M2M identity for the tracer")

// tenantTokenEntry keeps a digest of the client secret, never the secret
// itself, to tell whether the tenant's credentials changed.
type tenantTokenEntry struct {
	source       *M2MTokenSource
	clientID     string
	secretDigest [sha256.Size]byte
	createdAt    time.Time
	lastUsedAt   time.Time
}

// TenantTokenSource presents, for each request, a token minted from the
// credentials of the request's own tenant: a token issued for one tenant is
// never sent for another. Each tenant has its own M2MTokenSource, created on
// first use, replaced when its credentials change and dropped when idle or
// when the credentials themselves are refused.
type TenantTokenSource struct {
	credentials CredentialProvider
	minter      TokenMinter
	now         clock

	mu      sync.Mutex
	tenants map[string]*tenantTokenEntry
}

// NewTenantTokenSource builds a per-tenant token source.
func NewTenantTokenSource(credentials CredentialProvider, minter TokenMinter, clk clock) (*TenantTokenSource, error) {
	if credentials == nil || minter == nil {
		return nil, errors.New("tracer tenant token source requires a credential provider and a minter")
	}

	if clk == nil {
		clk = time.Now
	}

	return &TenantTokenSource{credentials: credentials, minter: minter, now: clk, tenants: make(map[string]*tenantTokenEntry)}, nil
}

// Token returns a token for the tenant in ctx. A context without a tenant is
// refused before anything is sent, as the Tracer would refuse it (0487).
// Absent, denied or malformed credentials, and a mint plugin-auth refuses,
// are refusals of the tenant: they wrap ErrTracerRequestRejected and
// ErrTenantIdentityUnprovisioned, and the first three drop the tenant's
// source. A transient custody failure keeps serving the tenant's current
// source, whose credentials were valid when last read; without one it wraps
// only constant.ErrTracerTokenUnavailable.
func (s *TenantTokenSource) Token(ctx context.Context) (string, error) {
	tenantID := contextTenantID(ctx)
	if tenantID == "" {
		return "", rejectedBy(constant.ErrReservationTenantRequired)
	}

	source, err := s.sourceFor(ctx, tenantID)
	if err != nil {
		return "", err
	}

	token, err := source.Token(ctx)
	if err != nil && isMintRefusal(err) {
		s.rejectCredentials(ctx, tenantID, source, "")

		return "", identityRefused(err)
	}

	return token, err
}

// Invalidate discards rejected from the tenant's source. A tenant without a
// source can obtain a different token, so it reports true.
func (s *TenantTokenSource) Invalidate(ctx context.Context, rejected string) bool {
	tenantID := contextTenantID(ctx)
	if tenantID == "" {
		return false
	}

	s.mu.Lock()
	entry, ok := s.tenants[tenantID]
	s.mu.Unlock()

	if !ok {
		return true
	}

	return entry.source.Invalidate(ctx, rejected)
}

// RejectCredentials drops the tenant's source and its cached credentials when
// the Tracer refused rejected after renewal. A source younger than
// tokenRenewBackoff is kept, so a burst of refusals costs the custody backend
// and the identity provider at most one read and one mint per window.
func (s *TenantTokenSource) RejectCredentials(ctx context.Context, rejected string) {
	tenantID := contextTenantID(ctx)
	if tenantID == "" || rejected == "" {
		return
	}

	s.mu.Lock()
	entry, ok := s.tenants[tenantID]
	s.mu.Unlock()

	if !ok {
		return
	}

	s.rejectCredentials(ctx, tenantID, entry.source, rejected)
}

func (s *TenantTokenSource) rejectCredentials(ctx context.Context, tenantID string, source *M2MTokenSource, rejected string) {
	s.mu.Lock()

	entry, ok := s.tenants[tenantID]
	if !ok || entry.source != source || s.now().Before(entry.createdAt.Add(tokenRenewBackoff)) {
		s.mu.Unlock()

		return
	}

	if current := source.snapshot().token; rejected != "" && current != "" && current != rejected {
		s.mu.Unlock()

		return
	}

	delete(s.tenants, tenantID)
	s.mu.Unlock()

	s.credentials.InvalidateCredentials(ctx)
}

// sourceFor resolves the tenant's credentials and returns the source minting
// with them, creating or replacing it when they changed.
func (s *TenantTokenSource) sourceFor(ctx context.Context, tenantID string) (*M2MTokenSource, error) {
	creds, credErr := s.credentials.GetCredentials(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()

	entry, ok := s.tenants[tenantID]
	if ok && !now.Before(entry.lastUsedAt.Add(tenantTokenSourceIdleTTL)) {
		delete(s.tenants, tenantID)

		entry, ok = nil, false
	}

	if credErr != nil {
		if !isPermanentCredentialFailure(credErr) {
			if ok {
				entry.lastUsedAt = now

				return entry.source, nil
			}

			return nil, fmt.Errorf("%w: tenant M2M credentials: %w", constant.ErrTracerTokenUnavailable, credErr)
		}

		delete(s.tenants, tenantID)

		return nil, identityRefused(fmt.Errorf("tenant M2M credentials: %w", credErr))
	}

	digest := sha256.Sum256([]byte(creds.ClientSecret))
	if ok && entry.clientID == creds.ClientID && subtle.ConstantTimeCompare(entry.secretDigest[:], digest[:]) == 1 {
		entry.lastUsedAt = now

		return entry.source, nil
	}

	source, err := NewM2MTokenSource(s.minter, creds.ClientID, creds.ClientSecret, s.now)
	if err != nil {
		delete(s.tenants, tenantID)

		return nil, identityRefused(fmt.Errorf("%w: %w", secretsmanager.ErrM2MInvalidCredentials, err))
	}

	if !ok {
		s.admitLocked(now)
	}

	s.tenants[tenantID] = &tenantTokenEntry{source: source, clientID: creds.ClientID, secretDigest: digest, createdAt: now, lastUsedAt: now}

	return source, nil
}

// admitLocked drops every idle source and, when the limit is still reached,
// the least recently used one.
func (s *TenantTokenSource) admitLocked(now time.Time) {
	var (
		oldestID string
		oldestAt time.Time
	)

	for id, entry := range s.tenants {
		if !now.Before(entry.lastUsedAt.Add(tenantTokenSourceIdleTTL)) {
			delete(s.tenants, id)

			continue
		}

		if oldestID == "" || entry.lastUsedAt.Before(oldestAt) {
			oldestID, oldestAt = id, entry.lastUsedAt
		}
	}

	if len(s.tenants) >= tenantTokenSourceLimit && oldestID != "" {
		delete(s.tenants, oldestID)
	}
}

// permanentCredentialFailures are the custody answers that describe the
// tenant's credentials themselves: absent, denied to the ledger, or unusable
// as stored. A retrieval failure of any other kind says nothing about them.
var permanentCredentialFailures = []error{
	secretsmanager.ErrM2MCredentialsNotFound,
	secretsmanager.ErrM2MVaultAccessDenied,
	secretsmanager.ErrM2MInvalidCredentials,
	secretsmanager.ErrM2MUnmarshalFailed,
	secretsmanager.ErrM2MBinarySecretNotSupported,
	secretsmanager.ErrM2MInvalidInput,
	secretsmanager.ErrM2MInvalidPathSegment,
}

// isPermanentCredentialFailure reports a custody failure a new read cannot
// change until the tenant is provisioned again. A cancelled or timed-out read
// is never one, whatever it wraps.
func isPermanentCredentialFailure(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	for _, permanent := range permanentCredentialFailures {
		if errors.Is(err, permanent) {
			return true
		}
	}

	return false
}

// mintRefusalStatuses are the statuses at which plugin-auth refuses the
// credentials themselves. A 5xx or a 429 is plugin-auth unable to answer.
var mintRefusalStatuses = []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden}

// mintRefusalCodes are the plugin-auth codes a client-credentials mint is
// refused with at one of mintRefusalStatuses: a malformed grant (400) or
// client credentials it does not accept (401).
var mintRefusalCodes = []string{
	"AUT-0001", // missing fields
	"AUT-0003", // unexpected fields
	"AUT-0009", // bad request
	"AUT-0013", // invalid grant type
	"AUT-0014", // grant type missing fields
	"AUT-1001", // unsupported grant type
	"AUT-1004", // invalid client credentials
}

// isMintRefusal reports that plugin-auth answered the mint and refused the
// credentials. lib-auth surfaces every non-2xx answer as a libCommons.Response
// without its status, so only a code that names a refusal status, or a
// plugin-auth code issued at one, counts: an answer without a code, such as a
// gateway error page or a rate limit, is unavailability.
func isMintRefusal(err error) bool {
	var refusal libCommons.Response
	if !errors.As(err, &refusal) || refusal.Code == "" {
		return false
	}

	if status, convErr := strconv.Atoi(refusal.Code); convErr == nil {
		return slices.Contains(mintRefusalStatuses, status)
	}

	return slices.Contains(mintRefusalCodes, refusal.Code)
}

// identityRefused marks cause as a refusal of the tenant's identity.
func identityRefused(cause error) error {
	return rejectedBy(fmt.Errorf("%w: %w: %w", ErrTenantIdentityUnprovisioned, constant.ErrTracerTokenUnavailable, cause))
}
