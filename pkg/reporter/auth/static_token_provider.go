// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
)

// ErrStaticAppTokenProviderMissingCredentials is returned when the provider is
// constructed without a CLIENT_ID or CLIENT_SECRET. The caller must surface
// this at startup instead of producing tokens with empty client credentials.
var ErrStaticAppTokenProviderMissingCredentials = errors.New(
	"static app token provider requires non-empty CLIENT_ID and CLIENT_SECRET",
)

// applicationTokenIssuer is the minimal slice of middleware.AuthClient that the
// provider consumes. Declared as an interface so unit tests can substitute a
// fake without spinning up a real plugin-auth.
type applicationTokenIssuer interface {
	GetApplicationToken(ctx context.Context, clientID, clientSecret string) (string, error)
}

// StaticAppTokenProvider implements fetcher.M2MTokenProvider for single-tenant
// deployments that still have PLUGIN_AUTH_ENABLED=true.
//
// In single-tenant mode there is no tenantId on the request context, so the
// tenant-aware M2MCredentialProvider (which resolves per-tenant credentials
// from AWS Secrets Manager) cannot be used. Instead this provider issues
// outbound application tokens using a fixed CLIENT_ID / CLIENT_SECRET pair
// supplied via environment variables — mirroring how plugin-fees authenticates
// against Midaz (see plugin-fees/pkg/net/http/midaz-service.go).
//
// The provider does NOT cache tokens. The underlying middleware.AuthClient
// hits plugin-auth's /v1/login/oauth/access_token endpoint on every call; this
// matches plugin-fees behavior and avoids stale-token issues when plugin-auth
// restarts or revokes credentials. Token exchange is fast (~ms) and the
// reporter→fetcher call volume is bounded by the report-generation flow.
type StaticAppTokenProvider struct {
	issuer       applicationTokenIssuer
	clientID     string
	clientSecret string
}

// NewStaticAppTokenProvider builds a single-tenant token provider that
// exchanges the supplied CLIENT_ID / CLIENT_SECRET for a JWT via the
// lib-auth/v2 AuthClient.
//
// Returns ErrStaticAppTokenProviderMissingCredentials if either credential is
// empty — refusing to silently emit unauthenticated requests when the operator
// asked for auth.
func NewStaticAppTokenProvider(authClient *middleware.AuthClient, clientID, clientSecret string) (*StaticAppTokenProvider, error) {
	if authClient == nil {
		return nil, fmt.Errorf("static app token provider: AuthClient must not be nil")
	}

	if clientID == "" || clientSecret == "" {
		return nil, ErrStaticAppTokenProviderMissingCredentials
	}

	return &StaticAppTokenProvider{
		issuer:       authClient,
		clientID:     clientID,
		clientSecret: clientSecret,
	}, nil
}

// GetToken implements fetcher.M2MTokenProvider. Performs a client_credentials
// exchange against plugin-auth using the static CLIENT_ID / CLIENT_SECRET.
//
// Note: when AuthClient was constructed with Enabled=false the underlying call
// returns ("", nil). The constructor blocks empty credentials at startup, so
// the only way to reach that path is operator misconfiguration of
// PLUGIN_AUTH_ENABLED — in which case an empty token is the most honest
// outcome (FetcherClient.applyAuth still sets the header, plugin-auth on the
// fetcher side rejects it, the operator gets a 401 that points at the
// configuration drift).
func (p *StaticAppTokenProvider) GetToken(ctx context.Context) (string, error) {
	token, err := p.issuer.GetApplicationToken(ctx, p.clientID, p.clientSecret)
	if err != nil {
		return "", fmt.Errorf("static app token exchange: %w", err)
	}

	return token, nil
}
