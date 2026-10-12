// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/secretsmanager"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	"golang.org/x/sync/singleflight"
)

// CredentialProvider resolves the client credential pair the ledger mints a
// tenant's M2M token with, for one canonical tenant id (empty in single-tenant
// mode). Invalidate drops whatever the provider cached for that tenant.
type CredentialProvider interface {
	Credentials(ctx context.Context, tenantID string) (Credentials, error)
	Invalidate(tenantID string)
}

// CredentialFetcher reads one tenant's M2M credential from the secret store,
// addressed by the tenant-manager's canonical tenant id.
type CredentialFetcher func(ctx context.Context, tenantOrgID string) (*secretsmanager.M2MCredentials, error)

// Credentials is a client credential pair. Its formatted forms never render the
// secret.
type Credentials struct {
	ClientID     string
	ClientSecret string `json:"-"`
}

// TenantCredentialsOption configures the multi-tenant credential provider.
type TenantCredentialsOption func(*tenantCredentials)

// M2MTargetService is the target-service segment of the ledger's M2M secret
// path for the tracer reservation seam.
const M2MTargetService = "tracer"

const (
	// permanentCredentialFailureTTL is how long a credential read that found a
	// malformed secret is answered from memory.
	permanentCredentialFailureTTL = 30 * time.Second
	// missingCredentialFailureTTL is how long a credential read that found no
	// secret is answered from memory. It is short because the tenant-manager
	// writes the secret while the tenant is being provisioned, and the first
	// calls may race that write.
	missingCredentialFailureTTL = 5 * time.Second
)

var (
	errMissingTenant          = errors.New("tenant M2M credential requires a tenant in context")
	errNilTenantCredential    = errors.New("tenant M2M credential read returned no credential")
	errCredentialReadPanicked = errors.New("tenant M2M credential read panicked")
)

// WithCredentialClock overrides the clock the multi-tenant credential provider
// measures cached failures and entry age with. A nil clock is ignored.
func WithCredentialClock(now func() time.Time) TenantCredentialsOption {
	return func(p *tenantCredentials) {
		if now != nil {
			p.now = now
		}
	}
}

// WithCredentialTTL bounds how long a cached pair is served before the secret
// store is read again: an entry older than ttl is refreshed on the next lookup,
// and when that refresh fails the stale pair is served and the failure logged
// once at Warn until a refresh succeeds. A non-positive ttl keeps every entry
// until it is invalidated.
func WithCredentialTTL(ttl time.Duration) TenantCredentialsOption {
	return func(p *tenantCredentials) {
		if ttl > 0 {
			p.ttl = ttl
		}
	}
}

// String renders the pair with the secret obfuscated.
func (c Credentials) String() string {
	return fmt.Sprintf("Credentials{ClientID:%q, ClientSecret:********}", c.ClientID)
}

// GoString renders the pair with the secret obfuscated.
func (c Credentials) GoString() string {
	return c.String()
}

// staticCredentials is the single-tenant credential: one pair for every call.
type staticCredentials struct {
	creds Credentials
}

// NewStaticCredentials returns a provider that always yields the given pair.
func NewStaticCredentials(clientID, clientSecret string) CredentialProvider {
	return staticCredentials{creds: Credentials{ClientID: clientID, ClientSecret: clientSecret}}
}

func (p staticCredentials) Credentials(context.Context, string) (Credentials, error) {
	return p.creds, nil
}

func (staticCredentials) Invalidate(string) {}

// tenantCredentials reads each tenant's own credential through a
// CredentialFetcher, keyed by the canonical tenant id, and keeps it until it is
// invalidated (the token source drops it when the Access Manager refuses it)
// or, with WithCredentialTTL, until it ages out. Concurrent reads of one tenant
// share a single fetch. A missing secret is answered from memory for
// missingCredentialFailureTTL, and a failure retrying cannot heal (an
// incomplete or unreadable secret, an invalid path) for
// permanentCredentialFailureTTL; any other failure is never cached.
type tenantCredentials struct {
	fetch CredentialFetcher
	now   func() time.Time
	ttl   time.Duration

	mu       sync.Mutex
	entries  map[string]cachedCredential
	failures map[string]cachedFailure
	flight   singleflight.Group
}

// cachedCredential is a stored pair, when it was read, and whether the stale
// period it is in (a refresh that keeps failing) has been logged.
type cachedCredential struct {
	creds       Credentials
	readAt      time.Time
	staleWarned bool
}

// NewTenantCredentials returns the multi-tenant provider over fetch, which is
// required.
func NewTenantCredentials(fetch CredentialFetcher, opts ...TenantCredentialsOption) (CredentialProvider, error) {
	if fetch == nil {
		return nil, errors.New("tenant M2M credentials require a credential fetcher")
	}

	provider := &tenantCredentials{
		fetch:    fetch,
		now:      time.Now,
		entries:  make(map[string]cachedCredential),
		failures: make(map[string]cachedFailure),
	}

	for _, opt := range opts {
		opt(provider)
	}

	return provider, nil
}

