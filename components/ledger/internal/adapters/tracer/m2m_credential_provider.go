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
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	"golang.org/x/sync/singleflight"
)

// CredentialProvider resolves the client credential pair the ledger mints its
// seam token with, for one canonical tenant id (empty in single-tenant mode).
// Invalidate drops whatever the provider cached for that tenant.
type CredentialProvider interface {
	Credentials(ctx context.Context, tenantID string) (Credentials, error)
	Invalidate(tenantID string)
}

// CredentialFetcher reads one tenant's ledger-to-tracer M2M credential from the
// secret store, addressed by the tenant-manager's canonical tenant id.
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
	errMissingTenant          = errors.New("tracer seam credential requires a tenant in context")
	errNilTenantCredential    = errors.New("tracer seam credential read returned no credential")
	errCredentialReadPanicked = errors.New("tracer seam credential read panicked")
)

// WithCredentialClock overrides the clock the multi-tenant credential provider
// measures cached failures with. A nil clock is ignored.
func WithCredentialClock(now func() time.Time) TenantCredentialsOption {
	return func(p *tenantCredentials) {
		if now != nil {
			p.now = now
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
// invalidated: the token source drops it when the Access Manager refuses it.
// Concurrent reads of one tenant share a single fetch. A missing secret is
// answered from memory for missingCredentialFailureTTL, and a failure retrying
// cannot heal (an incomplete or unreadable secret, an invalid path) for
// permanentCredentialFailureTTL; any other failure is never cached.
type tenantCredentials struct {
	fetch CredentialFetcher
	now   func() time.Time

	mu       sync.Mutex
	entries  map[string]Credentials
	failures map[string]cachedFailure
	flight   singleflight.Group
}

// NewTenantCredentials returns the multi-tenant provider over fetch, which is
// required.
func NewTenantCredentials(fetch CredentialFetcher, opts ...TenantCredentialsOption) (CredentialProvider, error) {
	if fetch == nil {
		return nil, errors.New("tracer seam tenant credentials require a credential fetcher")
	}

	provider := &tenantCredentials{
		fetch:    fetch,
		now:      time.Now,
		entries:  make(map[string]Credentials),
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

// cached answers from memory: a stored credential, or a permanent failure still
// inside its window.
func (p *tenantCredentials) cached(key string) (Credentials, error, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if creds, ok := p.entries[key]; ok {
		return creds, nil, true
	}

	if failure, ok := p.failures[key]; ok && p.now().Before(failure.until) {
		return Credentials{}, failure.err, true
	}

	return Credentials{}, nil, false
}

func (p *tenantCredentials) readRecovering(ctx context.Context, key string) (creds Credentials, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
			libRuntime.HandlePanicValue(ctx, logger, recovered, panicComponent, "tracer_seam_credential_read")

			creds, err = Credentials{}, errCredentialReadPanicked
		}
	}()

	return p.read(ctx, key)
}

// read fetches one tenant's credential. It logs nothing: the mint span that
// asked for the credential records the failure.
func (p *tenantCredentials) read(ctx context.Context, key string) (Credentials, error) {
	m2m, err := p.fetch(ctx, key)
	if err == nil && m2m == nil {
		err = errNilTenantCredential
	}

	if err != nil {
		if ttl, cached := credentialFailureTTL(err); cached {
			p.mu.Lock()
			p.failures[key] = cachedFailure{err: err, until: p.now().Add(ttl)}
			p.mu.Unlock()
		}

		return Credentials{}, err
	}

	creds := Credentials{ClientID: m2m.ClientID, ClientSecret: m2m.ClientSecret}

	p.mu.Lock()
	p.entries[key] = creds
	delete(p.failures, key)
	p.mu.Unlock()

	return creds, nil
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
