// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// disabledAuthClient returns a non-nil *middleware.AuthClient that does not
// touch the network. Used only to reach the credential-validation branch of
// NewStaticAppTokenProvider; the issuer is never actually invoked.
func disabledAuthClient() *middleware.AuthClient {
	return middleware.NewAuthClient("", false, nil)
}

type fakeAppTokenIssuer struct {
	token       string
	err         error
	gotClientID string
	gotSecret   string
	gotCtx      context.Context
	callCount   int
}

func (f *fakeAppTokenIssuer) GetApplicationToken(ctx context.Context, clientID, clientSecret string) (string, error) {
	f.callCount++
	f.gotCtx = ctx
	f.gotClientID = clientID
	f.gotSecret = clientSecret

	return f.token, f.err
}

func newProviderWithIssuer(issuer applicationTokenIssuer, clientID, secret string) *StaticAppTokenProvider {
	return &StaticAppTokenProvider{
		issuer:       issuer,
		clientID:     clientID,
		clientSecret: secret,
	}
}

func TestStaticAppTokenProvider_GetToken_Success(t *testing.T) {
	t.Parallel()

	issuer := &fakeAppTokenIssuer{token: "jwt-token-from-plugin-auth"}
	provider := newProviderWithIssuer(issuer, "fetcher-client", "fetcher-secret")

	ctx := context.Background()
	token, err := provider.GetToken(ctx)

	require.NoError(t, err)
	assert.Equal(t, "jwt-token-from-plugin-auth", token)
	assert.Equal(t, 1, issuer.callCount)
	assert.Equal(t, "fetcher-client", issuer.gotClientID, "the static CLIENT_ID must be forwarded as-is")
	assert.Equal(t, "fetcher-secret", issuer.gotSecret, "the static CLIENT_SECRET must be forwarded as-is")
	assert.Equal(t, ctx, issuer.gotCtx, "the caller context must be propagated for tracing/logging")
}

func TestStaticAppTokenProvider_GetToken_NoCaching(t *testing.T) {
	t.Parallel()

	issuer := &fakeAppTokenIssuer{token: "fresh-token"}
	provider := newProviderWithIssuer(issuer, "id", "secret")

	for i := 0; i < 3; i++ {
		_, err := provider.GetToken(context.Background())
		require.NoError(t, err)
	}

	assert.Equal(t, 3, issuer.callCount,
		"provider must NOT cache the token; every call must hit the AuthClient (matches plugin-fees and avoids stale-token issues on plugin-auth restart)")
}

func TestStaticAppTokenProvider_GetToken_IssuerError(t *testing.T) {
	t.Parallel()

	upstreamErr := errors.New("plugin-auth unreachable")
	issuer := &fakeAppTokenIssuer{err: upstreamErr}
	provider := newProviderWithIssuer(issuer, "id", "secret")

	token, err := provider.GetToken(context.Background())

	require.Error(t, err)
	assert.Empty(t, token)
	assert.ErrorIs(t, err, upstreamErr, "underlying AuthClient error must be wrapped, not swallowed")
	assert.Contains(t, err.Error(), "static app token exchange",
		"error context must identify which exchange failed for ops triage")
}

func TestNewStaticAppTokenProvider_RejectsEmptyCredentials(t *testing.T) {
	t.Parallel()

	// Cast nil to *middleware.AuthClient is not safe; we exercise the
	// credential check by directly calling the constructor with a non-nil
	// issuer surrogate via the struct (since middleware.AuthClient is a
	// concrete type, we test the credential-empty branch separately from
	// the nil-client branch).
	cases := []struct {
		name         string
		clientID     string
		clientSecret string
	}{
		{name: "empty client id", clientID: "", clientSecret: "secret"},
		{name: "empty client secret", clientID: "id", clientSecret: ""},
		{name: "both empty", clientID: "", clientSecret: ""},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			provider, err := NewStaticAppTokenProvider(disabledAuthClient(), tc.clientID, tc.clientSecret)

			require.Error(t, err)
			assert.Nil(t, provider)
			assert.ErrorIs(t, err, ErrStaticAppTokenProviderMissingCredentials,
				"empty credentials must be refused at startup, never silently emit tokens with empty client_id/secret")
		})
	}
}

func TestNewStaticAppTokenProvider_RejectsNilAuthClient(t *testing.T) {
	t.Parallel()

	provider, err := NewStaticAppTokenProvider(nil, "id", "secret")

	require.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), "AuthClient must not be nil")
}
