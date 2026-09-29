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

	"github.com/LerianStudio/lib-commons/v7/commons/secretsmanager"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"golang.org/x/sync/singleflight"
)

// CredentialProvider resolves the client credentials the ledger presents to
// plugin-auth for the tenant carried by ctx.
type CredentialProvider interface {
	// GetCredentials returns the tenant's credentials. A context without a
	// tenant is ErrTenantRequired; a custody failure wraps the lib-commons
	// secretsmanager sentinel that classifies it.
	GetCredentials(ctx context.Context) (*secretsmanager.M2MCredentials, error)

	// InvalidateCredentials discards the tenant's cached credentials so the
	// next GetCredentials reads the custody backend again.
	InvalidateCredentials(ctx context.Context)
}

// ErrTenantRequired reports a multi-tenant call whose context carries no
// tenant: no per-tenant credential can be addressed for it.
var ErrTenantRequired = errors.New("tracer M2M credentials require a tenant in context")

const (
	// credentialCacheTTL is how long a tenant's credentials are served from
	// memory before the custody backend is read again. It bounds how long a
	// rotated or revoked credential keeps being presented.
	credentialCacheTTL = 30 * time.Second

	// credentialAbsentTTL is how long an absent, denied or unusable secret is
	// remembered, so a tenant that is not provisioned costs the custody backend
	// one read per window instead of one per request.
	credentialAbsentTTL = 5 * time.Second

	// credentialFetchTimeout bounds one shared custody read.
	credentialFetchTimeout = 5 * time.Second

	// credentialAbsentLogWindow is how often the absence of a tenant's secret
	// is logged.
	credentialAbsentLogWindow = 5 * time.Minute
)

type credentialEntry struct {
	creds     *secretsmanager.M2MCredentials
	err       error
	expiresAt time.Time
}

// M2MCredentialProvider reads per-tenant client credentials from the custody
// backend at tenants/{env}/{tenant}/{application}/m2m/{target}/credentials.
// Credentials are cached in process memory only: no copy of a client secret is
// written to a shared cache. Concurrent misses for one tenant share one read,
// detached from the caller that started it. The secret is never logged.
type M2MCredentialProvider struct {
	client      secretsmanager.SecretsManagerClient
	env         string
	application string
	target      string
	now         clock

	flight      singleflight.Group
	cache       sync.Map // tenant -> *credentialEntry
	absentsSeen sync.Map // tenant -> time.Time of the last absence log
}

// NewM2MCredentialProvider builds a provider over a custody reader.
// application and target name the secret path segments; env may be empty.
func NewM2MCredentialProvider(client secretsmanager.SecretsManagerClient, env, application, target string, clk clock) (*M2MCredentialProvider, error) {
	if client == nil || strings.TrimSpace(application) == "" || strings.TrimSpace(target) == "" {
		return nil, errors.New("tracer M2M credential provider requires a custody reader, an application name and a target service")
	}

	for _, segment := range []string{env, application, target} {
		if strings.ContainsAny(segment, `/\`) || strings.Contains(segment, "..") {
			return nil, errors.New("tracer M2M credential path segments must not contain path separators")
		}
	}

	if clk == nil {
		clk = time.Now
	}

	return &M2MCredentialProvider{
		client:      client,
		env:         strings.TrimSpace(env),
		application: strings.TrimSpace(application),
		target:      strings.TrimSpace(target),
		now:         clk,
	}, nil
}

func (p *M2MCredentialProvider) GetCredentials(ctx context.Context) (*secretsmanager.M2MCredentials, error) {
	tenantID := contextTenantID(ctx)
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	if entry, ok := p.cached(tenantID); ok {
		return entry.creds, entry.err
	}

	detached := context.WithoutCancel(ctx)

	results := p.flight.DoChan(tenantID, func() (any, error) {
		if entry, ok := p.cached(tenantID); ok {
			return entry.creds, entry.err
		}

		fetchCtx, cancel := context.WithTimeout(detached, credentialFetchTimeout)
		defer cancel()

		return p.fetch(fetchCtx, tenantID)
	})

	select {
	case result := <-results:
		if result.Err != nil {
			return nil, result.Err
		}

		creds, ok := result.Val.(*secretsmanager.M2MCredentials)
		if !ok || creds == nil {
			return nil, secretsmanager.ErrM2MInvalidCredentials
		}

		return creds, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %w", secretsmanager.ErrM2MRetrievalFailed, ctx.Err())
	}
}

func (p *M2MCredentialProvider) InvalidateCredentials(ctx context.Context) {
	if tenantID := contextTenantID(ctx); tenantID != "" {
		p.cache.Delete(tenantID)
	}
}

func (p *M2MCredentialProvider) cached(tenantID string) (*credentialEntry, bool) {
	value, ok := p.cache.Load(tenantID)
	if !ok {
		return nil, false
	}

	entry, ok := value.(*credentialEntry)
	if !ok || !p.now().Before(entry.expiresAt) {
		p.cache.CompareAndDelete(tenantID, value)

		return nil, false
	}

	return entry, true
}

// fetch reads the custody backend once. A failure that describes the secret
// itself is remembered for credentialAbsentTTL; any other retrieval failure is
// not cached, and the next request tries again.
func (p *M2MCredentialProvider) fetch(ctx context.Context, tenantID string) (*secretsmanager.M2MCredentials, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.m2m_credentials.fetch")
	defer span.End()

	creds, err := secretsmanager.GetM2MCredentials(ctx, p.client, p.env, tenantID, p.application, p.target)
	if err == nil {
		p.cache.Store(tenantID, &credentialEntry{creds: creds, expiresAt: p.now().Add(credentialCacheTTL)})
		p.absentsSeen.Delete(tenantID)

		return creds, nil
	}

	if errors.Is(err, secretsmanager.ErrM2MCredentialsNotFound) {
		p.cache.Store(tenantID, &credentialEntry{err: err, expiresAt: p.now().Add(credentialAbsentTTL)})
		p.logAbsent(ctx, logger, tenantID)
		libOtel.HandleSpanBusinessErrorEvent(span, "Tenant not provisioned for ledger to tracer M2M", err)

		return nil, err
	}

	if isPermanentCredentialFailure(err) {
		p.cache.Store(tenantID, &credentialEntry{err: err, expiresAt: p.now().Add(credentialAbsentTTL)})
	}

	libOtel.HandleSpanError(span, "Failed to read tenant M2M credentials", err)
	logger.Log(ctx, libLog.LevelError, "Failed to read tenant M2M credentials for the tracer", libLog.String("tenant_id", tenantID), libLog.Err(err))

	return nil, err
}

func (p *M2MCredentialProvider) logAbsent(ctx context.Context, logger libLog.Logger, tenantID string) {
	now := p.now()

	if last, ok := p.absentsSeen.Load(tenantID); ok {
		if at, isTime := last.(time.Time); isTime && now.Before(at.Add(credentialAbsentLogWindow)) {
			return
		}
	}

	p.absentsSeen.Store(tenantID, now)
	logger.Log(ctx, libLog.LevelWarn, "Tenant not provisioned for ledger to tracer M2M", libLog.String("tenant_id", tenantID))
}

// contextTenantID returns the request tenant in the canonical form the
// tenant-manager uses in secret paths, so a dashed and a dashless spelling of
// the same tenant share one cache entry and one secret.
func contextTenantID(ctx context.Context) string {
	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		return ""
	}

	if canonical, err := tmcore.CanonicalTenantID(tenantID); err == nil {
		return canonical
	}

	return tenantID
}