// NewSecretsManagerCredentialFetcher binds a CredentialFetcher to the
// ledger-to-tracer M2M secret written by the tenant-manager at
// tenants/{env}/{tenantOrgID}/{applicationName}/m2m/tracer/credentials.
func NewSecretsManagerCredentialFetcher(client secretsmanager.SecretsManagerClient, env, applicationName string) CredentialFetcher {
	return func(ctx context.Context, tenantOrgID string) (*secretsmanager.M2MCredentials, error) {
		return secretsmanager.GetM2MCredentials(ctx, client, env, tenantOrgID, applicationName, M2MTargetService)
	}
}

func (p *tenantCredentials) Credentials(ctx context.Context, tenantID string) (Credentials, error) {
	key, err := credentialKey(tenantID)
	if err != nil {
		return Credentials{}, err
	}

	if creds, cachedErr, ok := p.cached(key); ok {
		return creds, cachedErr
	}

	val, err, _ := p.flight.Do(key, func() (any, error) {
		if creds, cachedErr, ok := p.cached(key); ok {
			return creds, cachedErr
		}

		return p.readRecovering(ctx, key)
	})
	if err != nil {
		return Credentials{}, err
	}

	creds, _ := val.(Credentials)

	return creds, nil
}

func (p *tenantCredentials) Invalidate(tenantID string) {
	key, err := credentialKey(tenantID)
	if err != nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.entries, key)
	delete(p.failures, key)
}

// cached answers from memory: a fresh credential, a stale one while a cached
// failure window is open, or a cached failure when no pair is held.
func (p *tenantCredentials) cached(key string) (Credentials, error, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, hasEntry := p.entries[key]
	if hasEntry && !p.expired(entry) {
		return entry.creds, nil, true
	}

	failure, hasFailure := p.failures[key]
	if !hasFailure || !p.now().Before(failure.until) {
		return Credentials{}, nil, false
	}

	if hasEntry {
		return entry.creds, nil, true
	}

	return Credentials{}, failure.err, true
}

// expired reports whether entry has aged past the TTL; without one it never does.
func (p *tenantCredentials) expired(entry cachedCredential) bool {
	return p.ttl > 0 && !p.now().Before(entry.readAt.Add(p.ttl))
}

func (p *tenantCredentials) readRecovering(ctx context.Context, key string) (creds Credentials, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
			libRuntime.HandlePanicValue(ctx, logger, recovered, panicComponent, "tenant_m2m_credential_read")

			creds, err = p.failed(ctx, key, errCredentialReadPanicked)
		}
	}()

	return p.read(ctx, key)
}

// read fetches one tenant's credential. A failed first read logs nothing: the
// mint span that asked for the credential records the failure.
func (p *tenantCredentials) read(ctx context.Context, key string) (Credentials, error) {
	m2m, err := p.fetch(ctx, key)
	if err == nil && m2m == nil {
		err = errNilTenantCredential
	}

	if err != nil {
		return p.failed(ctx, key, err)
	}

	creds := Credentials{ClientID: m2m.ClientID, ClientSecret: m2m.ClientSecret}

	p.mu.Lock()
	p.entries[key] = cachedCredential{creds: creds, readAt: p.now()}
	delete(p.failures, key)
	p.mu.Unlock()

	return creds, nil
}

// failed records a failed read: the failure is cached by class, and a tenant
// whose stale pair is still held is served it, with the failure logged once
// per stale period.
func (p *tenantCredentials) failed(ctx context.Context, key string, err error) (Credentials, error) {
	p.mu.Lock()

	if ttl, cached := credentialFailureTTL(err); cached {
		p.failures[key] = cachedFailure{err: err, until: p.now().Add(ttl)}
	}

	entry, ok := p.entries[key]
	if !ok {
		p.mu.Unlock()

		return Credentials{}, err
	}

	warn := !entry.staleWarned
	if warn {
		entry.staleWarned = true
		p.entries[key] = entry
	}

	p.mu.Unlock()

	if warn {
		libObservability.NewLoggerFromContext(ctx).Log(ctx, libLog.LevelWarn,
			"Tenant M2M credential refresh failed; serving the cached pair until a refresh succeeds",
			libLog.String("tenant_id", key), libLog.Err(err))
	}

	return entry.creds, nil
}

// credentialKey is the canonical tenant id the tenant-manager writes the
// credential under.
func credentialKey(tenantID string) (string, error) {
	if tenantID == "" {
		return "", errMissingTenant
	}

	return tmcore.CanonicalTenantID(tenantID)
}

// credentialFailureTTL reports how long a failed credential read is answered
// from memory, and whether it is at all: briefly for a secret not yet written,
// longer for one retrying cannot heal until someone repairs it, and never for a
// transient failure.
func credentialFailureTTL(err error) (time.Duration, bool) {
	switch {
	case errors.Is(err, secretsmanager.ErrM2MCredentialsNotFound):
		return missingCredentialFailureTTL, true
	case malformedCredentialFailure(err):
		return permanentCredentialFailureTTL, true
	default:
		return 0, false
	}
}

// malformedCredentialFailure reports whether a credential read found a secret
// it cannot use until someone repairs it.
func malformedCredentialFailure(err error) bool {
	return errors.Is(err, secretsmanager.ErrM2MInvalidCredentials) ||
		errors.Is(err, secretsmanager.ErrM2MUnmarshalFailed) ||
		errors.Is(err, secretsmanager.ErrM2MInvalidPathSegment) ||
		errors.Is(err, errNilTenantCredential)
}
